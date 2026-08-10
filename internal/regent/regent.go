package regent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/LISSConsulting/RalphSpec/internal/claude"
	"github.com/LISSConsulting/RalphSpec/internal/config"
	"github.com/LISSConsulting/RalphSpec/internal/loop"
	"github.com/LISSConsulting/RalphSpec/internal/quota"
	"github.com/LISSConsulting/RalphSpec/internal/testplan"
)

// RunFunc is the function the Regent supervises. Typically wraps loop.Loop.Run.
type RunFunc func(ctx context.Context) error

// Regent supervises the Ralph loop: crash detection, hang detection,
// test-gated rollback, and state persistence.
type Regent struct {
	cfg       config.RegentConfig
	dir       string
	git       GitOps
	events    chan<- loop.LogEntry
	testPlan  *testplan.Plan
	ludicrous bool

	// mu protects lastOutputAt and state
	mu           sync.Mutex
	lastOutputAt time.Time
	state        State
}

// New creates a Regent with the given configuration.
func New(cfg config.RegentConfig, dir string, git GitOps, events chan<- loop.LogEntry) *Regent {
	return &Regent{
		cfg:          cfg,
		dir:          dir,
		git:          git,
		events:       events,
		lastOutputAt: time.Now(),
	}
}

// SetTestPlan installs the immutable run-start plan used for post-iteration
// verification. The plan replaces the legacy single-command test gate.
func (r *Regent) SetTestPlan(plan *testplan.Plan) {
	r.testPlan = cloneTestPlan(plan)
}

// SetLudicrous records the effective completion mode in persisted run state.
func (r *Regent) SetLudicrous(enabled bool) {
	r.ludicrous = enabled
}

// Supervise runs the given function under Regent supervision. It handles crash
// detection with retry/backoff and hang detection via output timeout. Test-gated
// rollback is handled per-iteration via Loop.PostIteration (wired to
// RunPostIterationTests).
func (r *Regent) Supervise(ctx context.Context, run RunFunc) error {
	now := time.Now()
	r.mu.Lock()
	r.state = State{
		RalphPID:     os.Getpid(),
		LastOutputAt: now,
		StartedAt:    now,
		Status:       StatusRunning,
		Ludicrous:    r.ludicrous,
		TestPlan:     cloneTestPlan(r.testPlan),
	}
	r.mu.Unlock()
	r.saveState()

	var consecutiveErrors int
	for {
		select {
		case <-ctx.Done():
			r.emit("Shutting down gracefully")
			r.finishGraceful()
			return ctx.Err()
		default:
		}

		r.emit(fmt.Sprintf("Starting Ralph (attempt %d/%d)", consecutiveErrors+1, r.cfg.MaxRetries+1))
		r.touchOutput()

		err := r.runWithHangDetection(ctx, run)

		if errors.Is(err, loop.ErrOperatorStopped) {
			r.finishGraceful()
			r.emit("Operator stop honoured")
			return loop.ErrOperatorStopped
		}
		if err == nil {
			r.mu.Lock()
			r.state.ConsecutiveErrs = 0
			r.state.FinishedAt = time.Now()
			r.state.Passed = true
			r.state.Status = StatusPassed
			r.mu.Unlock()

			r.saveState()
			return nil
		}

		if ctx.Err() != nil {
			r.emit("Context cancelled — stopping")
			r.finishGraceful()
			return ctx.Err()
		}

		if outcome, ok := claude.AsOutcome(err); ok {
			switch outcome.Kind {
			case claude.OutcomeQuotaExhausted:
				r.finishBlocked(StatusPausedQuota, outcome.Message, outcome.RetryAt)
				r.emit("Quota exhausted — run paused without consuming a retry")
				return fmt.Errorf("regent: quota exhausted: %w", err)
			case claude.OutcomeAuthenticationRequired:
				r.finishBlocked(StatusBlocked, outcome.Message, time.Time{})
				r.emit("Authentication required — operator action needed")
				return fmt.Errorf("regent: authentication required: %w", err)
			case claude.OutcomeCancelled:
				r.finishGraceful()
				r.emit("Agent cancelled — stopping")
				return fmt.Errorf("regent: agent cancelled: %w", err)
			}
		}

		consecutiveErrors++
		r.mu.Lock()
		r.state.ConsecutiveErrs = consecutiveErrors
		r.mu.Unlock()
		r.saveState()

		r.emit(fmt.Sprintf("Ralph exited with error: %v", err))

		if consecutiveErrors > r.cfg.MaxRetries {
			r.mu.Lock()
			r.state.FinishedAt = time.Now()
			r.state.Passed = false
			r.state.Status = StatusFailed
			r.mu.Unlock()
			r.saveState()
			r.emit(fmt.Sprintf("Max retries (%d) exceeded — giving up", r.cfg.MaxRetries))
			return fmt.Errorf("regent: max retries exceeded after %d failures: %w", consecutiveErrors, err)
		}

		backoff := time.Duration(r.cfg.RetryBackoffSeconds) * time.Second
		r.emit(fmt.Sprintf("Retrying in %s (attempt %d/%d)", backoff, consecutiveErrors+1, r.cfg.MaxRetries+1))

		select {
		case <-ctx.Done():
			r.finishGraceful()
			return ctx.Err()
		case <-time.After(backoff):
		}
	}
}

// runWithHangDetection runs the function with a goroutine that monitors for hangs.
// If no output is received for hang_timeout_seconds, the context is cancelled.
func (r *Regent) runWithHangDetection(ctx context.Context, run RunFunc) error {
	hangTimeout := time.Duration(r.cfg.HangTimeoutSeconds) * time.Second
	if hangTimeout <= 0 {
		return run(ctx)
	}

	loopCtx, loopCancel := context.WithCancel(ctx)
	defer loopCancel()

	hangDone := make(chan struct{})
	go func() {
		defer close(hangDone)
		ticker := time.NewTicker(hangTimeout / 4)
		defer ticker.Stop()

		for {
			select {
			case <-loopCtx.Done():
				return
			case <-ticker.C:
				r.mu.Lock()
				elapsed := time.Since(r.lastOutputAt)
				r.mu.Unlock()

				if elapsed >= hangTimeout {
					r.emit(fmt.Sprintf("Hang detected — no output for %s — killing loop", hangTimeout))
					loopCancel()
					return
				}
			}
		}
	}()

	err := run(loopCtx)
	loopCancel()
	<-hangDone
	return err
}

// NotifyOutput resets the hang detection timer. Call this each time output
// is observed from the loop.
func (r *Regent) NotifyOutput() {
	r.touchOutput()
}

// UpdateState updates tracked state fields from a log entry, resets the
// hang detection timer, and persists state to disk so `ralph status` shows
// current progress during a running loop.
func (r *Regent) UpdateState(entry loop.LogEntry) {
	r.touchOutput()

	r.mu.Lock()
	changed := false
	if entry.Iteration > 0 {
		r.state.Iteration = entry.Iteration
		changed = true
	}
	if entry.TotalCost > 0 {
		r.state.TotalCostUSD = entry.TotalCost
		changed = true
	}
	if entry.Commit != "" {
		r.state.LastCommit = entry.Commit
		changed = true
	}
	if entry.Agent != "" {
		r.state.Agent = entry.Agent
		changed = true
	}
	if entry.Branch != "" {
		r.state.Branch = entry.Branch
		changed = true
	}
	if entry.Mode != "" {
		r.state.Mode = entry.Mode
		changed = true
	}
	if entry.Ludicrous {
		r.state.Ludicrous = true
		changed = true
	}
	if entry.QuotaDecision != nil {
		r.state.Quota = cloneQuotaDecision(entry.QuotaDecision)
		changed = true
	}
	r.mu.Unlock()

	if changed {
		r.saveState()
	}
}

func cloneTestPlan(plan *testplan.Plan) *testplan.Plan {
	if plan == nil {
		return nil
	}
	cloned := *plan
	cloned.Steps = append([]testplan.Step(nil), plan.Steps...)
	cloned.Diagnostics = append([]string(nil), plan.Diagnostics...)
	return &cloned
}

func cloneQuotaDecision(decision *quota.Decision) *quota.Decision {
	if decision == nil {
		return nil
	}
	cloned := *decision
	if decision.Snapshot != nil {
		snapshot := *decision.Snapshot
		snapshot.Windows = append([]quota.Window(nil), decision.Snapshot.Windows...)
		cloned.Snapshot = &snapshot
	}
	return &cloned
}

// RunPostIterationTests executes the run-start test plan and optionally reverts
// the last commit when verification fails. The return value is completion
func (r *Regent) RunPostIterationTests(ctx context.Context) bool {
	if r.testPlan != nil {
		for _, step := range r.testPlan.Steps {
			r.emit(fmt.Sprintf("Running tests: %s (%s)", step.Command, step.Dir))
		}
		result := testplan.Run(ctx, *r.testPlan)
		if result.Passed {
			commit, _ := r.git.LastCommit()
			r.emit(fmt.Sprintf("Tests passed — commit %s kept", commit))
			return true
		}
		for _, step := range result.Steps {
			if !step.Passed {
				r.emit(fmt.Sprintf("Tests failed: %s\n%s", step.Step.Command, step.Output))
				break
			}
		}
		if r.cfg.RollbackOnTestFailure {
			r.revertFailedIteration()
		}
		return false
	}
	if !r.cfg.RollbackOnTestFailure || r.cfg.TestCommand == "" {
		return true
	}

	r.emit("Running tests: " + r.cfg.TestCommand)
	result, err := RunTests(r.dir, r.cfg.TestCommand)
	if err != nil {
		r.emit(fmt.Sprintf("Failed to start tests: %v", err))
		return false
	}
	if result.Passed {
		commit, _ := r.git.LastCommit()
		r.emit(fmt.Sprintf("Tests passed — commit %s kept", commit))
		return true
	}
	r.revertFailedIteration()
	return false
}

func (r *Regent) revertFailedIteration() {
	r.emit("Tests failed — reverting last commit")
	sha, revertErr := RevertLastCommit(r.git)
	if revertErr != nil {
		r.emit(fmt.Sprintf("Failed to revert: %v", revertErr))
		return
	}
	r.emit(fmt.Sprintf("Reverted commit %s — pushed revert", sha))
}

// finishGraceful records an operator- or context-requested stop. A stopped run
// is not a successful completion.
func (r *Regent) finishGraceful() {
	r.mu.Lock()
	r.state.FinishedAt = time.Now()
	r.state.Passed = false
	r.state.ConsecutiveErrs = 0
	r.state.Status = StatusStopped
	r.mu.Unlock()
	r.saveState()
}

func (r *Regent) finishBlocked(status Status, reason string, resumeAt time.Time) {
	r.mu.Lock()
	r.state.FinishedAt = time.Now()
	r.state.Passed = false
	r.state.ConsecutiveErrs = 0
	r.state.Status = status
	r.state.BlockReason = reason
	r.state.ResumeAt = resumeAt
	r.mu.Unlock()
	r.saveState()
}

func (r *Regent) touchOutput() {
	r.mu.Lock()
	r.lastOutputAt = time.Now()
	r.state.LastOutputAt = r.lastOutputAt
	r.mu.Unlock()
}

func (r *Regent) emit(msg string) {
	if r.events == nil {
		return
	}
	entry := loop.LogEntry{
		Kind:      loop.LogRegent,
		Timestamp: time.Now(),
		Message:   msg,
		Agent:     r.state.Agent,
	}
	// Sending to a closed channel panics in Go even in a non-blocking select.
	// This can happen when saveState is called from the runWithRegent drain goroutine
	// after close(events): the goroutine still processes buffered entries, which
	// trigger UpdateState → saveState → emit on the already-closed channel.
	defer func() { _ = recover() }()
	select {
	case r.events <- entry:
	default:
	}
}

func (r *Regent) saveState() {
	r.mu.Lock()
	s := r.state
	r.mu.Unlock()

	if err := SaveState(r.dir, s); err != nil {
		r.emit(fmt.Sprintf("Failed to save state: %v", err))
	}
}

// FlushState persists the current in-memory state to disk. Callers should
// invoke this after all UpdateState calls are complete to ensure the final
// persisted state reflects all accumulated updates.
func (r *Regent) FlushState() {
	r.saveState()
}
