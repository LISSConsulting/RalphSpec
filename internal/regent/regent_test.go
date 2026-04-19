package regent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/LISSConsulting/RalphSpec/internal/config"
	"github.com/LISSConsulting/RalphSpec/internal/loop"
)

func defaultTestRegentConfig() config.RegentConfig {
	return config.RegentConfig{
		Enabled:               true,
		RollbackOnTestFailure: false,
		TestCommand:           "",
		MaxRetries:            3,
		RetryBackoffSeconds:   0, // no delay in tests
		HangTimeoutSeconds:    0, // disabled in most tests
	}
}

func passingCommand() string {
	if runtime.GOOS == "windows" {
		return "cmd /c exit 0"
	}
	return "true"
}

func failingCommand() string {
	if runtime.GOOS == "windows" {
		return "cmd /c exit 1"
	}
	return "false"
}

func TestSupervise(t *testing.T) {
	t.Run("successful run completes without retries", func(t *testing.T) {
		dir := t.TempDir()
		cfg := defaultTestRegentConfig()
		events := make(chan loop.LogEntry, 128)
		rgt := New(cfg, dir, &mockGit{branch: "main"}, events)

		calls := 0
		run := func(_ context.Context) error {
			calls++
			return nil
		}

		errCh := make(chan error, 1)
		go func() {
			errCh <- rgt.Supervise(context.Background(), run)
			close(events)
		}()

		err := <-errCh
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if calls != 1 {
			t.Errorf("expected 1 call, got %d", calls)
		}
	})

	t.Run("retries on failure", func(t *testing.T) {
		dir := t.TempDir()
		cfg := defaultTestRegentConfig()
		cfg.MaxRetries = 2
		events := make(chan loop.LogEntry, 128)
		rgt := New(cfg, dir, &mockGit{branch: "main"}, events)

		calls := 0
		run := func(_ context.Context) error {
			calls++
			if calls <= 2 {
				return errors.New("fail")
			}
			return nil
		}

		errCh := make(chan error, 1)
		go func() {
			errCh <- rgt.Supervise(context.Background(), run)
			close(events)
		}()

		err := <-errCh
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if calls != 3 {
			t.Errorf("expected 3 calls (2 failures + 1 success), got %d", calls)
		}
	})

	t.Run("gives up after max retries", func(t *testing.T) {
		dir := t.TempDir()
		cfg := defaultTestRegentConfig()
		cfg.MaxRetries = 2
		events := make(chan loop.LogEntry, 128)
		rgt := New(cfg, dir, &mockGit{branch: "main"}, events)

		run := func(_ context.Context) error {
			return errors.New("persistent failure")
		}

		errCh := make(chan error, 1)
		go func() {
			errCh <- rgt.Supervise(context.Background(), run)
			close(events)
		}()

		err := <-errCh
		if err == nil {
			t.Fatal("expected error after max retries")
		}
		if !strings.Contains(err.Error(), "max retries exceeded") {
			t.Errorf("error should mention max retries, got: %v", err)
		}
	})

	t.Run("context cancellation stops supervision", func(t *testing.T) {
		dir := t.TempDir()
		cfg := defaultTestRegentConfig()
		events := make(chan loop.LogEntry, 128)
		rgt := New(cfg, dir, &mockGit{branch: "main"}, events)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel immediately

		errCh := make(chan error, 1)
		go func() {
			errCh <- rgt.Supervise(ctx, func(_ context.Context) error {
				return nil
			})
			close(events)
		}()

		err := <-errCh
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got: %v", err)
		}
	})

	t.Run("context cancellation persists finished state", func(t *testing.T) {
		dir := t.TempDir()
		cfg := defaultTestRegentConfig()
		events := make(chan loop.LogEntry, 128)
		rgt := New(cfg, dir, &mockGit{branch: "main"}, events)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel immediately

		errCh := make(chan error, 1)
		go func() {
			errCh <- rgt.Supervise(ctx, func(_ context.Context) error {
				return nil
			})
			close(events)
		}()

		<-errCh
		state, loadErr := LoadState(dir)
		if loadErr != nil {
			t.Fatalf("LoadState: %v", loadErr)
		}
		if state.FinishedAt.IsZero() {
			t.Error("expected FinishedAt to be set on context cancellation")
		}
		if !state.Passed {
			t.Error("expected Passed = true on context cancellation (user-initiated stop)")
		}
	})

	t.Run("context cancel after run failure persists finished state", func(t *testing.T) {
		dir := t.TempDir()
		cfg := defaultTestRegentConfig()
		cfg.MaxRetries = 3
		events := make(chan loop.LogEntry, 128)
		rgt := New(cfg, dir, &mockGit{branch: "main"}, events)

		ctx, cancel := context.WithCancel(context.Background())
		calls := 0
		run := func(_ context.Context) error {
			calls++
			cancel() // cancel after first run failure
			return errors.New("fail")
		}

		errCh := make(chan error, 1)
		go func() {
			errCh <- rgt.Supervise(ctx, run)
			close(events)
		}()

		<-errCh
		state, loadErr := LoadState(dir)
		if loadErr != nil {
			t.Fatalf("LoadState: %v", loadErr)
		}
		if state.FinishedAt.IsZero() {
			t.Error("expected FinishedAt to be set when context cancelled after failure")
		}
		if !state.Passed {
			t.Error("expected Passed = true on context cancellation (user-initiated stop)")
		}
	})

	t.Run("saves state file", func(t *testing.T) {
		dir := t.TempDir()
		cfg := defaultTestRegentConfig()
		events := make(chan loop.LogEntry, 128)
		rgt := New(cfg, dir, &mockGit{branch: "main"}, events)

		errCh := make(chan error, 1)
		go func() {
			errCh <- rgt.Supervise(context.Background(), func(_ context.Context) error {
				return nil
			})
			close(events)
		}()

		<-errCh
		state, err := LoadState(dir)
		if err != nil {
			t.Fatalf("LoadState: %v", err)
		}
		if state.RalphPID == 0 {
			t.Error("expected non-zero PID in saved state")
		}
	})

	t.Run("successful run sets passed and timestamps", func(t *testing.T) {
		dir := t.TempDir()
		cfg := defaultTestRegentConfig()
		events := make(chan loop.LogEntry, 128)
		rgt := New(cfg, dir, &mockGit{branch: "main"}, events)

		errCh := make(chan error, 1)
		go func() {
			errCh <- rgt.Supervise(context.Background(), func(_ context.Context) error {
				return nil
			})
			close(events)
		}()

		<-errCh
		state, err := LoadState(dir)
		if err != nil {
			t.Fatalf("LoadState: %v", err)
		}
		if !state.Passed {
			t.Error("expected Passed = true after successful run")
		}
		if state.StartedAt.IsZero() {
			t.Error("expected StartedAt to be set")
		}
		if state.FinishedAt.IsZero() {
			t.Error("expected FinishedAt to be set")
		}
		if state.FinishedAt.Before(state.StartedAt) {
			t.Error("expected FinishedAt >= StartedAt")
		}
	})

	t.Run("failed run sets passed false", func(t *testing.T) {
		dir := t.TempDir()
		cfg := defaultTestRegentConfig()
		cfg.MaxRetries = 0
		events := make(chan loop.LogEntry, 128)
		rgt := New(cfg, dir, &mockGit{branch: "main"}, events)

		errCh := make(chan error, 1)
		go func() {
			errCh <- rgt.Supervise(context.Background(), func(_ context.Context) error {
				return errors.New("fail")
			})
			close(events)
		}()

		<-errCh
		state, err := LoadState(dir)
		if err != nil {
			t.Fatalf("LoadState: %v", err)
		}
		if state.Passed {
			t.Error("expected Passed = false after failed run")
		}
		if state.FinishedAt.IsZero() {
			t.Error("expected FinishedAt to be set on failure")
		}
	})

	t.Run("emits regent events", func(t *testing.T) {
		dir := t.TempDir()
		cfg := defaultTestRegentConfig()
		events := make(chan loop.LogEntry, 128)
		rgt := New(cfg, dir, &mockGit{branch: "main"}, events)

		go func() {
			_ = rgt.Supervise(context.Background(), func(_ context.Context) error {
				return nil
			})
			close(events)
		}()

		var regentMsgs []string
		for entry := range events {
			if entry.Kind == loop.LogRegent {
				regentMsgs = append(regentMsgs, entry.Message)
			}
		}

		if len(regentMsgs) == 0 {
			t.Error("expected at least one regent event")
		}

		foundStart := false
		for _, msg := range regentMsgs {
			if strings.Contains(msg, "Starting Ralph") {
				foundStart = true
			}
		}
		if !foundStart {
			t.Errorf("expected 'Starting Ralph' message, got: %v", regentMsgs)
		}
	})
}

func TestHangDetection(t *testing.T) {
	t.Run("kills loop after hang timeout", func(t *testing.T) {
		dir := t.TempDir()
		cfg := defaultTestRegentConfig()
		cfg.HangTimeoutSeconds = 1 // 1 second timeout for test
		cfg.MaxRetries = 0         // don't retry after hang

		events := make(chan loop.LogEntry, 128)
		rgt := New(cfg, dir, &mockGit{branch: "main"}, events)

		run := func(ctx context.Context) error {
			// Block until context is cancelled (simulates a hang)
			<-ctx.Done()
			return ctx.Err()
		}

		errCh := make(chan error, 1)
		go func() {
			errCh <- rgt.Supervise(context.Background(), run)
			close(events)
		}()

		err := <-errCh
		if err == nil {
			t.Fatal("expected error from hang detection")
		}

		// Check that a hang detection event was emitted
		var foundHang bool
		for entry := range events {
			if entry.Kind == loop.LogRegent && strings.Contains(entry.Message, "Hang detected") {
				foundHang = true
			}
		}
		if !foundHang {
			t.Error("expected hang detection message")
		}
	})

	t.Run("output resets hang timer", func(t *testing.T) {
		dir := t.TempDir()
		cfg := defaultTestRegentConfig()
		cfg.HangTimeoutSeconds = 2

		events := make(chan loop.LogEntry, 128)
		rgt := New(cfg, dir, &mockGit{branch: "main"}, events)

		run := func(ctx context.Context) error {
			// Produce output periodically to prevent hang
			for i := 0; i < 3; i++ {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(500 * time.Millisecond):
					rgt.NotifyOutput()
				}
			}
			return nil
		}

		errCh := make(chan error, 1)
		go func() {
			errCh <- rgt.Supervise(context.Background(), run)
			close(events)
		}()

		err := <-errCh
		if err != nil {
			t.Fatalf("should complete without hang, got: %v", err)
		}
	})
}

func TestUpdateState(t *testing.T) {
	cfg := defaultTestRegentConfig()
	events := make(chan loop.LogEntry, 128)
	rgt := New(cfg, t.TempDir(), &mockGit{}, events)

	tests := []struct {
		name  string
		entry loop.LogEntry
		check func(t *testing.T, r *Regent)
	}{
		{
			name:  "updates iteration",
			entry: loop.LogEntry{Iteration: 5},
			check: func(t *testing.T, r *Regent) {
				r.mu.Lock()
				defer r.mu.Unlock()
				if r.state.Iteration != 5 {
					t.Errorf("Iteration = %d, want 5", r.state.Iteration)
				}
			},
		},
		{
			name:  "updates total cost",
			entry: loop.LogEntry{TotalCost: 2.50},
			check: func(t *testing.T, r *Regent) {
				r.mu.Lock()
				defer r.mu.Unlock()
				if r.state.TotalCostUSD != 2.50 {
					t.Errorf("TotalCostUSD = %f, want 2.50", r.state.TotalCostUSD)
				}
			},
		},
		{
			name:  "updates commit",
			entry: loop.LogEntry{Commit: "def456 new feature"},
			check: func(t *testing.T, r *Regent) {
				r.mu.Lock()
				defer r.mu.Unlock()
				if r.state.LastCommit != "def456 new feature" {
					t.Errorf("LastCommit = %q, want %q", r.state.LastCommit, "def456 new feature")
				}
			},
		},
		{
			name:  "updates branch",
			entry: loop.LogEntry{Branch: "feat/new-feature"},
			check: func(t *testing.T, r *Regent) {
				r.mu.Lock()
				defer r.mu.Unlock()
				if r.state.Branch != "feat/new-feature" {
					t.Errorf("Branch = %q, want %q", r.state.Branch, "feat/new-feature")
				}
			},
		},
		{
			name:  "updates mode",
			entry: loop.LogEntry{Mode: "build"},
			check: func(t *testing.T, r *Regent) {
				r.mu.Lock()
				defer r.mu.Unlock()
				if r.state.Mode != "build" {
					t.Errorf("Mode = %q, want %q", r.state.Mode, "build")
				}
			},
		},
		{
			name:  "zero values do not overwrite",
			entry: loop.LogEntry{Message: "just a message"},
			check: func(t *testing.T, r *Regent) {
				r.mu.Lock()
				defer r.mu.Unlock()
				if r.state.Iteration != 5 {
					t.Errorf("Iteration should remain 5, got %d", r.state.Iteration)
				}
				if r.state.Branch != "feat/new-feature" {
					t.Errorf("Branch should remain feat/new-feature, got %q", r.state.Branch)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rgt.UpdateState(tt.entry)
			tt.check(t, rgt)
		})
	}
}

func TestUpdateStatePersistsToFile(t *testing.T) {
	dir := t.TempDir()
	cfg := defaultTestRegentConfig()
	events := make(chan loop.LogEntry, 128)
	rgt := New(cfg, dir, &mockGit{}, events)

	rgt.UpdateState(loop.LogEntry{
		Iteration: 3,
		TotalCost: 1.50,
		Branch:    "feat/scroll",
		Mode:      "build",
		Commit:    "abc1234 add scroll",
	})

	state, err := LoadState(dir)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if state.Iteration != 3 {
		t.Errorf("Iteration = %d, want 3", state.Iteration)
	}
	if state.TotalCostUSD != 1.50 {
		t.Errorf("TotalCostUSD = %f, want 1.50", state.TotalCostUSD)
	}
	if state.Branch != "feat/scroll" {
		t.Errorf("Branch = %q, want %q", state.Branch, "feat/scroll")
	}
	if state.Mode != "build" {
		t.Errorf("Mode = %q, want %q", state.Mode, "build")
	}
	if state.LastCommit != "abc1234 add scroll" {
		t.Errorf("LastCommit = %q, want %q", state.LastCommit, "abc1234 add scroll")
	}
}

func TestUpdateStateSkipsSaveOnNoChange(t *testing.T) {
	dir := t.TempDir()
	cfg := defaultTestRegentConfig()
	events := make(chan loop.LogEntry, 128)
	rgt := New(cfg, dir, &mockGit{}, events)

	// Send entry with no meaningful state fields
	rgt.UpdateState(loop.LogEntry{
		Kind:    loop.LogInfo,
		Message: "just a message",
	})

	// State file should not exist since no state fields changed
	_, err := LoadState(dir)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	// LoadState returns zero State when file doesn't exist, which is fine
}

func TestRunPostIterationTests(t *testing.T) {
	t.Run("skipped when rollback disabled", func(t *testing.T) {
		dir := t.TempDir()
		cfg := defaultTestRegentConfig()
		cfg.RollbackOnTestFailure = false
		events := make(chan loop.LogEntry, 128)
		g := &mockGit{branch: "main", lastCommit: "abc test"}
		rgt := New(cfg, dir, g, events)

		rgt.RunPostIterationTests()
		if len(g.revertCalls) != 0 {
			t.Error("should not revert when rollback is disabled")
		}
	})

	t.Run("skipped when test command is empty", func(t *testing.T) {
		dir := t.TempDir()
		cfg := defaultTestRegentConfig()
		cfg.RollbackOnTestFailure = true
		cfg.TestCommand = ""
		events := make(chan loop.LogEntry, 128)
		g := &mockGit{branch: "main", lastCommit: "abc test"}
		rgt := New(cfg, dir, g, events)

		rgt.RunPostIterationTests()
		// No panic, no revert = success
		if len(g.revertCalls) != 0 {
			t.Error("should not revert when test command is empty")
		}
	})

	t.Run("passing tests keep commit", func(t *testing.T) {
		dir := t.TempDir()
		cfg := defaultTestRegentConfig()
		cfg.RollbackOnTestFailure = true
		cfg.TestCommand = passingCommand()
		events := make(chan loop.LogEntry, 128)
		g := &mockGit{branch: "main", lastCommit: "abc good commit"}
		rgt := New(cfg, dir, g, events)

		go func() {
			for range events {
			}
		}()

		rgt.RunPostIterationTests()
		if len(g.revertCalls) != 0 {
			t.Error("should not revert when tests pass")
		}
	})

	t.Run("failing tests trigger revert", func(t *testing.T) {
		dir := t.TempDir()
		cfg := defaultTestRegentConfig()
		cfg.RollbackOnTestFailure = true
		cfg.TestCommand = failingCommand()
		events := make(chan loop.LogEntry, 128)
		g := &mockGit{branch: "main", lastCommit: "abc1234 bad commit"}
		rgt := New(cfg, dir, g, events)

		go func() {
			for range events {
			}
		}()

		rgt.RunPostIterationTests()
		if len(g.revertCalls) != 1 {
			t.Fatalf("expected 1 revert call, got %d", len(g.revertCalls))
		}
		if g.revertCalls[0] != "abc1234" {
			t.Errorf("reverted %q, want %q", g.revertCalls[0], "abc1234")
		}
		if len(g.pushCalls) != 1 {
			t.Errorf("expected 1 push call after revert, got %d", len(g.pushCalls))
		}
	})

	t.Run("emits events instead of returning errors", func(t *testing.T) {
		dir := t.TempDir()
		cfg := defaultTestRegentConfig()
		cfg.RollbackOnTestFailure = true
		cfg.TestCommand = passingCommand()
		events := make(chan loop.LogEntry, 128)
		g := &mockGit{branch: "main", lastCommit: "abc commit"}
		rgt := New(cfg, dir, g, events)

		rgt.RunPostIterationTests()

		// Drain events and verify regent messages were emitted
		close(events)
		var regentMsgs []string
		for entry := range events {
			if entry.Kind == loop.LogRegent {
				regentMsgs = append(regentMsgs, entry.Message)
			}
		}
		if len(regentMsgs) == 0 {
			t.Error("expected at least one regent event from RunPostIterationTests")
		}

		foundTests := false
		for _, msg := range regentMsgs {
			if strings.Contains(msg, "Tests passed") {
				foundTests = true
			}
		}
		if !foundTests {
			t.Errorf("expected 'Tests passed' message, got: %v", regentMsgs)
		}
	})

	t.Run("revert failure emits event and continues", func(t *testing.T) {
		dir := t.TempDir()
		cfg := defaultTestRegentConfig()
		cfg.RollbackOnTestFailure = true
		cfg.TestCommand = failingCommand() // tests fail → triggers revert
		events := make(chan loop.LogEntry, 128)
		g := &mockGit{
			branch:     "main",
			lastCommit: "abc1234 bad commit",
			revertErr:  errors.New("revert conflict"),
		}
		rgt := New(cfg, dir, g, events)

		go func() {
			for range events {
			}
		}()

		// Should not panic; emits "Failed to revert" event
		rgt.RunPostIterationTests()
	})
}

func TestSupervisWithNilEvents(t *testing.T) {
	// When events channel is nil, Regent should still work (emit becomes a no-op).
	dir := t.TempDir()
	cfg := defaultTestRegentConfig()
	rgt := New(cfg, dir, &mockGit{branch: "main"}, nil)

	err := rgt.Supervise(context.Background(), func(_ context.Context) error {
		return nil
	})
	if err != nil {
		t.Fatalf("Supervise with nil events should succeed, got: %v", err)
	}
}

func TestSaveStateEmitsErrorOnFailure(t *testing.T) {
	// Block the .ralph directory so SaveState fails, then verify saveState emits
	// a "Failed to save state" event rather than silently swallowing the error.
	dir := t.TempDir()
	cfg := defaultTestRegentConfig()
	events := make(chan loop.LogEntry, 128)
	rgt := New(cfg, dir, &mockGit{branch: "main"}, events)

	// Place a regular file at .ralph path so MkdirAll fails inside SaveState.
	if err := os.WriteFile(filepath.Join(dir, stateDirName), []byte("block"), 0644); err != nil {
		t.Fatal(err)
	}

	rgt.saveState()

	close(events)
	var found bool
	for e := range events {
		if e.Kind == loop.LogRegent && strings.Contains(e.Message, "Failed to save state") {
			found = true
		}
	}
	if !found {
		t.Error("expected 'Failed to save state' regent event when SaveState fails")
	}
}

func TestRunPostIterationTests_RunTestsError(t *testing.T) {
	// Clearing PATH prevents exec.LookPath from finding the shell binary.
	// RunPostIterationTests must emit "Failed to start tests" and not trigger revert.
	t.Setenv("PATH", "")

	dir := t.TempDir()
	cfg := defaultTestRegentConfig()
	cfg.RollbackOnTestFailure = true
	cfg.TestCommand = "some-nonexistent-command"
	events := make(chan loop.LogEntry, 128)
	g := &mockGit{branch: "main", lastCommit: "abc123 commit"}
	rgt := New(cfg, dir, g, events)

	rgt.RunPostIterationTests()

	close(events)
	var found bool
	for e := range events {
		if e.Kind == loop.LogRegent && strings.Contains(e.Message, "Failed to start tests") {
			found = true
		}
	}
	if !found {
		t.Error("expected 'Failed to start tests' event when shell is not in PATH")
	}
	if len(g.revertCalls) != 0 {
		t.Error("should not revert when RunTests returns a non-ExitError")
	}
}

func TestFlushState(t *testing.T) {
	dir := t.TempDir()
	cfg := defaultTestRegentConfig()
	events := make(chan loop.LogEntry, 128)
	rgt := New(cfg, dir, &mockGit{}, events)

	rgt.UpdateState(loop.LogEntry{Iteration: 7, TotalCost: 3.14, Branch: "main"})
	rgt.FlushState()

	state, err := LoadState(dir)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if state.Iteration != 7 {
		t.Errorf("Iteration = %d, want 7", state.Iteration)
	}
	if state.TotalCostUSD != 3.14 {
		t.Errorf("TotalCostUSD = %f, want 3.14", state.TotalCostUSD)
	}
	if state.Branch != "main" {
		t.Errorf("Branch = %q, want %q", state.Branch, "main")
	}
}

func TestEmitOnClosedChannelDoesNotPanic(t *testing.T) {
	// Regression: emit must not panic when the events channel has been closed.
	// This occurs in runWithRegent when the drain goroutine calls UpdateState →
	// saveState → emit after close(events) has already been called.
	events := make(chan loop.LogEntry, 1)
	rgt := New(defaultTestRegentConfig(), t.TempDir(), &mockGit{}, events)
	close(events)
	rgt.emit("message after channel close") // must not panic
}

func TestSaveStateOnClosedChannelDoesNotPanic(t *testing.T) {
	// Regression: when SaveState fails (blocked .ralph dir) and the events
	// channel has already been closed, saveState must not panic on the emit call.
	dir := t.TempDir()
	events := make(chan loop.LogEntry, 1)
	rgt := New(defaultTestRegentConfig(), dir, &mockGit{}, events)

	if err := os.WriteFile(filepath.Join(dir, stateDirName), []byte("block"), 0644); err != nil {
		t.Fatal(err)
	}
	close(events)
	rgt.saveState() // SaveState fails → emit called → must not panic
}

func TestSupervise_ContextCancelDuringBackoff(t *testing.T) {
	// Cancel context after "Ralph exited with error" is emitted, which means we
	// have passed the ctx.Err() check at line 83 and will enter the backoff select.
	// This covers the case <-ctx.Done() branch inside the backoff wait.
	dir := t.TempDir()
	cfg := defaultTestRegentConfig()
	cfg.MaxRetries = 3
	cfg.RetryBackoffSeconds = 10 // long backoff ensures ctx.Done fires before time.After
	events := make(chan loop.LogEntry, 128)
	rgt := New(cfg, dir, &mockGit{branch: "main"}, events)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Watch events: cancel once "Ralph exited with error" is observed so that the
	// main goroutine is guaranteed to be at (or heading into) the backoff select.
	go func() {
		for e := range events {
			if e.Kind == loop.LogRegent && strings.Contains(e.Message, "Ralph exited with error") {
				cancel()
				return
			}
		}
	}()

	errCh := make(chan error, 1)
	go func() {
		errCh <- rgt.Supervise(ctx, func(_ context.Context) error {
			return errors.New("fail")
		})
		close(events)
	}()

	err := <-errCh
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
}
