package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/LISSConsulting/RalphSpec/internal/claude"
	"github.com/LISSConsulting/RalphSpec/internal/config"
	"github.com/LISSConsulting/RalphSpec/internal/git"
	"github.com/LISSConsulting/RalphSpec/internal/loop"
	"github.com/LISSConsulting/RalphSpec/internal/orchestrator"
	"github.com/LISSConsulting/RalphSpec/internal/regent"
	"github.com/LISSConsulting/RalphSpec/internal/spec"
	"github.com/LISSConsulting/RalphSpec/internal/store"
	"github.com/LISSConsulting/RalphSpec/internal/tui"
	"github.com/LISSConsulting/RalphSpec/internal/worktree"
)

// runWithRegent runs the loop under Regent supervision without TUI.
// Events are drained to stdout using the provided formatter.
func runWithRegent(ctx context.Context, lp *loop.Loop, cfg *config.Config, gitRunner *git.Runner, dir string, sw store.Writer, formatter lineFormatter, run regent.RunFunc) error {
	events := make(chan loop.LogEntry, 128)
	lp.Events = events

	rgt := regent.New(cfg.Regent, dir, gitRunner, events)
	lp.PostIteration = rgt.RunPostIterationTests

	// Drain events to stdout and update regent state
	drainDone := make(chan struct{})
	go func() {
		defer close(drainDone)
		for entry := range events {
			if sw != nil {
				_ = sw.Append(entry)
			}
			if entry.Kind != loop.LogRegent {
				rgt.UpdateState(entry)
			}
			_, _ = fmt.Fprintln(os.Stdout, formatter.format(entry))
		}
	}()

	err := rgt.Supervise(ctx, run)
	close(events)
	<-drainDone
	// Flush persists any UpdateState changes made by the drain goroutine that
	// may have raced with Supervise's own saveState call. After drainDone, all
	// UpdateState calls are complete and a single flush produces the correct
	// final state on disk.
	rgt.FlushState()
	return err
}

// runWithRegentTUI runs the loop under Regent supervision with TUI display.
// Loop events are forwarded through the Regent for state/hang tracking, then
// sent to the TUI. Regent messages are sent directly to the TUI channel.
func runWithRegentTUI(ctx context.Context, lp *loop.Loop, cfg *config.Config, gitRunner *git.Runner, dir string, sw store.Writer, sr store.Reader, run regent.RunFunc) error {
	// Derive a loop-scoped context so we can tear the loop down (and its
	// Claude subprocess) the moment the TUI exits.
	loopCtx, cancelLoop := context.WithCancel(ctx)
	defer cancelLoop()

	loopEvents := make(chan loop.LogEntry, 128)
	tuiEvents := make(chan loop.LogEntry, 128)

	// Graceful stop: TUI 's' key closes stopCh; loop checks it after each iteration.
	stopCh := make(chan struct{})
	var stopOnce sync.Once
	requestStop := func() { stopOnce.Do(func() { close(stopCh) }) }
	lp.StopAfter = stopCh

	lp.Events = loopEvents
	rgt := regent.New(cfg.Regent, dir, gitRunner, tuiEvents)
	lp.PostIteration = rgt.RunPostIterationTests

	specFiles, _ := spec.List(dir)
	model := tui.New(tuiEvents, sr, cfg.TUI.AccentColor, cfg.Project.Name, dir, specFiles, requestStop, nil)
	program := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())

	// Forward loop events → regent state update → TUI
	forwardDone := make(chan struct{})
	go func() {
		defer close(forwardDone)
		for entry := range loopEvents {
			if sw != nil {
				_ = sw.Append(entry)
			}
			rgt.UpdateState(entry)
			select {
			case tuiEvents <- entry:
			default:
			}
		}
	}()

	// Run loop under Regent supervision; close channels when done
	errCh := make(chan error, 1)
	go func() {
		defer close(tuiEvents)
		superviseErr := rgt.Supervise(loopCtx, run)
		close(loopEvents)
		<-forwardDone
		errCh <- superviseErr
	}()

	tuiErr := finishTUI(program)

	// TUI exited — cancel and block on the loop goroutine so Claude is
	// actually killed before we return.
	cancelLoop()
	loopErr := <-errCh

	if tuiErr != nil {
		return tuiErr
	}
	if loopErr != nil && !errors.Is(loopErr, context.Canceled) {
		return loopErr
	}
	return nil
}

// finishTUI runs the bubbletea program and returns any loop error.
// Context cancellation errors are suppressed since they indicate normal
// shutdown (user quit, signal).
func finishTUI(program *tea.Program) error {
	finalModel, err := program.Run()
	if err != nil {
		return fmt.Errorf("tui: %w", err)
	}

	if m, ok := finalModel.(tui.Model); ok && m.Err() != nil {
		if errors.Is(m.Err(), context.Canceled) {
			return nil
		}
		return m.Err()
	}

	return nil
}

// runWithStateTracking runs the loop without Regent supervision in no-TUI mode,
// draining events to stdout and persisting state to .ralph/regent-state.json
// so that `ralph status` works even when the Regent is disabled.
func runWithStateTracking(ctx context.Context, lp *loop.Loop, dir string, gitRunner *git.Runner, mode string, sw store.Writer, formatter lineFormatter, run regent.RunFunc) error {
	events := make(chan loop.LogEntry, 128)
	lp.Events = events

	st := newStateTracker(dir, mode, lp.AgentType, gitRunner)
	st.save()

	drainDone := make(chan struct{})
	go func() {
		defer close(drainDone)
		for entry := range events {
			if sw != nil {
				_ = sw.Append(entry)
			}
			_, _ = fmt.Fprintln(os.Stdout, formatter.format(entry))
			st.trackEntry(entry)
		}
	}()

	runErr := run(ctx)
	close(events)
	<-drainDone

	st.finish(runErr)
	return runErr
}

// runWithTUIAndState runs the loop without Regent supervision with TUI display,
// forwarding events through a state tracker so `ralph status` works.
func runWithTUIAndState(ctx context.Context, lp *loop.Loop, dir string, gitRunner *git.Runner, mode string, accentColor string, projectName string, sw store.Writer, sr store.Reader, run regent.RunFunc) error {
	// Derive a loop-scoped context so we can tear the loop down (and its
	// Claude subprocess) the moment the TUI exits, without relying on
	// deferred cancel() chains that race with process exit.
	loopCtx, cancelLoop := context.WithCancel(ctx)
	defer cancelLoop()

	loopEvents := make(chan loop.LogEntry, 128)
	tuiEvents := make(chan loop.LogEntry, 128)

	// Graceful stop: TUI 's' key closes stopCh; loop checks it after each iteration.
	stopCh := make(chan struct{})
	var stopOnce sync.Once
	requestStop := func() { stopOnce.Do(func() { close(stopCh) }) }
	lp.StopAfter = stopCh

	lp.Events = loopEvents

	st := newStateTracker(dir, mode, lp.AgentType, gitRunner)
	st.save()

	specFiles, _ := spec.List(dir)
	model := tui.New(tuiEvents, sr, accentColor, projectName, dir, specFiles, requestStop, nil)
	program := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())

	// Forward loop events → state tracking → TUI
	forwardDone := make(chan struct{})
	go func() {
		defer close(forwardDone)
		for entry := range loopEvents {
			if sw != nil {
				_ = sw.Append(entry)
			}
			st.trackEntry(entry)
			select {
			case tuiEvents <- entry:
			default:
			}
		}
	}()

	errCh := make(chan error, 1)
	go func() {
		defer close(tuiEvents)
		runErr := run(loopCtx)
		close(loopEvents)
		<-forwardDone
		errCh <- runErr
	}()

	tuiErr := finishTUI(program)

	// TUI has exited — cancel the loop and block until its goroutine finishes
	// so exec.CommandContext actually tears Claude down before we return.
	cancelLoop()
	loopErr := <-errCh
	st.finish(loopErr)

	if tuiErr != nil {
		return tuiErr
	}
	if loopErr != nil && !errors.Is(loopErr, context.Canceled) {
		return loopErr
	}
	return nil
}

// stateTracker persists loop state to .ralph/regent-state.json for `ralph status`.
// Used in non-Regent paths where the Regent is not available to track state.
type stateTracker struct {
	state regent.State
	dir   string
}

func newStateTracker(dir, mode, agent string, gitRunner *git.Runner) *stateTracker {
	branch, _ := gitRunner.CurrentBranch()
	now := time.Now()
	return &stateTracker{
		dir: dir,
		state: regent.State{
			RalphPID:     os.Getpid(),
			Agent:        agent,
			Branch:       branch,
			Mode:         mode,
			StartedAt:    now,
			LastOutputAt: now,
		},
	}
}

func (s *stateTracker) trackEntry(entry loop.LogEntry) {
	changed := false
	if entry.Iteration > 0 {
		s.state.Iteration = entry.Iteration
		changed = true
	}
	if entry.TotalCost > 0 {
		s.state.TotalCostUSD = entry.TotalCost
		changed = true
	}
	if entry.Commit != "" {
		s.state.LastCommit = entry.Commit
		changed = true
	}
	if entry.Agent != "" {
		s.state.Agent = entry.Agent
		changed = true
	}
	if entry.Branch != "" {
		s.state.Branch = entry.Branch
		changed = true
	}
	if entry.Mode != "" {
		s.state.Mode = entry.Mode
		changed = true
	}
	s.state.LastOutputAt = time.Now()
	if changed {
		s.save()
	}
}

func (s *stateTracker) save() {
	_ = regent.SaveState(s.dir, s.state)
}

func (s *stateTracker) finish(err error) {
	s.state.FinishedAt = time.Now()
	s.state.Passed = err == nil || errors.Is(err, context.Canceled)
	s.save()
}

// loopController implements tui.LoopController for dashboard mode.
// It starts and stops loop runs in response to TUI key presses (b/p/R/x).
type loopController struct {
	cfg       *config.Config
	dir       string
	gitRunner *git.Runner
	sw        store.Writer
	tuiSend   chan<- loop.LogEntry
	outerCtx  context.Context
	mu        sync.Mutex
	cancel    context.CancelFunc
	done      chan struct{} // closed when the active runLoop goroutine exits; nil when idle
	// agent overrides the resolved runtime agent when non-nil.
	// Used in tests to inject a fast-failing fake.
	agent claude.Agent
}

// IsRunning reports whether a loop goroutine is currently active.
func (lc *loopController) IsRunning() bool {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	return lc.cancel != nil
}

// StartLoop starts a loop in the given mode ("build" or "roam").
// A no-op if a loop is already running.
func (lc *loopController) StartLoop(mode string) {
	lc.mu.Lock()
	if lc.cancel != nil {
		lc.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(lc.outerCtx)
	done := make(chan struct{})
	lc.cancel = cancel
	lc.done = done
	lc.mu.Unlock()

	go lc.runLoop(ctx, mode, done)
}

// StopLoop immediately cancels the running loop. No-op if idle.
func (lc *loopController) StopLoop() {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	if lc.cancel != nil {
		lc.cancel()
	}
}

// Shutdown cancels any running loop and blocks until its goroutine exits.
// Call this after the dashboard TUI closes so the Claude subprocess is torn
// down before Ralph returns to the shell.
func (lc *loopController) Shutdown() {
	lc.mu.Lock()
	cancel := lc.cancel
	done := lc.done
	lc.mu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	if done != nil {
		<-done
	}
}

// runLoop executes the loop and forwards events to the TUI channel.
func (lc *loopController) runLoop(ctx context.Context, mode string, done chan struct{}) {
	defer close(done)
	agent := lc.agent
	agentType, err := resolveAgent(lc.cfg, "")
	if err != nil {
		lc.emitLoopError(err)
		return
	}
	if err := validateAgentFlow(agentType, false, true); err != nil {
		lc.emitLoopError(err)
		return
	}
	if agent == nil {
		agent, err = buildAgent(agentType)
		if err != nil {
			lc.emitLoopError(err)
			return
		}
	}
	lp := &loop.Loop{
		Agent:     agent,
		AgentType: agentType,
		Git:       lc.gitRunner,
		Config:    lc.cfg,
		Dir:       lc.dir,
	}
	loopEvents := make(chan loop.LogEntry, 128)
	lp.Events = loopEvents

	forwardDone := make(chan struct{})
	go func() {
		defer close(forwardDone)
		for entry := range loopEvents {
			if lc.sw != nil {
				_ = lc.sw.Append(entry)
			}
			select {
			case lc.tuiSend <- entry:
			default:
			}
		}
	}()

	if mode == "roam" {
		lp.Roam = true
		lp.Focus = lc.cfg.Roam.Focus
	}
	runErr := lp.Run(ctx, loop.ModeBuild, 0)

	close(loopEvents)
	<-forwardDone

	lc.mu.Lock()
	lc.cancel = nil
	lc.done = nil
	lc.mu.Unlock()

	_ = runErr
}

func (lc *loopController) emitLoopError(err error) {
	select {
	case lc.tuiSend <- loop.LogEntry{Kind: loop.LogError, Message: fmt.Sprintf("Error: %v", err)}:
	default:
	}
	lc.mu.Lock()
	lc.cancel = nil
	lc.done = nil
	lc.mu.Unlock()
}

// runDashboard launches the TUI in idle (dashboard) state with no loop running.
// The user can press b/R to start a loop and x to stop it.
// When [worktree] is enabled in config, an Orchestrator is created and wired
// into the TUI so the W/x/M/D keybinds become active.
func runDashboard(ctx context.Context, cfg *config.Config, dir string, sw store.Writer, sr store.Reader) error {
	tuiEvents := make(chan loop.LogEntry, 128)
	// Note: tuiEvents is intentionally never closed; the TUI exits when user presses q.

	gitRunner := git.NewRunner(dir)
	ctrl := &loopController{
		cfg:       cfg,
		dir:       dir,
		gitRunner: gitRunner,
		sw:        sw,
		tuiSend:   tuiEvents,
		outerCtx:  ctx,
	}

	specFiles, _ := spec.List(dir)
	model := tui.New(tuiEvents, sr, cfg.TUI.AccentColor, cfg.Project.Name, dir, specFiles, nil, ctrl)

	// Wire orchestrator when worktree mode is enabled.
	if cfg.Worktree.Enabled {
		wtRunner := worktree.NewRunner(dir)
		wtRunner.WorktreeDir = cfg.Worktree.ResolvedWorktreeDir()
		if err := wtRunner.Detect(); err == nil {
			orch := orchestrator.New(cfg, wtRunner)
			model = model.WithOrchestrator(orch)
		}
		// If Detect fails (worktrunk not installed), silently skip worktree mode.
	}

	program := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	tuiErr := finishTUI(program)

	// Tear down any loop the user started from the dashboard so Claude doesn't
	// keep running after the TUI exits.
	ctrl.Shutdown()

	return tuiErr
}
