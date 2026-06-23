package orchestrator

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/LISSConsulting/RalphSpec/internal/config"
	"github.com/LISSConsulting/RalphSpec/internal/loop"
	"github.com/LISSConsulting/RalphSpec/internal/worktree"
)

// fakeWorktreeOps is a test double for WorktreeOps.
type fakeWorktreeOps struct {
	switchPath string
	switchErr  error
	mergeErr   error
	removeErr  error
	listResult []worktree.WorktreeInfo
}

func (f *fakeWorktreeOps) Detect() error                           { return nil }
func (f *fakeWorktreeOps) Switch(_ string, _ bool) (string, error) { return f.switchPath, f.switchErr }
func (f *fakeWorktreeOps) List() ([]worktree.WorktreeInfo, error)  { return f.listResult, nil }
func (f *fakeWorktreeOps) Merge(_, _ string) error                 { return f.mergeErr }
func (f *fakeWorktreeOps) Remove(_ string) error                   { return f.removeErr }

func defaultCfg() *config.Config {
	cfg := config.Defaults()
	cfg.Worktree.MaxParallel = 2
	return &cfg
}

func newTestOrchestrator(ops worktree.WorktreeOps) *Orchestrator {
	return New(defaultCfg(), ops)
}

// ─── ActiveAgents / AgentByBranch / RunningCount ─────────────────────────────

func TestActiveAgents_FiltersRemoved(t *testing.T) {
	o := newTestOrchestrator(&fakeWorktreeOps{switchPath: "/tmp/wt"})

	// Manually insert agents in various states.
	o.agents["running"] = &WorktreeAgent{Branch: "running", State: StateRunning}
	o.agents["completed"] = &WorktreeAgent{Branch: "completed", State: StateCompleted}
	o.agents["removed"] = &WorktreeAgent{Branch: "removed", State: StateRemoved}

	active := o.ActiveAgents()
	if len(active) != 2 {
		t.Fatalf("expected 2 active agents, got %d", len(active))
	}
	for _, a := range active {
		if a.State == StateRemoved {
			t.Errorf("removed agent should not appear in ActiveAgents()")
		}
	}
}

func TestAgentByBranch(t *testing.T) {
	o := newTestOrchestrator(&fakeWorktreeOps{switchPath: "/tmp/wt"})
	o.agents["feat/x"] = &WorktreeAgent{Branch: "feat/x", State: StateRunning}

	got := o.AgentByBranch("feat/x")
	if got == nil || got.Branch != "feat/x" {
		t.Errorf("expected agent for feat/x, got %v", got)
	}
	if o.AgentByBranch("nonexistent") != nil {
		t.Error("expected nil for unknown branch")
	}
}

func TestRunningCount(t *testing.T) {
	o := newTestOrchestrator(&fakeWorktreeOps{switchPath: "/tmp/wt"})
	o.agents["a"] = &WorktreeAgent{State: StateRunning}
	o.agents["b"] = &WorktreeAgent{State: StateRunning}
	o.agents["c"] = &WorktreeAgent{State: StateCompleted}

	if got := o.RunningCount(); got != 2 {
		t.Errorf("RunningCount: got %d, want 2", got)
	}
}

// ─── Stop ─────────────────────────────────────────────────────────────────────

func TestStop_RunningAgent(t *testing.T) {
	o := newTestOrchestrator(&fakeWorktreeOps{switchPath: "/tmp/wt"})
	stopCh := make(chan struct{})
	o.agents["feat/stop"] = &WorktreeAgent{Branch: "feat/stop", State: StateRunning, StopCh: stopCh}

	if err := o.Stop("feat/stop"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if o.agents["feat/stop"].State != StateStopped {
		t.Errorf("expected StateStopped, got %v", o.agents["feat/stop"].State)
	}
	// StopCh should be closed.
	select {
	case <-stopCh:
	case <-time.After(100 * time.Millisecond):
		t.Error("StopCh was not closed")
	}
}

func TestStop_NotRunning(t *testing.T) {
	o := newTestOrchestrator(&fakeWorktreeOps{switchPath: "/tmp/wt"})
	o.agents["feat/done"] = &WorktreeAgent{Branch: "feat/done", State: StateCompleted}

	err := o.Stop("feat/done")
	if err == nil {
		t.Fatal("expected error stopping non-running agent")
	}
}

func TestStop_NoAgent(t *testing.T) {
	o := newTestOrchestrator(&fakeWorktreeOps{switchPath: "/tmp/wt"})
	if err := o.Stop("nonexistent"); err == nil {
		t.Fatal("expected error for unknown branch")
	}
}

// ─── Merge ────────────────────────────────────────────────────────────────────

func TestMerge_CompletedAgent(t *testing.T) {
	ops := &fakeWorktreeOps{switchPath: "/tmp/wt"}
	o := newTestOrchestrator(ops)
	o.agents["feat/m"] = &WorktreeAgent{Branch: "feat/m", State: StateCompleted, WorktreePath: "/tmp/wt"}

	if err := o.Merge("feat/m"); err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if o.agents["feat/m"].State != StateMerged {
		t.Errorf("expected StateMerged, got %v", o.agents["feat/m"].State)
	}
}

func TestMerge_RunningRejected(t *testing.T) {
	o := newTestOrchestrator(&fakeWorktreeOps{switchPath: "/tmp/wt"})
	o.agents["feat/r"] = &WorktreeAgent{Branch: "feat/r", State: StateRunning}

	err := o.Merge("feat/r")
	if err == nil {
		t.Fatal("expected error merging running agent")
	}
}

func TestMerge_NoAgent(t *testing.T) {
	o := newTestOrchestrator(&fakeWorktreeOps{switchPath: "/tmp/wt"})

	err := o.Merge("nonexistent-branch")
	if err == nil {
		t.Fatal("expected error for unknown branch")
	}
	if !strings.Contains(err.Error(), "nonexistent-branch") {
		t.Errorf("error should mention branch name, got: %v", err)
	}
}

func TestMerge_ConflictLeavesMergeFailed(t *testing.T) {
	ops := &fakeWorktreeOps{switchPath: "/tmp/wt", mergeErr: errors.New("conflict")}
	o := newTestOrchestrator(ops)
	o.agents["feat/conflict"] = &WorktreeAgent{Branch: "feat/conflict", State: StateCompleted}

	err := o.Merge("feat/conflict")
	if err == nil {
		t.Fatal("expected error from merge conflict")
	}
	if o.agents["feat/conflict"].State != StateMergeFailed {
		t.Errorf("expected StateMergeFailed, got %v", o.agents["feat/conflict"].State)
	}
}

// ─── Clean ────────────────────────────────────────────────────────────────────

func TestClean_CompletedAgent(t *testing.T) {
	ops := &fakeWorktreeOps{switchPath: "/tmp/wt"}
	o := newTestOrchestrator(ops)
	o.agents["feat/done"] = &WorktreeAgent{Branch: "feat/done", State: StateCompleted}

	if err := o.Clean("feat/done"); err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if o.agents["feat/done"].State != StateRemoved {
		t.Errorf("expected StateRemoved, got %v", o.agents["feat/done"].State)
	}
}

func TestClean_RunningRejected(t *testing.T) {
	o := newTestOrchestrator(&fakeWorktreeOps{switchPath: "/tmp/wt"})
	o.agents["feat/busy"] = &WorktreeAgent{Branch: "feat/busy", State: StateRunning}

	if err := o.Clean("feat/busy"); err == nil {
		t.Fatal("expected error cleaning running agent")
	}
}

func TestClean_NoAgent(t *testing.T) {
	o := newTestOrchestrator(&fakeWorktreeOps{switchPath: "/tmp/wt"})

	err := o.Clean("nonexistent-branch")
	if err == nil {
		t.Fatal("expected error for unknown branch")
	}
	if !strings.Contains(err.Error(), "nonexistent-branch") {
		t.Errorf("error should mention branch name, got: %v", err)
	}
}

func TestClean_RemoveFails(t *testing.T) {
	ops := &fakeWorktreeOps{switchPath: "/tmp/wt", removeErr: errors.New("disk full")}
	o := newTestOrchestrator(ops)
	o.agents["feat/done"] = &WorktreeAgent{Branch: "feat/done", State: StateCompleted}

	err := o.Clean("feat/done")
	if err == nil {
		t.Fatal("expected error when Remove fails")
	}
	if !strings.Contains(err.Error(), "disk full") {
		t.Errorf("error should mention remove failure, got: %v", err)
	}
	if o.agents["feat/done"].State == StateRemoved {
		t.Error("state should not be StateRemoved when Remove fails")
	}
}

// ─── Launch ───────────────────────────────────────────────────────────────────

func TestLaunch_MaxParallelRejected(t *testing.T) {
	ops := &fakeWorktreeOps{switchPath: "/tmp/wt"}
	o := newTestOrchestrator(ops)
	// Pre-fill with running agents up to MaxParallel.
	o.agents["a"] = &WorktreeAgent{State: StateRunning}
	o.agents["b"] = &WorktreeAgent{State: StateRunning}

	ctx := context.Background()
	err := o.Launch(ctx, "feat/new", "", "", loop.ModeBuild, 0)
	if err == nil {
		t.Fatal("expected error when at max parallel")
	}
	if o.AgentByBranch("feat/new") != nil {
		t.Error("agent should not be created when at max parallel")
	}
}

func TestLaunch_DuplicateBranchRejected(t *testing.T) {
	ops := &fakeWorktreeOps{switchPath: "/tmp/wt"}
	o := newTestOrchestrator(ops)
	o.agents["feat/dupe"] = &WorktreeAgent{Branch: "feat/dupe", State: StateRunning}

	ctx := context.Background()
	err := o.Launch(ctx, "feat/dupe", "", "", loop.ModeBuild, 0)
	if err == nil {
		t.Fatal("expected error for duplicate running branch")
	}
}

func TestLaunch_SwitchError(t *testing.T) {
	ops := &fakeWorktreeOps{switchErr: errors.New("wt switch failed")}
	o := newTestOrchestrator(ops)

	ctx := context.Background()
	err := o.Launch(ctx, "feat/err", "", "", loop.ModeBuild, 0)
	if err == nil {
		t.Fatal("expected error when wt switch fails")
	}
	agent := o.AgentByBranch("feat/err")
	if agent == nil {
		t.Fatal("agent should be present after failed launch")
	}
	if agent.State != StateFailed {
		t.Errorf("expected StateFailed, got %v", agent.State)
	}
}

// ─── Fan-in ───────────────────────────────────────────────────────────────────

func TestFanIn_EventsTaggedCorrectly(t *testing.T) {
	merged := make(chan TaggedLogEntry, 16)
	events := make(chan loop.LogEntry, 4)
	var wg sync.WaitGroup

	startFanIn("feat/fan", "codex", events, merged, nil, &wg)

	events <- loop.LogEntry{Kind: loop.LogInfo, Message: "hello from fan-in"}
	close(events)

	wg.Wait()
	close(merged)

	var got []TaggedLogEntry
	for e := range merged {
		got = append(got, e)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 tagged entry, got %d", len(got))
	}
	if got[0].Branch != "feat/fan" {
		t.Errorf("branch: got %q, want %q", got[0].Branch, "feat/fan")
	}
	if got[0].Agent != "codex" {
		t.Errorf("agent: got %q, want %q", got[0].Agent, "codex")
	}
	if got[0].Entry.Message != "hello from fan-in" {
		t.Errorf("message: got %q", got[0].Entry.Message)
	}
}

func TestFanIn_ChannelCloseHandled(t *testing.T) {
	merged := make(chan TaggedLogEntry, 16)
	events := make(chan loop.LogEntry)
	var wg sync.WaitGroup

	startFanIn("feat/close", "claude", events, merged, nil, &wg)
	close(events) // goroutine should exit cleanly
	wg.Wait()
}

// ─── WorktreePaths ────────────────────────────────────────────────────────────

// ─── autoMergeIfNeeded ────────────────────────────────────────────────────────

func TestAutoMergeIfNeeded_Disabled_NoAction(t *testing.T) {
	ops := &fakeWorktreeOps{switchPath: "/tmp/wt"}
	o := newTestOrchestrator(ops)
	o.AutoMerge = false

	agent := &WorktreeAgent{Branch: "feat/am", State: StateCompleted, WorktreePath: "/tmp/wt"}
	o.agents["feat/am"] = agent

	o.autoMergeIfNeeded(agent, "feat/am")

	if agent.State != StateCompleted {
		t.Errorf("expected StateCompleted, got %v (auto-merge should not trigger when disabled)", agent.State)
	}
}

func TestAutoMergeIfNeeded_Success_NoTestCommand(t *testing.T) {
	ops := &fakeWorktreeOps{switchPath: "/tmp/wt"}
	o := newTestOrchestrator(ops)
	o.AutoMerge = true
	// No test_command configured — merge should proceed immediately.

	agent := &WorktreeAgent{Branch: "feat/am", State: StateCompleted, WorktreePath: "/tmp/wt"}
	o.agents["feat/am"] = agent

	o.autoMergeIfNeeded(agent, "feat/am")

	if agent.State != StateMerged {
		t.Errorf("expected StateMerged, got %v", agent.State)
	}
}

func TestAutoMergeIfNeeded_MergeConflict(t *testing.T) {
	ops := &fakeWorktreeOps{switchPath: "/tmp/wt", mergeErr: errors.New("conflict")}
	o := newTestOrchestrator(ops)
	o.AutoMerge = true

	var notified []loop.LogEntry
	o.NotificationHook = func(e loop.LogEntry) { notified = append(notified, e) }

	agent := &WorktreeAgent{Branch: "feat/conflict", State: StateCompleted, WorktreePath: "/tmp/wt"}
	o.agents["feat/conflict"] = agent

	o.autoMergeIfNeeded(agent, "feat/conflict")

	if agent.State != StateMergeFailed {
		t.Errorf("expected StateMergeFailed, got %v", agent.State)
	}
	if len(notified) == 0 {
		t.Error("expected notification hook called on merge failure")
	}
	if notified[0].Kind != loop.LogError {
		t.Errorf("notification kind = %v, want LogError", notified[0].Kind)
	}
}

func TestAutoMergeIfNeeded_TestFailed_SkipsMerge(t *testing.T) {
	ops := &fakeWorktreeOps{switchPath: "/tmp/wt"}
	o := newTestOrchestrator(ops)
	o.AutoMerge = true
	// Set a test command that always fails.
	o.cfg.Regent.TestCommand = "exit 1"

	var notified []loop.LogEntry
	o.NotificationHook = func(e loop.LogEntry) { notified = append(notified, e) }

	agent := &WorktreeAgent{Branch: "feat/testfail", State: StateCompleted, WorktreePath: t.TempDir()}
	o.agents["feat/testfail"] = agent

	o.autoMergeIfNeeded(agent, "feat/testfail")

	// Agent should stay Completed — merge skipped.
	if agent.State != StateCompleted {
		t.Errorf("expected StateCompleted (no merge), got %v", agent.State)
	}
	if len(notified) == 0 {
		t.Error("expected notification hook called when tests fail")
	}
}

// TestAutoMergeIfNeeded_RunTestsError verifies that when regent.RunTests
// returns a hard error (shell not found), the auto-merge is skipped and a
// LogError notification is emitted.
func TestAutoMergeIfNeeded_RunTestsError_SkipsMerge(t *testing.T) {
	if runtime.GOOS == "windows" {
		// cmd.exe is located via absolute path on Windows, so clearing PATH
		// does not prevent the shell from being found.
		t.Skip("PATH clearing does not affect cmd.exe lookup on Windows")
	}
	ops := &fakeWorktreeOps{switchPath: "/tmp/wt"}
	o := newTestOrchestrator(ops)
	o.AutoMerge = true
	o.cfg.Regent.TestCommand = "go test ./..."
	t.Setenv("PATH", "") // make 'sh' unfindable → exec returns non-ExitError

	var notified []loop.LogEntry
	o.NotificationHook = func(e loop.LogEntry) { notified = append(notified, e) }

	agent := &WorktreeAgent{Branch: "feat/runerr", State: StateCompleted, WorktreePath: t.TempDir()}
	o.agents["feat/runerr"] = agent

	o.autoMergeIfNeeded(agent, "feat/runerr")

	// Agent should stay Completed — merge was skipped due to test run error.
	if agent.State != StateCompleted {
		t.Errorf("expected StateCompleted (no merge), got %v", agent.State)
	}
	if len(notified) == 0 {
		t.Error("expected notification hook called when test run returns error")
	}
	if notified[0].Kind != loop.LogError {
		t.Errorf("notification kind = %v, want LogError", notified[0].Kind)
	}
}

// TestAutoMergeIfNeeded_TestPassed_MergeProceeds verifies that when TestCommand
// is set and the tests pass, autoMergeIfNeeded falls through to perform the merge.
func TestAutoMergeIfNeeded_TestPassed_MergeProceeds(t *testing.T) {
	ops := &fakeWorktreeOps{switchPath: "/tmp/wt"}
	o := newTestOrchestrator(ops)
	o.AutoMerge = true
	// "exit 0" succeeds on both Unix (sh -c) and Windows (cmd /C).
	o.cfg.Regent.TestCommand = "exit 0"

	var notified []loop.LogEntry
	o.NotificationHook = func(e loop.LogEntry) { notified = append(notified, e) }

	agent := &WorktreeAgent{Branch: "feat/testpass", State: StateCompleted, WorktreePath: t.TempDir()}
	o.agents["feat/testpass"] = agent

	o.autoMergeIfNeeded(agent, "feat/testpass")

	// Tests passed — merge should proceed. With mergeErr=nil the success event is emitted.
	if len(notified) == 0 {
		t.Error("expected notification hook called when tests pass and merge succeeds")
	}
	if notified[0].Kind != loop.LogInfo {
		t.Errorf("notification kind = %v, want LogInfo", notified[0].Kind)
	}
}

func TestAutoMergeIfNeeded_NotificationHook_OnSuccess(t *testing.T) {
	ops := &fakeWorktreeOps{switchPath: "/tmp/wt"}
	o := newTestOrchestrator(ops)
	o.AutoMerge = true

	var notified []loop.LogEntry
	o.NotificationHook = func(e loop.LogEntry) { notified = append(notified, e) }

	agent := &WorktreeAgent{Branch: "feat/notify", State: StateCompleted, WorktreePath: "/tmp/wt"}
	o.agents["feat/notify"] = agent

	o.autoMergeIfNeeded(agent, "feat/notify")

	if len(notified) == 0 {
		t.Error("expected notification hook called on successful merge")
	}
	if notified[0].Kind != loop.LogInfo {
		t.Errorf("notification kind = %v, want LogInfo", notified[0].Kind)
	}
}

// ─── Fan-in stats callback ────────────────────────────────────────────────────

func TestFanIn_OnEntryCallback_UpdatesStats(t *testing.T) {
	merged := make(chan TaggedLogEntry, 16)
	events := make(chan loop.LogEntry, 4)
	var wg sync.WaitGroup

	var callCount int
	startFanIn("feat/stats", "claude", events, merged, func(e loop.LogEntry) {
		callCount++
	}, &wg)

	events <- loop.LogEntry{Kind: loop.LogInfo, Message: "msg1"}
	events <- loop.LogEntry{Kind: loop.LogIterComplete, CostUSD: 0.05}
	close(events)
	wg.Wait()
	close(merged)

	if callCount != 2 {
		t.Errorf("onEntry called %d times, want 2", callCount)
	}
}

// ─── Per-agent Regent supervision (T046-T049) ─────────────────────────────────

// waitAgentTerminal polls until the agent for branch is in a non-Running,
// non-Creating state, or the timeout expires.
func waitAgentTerminal(t *testing.T, o *Orchestrator, branch string, timeout time.Duration) *WorktreeAgent {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if s, ok := o.AgentState(branch); ok && s != StateRunning && s != StateCreating {
			return o.AgentByBranch(branch)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("agent %q did not reach terminal state within %s", branch, timeout)
	return nil
}

// TestLaunch_RegentEnabled_AgentFails verifies that when Regent is enabled and
// the loop fails (no claude binary), the agent eventually reaches StateFailed
// after max_retries exhausted.
func TestLaunch_RegentEnabled_AgentFails(t *testing.T) {
	wtDir := t.TempDir()
	ops := &fakeWorktreeOps{switchPath: wtDir}

	cfg := defaultCfg()
	cfg.Regent.Enabled = true
	cfg.Regent.MaxRetries = 0          // give up after first failure
	cfg.Regent.RetryBackoffSeconds = 0 // no sleep between retries
	cfg.Regent.HangTimeoutSeconds = 0  // no hang detection
	cfg.Build.PromptFile = "BUILD.md"  // loop needs this but it won't exist → fail fast

	o := New(cfg, ops)

	ctx := context.Background()
	err := o.Launch(ctx, "feat/regent-fail", "", "", loop.ModeBuild, 1)
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}

	agent := waitAgentTerminal(t, o, "feat/regent-fail", 5*time.Second)
	if agent.State != StateFailed {
		t.Errorf("expected StateFailed, got %v", agent.State)
	}
}

// TestLaunch_RegentIsolation_TwoAgentsFail verifies that two agents supervised
// independently both reach terminal states without interfering with each other
// (FR-020: failure in one agent must not affect others).
func TestLaunch_RegentIsolation_TwoAgentsFail(t *testing.T) {
	wtDirA := t.TempDir()
	wtDirB := t.TempDir()

	// Alternate switch paths per branch.
	callCount := 0
	dirs := []string{wtDirA, wtDirB}
	ops := &fakeWorktreeOps{}
	switchFn := func() string {
		d := dirs[callCount%2]
		callCount++
		return d
	}

	cfg := defaultCfg()
	cfg.Regent.Enabled = true
	cfg.Regent.MaxRetries = 0
	cfg.Regent.RetryBackoffSeconds = 0
	cfg.Regent.HangTimeoutSeconds = 0
	cfg.Build.PromptFile = "BUILD.md"

	o := New(cfg, ops)

	// Override switch to return different dirs per agent.
	ops.switchPath = switchFn()
	err := o.Launch(context.Background(), "feat/a", "", "", loop.ModeBuild, 1)
	if err != nil {
		t.Fatalf("Launch a: %v", err)
	}
	ops.switchPath = switchFn()
	err = o.Launch(context.Background(), "feat/b", "", "", loop.ModeBuild, 1)
	if err != nil {
		t.Fatalf("Launch b: %v", err)
	}

	agentA := waitAgentTerminal(t, o, "feat/a", 5*time.Second)
	agentB := waitAgentTerminal(t, o, "feat/b", 5*time.Second)

	// Both must be terminal — neither blocks the other.
	if agentA.State == StateRunning || agentA.State == StateCreating {
		t.Errorf("agent a still running after timeout")
	}
	if agentB.State == StateRunning || agentB.State == StateCreating {
		t.Errorf("agent b still running after timeout")
	}
}

// TestLaunch_RegentDisabled_AgentFails ensures that without Regent the loop
// failure still sets StateFailed (regression guard for non-Regent path).
func TestLaunch_RegentDisabled_AgentFails(t *testing.T) {
	wtDir := t.TempDir()
	ops := &fakeWorktreeOps{switchPath: wtDir}

	cfg := defaultCfg()
	cfg.Regent.Enabled = false
	cfg.Build.PromptFile = "BUILD.md"

	o := New(cfg, ops)

	err := o.Launch(context.Background(), "feat/no-regent", "", "", loop.ModeBuild, 1)
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}

	agent := waitAgentTerminal(t, o, "feat/no-regent", 5*time.Second)
	if agent.State != StateFailed {
		t.Errorf("expected StateFailed, got %v", agent.State)
	}
}

// ─── WorktreePaths ────────────────────────────────────────────────────────────

func TestWorktreePaths(t *testing.T) {
	o := newTestOrchestrator(&fakeWorktreeOps{switchPath: "/tmp/wt"})
	o.agents["a"] = &WorktreeAgent{WorktreePath: "/tmp/a", State: StateRunning}
	o.agents["b"] = &WorktreeAgent{WorktreePath: "/tmp/b", State: StateCompleted}
	o.agents["c"] = &WorktreeAgent{WorktreePath: "/tmp/c", State: StateRemoved}

	paths := o.WorktreePaths()
	if len(paths) != 2 {
		t.Errorf("expected 2 paths (non-removed), got %d: %v", len(paths), paths)
	}
	for _, p := range paths {
		if p == "/tmp/c" {
			t.Errorf("removed agent path should not appear: %v", paths)
		}
	}
}

// ─── StopAll ──────────────────────────────────────────────────────────────────

func TestStopAll_StopsRunningAgents(t *testing.T) {
	o := newTestOrchestrator(&fakeWorktreeOps{switchPath: "/tmp/wt"})

	stopA := make(chan struct{})
	stopB := make(chan struct{})
	o.agents["a"] = &WorktreeAgent{Branch: "a", State: StateRunning, StopCh: stopA}
	o.agents["b"] = &WorktreeAgent{Branch: "b", State: StateRunning, StopCh: stopB}
	o.agents["c"] = &WorktreeAgent{Branch: "c", State: StateCompleted}

	o.StopAll()

	if o.agents["a"].State != StateStopped {
		t.Errorf("agent a: expected StateStopped, got %v", o.agents["a"].State)
	}
	if o.agents["b"].State != StateStopped {
		t.Errorf("agent b: expected StateStopped, got %v", o.agents["b"].State)
	}
	// Non-running agent should be unaffected.
	if o.agents["c"].State != StateCompleted {
		t.Errorf("agent c: expected StateCompleted, got %v", o.agents["c"].State)
	}
}

// ─── AgentState.String ────────────────────────────────────────────────────────

func TestAgentState_String(t *testing.T) {
	tests := []struct {
		state AgentState
		want  string
	}{
		{StateCreating, "creating"},
		{StateRunning, "running"},
		{StateCompleted, "completed"},
		{StateFailed, "failed"},
		{StateStopped, "stopped"},
		{StateMerging, "merging"},
		{StateMerged, "merged"},
		{StateMergeFailed, "merge_failed"},
		{StateRemoved, "removed"},
		{AgentState(999), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("AgentState(%d).String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}
