package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/LISSConsulting/RalphSpec/internal/claude"
	"github.com/LISSConsulting/RalphSpec/internal/codex"
	"github.com/LISSConsulting/RalphSpec/internal/config"
	"github.com/LISSConsulting/RalphSpec/internal/git"
	"github.com/LISSConsulting/RalphSpec/internal/loop"
	"github.com/LISSConsulting/RalphSpec/internal/notify"
	"github.com/LISSConsulting/RalphSpec/internal/regent"
	"github.com/LISSConsulting/RalphSpec/internal/spec"
	"github.com/LISSConsulting/RalphSpec/internal/store"
	"github.com/LISSConsulting/RalphSpec/internal/worktree"
)

// loopSetup holds all shared state initialised by setupLoop.
// Callers must defer both cancel() and cleanup() after a successful call.
type loopSetup struct {
	cfg           *config.Config
	dir           string
	ctx           context.Context
	cancel        context.CancelFunc
	gitRunner     *git.Runner
	lp            *loop.Loop
	effectiveRoam bool
	sw            store.Writer
	sr            store.Reader
	formatter     lineFormatter
	cleanup       func() // closes the JSONL store if one was opened
	agentType     string
}

// setupLoop performs the common initialisation shared by executeLoop and
// executeSmartRun: config load, validation, working dir, signal context, git
// runner, loop struct init, spec resolution, and store init.
func setupLoop(noTUI, roam, noColor bool, agentOverride string) (*loopSetup, error) {
	cfg, err := config.Load("")
	if err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}

	dir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get working directory: %w", err)
	}

	var ctx context.Context
	var cancel context.CancelFunc
	var stopCh <-chan struct{}
	if noTUI {
		ctx, cancel, stopCh = signalContextGraceful()
	} else {
		ctx, cancel = signalContext()
	}

	gitRunner := git.NewRunner(dir)
	effectiveRoam := roam || cfg.Build.Roam
	agentType, err := resolveAgent(cfg, agentOverride)
	if err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}
	agentImpl, err := buildAgent(agentType)
	if err != nil {
		return nil, err
	}

	lp := &loop.Loop{
		Agent:     agentImpl,
		AgentType: agentType,
		Git:       gitRunner,
		Config:    cfg,
		Dir:       dir,
	}
	if stopCh != nil {
		lp.StopAfter = stopCh
	}
	if cfg.Notifications.URL != "" {
		n := notify.New(cfg.Notifications.URL, cfg.Project.Name,
			cfg.Notifications.OnComplete, cfg.Notifications.OnError, cfg.Notifications.OnStop)
		lp.NotificationHook = n.Hook
	}

	if !effectiveRoam {
		if branch, branchErr := gitRunner.CurrentBranch(); branchErr == nil {
			if as, resolveErr := spec.Resolve(dir, "", branch); resolveErr == nil {
				lp.Spec = as.Name
				lp.SpecDir = as.Dir
			}
		}
	}

	logsDir := filepath.Join(dir, ".ralph", "logs")
	var sw store.Writer
	var sr store.Reader
	cleanup := func() {}
	if s, storeErr := store.NewJSONL(logsDir); storeErr != nil {
		fmt.Fprintf(os.Stderr, "ralph: session log unavailable: %v\n", storeErr)
	} else {
		if retErr := store.EnforceRetention(logsDir, cfg.TUI.LogRetention); retErr != nil {
			fmt.Fprintf(os.Stderr, "ralph: log retention: %v\n", retErr)
		}
		sw = s
		sr = s
		cleanup = func() { _ = s.Close() }
	}

	return &loopSetup{
		cfg:           cfg,
		dir:           dir,
		ctx:           ctx,
		cancel:        cancel,
		gitRunner:     gitRunner,
		lp:            lp,
		effectiveRoam: effectiveRoam,
		sw:            sw,
		sr:            sr,
		formatter:     lineFormatter{color: !noColor},
		cleanup:       cleanup,
		agentType:     agentType,
	}, nil
}

// executeLoop loads config, builds the loop, and runs it in the given mode.
func executeLoop(mode loop.Mode, maxOverride int, noTUI bool, roam bool, focus string, noColor bool, useWorktree bool, agentOverride string) error {
	setup, err := setupLoop(noTUI, roam, noColor, agentOverride)
	if err != nil {
		return err
	}
	defer setup.cancel()
	defer setup.cleanup()

	if err := validateAgentFlow(setup.agentType, useWorktree, false); err != nil {
		return err
	}

	// Worktree mode: create an isolated worktree and run the loop inside it.
	if useWorktree {
		if wtErr := setupWorktree(setup); wtErr != nil {
			return wtErr
		}
	}

	// Pre-flight: verify the prompt file exists before launching TUI or Regent.
	// Without this check the TUI initialises, then fails on the first iteration
	// with a confusing "loop: read prompt …: open …: no such file or directory".
	var promptFile string
	switch mode {
	case loop.ModePlan:
		promptFile = setup.cfg.Plan.PromptFile
	default:
		promptFile = setup.cfg.Build.PromptFile
	}
	if _, statErr := os.Stat(filepath.Join(setup.lp.Dir, promptFile)); statErr != nil {
		return fmt.Errorf("prompt file %s: %w", promptFile, statErr)
	}

	setup.lp.Roam = setup.effectiveRoam
	if focus != "" {
		setup.lp.Focus = focus
	} else {
		setup.lp.Focus = setup.cfg.Build.Focus
	}

	runFn := func(ctx context.Context) error {
		return setup.lp.Run(ctx, mode, maxOverride)
	}

	if !setup.cfg.Regent.Enabled {
		if noTUI {
			return runWithStateTracking(setup.ctx, setup.lp, setup.lp.Dir, setup.gitRunner, string(mode), setup.sw, setup.formatter, runFn)
		}
		return runWithTUIAndState(setup.ctx, setup.lp, setup.lp.Dir, setup.gitRunner, string(mode), setup.cfg.TUI.AccentColor, setup.cfg.Project.Name, setup.sw, setup.sr, runFn)
	}

	if noTUI {
		return runWithRegent(setup.ctx, setup.lp, setup.cfg, setup.gitRunner, setup.lp.Dir, setup.sw, setup.formatter, runFn)
	}
	return runWithRegentTUI(setup.ctx, setup.lp, setup.cfg, setup.gitRunner, setup.lp.Dir, setup.sw, setup.sr, runFn)
}

// setupWorktree detects worktrunk, creates/switches to the worktree for the
// current branch, and updates setup.lp.Dir and setup.gitRunner to point at the
// worktree directory. Must be called before any prompt pre-flight checks.
func setupWorktree(setup *loopSetup) error {
	wtr := worktree.NewRunner(setup.dir)
	if setup.cfg != nil {
		wtr.WorktreeDir = setup.cfg.Worktree.ResolvedWorktreeDir()
	}
	if err := wtr.Detect(); err != nil {
		return err
	}

	branch, err := setup.gitRunner.CurrentBranch()
	if err != nil {
		return fmt.Errorf("worktree: get current branch: %w", err)
	}

	// Try to create a new worktree; if it already exists, switch to it.
	wtPath, err := wtr.Switch(branch, true)
	if err != nil {
		// Retry without -c (reuse existing worktree).
		wtPath, err = wtr.Switch(branch, false)
		if err != nil {
			return fmt.Errorf("worktree: switch to %s: %w", branch, err)
		}
		fmt.Fprintf(os.Stderr, "ralph: reusing existing worktree for %s at %s\n", branch, wtPath)
	} else {
		fmt.Fprintf(os.Stderr, "ralph: created worktree for %s at %s\n", branch, wtPath)
	}

	// Guard: if the worktree path resolves to the same directory we
	// started in, worktrunk didn't actually create a separate worktree
	// (e.g. the branch is already checked out in the main working tree).
	absWT, _ := filepath.Abs(wtPath)
	absDir, _ := filepath.Abs(setup.dir)
	if absWT == absDir {
		return fmt.Errorf("worktree: branch %s is already checked out in %s — switch to a different branch before using --worktree", branch, absDir)
	}

	// Update the loop to operate in the worktree directory.
	setup.lp.Dir = wtPath
	setup.lp.Git = git.NewRunner(wtPath)
	setup.gitRunner = git.NewRunner(wtPath)

	// Re-initialise the session log in the worktree.
	logsDir := filepath.Join(wtPath, ".ralph", "logs")
	if s, storeErr := store.NewJSONL(logsDir); storeErr != nil {
		fmt.Fprintf(os.Stderr, "ralph: worktree session log unavailable: %v\n", storeErr)
	} else {
		// Close the old store if one was opened.
		setup.cleanup()
		setup.sw = s
		setup.sr = s
		setup.cleanup = func() { _ = s.Close() }
	}

	return nil
}

// executeSmartRun runs plan if CHRONICLE.md doesn't exist, then build.
func executeSmartRun(maxOverride int, noTUI bool, roam bool, focus string, noColor bool, useWorktree bool, agentOverride string) error {
	setup, err := setupLoop(noTUI, roam, noColor, agentOverride)
	if err != nil {
		return err
	}
	defer setup.cancel()
	defer setup.cleanup()

	if err := validateAgentFlow(setup.agentType, useWorktree, false); err != nil {
		return err
	}

	if useWorktree {
		if wtErr := setupWorktree(setup); wtErr != nil {
			return wtErr
		}
	}

	effectiveFocus := focus
	if effectiveFocus == "" {
		effectiveFocus = setup.cfg.Build.Focus
	}

	smartRunFn := func(ctx context.Context) error {
		// Check inside the closure so Regent retries re-evaluate whether
		// the plan file exists (it may have been created by a prior attempt).
		planPath := filepath.Join(setup.lp.Dir, "CHRONICLE.md")
		info, statErr := os.Stat(planPath)
		if needsPlanPhase(info, statErr) {
			if planErr := setup.lp.Run(ctx, loop.ModePlan, 0); planErr != nil {
				return fmt.Errorf("plan phase: %w", planErr)
			}
		}
		setup.lp.Roam = setup.effectiveRoam
		setup.lp.Focus = effectiveFocus
		return setup.lp.Run(ctx, loop.ModeBuild, maxOverride)
	}

	if !setup.cfg.Regent.Enabled {
		if noTUI {
			return runWithStateTracking(setup.ctx, setup.lp, setup.lp.Dir, setup.gitRunner, "run", setup.sw, setup.formatter, smartRunFn)
		}
		return runWithTUIAndState(setup.ctx, setup.lp, setup.lp.Dir, setup.gitRunner, "run", setup.cfg.TUI.AccentColor, setup.cfg.Project.Name, setup.sw, setup.sr, smartRunFn)
	}

	if noTUI {
		return runWithRegent(setup.ctx, setup.lp, setup.cfg, setup.gitRunner, setup.lp.Dir, setup.sw, setup.formatter, smartRunFn)
	}
	return runWithRegentTUI(setup.ctx, setup.lp, setup.cfg, setup.gitRunner, setup.lp.Dir, setup.sw, setup.sr, smartRunFn)
}

// showStatus reads .ralph/regent-state.json and prints a formatted summary
// including branch, last commit, iteration count, total cost, duration, and pass/fail.
func showStatus() error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	state, err := regent.LoadState(dir)
	if err != nil {
		return err
	}

	fmt.Print(formatStatus(state, time.Now()))
	return nil
}

// formatStatus renders a Regent state snapshot as a human-readable status
// string. The now parameter pins the current time for deterministic output.
func formatStatus(state regent.State, now time.Time) string {
	return formatStatusWithPIDCheck(state, now, processExists)
}

func formatStatusWithPIDCheck(state regent.State, now time.Time, pidRunning func(int) bool) string {
	result := classifyResultWithPIDCheck(state, pidRunning)
	if result == statusNoState {
		return "No state found. Run 'ralph build' or 'ralph run' first.\n"
	}

	var b strings.Builder
	b.WriteString("Ralph Status\n")
	b.WriteString("────────────\n")

	if state.Branch != "" {
		fmt.Fprintf(&b, "  %-20s %s\n", "Branch:", state.Branch)
	}
	if state.Agent != "" {
		fmt.Fprintf(&b, "  %-20s %s\n", "Agent:", state.Agent)
	}
	if state.Mode != "" {
		fmt.Fprintf(&b, "  %-20s %s\n", "Mode:", state.Mode)
	}
	if state.LastCommit != "" {
		fmt.Fprintf(&b, "  %-20s %s\n", "Last commit:", state.LastCommit)
	}
	fmt.Fprintf(&b, "  %-20s %d\n", "Iteration:", state.Iteration)
	fmt.Fprintf(&b, "  %-20s $%.2f\n", "Total cost:", state.TotalCostUSD)

	if result == statusRunning {
		elapsed := now.Sub(state.StartedAt).Round(time.Second)
		fmt.Fprintf(&b, "  %-20s %s (running)\n", "Duration:", elapsed)
	} else if !state.StartedAt.IsZero() && state.FinishedAt.IsZero() {
		elapsed := now.Sub(state.StartedAt).Round(time.Second)
		fmt.Fprintf(&b, "  %-20s %s\n", "Duration:", elapsed)
	} else if !state.StartedAt.IsZero() && !state.FinishedAt.IsZero() {
		dur := state.FinishedAt.Sub(state.StartedAt).Round(time.Second)
		fmt.Fprintf(&b, "  %-20s %s\n", "Duration:", dur)
	}

	if result == statusRunning && !state.LastOutputAt.IsZero() {
		ago := now.Sub(state.LastOutputAt).Round(time.Second)
		fmt.Fprintf(&b, "  %-20s %s ago\n", "Last output:", ago)
	}

	switch result {
	case statusRunning:
		fmt.Fprintf(&b, "  %-20s %s\n", "Result:", "running")
	case statusPass:
		fmt.Fprintf(&b, "  %-20s %s\n", "Result:", "pass")
	case statusFailWithErrors:
		fmt.Fprintf(&b, "  %-20s fail (%d consecutive errors)\n", "Result:", state.ConsecutiveErrs)
	case statusFail:
		fmt.Fprintf(&b, "  %-20s %s\n", "Result:", "fail")
	}

	return b.String()
}

func resolveAgent(cfg *config.Config, override string) (string, error) {
	agentType := cfg.Agent.Type
	if override != "" {
		agentType = override
	}
	if agentType == "" {
		agentType = config.AgentClaude
	}
	if agentType != config.AgentClaude && agentType != config.AgentCodex {
		return "", fmt.Errorf("agent.type must be one of %s,%s", config.AgentClaude, config.AgentCodex)
	}
	return agentType, nil
}

func buildAgent(agentType string) (claude.Agent, error) {
	if agentType == config.AgentCodex {
		if err := codex.CheckAvailable(""); err != nil {
			return nil, fmt.Errorf("codex agent unavailable: %w; install or log into Codex CLI, or use --agent claude", err)
		}
		return codex.NewAgent(), nil
	}
	return loop.NewClaudeAgent(), nil
}

func validateAgentFlow(agentType string, useWorktree bool, dashboard bool) error {
	if agentType != config.AgentCodex {
		return nil
	}
	if useWorktree {
		return fmt.Errorf("codex agent unsupported for worktree mode; use --agent claude or omit --worktree")
	}
	if dashboard {
		return fmt.Errorf("codex agent unsupported for dashboard mode; start Codex with ralph build --agent codex --no-tui or use claude in the dashboard")
	}
	return nil
}

// statusResult represents the outcome classification for ralph status display.
type statusResult int

const (
	statusNoState statusResult = iota
	statusRunning
	statusPass
	statusFailWithErrors
	statusFail
)

// classifyResult determines the result label from a Regent state snapshot.
// The priority order is: no-state, running, pass, fail-with-errors, plain-fail.
func classifyResult(state regent.State) statusResult {
	return classifyResultWithPIDCheck(state, processExists)
}

func classifyResultWithPIDCheck(state regent.State, pidRunning func(int) bool) statusResult {
	if state.RalphPID == 0 && state.Iteration == 0 {
		return statusNoState
	}
	running := !state.StartedAt.IsZero() && state.FinishedAt.IsZero()
	switch {
	case running && pidRunning(state.RalphPID):
		return statusRunning
	case state.Passed:
		return statusPass
	case state.ConsecutiveErrs > 0:
		return statusFailWithErrors
	case running:
		return statusFail
	case state.Passed:
		return statusPass
	case state.ConsecutiveErrs > 0:
		return statusFailWithErrors
	case !state.FinishedAt.IsZero():
		return statusFail
	default:
		return statusNoState
	}
}

// needsPlanPhase reports whether the plan phase should run based on the
// result of os.Stat on CHRONICLE.md. Returns true if the file
// does not exist or is empty.
func needsPlanPhase(info fs.FileInfo, statErr error) bool {
	return statErr != nil || info == nil || info.Size() == 0
}

// executeDashboard launches the TUI in idle/dashboard state.
// The user can press b/p/R to start a loop and x to stop it from within the TUI.
func executeDashboard() error {
	cfg, err := config.Load("")
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("config validation: %w", err)
	}

	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	ctx, cancel := signalContext()
	defer cancel()

	logsDir := filepath.Join(dir, ".ralph", "logs")
	var sw store.Writer
	var sr store.Reader
	if s, err := store.NewJSONL(logsDir); err != nil {
		fmt.Fprintf(os.Stderr, "ralph: session log unavailable: %v\n", err)
	} else {
		if retErr := store.EnforceRetention(logsDir, cfg.TUI.LogRetention); retErr != nil {
			fmt.Fprintf(os.Stderr, "ralph: log retention: %v\n", retErr)
		}
		sw = s
		sr = s
		defer func() { _ = s.Close() }()
	}

	return runDashboard(ctx, cfg, dir, sw, sr)
}

// executeSpeckit spawns claude with the given skill. When interactive is true,
// claude runs without -p flag so the user can answer questions inline.
// When interactive is false, uses -p prompt mode for non-interactive execution.
// Returns Claude's exit code as an error when non-zero.
func executeSpeckit(ctx context.Context, skill string, args []string, interactive bool) error {
	var cmdArgs []string
	if interactive {
		// Interactive mode: pass the skill as a slash command without -p so
		// Claude stays open and can ask follow-up questions.
		slashCmd := "/" + skill
		if len(args) > 0 {
			slashCmd += " " + strings.Join(args, " ")
		}
		cmdArgs = []string{slashCmd, "--verbose"}
	} else {
		prompt := "/" + skill
		if len(args) > 0 {
			prompt += " " + strings.Join(args, " ")
		}
		cmdArgs = []string{"-p", prompt, "--verbose"}
	}
	cmd := exec.CommandContext(ctx, "claude", cmdArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
