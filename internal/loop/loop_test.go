package loop

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/LISSConsulting/RalphSpec/internal/claude"
	"github.com/LISSConsulting/RalphSpec/internal/config"
)

// mockAgent is a test double for claude.Agent.
type mockAgent struct {
	events     []claude.Event
	err        error
	calls      int
	lastPrompt string // captures the prompt passed to the most recent Run() call
	lastOpts   claude.RunOptions
	onRun      func()
}

func (m *mockAgent) Run(_ context.Context, prompt string, opts claude.RunOptions) (<-chan claude.Event, error) {
	m.calls++
	m.lastPrompt = prompt
	m.lastOpts = opts
	if m.err != nil {
		return nil, m.err
	}
	if m.onRun != nil {
		m.onRun()
	}
	ch := make(chan claude.Event, len(m.events))
	for _, ev := range m.events {
		ch <- ev
	}
	close(ch)
	return ch, nil
}

// mockGit is a test double for GitOps.
type mockGit struct {
	branch         string
	branchErr      error // error returned by CurrentBranch
	dirty          bool
	dirtySequence  []bool
	dirtyCallIdx   int
	diffFromRemote bool
	pullErr        error
	pushErr        error
	stashErr       error
	stashPopErr    error
	dirtyErr       error // error returned by HasUncommittedChanges
	diffErr        error // error returned by DiffFromRemote
	lastCommit     string
	lastCommitErr  error

	// lastCommitSequence, when non-empty, is consumed in order by LastCommit().
	// Once exhausted, the final element is repeated. Takes precedence over lastCommit.
	lastCommitSequence []string
	lastCommitCallIdx  int

	pullCalls     int
	pushCalls     int
	stashCalls    int
	stashPopCalls int
}

func (m *mockGit) CurrentBranch() (string, error) { return m.branch, m.branchErr }
func (m *mockGit) HasUncommittedChanges() (bool, error) {
	if m.dirtyErr != nil {
		return false, m.dirtyErr
	}
	if len(m.dirtySequence) > 0 {
		idx := m.dirtyCallIdx
		if idx >= len(m.dirtySequence) {
			idx = len(m.dirtySequence) - 1
		}
		m.dirtyCallIdx++
		return m.dirtySequence[idx], nil
	}
	return m.dirty, nil
}
func (m *mockGit) HasRemoteBranch(_ string) bool         { return true }
func (m *mockGit) Pull(_ string) error                   { m.pullCalls++; return m.pullErr }
func (m *mockGit) Push(_ string) error                   { m.pushCalls++; return m.pushErr }
func (m *mockGit) Stash() (bool, error)                  { m.stashCalls++; return m.stashErr == nil, m.stashErr }
func (m *mockGit) StashPop() error                       { m.stashPopCalls++; return m.stashPopErr }
func (m *mockGit) DiffFromRemote(_ string) (bool, error) { return m.diffFromRemote, m.diffErr }

func (m *mockGit) LastCommit() (string, error) {
	if m.lastCommitErr != nil {
		return "", m.lastCommitErr
	}
	if len(m.lastCommitSequence) > 0 {
		idx := m.lastCommitCallIdx
		if idx >= len(m.lastCommitSequence) {
			idx = len(m.lastCommitSequence) - 1
		}
		m.lastCommitCallIdx++
		return m.lastCommitSequence[idx], nil
	}
	return m.lastCommit, nil
}

func defaultTestConfig() *config.Config {
	cfg := config.Defaults()
	return &cfg
}

func setupTestLoop(t *testing.T, agent claude.Agent, git GitOps, cfg *config.Config) (*Loop, *bytes.Buffer) {
	t.Helper()
	dir := t.TempDir()

	// Write prompt files
	if err := os.WriteFile(filepath.Join(dir, cfg.Roam.PromptFile), []byte("roam prompt"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, cfg.Build.PromptFile), []byte("build prompt"), 0644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	return &Loop{
		Agent:  agent,
		Git:    git,
		Config: cfg,
		Log:    &buf,
		Dir:    dir,
	}, &buf
}

func TestRun(t *testing.T) {
	t.Run("build mode runs configured iterations", func(t *testing.T) {
		agent := &mockAgent{
			events: []claude.Event{claude.ResultEvent(0.10, 2.5, "success")},
		}
		git := &mockGit{branch: "main", lastCommit: "abc123 initial"}
		cfg := defaultTestConfig()
		cfg.Build.MaxIterations = 2

		lp, _ := setupTestLoop(t, agent, git, cfg)
		err := lp.Run(context.Background(), ModeBuild, 0)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if agent.calls != 2 {
			t.Errorf("expected 2 agent calls, got %d", agent.calls)
		}
	})

	t.Run("build mode with max override", func(t *testing.T) {
		agent := &mockAgent{
			events: []claude.Event{claude.ResultEvent(0.05, 1.0, "success")},
		}
		// Each iteration produces a new commit (different headBefore/headAfter) so
		// the completion state machine never fires early. 7 calls: 1 initial +
		// 3 iters × 2 (headBefore, headAfter).
		git := &mockGit{
			branch:             "feat/test",
			lastCommitSequence: []string{"h0", "h1", "h2", "h2", "h3", "h3", "h4"},
		}
		cfg := defaultTestConfig()
		cfg.Build.MaxIterations = 0 // unlimited in config

		lp, _ := setupTestLoop(t, agent, git, cfg)
		err := lp.Run(context.Background(), ModeBuild, 3)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if agent.calls != 3 {
			t.Errorf("expected 3 agent calls (override), got %d", agent.calls)
		}
	})

	t.Run("missing prompt file returns error", func(t *testing.T) {
		agent := &mockAgent{}
		git := &mockGit{branch: "main"}
		cfg := defaultTestConfig()
		cfg.Build.PromptFile = "nonexistent.md"

		dir := t.TempDir()
		var buf bytes.Buffer
		lp := &Loop{Agent: agent, Git: git, Config: cfg, Log: &buf, Dir: dir}

		err := lp.Run(context.Background(), ModeBuild, 0)
		if err == nil {
			t.Fatal("expected error for missing prompt file")
		}
		if !strings.Contains(err.Error(), "nonexistent.md") {
			t.Errorf("error should mention file name, got: %v", err)
		}
	})

	t.Run("context cancellation stops loop", func(t *testing.T) {
		agent := &mockAgent{
			events: []claude.Event{claude.ResultEvent(0.10, 1.0, "success")},
		}
		git := &mockGit{branch: "main", lastCommit: "abc123 test"}
		cfg := defaultTestConfig()
		cfg.Build.MaxIterations = 100

		lp, _ := setupTestLoop(t, agent, git, cfg)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel immediately

		err := lp.Run(ctx, ModeBuild, 0)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got %v", err)
		}
		if agent.calls != 0 {
			t.Errorf("expected 0 agent calls after cancel, got %d", agent.calls)
		}
	})

	t.Run("agent error propagates", func(t *testing.T) {
		agent := &mockAgent{err: errors.New("agent failed")}
		git := &mockGit{branch: "main"}
		cfg := defaultTestConfig()
		cfg.Build.MaxIterations = 1

		lp, _ := setupTestLoop(t, agent, git, cfg)
		err := lp.Run(context.Background(), ModeBuild, 0)

		if err == nil {
			t.Fatal("expected error from agent failure")
		}
		if !strings.Contains(err.Error(), "agent failed") {
			t.Errorf("error should contain agent message, got: %v", err)
		}
	})

	t.Run("default claude config is preserved", func(t *testing.T) {
		agent := &mockAgent{events: []claude.Event{claude.ResultEvent(0.05, 1.0, "success")}}
		git := &mockGit{branch: "main", lastCommit: "abc123 initial"}
		cfg := defaultTestConfig()
		cfg.Build.MaxIterations = 1
		cfg.Claude.Model = "opus"
		cfg.Claude.MaxTurns = 7
		cfg.Claude.DangerSkipPermissions = true

		lp, buf := setupTestLoop(t, agent, git, cfg)
		err := lp.Run(context.Background(), ModeBuild, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if agent.lastOpts.Model != "opus" || agent.lastOpts.MaxTurns != 7 || !agent.lastOpts.DangerSkipPermissions {
			t.Fatalf("unexpected claude opts: %+v", agent.lastOpts)
		}
		if !strings.Contains(buf.String(), "with claude") {
			t.Fatalf("expected log output to mention claude, got %q", buf.String())
		}
	})

	t.Run("codex config uses codex model and skips claude-only options", func(t *testing.T) {
		agent := &mockAgent{events: []claude.Event{claude.ResultEvent(0.05, 1.0, "success")}}
		git := &mockGit{branch: "main", lastCommit: "abc123 initial"}
		cfg := defaultTestConfig()
		cfg.Build.MaxIterations = 1
		cfg.Codex.Model = "gpt-5-codex"

		lp, _ := setupTestLoop(t, agent, git, cfg)
		lp.AgentType = config.AgentCodex
		err := lp.Run(context.Background(), ModeBuild, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if agent.lastOpts.Model != "gpt-5-codex" || agent.lastOpts.MaxTurns != 0 || agent.lastOpts.DangerSkipPermissions {
			t.Fatalf("unexpected codex opts: %+v", agent.lastOpts)
		}
	})
}

func TestRunCurrentBranchError(t *testing.T) {
	agent := &mockAgent{}
	git := &mockGit{branchErr: errors.New("not a git repository")}
	cfg := defaultTestConfig()

	lp, _ := setupTestLoop(t, agent, git, cfg)
	err := lp.Run(context.Background(), ModeBuild, 0)

	if err == nil {
		t.Fatal("expected error when CurrentBranch fails")
	}
	if !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("error should mention cause, got: %v", err)
	}
}

func TestIteration(t *testing.T) {
	t.Run("emits one completion for multiple agent results", func(t *testing.T) {
		agent := &mockAgent{events: []claude.Event{
			claude.ResultEvent(0.01, 3.9, "success"),
			claude.ResultEvent(0.02, 9.9, "success"),
			claude.ResultEvent(0.10, 540, "success"),
		}}
		git := &mockGit{branch: "main", lastCommit: "abc test"}
		cfg := defaultTestConfig()

		lp, _ := setupTestLoop(t, agent, git, cfg)
		var completions []LogEntry
		lp.NotificationHook = func(entry LogEntry) {
			if entry.Kind == LogIterComplete {
				completions = append(completions, entry)
			}
		}

		cost, subtype, _, _, err := lp.iteration(context.Background(), 10, 10, "prompt", "main")
		if err != nil {
			t.Fatalf("iteration() returned error: %v", err)
		}
		if len(completions) != 1 {
			t.Fatalf("got %d completion events, want 1", len(completions))
		}
		if cost != 0.10 || subtype != "success" {
			t.Fatalf("result = ($%.2f, %q), want ($0.10, %q)", cost, subtype, "success")
		}
		if completions[0].CostUSD != 0.10 || completions[0].Duration != 540 {
			t.Fatalf("completion = ($%.2f, %.1fs), want ($0.10, 540.0s)", completions[0].CostUSD, completions[0].Duration)
		}
	})

	t.Run("uses elapsed wall time when agent omits result duration", func(t *testing.T) {
		start := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
		now := start
		var completed LogEntry
		agent := &mockAgent{
			events: []claude.Event{claude.ResultEvent(0.10, 0, "success")},
			onRun: func() {
				now = start.Add(2500 * time.Millisecond)
			},
		}
		git := &mockGit{branch: "main", lastCommit: "abc test"}
		cfg := defaultTestConfig()

		lp, _ := setupTestLoop(t, agent, git, cfg)
		lp.now = func() time.Time { return now }
		lp.NotificationHook = func(entry LogEntry) {
			if entry.Kind == LogIterComplete {
				completed = entry
			}
		}

		_, _, _, _, err := lp.iteration(context.Background(), 1, 1, "prompt", "main")
		if err != nil {
			t.Fatalf("iteration() returned error: %v", err)
		}
		if completed.Duration != 2.5 {
			t.Fatalf("Duration = %.1f, want 2.5", completed.Duration)
		}
		if !strings.Contains(completed.Message, "2.5s") {
			t.Fatalf("completion message should include fallback duration, got %q", completed.Message)
		}
	})

	t.Run("pulls before running claude", func(t *testing.T) {
		agent := &mockAgent{
			events: []claude.Event{claude.ResultEvent(0.10, 1.0, "success")},
		}
		git := &mockGit{branch: "main", lastCommit: "abc test"}
		cfg := defaultTestConfig()
		cfg.Git.AutoPullRebase = true
		cfg.Build.MaxIterations = 1

		lp, _ := setupTestLoop(t, agent, git, cfg)
		err := lp.Run(context.Background(), ModeBuild, 0)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if git.pullCalls != 1 {
			t.Errorf("expected 1 pull call, got %d", git.pullCalls)
		}
	})

	t.Run("skips pull when auto_pull_rebase is false", func(t *testing.T) {
		agent := &mockAgent{
			events: []claude.Event{claude.ResultEvent(0.10, 1.0, "success")},
		}
		git := &mockGit{branch: "main", lastCommit: "abc test"}
		cfg := defaultTestConfig()
		cfg.Git.AutoPullRebase = false
		cfg.Build.MaxIterations = 1

		lp, _ := setupTestLoop(t, agent, git, cfg)
		err := lp.Run(context.Background(), ModeBuild, 0)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if git.pullCalls != 0 {
			t.Errorf("expected 0 pull calls, got %d", git.pullCalls)
		}
	})

	t.Run("stashes dirty working tree", func(t *testing.T) {
		agent := &mockAgent{
			events: []claude.Event{claude.ResultEvent(0.10, 1.0, "success")},
		}
		git := &mockGit{branch: "main", dirty: true, lastCommit: "abc test"}
		cfg := defaultTestConfig()
		cfg.Build.MaxIterations = 1

		lp, _ := setupTestLoop(t, agent, git, cfg)
		err := lp.Run(context.Background(), ModeBuild, 0)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if git.stashCalls != 1 {
			t.Errorf("expected 1 stash call, got %d", git.stashCalls)
		}
		if git.stashPopCalls != 1 {
			t.Errorf("expected 1 stash pop call, got %d", git.stashPopCalls)
		}
	})

	t.Run("skips stash when working tree is clean", func(t *testing.T) {
		agent := &mockAgent{
			events: []claude.Event{claude.ResultEvent(0.10, 1.0, "success")},
		}
		git := &mockGit{branch: "main", dirty: false, lastCommit: "abc test"}
		cfg := defaultTestConfig()
		cfg.Build.MaxIterations = 1

		lp, _ := setupTestLoop(t, agent, git, cfg)
		err := lp.Run(context.Background(), ModeBuild, 0)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if git.stashCalls != 0 {
			t.Errorf("expected 0 stash calls, got %d", git.stashCalls)
		}
		if git.stashPopCalls != 0 {
			t.Errorf("expected 0 stash pop calls, got %d", git.stashPopCalls)
		}
	})

	t.Run("pushes when there are new local commits", func(t *testing.T) {
		agent := &mockAgent{
			events: []claude.Event{claude.ResultEvent(0.10, 1.0, "success")},
		}
		git := &mockGit{
			branch:         "main",
			diffFromRemote: true,
			lastCommit:     "abc new commit",
		}
		cfg := defaultTestConfig()
		cfg.Git.AutoPush = true
		cfg.Build.MaxIterations = 1

		lp, _ := setupTestLoop(t, agent, git, cfg)
		err := lp.Run(context.Background(), ModeBuild, 0)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if git.pushCalls != 1 {
			t.Errorf("expected 1 push call, got %d", git.pushCalls)
		}
	})

	t.Run("skips push when no new commits", func(t *testing.T) {
		agent := &mockAgent{
			events: []claude.Event{claude.ResultEvent(0.10, 1.0, "success")},
		}
		git := &mockGit{
			branch:         "main",
			diffFromRemote: false,
			lastCommit:     "abc same",
		}
		cfg := defaultTestConfig()
		cfg.Git.AutoPush = true
		cfg.Build.MaxIterations = 1

		lp, _ := setupTestLoop(t, agent, git, cfg)
		err := lp.Run(context.Background(), ModeBuild, 0)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if git.pushCalls != 0 {
			t.Errorf("expected 0 push calls, got %d", git.pushCalls)
		}
	})

	t.Run("skips push when auto_push is false", func(t *testing.T) {
		agent := &mockAgent{
			events: []claude.Event{claude.ResultEvent(0.10, 1.0, "success")},
		}
		git := &mockGit{
			branch:         "main",
			diffFromRemote: true,
			lastCommit:     "abc new commit",
		}
		cfg := defaultTestConfig()
		cfg.Git.AutoPush = false
		cfg.Build.MaxIterations = 1

		lp, _ := setupTestLoop(t, agent, git, cfg)
		err := lp.Run(context.Background(), ModeBuild, 0)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if git.pushCalls != 0 {
			t.Errorf("expected 0 push calls when auto_push=false, got %d", git.pushCalls)
		}
	})
}

func TestLogOutput(t *testing.T) {
	t.Run("logs tool use events", func(t *testing.T) {
		agent := &mockAgent{
			events: []claude.Event{
				claude.ToolUseEvent("read_file", map[string]any{"file_path": "main.go"}),
				claude.ResultEvent(0.10, 1.0, "success"),
			},
		}
		git := &mockGit{branch: "main", lastCommit: "abc test"}
		cfg := defaultTestConfig()
		cfg.Build.MaxIterations = 1

		lp, buf := setupTestLoop(t, agent, git, cfg)
		err := lp.Run(context.Background(), ModeBuild, 0)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		output := buf.String()
		if !strings.Contains(output, "read_file") {
			t.Error("log should contain tool name")
		}
		if !strings.Contains(output, "main.go") {
			t.Error("log should contain tool input summary")
		}
	})

	t.Run("logs error events", func(t *testing.T) {
		agent := &mockAgent{
			events: []claude.Event{
				claude.ErrorEvent("something went wrong"),
				claude.ResultEvent(0.00, 0.5, "success"),
			},
		}
		git := &mockGit{branch: "main", lastCommit: "abc test"}
		cfg := defaultTestConfig()
		cfg.Build.MaxIterations = 1

		lp, buf := setupTestLoop(t, agent, git, cfg)
		err := lp.Run(context.Background(), ModeBuild, 0)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		output := buf.String()
		if !strings.Contains(output, "something went wrong") {
			t.Error("log should contain error message")
		}
	})

	t.Run("logs text events as reasoning", func(t *testing.T) {
		agent := &mockAgent{
			events: []claude.Event{
				claude.TextEvent("I'll start by reading the config file."),
				claude.ResultEvent(0.10, 1.0, "success"),
			},
		}
		git := &mockGit{branch: "main", lastCommit: "abc test"}
		cfg := defaultTestConfig()
		cfg.Build.MaxIterations = 1

		lp, buf := setupTestLoop(t, agent, git, cfg)
		err := lp.Run(context.Background(), ModeBuild, 0)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		output := buf.String()
		if !strings.Contains(output, "I'll start by reading the config file.") {
			t.Error("log should contain text event message")
		}
	})

	t.Run("empty text events are not logged", func(t *testing.T) {
		agent := &mockAgent{
			events: []claude.Event{
				claude.TextEvent(""),
				claude.ResultEvent(0.05, 0.5, "success"),
			},
		}
		git := &mockGit{branch: "main", lastCommit: "abc test"}
		cfg := defaultTestConfig()
		cfg.Build.MaxIterations = 1

		lp, buf := setupTestLoop(t, agent, git, cfg)
		err := lp.Run(context.Background(), ModeBuild, 0)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		_ = buf // no assertions needed; just verify no panic
	})
}

func TestTextEventInEventChannel(t *testing.T) {
	ch := make(chan LogEntry, 16)
	agent := &mockAgent{
		events: []claude.Event{
			claude.TextEvent("I'll examine the project structure first."),
			claude.ResultEvent(0.10, 1.0, "success"),
		},
	}
	git := &mockGit{branch: "main", lastCommit: "abc test"}
	cfg := defaultTestConfig()
	cfg.Build.MaxIterations = 1

	lp, _ := setupTestLoop(t, agent, git, cfg)
	lp.Events = ch

	err := lp.Run(context.Background(), ModeBuild, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	close(ch)
	var textEntry *LogEntry
	for e := range ch {
		e := e
		if e.Kind == LogText {
			textEntry = &e
			break
		}
	}

	if textEntry == nil {
		t.Fatal("expected a LogText entry on the Events channel")
	}
	if textEntry.Message != "I'll examine the project structure first." {
		t.Errorf("expected text message, got %q", textEntry.Message)
	}
}

func TestSteerNudge(t *testing.T) {
	events := make(chan LogEntry, 64)
	steerCh := make(chan string, 4)
	agent := &mockAgent{
		events: []claude.Event{claude.ResultEvent(0.01, 0.1, "success")},
	}
	// Distinct headBefore/headAfter per iteration prevents spec-complete early
	// exit so the second iteration always runs.
	git := &mockGit{
		branch:             "main",
		lastCommitSequence: []string{"h0", "h1", "h2", "h2", "h3"},
	}
	cfg := defaultTestConfig()
	cfg.Build.MaxIterations = 2

	lp, _ := setupTestLoop(t, agent, git, cfg)
	lp.Events = events
	lp.Steer = steerCh
	agent.onRun = func() {
		if agent.calls == 1 {
			steerCh <- "prefer the Edit tool"
		}
	}

	if err := lp.Run(context.Background(), ModeBuild, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(agent.lastPrompt, "## User steering") {
		t.Errorf("iteration 2 prompt missing steering section:\n%s", agent.lastPrompt)
	}
	if !strings.Contains(agent.lastPrompt, "prefer the Edit tool") {
		t.Errorf("iteration 2 prompt missing steer message:\n%s", agent.lastPrompt)
	}

	close(events)
	var steerEntry *LogEntry
	for e := range events {
		e := e
		if e.Kind == LogSteer {
			steerEntry = &e
		}
	}
	if steerEntry == nil {
		t.Fatal("expected a LogSteer entry on the Events channel")
	}
	if steerEntry.Message != "prefer the Edit tool" {
		t.Errorf("LogSteer message = %q", steerEntry.Message)
	}
}

func TestSteerRunOptionsPassThrough(t *testing.T) {
	t.Run("stdin_steer enabled passes channel to agent", func(t *testing.T) {
		agent := &mockAgent{
			events: []claude.Event{claude.ResultEvent(0.01, 0.1, "success")},
		}
		git := &mockGit{branch: "main", lastCommit: "h0"}
		cfg := defaultTestConfig()
		cfg.Build.MaxIterations = 1
		cfg.Agent.StdinSteer = true

		lp, _ := setupTestLoop(t, agent, git, cfg)
		lp.Steer = make(chan string, 1)

		if err := lp.Run(context.Background(), ModeBuild, 0); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if agent.lastOpts.Steer == nil {
			t.Error("expected RunOptions.Steer to be set when stdin_steer is true")
		}
	})

	t.Run("stdin_steer disabled leaves RunOptions.Steer nil", func(t *testing.T) {
		agent := &mockAgent{
			events: []claude.Event{claude.ResultEvent(0.01, 0.1, "success")},
		}
		git := &mockGit{branch: "main", lastCommit: "h0"}
		cfg := defaultTestConfig()
		cfg.Build.MaxIterations = 1

		lp, _ := setupTestLoop(t, agent, git, cfg)
		lp.Steer = make(chan string, 1)

		if err := lp.Run(context.Background(), ModeBuild, 0); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if agent.lastOpts.Steer != nil {
			t.Error("RunOptions.Steer must be nil when stdin_steer is false (legacy invocation)")
		}
	})
}

func TestSubtypeInLogOutput(t *testing.T) {
	t.Run("subtype included in iteration complete message", func(t *testing.T) {
		agent := &mockAgent{
			events: []claude.Event{claude.ResultEvent(0.14, 4.2, "success")},
		}
		git := &mockGit{branch: "main", lastCommit: "abc test"}
		cfg := defaultTestConfig()
		cfg.Build.MaxIterations = 1

		lp, buf := setupTestLoop(t, agent, git, cfg)
		err := lp.Run(context.Background(), ModeBuild, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "success") {
			t.Errorf("log should contain subtype 'success', got: %s", buf.String())
		}
	})

	t.Run("error_max_turns subtype in log", func(t *testing.T) {
		agent := &mockAgent{
			events: []claude.Event{claude.ResultEvent(0.30, 5.0, "error_max_turns")},
		}
		git := &mockGit{branch: "main", lastCommit: "abc test"}
		cfg := defaultTestConfig()
		cfg.Build.MaxIterations = 1

		lp, buf := setupTestLoop(t, agent, git, cfg)
		err := lp.Run(context.Background(), ModeBuild, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "error_max_turns") {
			t.Errorf("log should contain subtype 'error_max_turns', got: %s", buf.String())
		}
	})

	t.Run("empty subtype omitted from message", func(t *testing.T) {
		agent := &mockAgent{
			events: []claude.Event{claude.ResultEvent(0.10, 1.0, "")},
		}
		git := &mockGit{branch: "main", lastCommit: "abc test"}
		cfg := defaultTestConfig()
		cfg.Build.MaxIterations = 1

		lp, buf := setupTestLoop(t, agent, git, cfg)
		err := lp.Run(context.Background(), ModeBuild, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		output := buf.String()
		if !strings.Contains(output, "Iteration 1 complete") {
			t.Errorf("should contain iteration complete, got: %s", output)
		}
		// Should not have trailing separator for empty subtype
		if strings.Contains(output, "1.0s —") {
			t.Errorf("should not show trailing separator for empty subtype, got: %s", output)
		}
	})
}

func TestSubtypeInEventChannel(t *testing.T) {
	ch := make(chan LogEntry, 16)
	agent := &mockAgent{
		events: []claude.Event{claude.ResultEvent(0.14, 4.2, "success")},
	}
	git := &mockGit{branch: "main", lastCommit: "abc test"}
	cfg := defaultTestConfig()
	cfg.Build.MaxIterations = 1

	lp, _ := setupTestLoop(t, agent, git, cfg)
	lp.Events = ch

	err := lp.Run(context.Background(), ModeBuild, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	close(ch)
	var iterComplete *LogEntry
	for e := range ch {
		if e.Kind == LogIterComplete {
			iterComplete = &e
			break
		}
	}

	if iterComplete == nil {
		t.Fatal("expected an IterComplete event")
	}
	if iterComplete.Subtype != "success" {
		t.Errorf("expected Subtype 'success', got %q", iterComplete.Subtype)
	}
}

func TestModeConfig(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.Build.PromptFile = "BUILD.md"
	cfg.Build.MaxIterations = 10
	cfg.Roam.PromptFile = "ROAM.md"
	cfg.Roam.MaxIterations = 5

	lp := &Loop{Config: cfg}

	t.Run("build mode", func(t *testing.T) {
		file, max := lp.modeConfig(ModeBuild)
		if file != "BUILD.md" {
			t.Errorf("expected BUILD.md, got %s", file)
		}
		if max != 10 {
			t.Errorf("expected 10, got %d", max)
		}
	})

	t.Run("roam mode", func(t *testing.T) {
		lp.Roam = true
		file, max := lp.modeConfig(ModeBuild)
		if file != "ROAM.md" {
			t.Errorf("expected ROAM.md, got %s", file)
		}
		if max != 5 {
			t.Errorf("expected 5, got %d", max)
		}
	})
}

func TestSummarizeInput(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]any
		want  string
	}{
		{"file_path", map[string]any{"file_path": "main.go"}, "main.go"},
		{"command", map[string]any{"command": "go build"}, "go build"},
		{"path", map[string]any{"path": "/tmp"}, "/tmp"},
		{"url", map[string]any{"url": "https://example.com"}, "https://example.com"},
		{"pattern", map[string]any{"pattern": "*.go"}, "*.go"},
		{"description", map[string]any{"description": "Run tests"}, "Run tests"},
		{"prompt", map[string]any{"prompt": "Write a test"}, "Write a test"},
		{"query", map[string]any{"query": "golang errors"}, "golang errors"},
		{"notebook_path", map[string]any{"notebook_path": "nb.ipynb"}, "nb.ipynb"},
		{"task_id", map[string]any{"task_id": "abc123"}, "abc123"},
		{"empty", map[string]any{"other": "value"}, ""},
		{"nil", nil, ""},
		{"prefers file_path over command", map[string]any{"file_path": "a.go", "command": "ls"}, "a.go"},
		{"prefers description over prompt", map[string]any{"description": "short", "prompt": "long text"}, "short"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := summarizeInput(tt.input)
			if got != tt.want {
				t.Errorf("summarizeInput(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestPostIteration(t *testing.T) {
	t.Run("hook called after each iteration", func(t *testing.T) {
		agent := &mockAgent{
			events: []claude.Event{claude.ResultEvent(0.10, 1.0, "success")},
		}
		// Produce commits on every iteration so the spec-completion state machine
		// never fires early (prevSubtype=="success" && commitsProduced==true → no exit).
		// 7 calls: 1 initial + 3 iters × 2 (headBefore, headAfter).
		git := &mockGit{
			branch:             "main",
			lastCommitSequence: []string{"h0", "h1", "h2", "h2", "h3", "h3", "h4"},
		}
		cfg := defaultTestConfig()
		cfg.Build.MaxIterations = 3

		lp, _ := setupTestLoop(t, agent, git, cfg)
		var hookCalls int
		lp.PostIteration = func() { hookCalls++ }

		err := lp.Run(context.Background(), ModeBuild, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if hookCalls != 3 {
			t.Errorf("expected PostIteration called 3 times, got %d", hookCalls)
		}
	})

	t.Run("hook not called when not set", func(t *testing.T) {
		agent := &mockAgent{
			events: []claude.Event{claude.ResultEvent(0.10, 1.0, "success")},
		}
		git := &mockGit{branch: "main", lastCommit: "abc test"}
		cfg := defaultTestConfig()
		cfg.Build.MaxIterations = 1

		lp, _ := setupTestLoop(t, agent, git, cfg)
		// PostIteration is nil by default

		err := lp.Run(context.Background(), ModeBuild, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// No panic = success
	})

	t.Run("hook called after push", func(t *testing.T) {
		agent := &mockAgent{
			events: []claude.Event{claude.ResultEvent(0.10, 1.0, "success")},
		}
		git := &mockGit{
			branch:         "main",
			diffFromRemote: true,
			lastCommit:     "abc new",
		}
		cfg := defaultTestConfig()
		cfg.Git.AutoPush = true
		cfg.Build.MaxIterations = 1

		lp, _ := setupTestLoop(t, agent, git, cfg)
		var hookCalled bool
		var pushCountAtHook int
		lp.PostIteration = func() {
			hookCalled = true
			pushCountAtHook = git.pushCalls
		}

		err := lp.Run(context.Background(), ModeBuild, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !hookCalled {
			t.Fatal("PostIteration was not called")
		}
		if pushCountAtHook != 1 {
			t.Errorf("expected push to happen before hook, pushCalls at hook time = %d", pushCountAtHook)
		}
	})
}

func TestInitialCommitInEvent(t *testing.T) {
	// Verifies that the very first log event includes the current HEAD commit,
	// so the TUI footer shows it from startup instead of showing "—".
	agent := &mockAgent{
		events: []claude.Event{claude.ResultEvent(0.10, 1.0, "success")},
	}
	git := &mockGit{branch: "main", lastCommit: "abc123 initial"}
	cfg := defaultTestConfig()
	cfg.Build.MaxIterations = 1
	cfg.Git.AutoPush = false // no push — commit must come from initial emit

	ch := make(chan LogEntry, 32)
	lp, _ := setupTestLoop(t, agent, git, cfg)
	lp.Events = ch

	err := lp.Run(context.Background(), ModeBuild, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	close(ch)

	var found bool
	for e := range ch {
		if e.Kind == LogInfo && e.Commit != "" {
			found = true
			if e.Commit != "abc123 initial" {
				t.Errorf("expected commit 'abc123 initial', got %q", e.Commit)
			}
			break
		}
	}
	if !found {
		t.Error("expected initial LogInfo event to have Commit set")
	}
}

func TestIterLabel(t *testing.T) {
	tests := []struct {
		max  int
		want string
	}{
		{0, "unlimited"},
		{1, "1"},
		{10, "10"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := iterLabel(tt.max); got != tt.want {
				t.Errorf("iterLabel(%d) = %q, want %q", tt.max, got, tt.want)
			}
		})
	}
}

func TestStashIfDirtyErrors(t *testing.T) {
	t.Run("HasUncommittedChanges error propagates", func(t *testing.T) {
		agent := &mockAgent{
			events: []claude.Event{claude.ResultEvent(0.10, 1.0, "success")},
		}
		git := &mockGit{
			branch:   "main",
			dirtyErr: errors.New("git status failed"),
		}
		cfg := defaultTestConfig()
		cfg.Build.MaxIterations = 1

		lp, _ := setupTestLoop(t, agent, git, cfg)
		err := lp.Run(context.Background(), ModeBuild, 0)

		if err == nil {
			t.Fatal("expected error when HasUncommittedChanges fails")
		}
		if !strings.Contains(err.Error(), "git status failed") {
			t.Errorf("error should contain cause, got: %v", err)
		}
	})

	t.Run("Stash error propagates", func(t *testing.T) {
		agent := &mockAgent{}
		git := &mockGit{
			branch:   "main",
			dirty:    true,
			stashErr: errors.New("stash failed"),
		}
		cfg := defaultTestConfig()
		cfg.Build.MaxIterations = 1

		lp, _ := setupTestLoop(t, agent, git, cfg)
		err := lp.Run(context.Background(), ModeBuild, 0)

		if err == nil {
			t.Fatal("expected error when Stash fails")
		}
		if !strings.Contains(err.Error(), "stash failed") {
			t.Errorf("error should contain cause, got: %v", err)
		}
	})
}

func TestPushIfNeededErrors(t *testing.T) {
	t.Run("DiffFromRemote error pushes anyway and logs", func(t *testing.T) {
		agent := &mockAgent{
			events: []claude.Event{claude.ResultEvent(0.10, 1.0, "success")},
		}
		git := &mockGit{
			branch:     "main",
			diffErr:    errors.New("diff failed"),
			lastCommit: "abc new",
		}
		cfg := defaultTestConfig()
		cfg.Git.AutoPush = true
		cfg.Build.MaxIterations = 1

		lp, buf := setupTestLoop(t, agent, git, cfg)
		err := lp.Run(context.Background(), ModeBuild, 0)

		if err != nil {
			t.Fatalf("DiffFromRemote error should not abort loop, got: %v", err)
		}
		if git.pushCalls != 1 {
			t.Errorf("should push when diff check fails (e.g., new branch), got %d push calls", git.pushCalls)
		}
		if !strings.Contains(buf.String(), "Diff check failed") {
			t.Errorf("should log diff failure, got: %s", buf.String())
		}
	})

	t.Run("LastCommit error shows fallback in push message", func(t *testing.T) {
		agent := &mockAgent{
			events: []claude.Event{claude.ResultEvent(0.10, 1.0, "success")},
		}
		git := &mockGit{
			branch:         "main",
			diffFromRemote: true,
			lastCommitErr:  errors.New("no commits"),
		}
		cfg := defaultTestConfig()
		cfg.Git.AutoPush = true
		cfg.Build.MaxIterations = 1

		lp, buf := setupTestLoop(t, agent, git, cfg)
		err := lp.Run(context.Background(), ModeBuild, 0)

		if err != nil {
			t.Fatalf("LastCommit error should not abort loop, got: %v", err)
		}
		if !strings.Contains(buf.String(), "(unknown)") {
			t.Errorf("should show fallback commit message, got: %s", buf.String())
		}
	})

	t.Run("Push error logged but does not abort loop", func(t *testing.T) {
		agent := &mockAgent{
			events: []claude.Event{claude.ResultEvent(0.10, 1.0, "success")},
		}
		git := &mockGit{
			branch:         "main",
			diffFromRemote: true,
			pushErr:        errors.New("push rejected"),
			lastCommit:     "abc new",
		}
		cfg := defaultTestConfig()
		cfg.Git.AutoPush = true
		cfg.Build.MaxIterations = 1

		lp, buf := setupTestLoop(t, agent, git, cfg)
		err := lp.Run(context.Background(), ModeBuild, 0)

		if err != nil {
			t.Fatalf("push error should not abort loop, got: %v", err)
		}
		if !strings.Contains(buf.String(), "Push error") {
			t.Errorf("should log push error, got: %s", buf.String())
		}
	})
}

func TestIterationContinuesOnPullError(t *testing.T) {
	agent := &mockAgent{
		events: []claude.Event{claude.ResultEvent(0.10, 1.0, "success")},
	}
	git := &mockGit{
		branch:     "main",
		lastCommit: "abc test",
		pullErr:    errors.New("network error"),
	}
	cfg := defaultTestConfig()
	cfg.Git.AutoPullRebase = true
	cfg.Build.MaxIterations = 1

	lp, buf := setupTestLoop(t, agent, git, cfg)
	err := lp.Run(context.Background(), ModeBuild, 0)

	// Pull error is logged but loop continues
	if err != nil {
		t.Fatalf("pull error should not abort loop, got: %v", err)
	}
	if agent.calls != 1 {
		t.Errorf("expected 1 agent call despite pull error, got %d", agent.calls)
	}
	if !strings.Contains(buf.String(), "Pull failed") {
		t.Errorf("log should mention pull failure, got: %s", buf.String())
	}
}

func TestIterationContinuesOnStashPopError(t *testing.T) {
	agent := &mockAgent{
		events: []claude.Event{claude.ResultEvent(0.10, 1.0, "success")},
	}
	git := &mockGit{
		branch:      "main",
		dirty:       true,
		lastCommit:  "abc test",
		stashPopErr: errors.New("stash pop conflict"),
	}
	cfg := defaultTestConfig()
	cfg.Build.MaxIterations = 1

	lp, buf := setupTestLoop(t, agent, git, cfg)
	err := lp.Run(context.Background(), ModeBuild, 0)

	// Stash pop error is logged but loop continues
	if err != nil {
		t.Fatalf("stash pop error should not abort loop, got: %v", err)
	}
	if agent.calls != 1 {
		t.Errorf("expected 1 agent call despite stash pop error, got %d", agent.calls)
	}
	if !strings.Contains(buf.String(), "Stash pop failed") {
		t.Errorf("log should mention stash pop failure, got: %s", buf.String())
	}
}

func TestEmitNilLog(t *testing.T) {
	// When Events is nil and Log is nil, emit should fall back to os.Stdout without panic.
	agent := &mockAgent{
		events: []claude.Event{claude.ResultEvent(0.10, 1.0, "success")},
	}
	git := &mockGit{branch: "main", lastCommit: "abc test"}
	cfg := defaultTestConfig()
	cfg.Build.MaxIterations = 1

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, cfg.Roam.PromptFile), []byte("roam prompt"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, cfg.Build.PromptFile), []byte("build prompt"), 0644); err != nil {
		t.Fatal(err)
	}

	lp := &Loop{
		Agent:  agent,
		Git:    git,
		Config: cfg,
		Dir:    dir,
		// Log is nil, Events is nil — emit should use os.Stdout
	}

	err := lp.Run(context.Background(), ModeBuild, 0)
	if err != nil {
		t.Fatalf("nil Log should fall back to stdout without error, got: %v", err)
	}
}

func TestRunStopAfter(t *testing.T) {
	t.Run("pre-closed channel returns immediately without running", func(t *testing.T) {
		agent := &mockAgent{
			events: []claude.Event{claude.ResultEvent(0.10, 2.0, "success")},
		}
		git := &mockGit{branch: "main", lastCommit: "abc123 feat"}
		cfg := defaultTestConfig()
		cfg.Build.MaxIterations = 5

		lp, _ := setupTestLoop(t, agent, git, cfg)

		// Close the channel before Run — early check returns immediately.
		stopCh := make(chan struct{})
		close(stopCh)
		lp.StopAfter = stopCh

		err := lp.Run(context.Background(), ModeBuild, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if agent.calls != 0 {
			t.Errorf("expected 0 agent calls, got %d", agent.calls)
		}
	})

	t.Run("stops after first iteration when closed during run", func(t *testing.T) {
		stopCh := make(chan struct{})
		var once sync.Once
		agent := &mockAgent{
			events: []claude.Event{claude.ResultEvent(0.10, 2.0, "success")},
		}
		git := &mockGit{branch: "main", lastCommit: "abc123 feat"}
		cfg := defaultTestConfig()
		cfg.Build.MaxIterations = 5

		lp, buf := setupTestLoop(t, agent, git, cfg)

		// Use PostIteration to close stopCh after the first iteration completes,
		// simulating Ctrl+C between iterations.
		lp.StopAfter = stopCh
		lp.PostIteration = func() {
			once.Do(func() { close(stopCh) })
		}

		err := lp.Run(context.Background(), ModeBuild, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if agent.calls != 1 {
			t.Errorf("expected 1 agent call, got %d", agent.calls)
		}
		if !strings.Contains(buf.String(), "Stop requested") {
			t.Errorf("log should contain stop message, got: %s", buf.String())
		}
	})

	t.Run("runs multiple iterations when channel is not closed", func(t *testing.T) {
		agent := &mockAgent{
			events: []claude.Event{claude.ResultEvent(0.05, 1.0, "success")},
		}
		// Produce commits so the completion state machine does not fire early.
		git := &mockGit{
			branch:             "main",
			lastCommitSequence: []string{"h0", "h1", "h2", "h2", "h3", "h3", "h4"},
		}
		cfg := defaultTestConfig()
		cfg.Build.MaxIterations = 3

		lp, _ := setupTestLoop(t, agent, git, cfg)

		// Channel present but never closed — should run all iterations normally.
		stopCh := make(chan struct{})
		lp.StopAfter = stopCh

		err := lp.Run(context.Background(), ModeBuild, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if agent.calls != 3 {
			t.Errorf("expected 3 agent calls, got %d", agent.calls)
		}
	})

	t.Run("nil StopAfter runs all iterations", func(t *testing.T) {
		agent := &mockAgent{
			events: []claude.Event{claude.ResultEvent(0.05, 1.0, "success")},
		}
		git := &mockGit{branch: "main"}
		cfg := defaultTestConfig()
		cfg.Build.MaxIterations = 2

		lp, _ := setupTestLoop(t, agent, git, cfg)
		// lp.StopAfter is nil by default

		err := lp.Run(context.Background(), ModeBuild, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if agent.calls != 2 {
			t.Errorf("expected 2 agent calls, got %d", agent.calls)
		}
	})
}

// TestSpecCompletion verifies the two-signal spec-boundary completion detection:
// prevSubtype=="success" + commitsProduced==false in the current iteration → early exit.
func TestSpecCompletion(t *testing.T) {
	t.Run("two successes second has no commits emits LogSpecComplete", func(t *testing.T) {
		// Iter1: success + commits (h1→h2). Iter2: success + no commits (h2→h2).
		// Call order: initial(h0), iter1-headBefore(h1), iter1-headAfter(h2),
		// iter2-headBefore(h2), iter2-headAfter(h2).
		agent := &mockAgent{
			events: []claude.Event{claude.ResultEvent(0.10, 1.0, "success")},
		}
		git := &mockGit{
			branch:             "feat/spec",
			lastCommitSequence: []string{"h0", "h1", "h2", "h2", "h2"},
		}
		cfg := defaultTestConfig()
		cfg.Build.MaxIterations = 0 // unlimited

		ch := make(chan LogEntry, 32)
		lp, _ := setupTestLoop(t, agent, git, cfg)
		lp.Events = ch

		err := lp.Run(context.Background(), ModeBuild, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if agent.calls != 2 {
			t.Errorf("expected 2 agent calls, got %d", agent.calls)
		}
		close(ch)
		var found bool
		for e := range ch {
			if e.Kind == LogSpecComplete {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected LogSpecComplete event")
		}
	})

	t.Run("success with commits in every iteration does not trigger early exit", func(t *testing.T) {
		// All 3 iterations produce commits → no early exit → LogDone at maxIter.
		agent := &mockAgent{
			events: []claude.Event{claude.ResultEvent(0.10, 1.0, "success")},
		}
		git := &mockGit{
			branch:             "feat/spec",
			lastCommitSequence: []string{"h0", "h1", "h2", "h2", "h3", "h3", "h4"},
		}
		cfg := defaultTestConfig()
		cfg.Build.MaxIterations = 3

		lp, _ := setupTestLoop(t, agent, git, cfg)
		err := lp.Run(context.Background(), ModeBuild, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if agent.calls != 3 {
			t.Errorf("expected 3 agent calls (no early exit when commits produced), got %d", agent.calls)
		}
	})

	t.Run("single iteration max1 success no-commits emits LogDone not LogSpecComplete", func(t *testing.T) {
		// With max=1, prevSubtype is "" when checked after iter1, so no early exit.
		agent := &mockAgent{
			events: []claude.Event{claude.ResultEvent(0.10, 1.0, "success")},
		}
		git := &mockGit{branch: "feat/spec", lastCommit: "abc same"}
		cfg := defaultTestConfig()

		ch := make(chan LogEntry, 32)
		lp, _ := setupTestLoop(t, agent, git, cfg)
		lp.Events = ch

		err := lp.Run(context.Background(), ModeBuild, 1) // maxOverride=1
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if agent.calls != 1 {
			t.Errorf("expected 1 agent call, got %d", agent.calls)
		}
		close(ch)
		var gotLogDone, gotLogSpecComplete bool
		for e := range ch {
			switch e.Kind {
			case LogDone:
				gotLogDone = true
			case LogSpecComplete:
				gotLogSpecComplete = true
			}
		}
		if !gotLogDone {
			t.Error("expected LogDone when max=1, even with success+no-commits")
		}
		if gotLogSpecComplete {
			t.Error("should not emit LogSpecComplete when only 1 iteration runs")
		}
	})

	t.Run("error_max_turns does not trigger completion even with no commits", func(t *testing.T) {
		// error_max_turns never satisfies the "success" precondition.
		agent := &mockAgent{
			events: []claude.Event{claude.ResultEvent(0.10, 1.0, "error_max_turns")},
		}
		git := &mockGit{branch: "feat/spec", lastCommit: "abc same"}
		cfg := defaultTestConfig()
		cfg.Build.MaxIterations = 2

		lp, _ := setupTestLoop(t, agent, git, cfg)
		err := lp.Run(context.Background(), ModeBuild, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if agent.calls != 2 {
			t.Errorf("expected 2 agent calls (error_max_turns never triggers completion), got %d", agent.calls)
		}
	})

	t.Run("dirty worktree prevents success no-commit spec completion", func(t *testing.T) {
		agent := &mockAgent{
			events: []claude.Event{claude.ResultEvent(0.10, 1.0, "success")},
		}
		git := &mockGit{
			branch:             "feat/spec",
			lastCommitSequence: []string{"h0", "h1", "h2", "h2", "h2"},
			dirtySequence:      []bool{false, false, false, true},
		}
		cfg := defaultTestConfig()
		cfg.Build.MaxIterations = 2

		ch := make(chan LogEntry, 32)
		lp, _ := setupTestLoop(t, agent, git, cfg)
		lp.Events = ch

		err := lp.Run(context.Background(), ModeBuild, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		close(ch)

		var gotDone, gotSpecComplete, gotDirtyNotice bool
		for e := range ch {
			switch e.Kind {
			case LogDone:
				gotDone = true
			case LogSpecComplete:
				gotSpecComplete = true
			case LogInfo:
				if strings.Contains(e.Message, "uncommitted changes after iteration") {
					gotDirtyNotice = true
				}
			}
		}
		if !gotDone {
			t.Error("expected LogDone at max iterations")
		}
		if gotSpecComplete {
			t.Error("should not emit LogSpecComplete while worktree is dirty")
		}
		if !gotDirtyNotice {
			t.Error("expected dirty-worktree completion notice")
		}
	})
}

// TestRoamCompletion verifies that roam mode emits LogSweepComplete instead of
// LogSpecComplete when the two-signal completion fires.
func TestRoamCompletion(t *testing.T) {
	t.Run("roam mode emits LogSweepComplete on completion", func(t *testing.T) {
		agent := &mockAgent{
			events: []claude.Event{claude.ResultEvent(0.10, 1.0, "success")},
		}
		git := &mockGit{
			branch:             "feature/anything",
			lastCommitSequence: []string{"h0", "h1", "h2", "h2", "h2"},
		}
		cfg := defaultTestConfig()
		cfg.Build.MaxIterations = 0

		ch := make(chan LogEntry, 32)
		lp, _ := setupTestLoop(t, agent, git, cfg)
		lp.Events = ch
		lp.Roam = true

		err := lp.Run(context.Background(), ModeBuild, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		close(ch)
		var gotRoamComplete, gotSpec bool
		for e := range ch {
			switch e.Kind {
			case LogSweepComplete:
				gotRoamComplete = true
			case LogSpecComplete:
				gotSpec = true
			}
		}
		if !gotRoamComplete {
			t.Error("expected LogSweepComplete in roam mode")
		}
		if gotSpec {
			t.Error("should not emit LogSpecComplete in roam mode")
		}
	})
}

// TestAugmentPrompt directly tests the augmentPrompt helper.
func TestAugmentPrompt(t *testing.T) {
	tests := []struct {
		name          string
		prompt        string
		spec          string
		specDir       string
		roam          bool
		wantParts     []string
		wantUnchanged bool
	}{
		{
			name:          "roam mode without focus returns prompt unchanged",
			prompt:        "base",
			roam:          true,
			wantUnchanged: true,
		},
		{
			name:      "spec set adds spec-boundary section",
			prompt:    "base",
			spec:      "005-test",
			specDir:   "specs/005-test",
			wantParts: []string{"## Spec Context", "005-test", "specs/005-test"},
		},
		{
			name:          "no spec no roam returns prompt unchanged",
			prompt:        "base",
			wantUnchanged: true,
		},
		{
			name:          "roam takes precedence and suppresses spec section",
			prompt:        "base",
			spec:          "some-spec",
			specDir:       "specs/some-spec",
			roam:          true,
			wantUnchanged: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := augmentPrompt(tt.prompt, tt.spec, tt.specDir, tt.roam, "")
			for _, part := range tt.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("augmentPrompt() output should contain %q\ngot: %q", part, got)
				}
			}
			if tt.wantUnchanged && got != tt.prompt {
				t.Errorf("augmentPrompt() should return prompt unchanged\nwant: %q\ngot:  %q", tt.prompt, got)
			}
		})
	}
}

// TestAugmentPromptFocus verifies the focus parameter appends a directive.
func TestAugmentPromptFocus(t *testing.T) {
	tests := []struct {
		name      string
		spec      string
		specDir   string
		roam      bool
		focus     string
		wantParts []string
	}{
		{
			name:      "focus in roam mode",
			roam:      true,
			focus:     "UI/UX",
			wantParts: []string{"## Roam Focus", "Focus your work on: UI/UX"},
		},
		{
			name:      "focus with spec",
			spec:      "005-test",
			specDir:   "specs/005-test",
			focus:     "testing",
			wantParts: []string{"Active spec:", "Focus your work on: testing"},
		},
		{
			name:      "focus only (no spec, no roam)",
			focus:     "performance",
			wantParts: []string{"## Spec Context", "Focus your work on: performance"},
		},
		{
			name:      "empty focus — no directive appended",
			roam:      true,
			focus:     "",
			wantParts: []string{"base"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := augmentPrompt("base", tt.spec, tt.specDir, tt.roam, tt.focus)
			for _, part := range tt.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("augmentPrompt() output should contain %q\ngot: %q", part, got)
				}
			}
		})
	}
}

// TestPromptAugmentationInRun verifies that Loop.Run() passes an augmented
// prompt to the agent when Spec or Roam fields are set.
func TestPromptAugmentationInRun(t *testing.T) {
	t.Run("spec set includes spec context in prompt", func(t *testing.T) {
		agent := &mockAgent{
			events: []claude.Event{claude.ResultEvent(0.10, 1.0, "success")},
		}
		git := &mockGit{branch: "feat/spec", lastCommit: "abc test"}
		cfg := defaultTestConfig()

		lp, _ := setupTestLoop(t, agent, git, cfg)
		lp.Spec = "005-spec-bounded-roam"
		lp.SpecDir = "specs/005-spec-bounded-roam"

		err := lp.Run(context.Background(), ModeBuild, 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(agent.lastPrompt, "## Spec Context") {
			t.Errorf("prompt should contain Spec Context section, got: %q", agent.lastPrompt)
		}
		if !strings.Contains(agent.lastPrompt, "005-spec-bounded-roam") {
			t.Errorf("prompt should contain spec name, got: %q", agent.lastPrompt)
		}
		if !strings.Contains(agent.lastPrompt, "specs/005-spec-bounded-roam") {
			t.Errorf("prompt should contain spec directory, got: %q", agent.lastPrompt)
		}
	})

	t.Run("roam mode uses roam prompt without extra spec context", func(t *testing.T) {
		agent := &mockAgent{
			events: []claude.Event{claude.ResultEvent(0.10, 1.0, "success")},
		}
		git := &mockGit{branch: "feature/whatever", lastCommit: "abc test"}
		cfg := defaultTestConfig()

		lp, _ := setupTestLoop(t, agent, git, cfg)
		lp.Roam = true

		err := lp.Run(context.Background(), ModeBuild, 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if agent.lastPrompt != "roam prompt" {
			t.Errorf("prompt should be the ROAM.md prompt, got: %q", agent.lastPrompt)
		}
		if strings.Contains(agent.lastPrompt, "## Spec Context") {
			t.Errorf("roam prompt should not get spec context appended, got: %q", agent.lastPrompt)
		}
	})

	t.Run("no spec and no roam leaves prompt unchanged", func(t *testing.T) {
		agent := &mockAgent{
			events: []claude.Event{claude.ResultEvent(0.10, 1.0, "success")},
		}
		git := &mockGit{branch: "main", lastCommit: "abc test"}
		cfg := defaultTestConfig()

		lp, _ := setupTestLoop(t, agent, git, cfg)
		// Spec="" and Roam=false by default — prompt must be passed through unmodified.

		err := lp.Run(context.Background(), ModeBuild, 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(agent.lastPrompt, "## Spec Context") {
			t.Errorf("prompt should not be augmented when no spec and no roam, got: %q", agent.lastPrompt)
		}
		if agent.lastPrompt != "build prompt" {
			t.Errorf("prompt should be unchanged, want %q got %q", "build prompt", agent.lastPrompt)
		}
	})
}
