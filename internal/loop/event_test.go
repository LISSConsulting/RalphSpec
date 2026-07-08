package loop

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/LISSConsulting/RalphSpec/internal/claude"
	"github.com/LISSConsulting/RalphSpec/internal/config"
)

func TestEmitToChannel(t *testing.T) {
	ch := make(chan LogEntry, 8)
	agent := &mockAgent{
		events: []claude.Event{claude.ResultEvent(0.10, 2.0, "success")},
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

	// Drain the channel
	close(ch)
	var entries []LogEntry
	for e := range ch {
		entries = append(entries, e)
	}

	if len(entries) == 0 {
		t.Fatal("expected events to be sent to channel")
	}
	if entries[0].Agent != config.AgentClaude {
		t.Errorf("expected first event agent %q, got %q", config.AgentClaude, entries[0].Agent)
	}

	// Verify we got a mix of event types
	kinds := map[LogKind]bool{}
	for _, e := range entries {
		kinds[e.Kind] = true
	}

	if !kinds[LogInfo] {
		t.Error("expected at least one LogInfo event")
	}
	if !kinds[LogIterStart] {
		t.Error("expected at least one LogIterStart event")
	}
	if !kinds[LogIterComplete] {
		t.Error("expected at least one LogIterComplete event")
	}
}

func TestEmitToChannelWithToolUse(t *testing.T) {
	ch := make(chan LogEntry, 16)
	agent := &mockAgent{
		events: []claude.Event{
			claude.ToolUseEvent("read_file", map[string]any{"file_path": "main.go"}),
			claude.ResultEvent(0.05, 1.0, "success"),
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
	var toolEvents []LogEntry
	for e := range ch {
		if e.Kind == LogToolUse {
			toolEvents = append(toolEvents, e)
		}
	}

	if len(toolEvents) != 1 {
		t.Fatalf("expected 1 tool use event, got %d", len(toolEvents))
	}
	if toolEvents[0].ToolName != "read_file" {
		t.Errorf("expected tool name read_file, got %s", toolEvents[0].ToolName)
	}
	if toolEvents[0].ToolInput != "main.go" {
		t.Errorf("expected tool input main.go, got %s", toolEvents[0].ToolInput)
	}
}

func TestEmitFallsBackToWriter(t *testing.T) {
	agent := &mockAgent{
		events: []claude.Event{claude.ResultEvent(0.10, 1.0, "success")},
	}
	git := &mockGit{branch: "main", lastCommit: "abc test"}
	cfg := defaultTestConfig()
	cfg.Build.MaxIterations = 1

	lp, buf := setupTestLoop(t, agent, git, cfg)
	// Events is nil — should fall back to Log writer

	err := lp.Run(context.Background(), ModeBuild, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if output == "" {
		t.Error("expected output written to Log writer when Events is nil")
	}
	if !strings.Contains(output, "Starting build loop") {
		t.Error("expected log to contain starting message")
	}
}

func TestEmitDoesNotWriteToLogWhenChannelSet(t *testing.T) {
	ch := make(chan LogEntry, 16)
	agent := &mockAgent{
		events: []claude.Event{claude.ResultEvent(0.10, 1.0, "success")},
	}
	git := &mockGit{branch: "main", lastCommit: "abc test"}
	cfg := defaultTestConfig()
	cfg.Build.MaxIterations = 1

	lp, buf := setupTestLoop(t, agent, git, cfg)
	lp.Events = ch

	err := lp.Run(context.Background(), ModeBuild, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Drain channel to prevent leaks
	close(ch)
	for range ch {
	}

	if buf.String() != "" {
		t.Errorf("expected no output to Log writer when Events is set, got: %s", buf.String())
	}
}

func TestEmitNonBlocking(t *testing.T) {
	// Channel with zero buffer — emit should not block
	ch := make(chan LogEntry)
	var buf bytes.Buffer
	lp := &Loop{
		Config: &config.Config{},
		Log:    &buf,
		Events: ch,
	}

	// This should not block even though nobody reads from ch
	done := make(chan struct{})
	go func() {
		lp.emit(LogEntry{Kind: LogInfo, Message: "test"})
		close(done)
	}()

	select {
	case <-done:
		// Good — emit returned without blocking
	case <-time.After(time.Second):
		t.Fatal("emit blocked on full channel — should be non-blocking")
	}
}

func TestEmitSetsTimestamp(t *testing.T) {
	ch := make(chan LogEntry, 1)
	lp := &Loop{
		Config: &config.Config{},
		Events: ch,
	}

	before := time.Now()
	lp.emit(LogEntry{Kind: LogInfo, Message: "test"})
	after := time.Now()

	entry := <-ch
	if entry.Timestamp.Before(before) || entry.Timestamp.After(after) {
		t.Errorf("expected timestamp between %v and %v, got %v", before, after, entry.Timestamp)
	}
}

func TestEmitPreservesExistingTimestamp(t *testing.T) {
	ch := make(chan LogEntry, 1)
	lp := &Loop{
		Config: &config.Config{},
		Events: ch,
	}

	ts := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	lp.emit(LogEntry{Kind: LogInfo, Message: "test", Timestamp: ts})

	entry := <-ch
	if !entry.Timestamp.Equal(ts) {
		t.Errorf("expected timestamp %v to be preserved, got %v", ts, entry.Timestamp)
	}
}

func TestEmitCallsNotificationHook(t *testing.T) {
	var received []LogEntry
	lp := &Loop{
		Config: &config.Config{},
		Log:    &bytes.Buffer{},
		NotificationHook: func(e LogEntry) {
			received = append(received, e)
		},
	}

	lp.emit(LogEntry{Kind: LogInfo, Message: "hello"})
	lp.emit(LogEntry{Kind: LogError, Message: "oops"})

	if len(received) != 2 {
		t.Fatalf("expected 2 hook calls, got %d", len(received))
	}
	if received[0].Kind != LogInfo || received[0].Message != "hello" {
		t.Errorf("unexpected first entry: %+v", received[0])
	}
	if received[1].Kind != LogError || received[1].Message != "oops" {
		t.Errorf("unexpected second entry: %+v", received[1])
	}
}

func TestEmitBranchAndIterationInEvents(t *testing.T) {
	ch := make(chan LogEntry, 16)
	agent := &mockAgent{
		events: []claude.Event{claude.ResultEvent(0.10, 1.0, "success")},
	}
	git := &mockGit{branch: "feat/tui", lastCommit: "abc test"}
	cfg := defaultTestConfig()
	cfg.Build.MaxIterations = 1

	lp, _ := setupTestLoop(t, agent, git, cfg)
	lp.Events = ch

	err := lp.Run(context.Background(), ModeBuild, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	close(ch)
	var iterStart *LogEntry
	for e := range ch {
		if e.Kind == LogIterStart {
			iterStart = &e
			break
		}
	}

	if iterStart == nil {
		t.Fatal("expected an IterStart event")
	}
	if iterStart.Branch != "feat/tui" {
		t.Errorf("expected branch feat/tui, got %s", iterStart.Branch)
	}
	if iterStart.Iteration != 1 {
		t.Errorf("expected iteration 1, got %d", iterStart.Iteration)
	}
	if iterStart.Agent != config.AgentClaude {
		t.Errorf("expected agent %q, got %q", config.AgentClaude, iterStart.Agent)
	}
}
