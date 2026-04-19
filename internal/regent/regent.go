package regent

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/LISSConsulting/RalphSpec/internal/config"
	"github.com/LISSConsulting/RalphSpec/internal/loop"
)

// RunFunc is the function the Regent supervises. Typically wraps loop.Loop.Run.
type RunFunc func(ctx context.Context) error

// Regent supervises the Ralph loop: crash detection, hang detection,
// test-gated rollback, and state persistence.
type Regent struct {
	cfg    config.RegentConfig
	dir    string
	git    GitOps
	events chan<- loop.LogEntry

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

		if err == nil {
			r.mu.Lock()
			r.state.ConsecutiveErrs = 0
			r.state.FinishedAt = time.Now()
			r.state.Passed = true
			r.mu.Unlock()

			r.saveState()
			return nil
		}

		if ctx.Err() != nil {
			r.emit("Context cancelled — stopping")
			r.finishGraceful()
			return ctx.Err()
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
	r.mu.Unlock()

	if changed {
		r.saveState()
	}
}

// RunPostIterationTests runs the configured test command and reverts the last
// commit if tests fail. Designed to be called via Loop.PostIteration after each
// iteration for per-iteration test-gated rollback per the Regent spec. Errors
// are emitted as events rather than returned, so the loop continues to the
// next iteration.
func (r *Regent) RunPostIterationTests() {
	if !r.cfg.RollbackOnTestFailure || r.cfg.TestCommand == "" {
		return
	}

	r.emit("Running tests: " + r.cfg.TestCommand)
	result, err := RunTests(r.dir, r.cfg.TestCommand)
	if err != nil {
		r.emit(fmt.Sprintf("Failed to start tests: %v", err))
		return
	}

	if result.Passed {
		commit, _ := r.git.LastCommit()
		r.emit(fmt.Sprintf("Tests passed ✅ — commit %s kept", commit))
		return
	}

	r.emit("Tests failed ❌ — reverting last commit")
	sha, revertErr := RevertLastCommit(r.git)
	if revertErr != nil {
		r.emit(fmt.Sprintf("Failed to revert: %v", revertErr))
		return
	}
	r.emit(fmt.Sprintf("Reverted commit %s — pushed revert", sha))
}

// finishGraceful sets FinishedAt and Passed=true for context-cancelled exits.
// Context cancellation is a user-initiated stop, not a failure.
func (r *Regent) finishGraceful() {
	r.mu.Lock()
	r.state.FinishedAt = time.Now()
	r.state.Passed = true
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
