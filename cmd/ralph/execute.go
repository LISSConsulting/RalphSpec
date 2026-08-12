package main

import (
	"context"
	"fmt"
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
	"github.com/LISSConsulting/RalphSpec/internal/quota"
	"github.com/LISSConsulting/RalphSpec/internal/regent"
	"github.com/LISSConsulting/RalphSpec/internal/spec"
	"github.com/LISSConsulting/RalphSpec/internal/store"
	"github.com/LISSConsulting/RalphSpec/internal/testplan"
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
// executeRun: config load, validation, working dir, signal context, git
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

	discoveredPlan, err := discoverRunTestPlan(cfg, dir)
	if err != nil {
		return nil, err
	}

	gitRunner := git.NewRunner(dir)
	effectiveRoam := roam || cfg.Roam.Enabled
	agentType, err := resolveAgent(cfg, agentOverride)
	if err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}
	agentImpl, err := buildAgent(cfg, agentType)
	if err != nil {
		return nil, err
	}

	lp := &loop.Loop{
		Agent:     agentImpl,
		AgentType: agentType,
		Git:       gitRunner,
		Config:    cfg,
		Dir:       dir,
		TestPlan:  discoveredPlan,
	}
	if cfg.Quota.Enabled {
		var provider quota.Provider
		if supported, ok := agentImpl.(quota.Provider); ok {
			provider = supported
		}
		lp.QuotaGate = quota.NewGate(cfg.Quota, provider)
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

// executeLoop preserves the programmatic default without command-line overrides.
func executeLoop(mode loop.Mode, maxOverride int, noTUI bool, roam bool, focus string, noColor bool, useWorktree bool, agentOverride string) error {
	return executeLoopWithOptions(mode, maxOverride, noTUI, roam, focus, noColor, useWorktree, agentOverride, false)
}

func executeLoopWithOptions(mode loop.Mode, maxOverride int, noTUI bool, roam bool, focus string, noColor bool, useWorktree bool, agentOverride string, ludicrous bool) error {
	setup, err := setupLoop(noTUI, roam, noColor, agentOverride)
	if err != nil {
		return err
	}
	defer setup.cancel()
	if ludicrous {
		setup.cfg.Build.Ludicrous = true
	}
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

	setup.lp.Roam = setup.effectiveRoam
	if focus != "" {
		setup.lp.Focus = focus
	} else {
		setup.lp.Focus = setup.cfg.Roam.Focus
	}

	// Pre-flight: verify the prompt file exists before launching TUI or Regent.
	// Without this check the TUI initialises, then fails on the first iteration
	// with a confusing "loop: read prompt …: open …: no such file or directory".
	promptFile := promptFileForRun(setup.cfg, setup.lp.Roam)
	if _, statErr := os.Stat(filepath.Join(setup.lp.Dir, promptFile)); statErr != nil {
		return fmt.Errorf("prompt file %s: %w", promptFile, statErr)
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

// setupWorktree detects worktrunk, creates/switches to a Ralph-owned worktree
// branch based on the current branch, and updates setup.lp.Dir and
// setup.gitRunner to point at the worktree directory. Must be called before any
// prompt pre-flight checks.
func setupWorktree(setup *loopSetup) error {
	wtr := worktree.NewRunner(setup.dir)
	if setup.cfg != nil {
		wtr.WorktreeDir = setup.cfg.Worktree.ResolvedWorktreeDir()
	}
	if err := wtr.Detect(); err != nil {
		return err
	}

	baseBranch, err := setup.gitRunner.CurrentBranch()
	if err != nil {
		return fmt.Errorf("worktree: get current branch: %w", err)
	}
	branch := ralphWorktreeBranch(baseBranch, time.Now())

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
		fmt.Fprintf(os.Stderr, "ralph: created worktree for %s from %s at %s\n", branch, baseBranch, wtPath)
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

func ralphWorktreeBranch(baseBranch string, now time.Time) string {
	baseBranch = strings.TrimSpace(strings.TrimPrefix(baseBranch, "refs/heads/"))
	if baseBranch == "" {
		baseBranch = "detached"
	}
	stamp := now.UTC().Format("20060102-150405") + fmt.Sprintf("-%09d", now.UTC().Nanosecond())
	return "ralph/" + baseBranch + "/" + stamp
}

// executeRun runs the build loop; --roam switches it to ROAM.md.
func executeRun(maxOverride int, noTUI bool, roam bool, focus string, noColor bool, useWorktree bool, agentOverride string) error {
	return executeRunWithOptions(maxOverride, noTUI, roam, focus, noColor, useWorktree, agentOverride, false)
}

func executeRunWithOptions(maxOverride int, noTUI bool, roam bool, focus string, noColor bool, useWorktree bool, agentOverride string, ludicrous bool) error {
	setup, err := setupLoop(noTUI, roam, noColor, agentOverride)
	if err != nil {
		return err
	}
	defer setup.cancel()
	defer setup.cleanup()
	if ludicrous {
		setup.cfg.Build.Ludicrous = true
	}

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
		effectiveFocus = setup.cfg.Roam.Focus
	}
	setup.lp.Roam = setup.effectiveRoam
	setup.lp.Focus = effectiveFocus

	promptFile := promptFileForRun(setup.cfg, setup.lp.Roam)
	if _, statErr := os.Stat(filepath.Join(setup.lp.Dir, promptFile)); statErr != nil {
		return fmt.Errorf("prompt file %s: %w", promptFile, statErr)
	}

	runFn := func(ctx context.Context) error {
		return setup.lp.Run(ctx, loop.ModeBuild, maxOverride)
	}

	if !setup.cfg.Regent.Enabled {
		if noTUI {
			return runWithStateTracking(setup.ctx, setup.lp, setup.lp.Dir, setup.gitRunner, "run", setup.sw, setup.formatter, runFn)
		}
		return runWithTUIAndState(setup.ctx, setup.lp, setup.lp.Dir, setup.gitRunner, "run", setup.cfg.TUI.AccentColor, setup.cfg.Project.Name, setup.sw, setup.sr, runFn)
	}

	if noTUI {
		return runWithRegent(setup.ctx, setup.lp, setup.cfg, setup.gitRunner, setup.lp.Dir, setup.sw, setup.formatter, runFn)
	}
	return runWithRegentTUI(setup.ctx, setup.lp, setup.cfg, setup.gitRunner, setup.lp.Dir, setup.sw, setup.sr, runFn)
}

func promptFileForRun(cfg *config.Config, roam bool) string {
	if roam {
		return cfg.Roam.PromptFile
	}
	return cfg.Build.PromptFile
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
	result := classifyResult(state)
	return formatStatusWithResult(state, now, result)
}

func formatStatusWithPIDCheck(state regent.State, now time.Time, pidRunning func(int) bool) string {
	result := classifyResultWithPIDCheck(state, pidRunning)
	return formatStatusWithResult(state, now, result)
}

func formatStatusWithResult(state regent.State, now time.Time, result statusResult) string {
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
	if state.Provider != "" {
		fmt.Fprintf(&b, "  %-20s %s\n", "Provider:", state.Provider)
	}
	mode := state.Mode
	if state.Ludicrous {
		if mode == "" {
			mode = "ludicrous"
		} else {
			mode += " (ludicrous)"
		}
	}
	if mode != "" {
		fmt.Fprintf(&b, "  %-20s %s\n", "Mode:", mode)
	}
	if state.Status != "" {
		fmt.Fprintf(&b, "  %-20s %s\n", "State:", state.Status)
	}
	if state.BlockReason != "" {
		fmt.Fprintf(&b, "  %-20s %s\n", "Blocked:", state.BlockReason)
	}
	if !state.ResumeAt.IsZero() {
		fmt.Fprintf(&b, "  %-20s %s\n", "Resume at:", state.ResumeAt.Format(time.RFC3339))
	}
	if state.TestPlan != nil {
		source := "discovered"
		if state.TestPlan.Explicit {
			source = "explicit"
		}
		commands := make([]string, 0, len(state.TestPlan.Steps))
		for _, step := range state.TestPlan.Steps {
			commands = append(commands, step.Command)
		}
		fmt.Fprintf(&b, "  %-20s %s (%d): %s\n", "Test plan:", source, len(commands), strings.Join(commands, "; "))
		for _, diagnostic := range state.TestPlan.Diagnostics {
			fmt.Fprintf(&b, "  %-20s %s\n", "Test diagnostic:", diagnostic)
		}
	}
	if state.Quota != nil {
		fmt.Fprintf(&b, "  %-20s %s — %s\n", "Quota:", state.Quota.Action, state.Quota.Reason)
	}
	if state.LastCommit != "" {
		fmt.Fprintf(&b, "  %-20s %s\n", "Last commit:", state.LastCommit)
	}
	fmt.Fprintf(&b, "  %-20s %d\n", "Iteration:", state.Iteration)
	fmt.Fprintf(&b, "  %-20s $%.2f\n", "Total cost:", state.TotalCostUSD)

	switch {
	case result == statusRunning:
		elapsed := now.Sub(state.StartedAt).Round(time.Second)
		fmt.Fprintf(&b, "  %-20s %s (running)\n", "Duration:", elapsed)
	case !state.StartedAt.IsZero() && state.FinishedAt.IsZero():
		elapsed := now.Sub(state.StartedAt).Round(time.Second)
		fmt.Fprintf(&b, "  %-20s %s\n", "Duration:", elapsed)
	case !state.StartedAt.IsZero() && !state.FinishedAt.IsZero():
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
	case statusPaused:
		fmt.Fprintf(&b, "  %-20s %s\n", "Result:", "paused for quota")
	case statusBlocked:
		fmt.Fprintf(&b, "  %-20s %s\n", "Result:", "blocked for operator action")
	case statusStopped:
		fmt.Fprintf(&b, "  %-20s %s\n", "Result:", "stopped")
	case statusFailWithErrors:
		fmt.Fprintf(&b, "  %-20s fail (%d consecutive errors)\n", "Result:", state.ConsecutiveErrs)
	case statusFail:
		fmt.Fprintf(&b, "  %-20s %s\n", "Result:", "fail")
	}

	return b.String()
}
func discoverRunTestPlan(cfg *config.Config, dir string) (*testplan.Plan, error) {
	if !cfg.Regent.AutoDiscoverTests && strings.TrimSpace(cfg.Regent.TestCommand) == "" {
		return nil, nil
	}
	plan, err := testplan.Discover(dir, cfg.Regent.TestCommand)
	if err != nil {
		return nil, fmt.Errorf("test plan discovery: %w", err)
	}
	if len(plan.Steps) == 0 {
		return nil, fmt.Errorf("test plan discovery was inconclusive; configure regent.test_command or add declared test metadata")
	}
	return &plan, nil
}

func resolveAgent(cfg *config.Config, override string) (string, error) {
	agentType := cfg.Agent.Type
	if override != "" {
		agentType = override
	}
	if agentType == "" {
		agentType = config.AgentClaude
	}
	switch agentType {
	case config.AgentClaude, config.AgentCodex:
		return agentType, nil
	default:
		return "", fmt.Errorf("agent.type must be one of %s,%s", config.AgentClaude, config.AgentCodex)
	}
}

// harnessExecutable resolves the executable for an agent type, honoring the
// [harness] overrides (e.g. claude-kimi, codex-minimax shims).
func harnessExecutable(cfg *config.Config, agentType string) string {
	switch agentType {
	case config.AgentCodex:
		if cfg.Harness.Codex != "" {
			return cfg.Harness.Codex
		}
		return "codex"
	default:
		if cfg.Harness.Claude != "" {
			return cfg.Harness.Claude
		}
		return "claude"
	}
}

func buildAgent(cfg *config.Config, agentType string) (claude.Agent, error) {
	exe := harnessExecutable(cfg, agentType)
	switch agentType {
	case config.AgentCodex:
		if err := codex.CheckAvailable(exe); err != nil {
			return nil, fmt.Errorf("codex agent unavailable: %w; install or log into Codex CLI, or use --agent claude", err)
		}
		return &codex.Agent{Executable: exe}, nil
	case config.AgentClaude:
		router, err := claude.LoadProviderRouter(
			cfg.Claude.Provider,
			cfg.Claude.FallbackProviders,
			cfg.Claude.ProviderConfigFile,
			os.Environ(),
		)
		if err != nil {
			return nil, fmt.Errorf("claude provider fallback: %w", err)
		}
		return &loop.ClaudeAgent{
			Executable:          exe,
			QuotaSnapshotFile:   cfg.Claude.QuotaSnapshotFile,
			QuotaSnapshotMaxAge: time.Duration(cfg.Claude.QuotaSnapshotMaxAgeSeconds) * time.Second,
			ProviderRouter:      router,
		}, nil
	default:
		return nil, fmt.Errorf("agent.type must be one of %s,%s", config.AgentClaude, config.AgentCodex)
	}
}

func validateAgentFlow(agentType string, useWorktree bool, dashboard bool) error {
	_ = agentType
	_ = useWorktree
	_ = dashboard
	return nil
}

// statusResult represents the outcome classification for ralph status display.
type statusResult int

const (
	statusNoState statusResult = iota
	statusRunning
	statusPass
	statusPaused
	statusBlocked
	statusStopped
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
	switch state.Status {
	case regent.StatusRunning:
		if pidRunning(state.RalphPID) {
			return statusRunning
		}
	case regent.StatusPassed:
		return statusPass
	case regent.StatusPausedQuota:
		return statusPaused
	case regent.StatusBlocked:
		return statusBlocked
	case regent.StatusStopped:
		return statusStopped
	case regent.StatusFailed:
		if state.ConsecutiveErrs > 0 {
			return statusFailWithErrors
		}
		return statusFail
	}
	running := !state.StartedAt.IsZero() && state.FinishedAt.IsZero()
	switch {
	case running && pidRunning(state.RalphPID):
		return statusRunning
	case running && state.ConsecutiveErrs > 0:
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
	exe := "claude"
	if cfg, err := config.Load(""); err == nil {
		exe = harnessExecutable(cfg, config.AgentClaude)
	}
	cmd := exec.CommandContext(ctx, exe, cmdArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
