package loop

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LISSConsulting/RalphSpec/internal/claude"
)

func TestLudicrousModeRequiresVerifiedCompletion(t *testing.T) {
	agent := &mockAgent{events: []claude.Event{claude.ResultEvent(0, 0.1, "success")}}
	git := &mockGit{branch: "main", lastCommit: "abc"}
	cfg := defaultTestConfig()
	cfg.Build.MaxIterations = 2
	cfg.Build.Ludicrous = true
	lp, output := setupTestLoop(t, agent, git, cfg)
	verificationCalls := 0
	lp.PostIteration = func(_ context.Context) bool {
		verificationCalls++
		return false
	}

	err := lp.Run(context.Background(), ModeBuild, 2)
	if err == nil {
		t.Fatal("ludicrous mode must fail a finite run without passing verification evidence")
	}
	outcome, ok := claude.AsOutcome(err)
	if !ok || outcome.Kind != claude.OutcomeAgentFailure {
		t.Fatalf("outcome = %#v, %v", outcome, ok)
	}
	if verificationCalls != 2 {
		t.Fatalf("verification calls = %d, want 2", verificationCalls)
	}
	if strings.Contains(output.String(), "Spec complete") {
		t.Fatalf("failed verification emitted completion:\n%s", output.String())
	}
	if !strings.Contains(agent.lastPrompt, "Goal-Persistence Contract") {
		t.Fatalf("ludicrous prompt contract missing:\n%s", agent.lastPrompt)
	}
}

func TestLudicrousModeCompletesOnIndependentSignalsAndTests(t *testing.T) {
	agent := &mockAgent{events: []claude.Event{claude.ResultEvent(0, 0.1, "success")}}
	git := &mockGit{branch: "main", lastCommit: "abc"}
	cfg := defaultTestConfig()
	cfg.Build.MaxIterations = 3
	cfg.Build.Ludicrous = true
	lp, output := setupTestLoop(t, agent, git, cfg)
	lp.PostIteration = func(_ context.Context) bool { return true }

	if err := lp.Run(context.Background(), ModeBuild, 0); err != nil {
		t.Fatalf("run: %v", err)
	}
	if agent.calls != 2 {
		t.Fatalf("agent calls = %d, want 2 independent success signals", agent.calls)
	}
	if !strings.Contains(output.String(), "Spec complete") {
		t.Fatalf("completion event missing:\n%s", output.String())
	}
}

func TestLudicrousModeIgnoresConfiguredMaxUntilTasksComplete(t *testing.T) {
	agent := &mockAgent{events: []claude.Event{claude.ResultEvent(0, 0.1, "success")}}
	git := &mockGit{branch: "main", lastCommit: "abc"}
	cfg := defaultTestConfig()
	cfg.Build.MaxIterations = 1
	cfg.Build.Ludicrous = true
	lp, output := setupTestLoop(t, agent, git, cfg)
	lp.SpecDir = filepath.Join("specs", "active")
	if err := os.MkdirAll(filepath.Join(lp.Dir, lp.SpecDir), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lp.tasksPath(), []byte("- [ ] unfinished\n"), 0644); err != nil {
		t.Fatal(err)
	}
	stopCh := make(chan struct{})
	lp.StopAfter = stopCh
	lp.PostIteration = func(_ context.Context) bool {
		if agent.calls == 3 {
			close(stopCh)
		}
		return true
	}

	if err := lp.Run(context.Background(), ModeBuild, 0); !errors.Is(err, ErrOperatorStopped) {
		t.Fatalf("run error = %v, want ErrOperatorStopped", err)
	}
	if agent.calls != 3 {
		t.Fatalf("agent calls = %d, want configured max ignored until stop", agent.calls)
	}
	if strings.Contains(output.String(), "Spec complete") {
		t.Fatalf("unfinished tasks emitted completion:\n%s", output.String())
	}
	if !strings.Contains(output.String(), "declared tasks are incomplete") {
		t.Fatalf("missing task-evidence diagnostic:\n%s", output.String())
	}
}

func TestLudicrousModeRespectsStopAfter(t *testing.T) {
	agent := &mockAgent{events: []claude.Event{claude.ResultEvent(0, 0.1, "success")}}
	cfg := defaultTestConfig()
	cfg.Build.Ludicrous = true
	lp, _ := setupTestLoop(t, agent, &mockGit{branch: "main", lastCommit: "abc"}, cfg)
	stopCh := make(chan struct{})
	lp.StopAfter = stopCh
	lp.PostIteration = func(_ context.Context) bool {
		close(stopCh)
		return false
	}

	if err := lp.Run(context.Background(), ModeBuild, 0); !errors.Is(err, ErrOperatorStopped) {
		t.Fatalf("run error = %v, want ErrOperatorStopped", err)
	}
	if agent.calls != 1 {
		t.Fatalf("agent calls = %d, want 1", agent.calls)
	}
}

func TestLudicrousModeRespectsContextCancel(t *testing.T) {
	agent := &mockAgent{events: []claude.Event{claude.ResultEvent(0, 0.1, "success")}}
	cfg := defaultTestConfig()
	cfg.Build.Ludicrous = true
	lp, _ := setupTestLoop(t, agent, &mockGit{branch: "main", lastCommit: "abc"}, cfg)
	ctx, cancel := context.WithCancel(context.Background())
	lp.PostIteration = func(_ context.Context) bool {
		cancel()
		return false
	}

	err := lp.Run(ctx, ModeBuild, 0)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("run error = %v, want context cancellation", err)
	}
	if agent.calls != 1 {
		t.Fatalf("agent calls = %d, want 1", agent.calls)
	}
}
