package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/LISSConsulting/RalphSpec/internal/claude"
)

// init runs as a fake Claude subprocess when _FAKE_CLAUDE=1 is set.
// Placing the fake-mode guard in init() (before flag.Parse in TestMain/m.Run)
// avoids flag-parse failures caused by Claude CLI arguments such as
// --output-format that are unrecognised by the Go test runner.
func init() {
	if os.Getenv("_FAKE_CLAUDE") != "1" {
		return
	}
	if f := os.Getenv("_FAKE_CLAUDE_STDIN_FILE"); f != "" {
		data, _ := io.ReadAll(os.Stdin)
		_ = os.WriteFile(f, data, 0644)
	}
	if f := os.Getenv("_FAKE_CLAUDE_STDOUT_FILE"); f != "" {
		if data, err := os.ReadFile(f); err == nil {
			_, _ = os.Stdout.Write(data)
		}
	}
	if s := os.Getenv("_FAKE_CLAUDE_STDERR"); s != "" {
		_, _ = fmt.Fprint(os.Stderr, s)
	}
	if os.Getenv("_FAKE_CLAUDE_SLEEP") == "1" {
		time.Sleep(time.Minute)
	}
	code := 0
	if s := os.Getenv("_FAKE_CLAUDE_EXIT"); s != "" {
		_, _ = fmt.Sscan(s, &code)
	}
	os.Exit(code)
}

func TestNewClaudeAgent(t *testing.T) {
	agent := NewClaudeAgent()
	if agent.Executable != "claude" {
		t.Errorf("expected executable %q, got %q", "claude", agent.Executable)
	}
}

func TestBuildArgs(t *testing.T) {
	agent := &ClaudeAgent{}

	tests := []struct {
		name     string
		prompt   string
		opts     claude.RunOptions
		contains []string
		excludes []string
	}{
		{
			name:   "basic prompt",
			prompt: "test prompt",
			opts:   claude.RunOptions{},
			contains: []string{
				"-p", "test prompt",
				"--output-format", "stream-json",
				"--verbose",
			},
			excludes: []string{"--model", "--dangerously-skip-permissions", "--max-turns"},
		},
		{
			name:   "with model",
			prompt: "test",
			opts:   claude.RunOptions{Model: "opus"},
			contains: []string{
				"--model", "opus",
			},
		},
		{
			name:   "with max turns",
			prompt: "test",
			opts:   claude.RunOptions{MaxTurns: 25},
			contains: []string{
				"--max-turns", "25",
			},
		},
		{
			name:     "zero max turns omitted",
			prompt:   "test",
			opts:     claude.RunOptions{MaxTurns: 0},
			excludes: []string{"--max-turns"},
		},
		{
			name:   "with danger skip permissions",
			prompt: "test",
			opts:   claude.RunOptions{DangerSkipPermissions: true},
			contains: []string{
				"--dangerously-skip-permissions",
			},
		},
		{
			name:   "all options",
			prompt: "full test",
			opts:   claude.RunOptions{Model: "sonnet", MaxTurns: 50, DangerSkipPermissions: true},
			contains: []string{
				"-p", "full test",
				"--output-format", "stream-json",
				"--verbose",
				"--model", "sonnet",
				"--max-turns", "50",
				"--dangerously-skip-permissions",
			},
		},
		{
			name:   "live steer uses SDK pipe mode",
			prompt: "test prompt",
			opts:   claude.RunOptions{Steer: make(chan string)},
			contains: []string{
				"-p",
				"--output-format", "stream-json",
				"--input-format", "stream-json",
				"--verbose",
			},
			excludes: []string{"test prompt"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := agent.buildArgs(tt.prompt, tt.opts)

			for _, want := range tt.contains {
				if !containsArg(args, want) {
					t.Errorf("args %v missing expected %q", args, want)
				}
			}
			for _, unwanted := range tt.excludes {
				if containsArg(args, unwanted) {
					t.Errorf("args %v should not contain %q", args, unwanted)
				}
			}
		})
	}
}

// TestClaudeAgentRun tests the full subprocess lifecycle using the test binary
// as a cross-platform fake Claude CLI (via the init() fake-mode guard above).
// No shell scripts are required, so these tests run on all platforms.
func TestClaudeAgentRun(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}

	t.Run("streams events from subprocess", func(t *testing.T) {
		output := `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"read_file","input":{"file_path":"main.go"}}]}}
{"type":"result","cost_usd":0.10,"duration_ms":2500}`
		agent := setUpFakeClaude(t, exe, 0, output, "")

		ch, err := agent.Run(context.Background(), "test prompt", claude.RunOptions{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var events []claude.Event
		for ev := range ch {
			events = append(events, ev)
		}

		if len(events) != 2 {
			t.Fatalf("expected 2 events, got %d", len(events))
		}
		if events[0].Type != claude.EventToolUse {
			t.Errorf("event[0] type: expected %q, got %q", claude.EventToolUse, events[0].Type)
		}
		if events[0].ToolName != "read_file" {
			t.Errorf("event[0] tool name: expected %q, got %q", "read_file", events[0].ToolName)
		}
		if events[1].Type != claude.EventResult {
			t.Errorf("event[1] type: expected %q, got %q", claude.EventResult, events[1].Type)
		}
		if events[1].CostUSD != 0.10 {
			t.Errorf("event[1] cost: expected 0.10, got %.2f", events[1].CostUSD)
		}
	})

	t.Run("non-zero exit sends error event", func(t *testing.T) {
		output := `{"type":"result","cost_usd":0.05,"duration_ms":1000}`
		agent := setUpFakeClaude(t, exe, 1, output, "")

		ch, err := agent.Run(context.Background(), "test", claude.RunOptions{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var events []claude.Event
		for ev := range ch {
			events = append(events, ev)
		}

		// Should have result event + error event from non-zero exit
		if len(events) < 2 {
			t.Fatalf("expected at least 2 events, got %d", len(events))
		}
		last := events[len(events)-1]
		if last.Type != claude.EventError {
			t.Errorf("last event type: expected %q, got %q", claude.EventError, last.Type)
		}
	})

	t.Run("non-zero exit includes stderr in error", func(t *testing.T) {
		agent := setUpFakeClaude(t, exe, 1, "", "API rate limit exceeded")

		ch, err := agent.Run(context.Background(), "test", claude.RunOptions{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var events []claude.Event
		for ev := range ch {
			events = append(events, ev)
		}

		if len(events) == 0 {
			t.Fatal("expected at least 1 event")
		}
		last := events[len(events)-1]
		if last.Type != claude.EventError {
			t.Fatalf("last event type: expected %q, got %q", claude.EventError, last.Type)
		}
		if !strings.Contains(last.Error, "API rate limit exceeded") {
			t.Errorf("error should contain stderr text, got: %s", last.Error)
		}
	})

	t.Run("non-zero exit without stderr omits detail", func(t *testing.T) {
		agent := setUpFakeClaude(t, exe, 1, "", "")

		ch, err := agent.Run(context.Background(), "test", claude.RunOptions{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var events []claude.Event
		for ev := range ch {
			events = append(events, ev)
		}

		if len(events) == 0 {
			t.Fatal("expected at least 1 event")
		}
		last := events[len(events)-1]
		if last.Type != claude.EventError {
			t.Fatalf("last event type: expected %q, got %q", claude.EventError, last.Type)
		}
		// Should just have the exit status, no trailing ": "
		if strings.HasSuffix(last.Error, ": ") {
			t.Errorf("error should not have trailing colon-space when stderr is empty: %s", last.Error)
		}
	})

	t.Run("context cancellation stops process", func(t *testing.T) {
		t.Setenv("_FAKE_CLAUDE", "1")
		t.Setenv("_FAKE_CLAUDE_SLEEP", "1")
		agent := &ClaudeAgent{Executable: exe}

		ctx, cancel := context.WithCancel(context.Background())

		ch, err := agent.Run(ctx, "test", claude.RunOptions{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		cancel()

		// Drain channel — should close without hanging
		for range ch {
		}
	})

	t.Run("Dir option sets working directory", func(t *testing.T) {
		workDir := t.TempDir()
		agent := setUpFakeClaude(t, exe, 0, "", "")

		ch, err := agent.Run(context.Background(), "test", claude.RunOptions{Dir: workDir})
		if err != nil {
			t.Fatalf("unexpected error with Dir set: %v", err)
		}
		for range ch {
		}
	})

	t.Run("invalid executable returns error", func(t *testing.T) {
		agent := &ClaudeAgent{Executable: "/nonexistent/binary"}
		_, err := agent.Run(context.Background(), "test", claude.RunOptions{})
		if err == nil {
			t.Fatal("expected error for invalid executable")
		}
	})

	t.Run("empty executable defaults to claude binary name", func(t *testing.T) {
		// Clear PATH so "claude" cannot be found — Start() fails with a
		// start error, confirming the exe = "claude" default was reached.
		t.Setenv("PATH", "")
		agent := &ClaudeAgent{} // Executable is ""
		_, err := agent.Run(context.Background(), "test", claude.RunOptions{})
		if err == nil {
			t.Fatal("expected error when Executable is empty and claude not in PATH")
		}
		if !strings.Contains(err.Error(), "claude agent: start:") {
			t.Errorf("error should mention claude agent start, got: %v", err)
		}
	})
	t.Run("live steer writes prompt and messages to stdin", func(t *testing.T) {
		dir := t.TempDir()
		stdinFile := filepath.Join(dir, "stdin.txt")
		output := `{"type":"result","cost_usd":0.01,"duration_ms":100}`
		agent := setUpFakeClaude(t, exe, 0, output, "")
		t.Setenv("_FAKE_CLAUDE_STDIN_FILE", stdinFile)

		steerCh := make(chan string, 1)
		ch, err := agent.Run(context.Background(), "the iteration prompt", claude.RunOptions{Steer: steerCh})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		steerCh <- "prefer the Edit tool"
		// Closing the channel makes the writer close stdin, letting the fake
		// process reach EOF, emit its result, and exit.
		close(steerCh)
		for range ch {
		}

		data, err := os.ReadFile(stdinFile)
		if err != nil {
			t.Fatalf("read captured stdin: %v", err)
		}
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		if len(lines) != 2 {
			t.Fatalf("expected 2 stdin JSON lines (prompt + steer), got %d: %q", len(lines), data)
		}
		for i, want := range []string{"the iteration prompt", "prefer the Edit tool"} {
			var msg struct {
				Type    string `json:"type"`
				Message struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"message"`
			}
			if err := json.Unmarshal([]byte(lines[i]), &msg); err != nil {
				t.Fatalf("stdin line %d is not valid JSON: %v", i, err)
			}
			if msg.Type != "user" || msg.Message.Role != "user" {
				t.Errorf("stdin line %d: expected user message, got type=%q role=%q", i, msg.Type, msg.Message.Role)
			}
			if msg.Message.Content != want {
				t.Errorf("stdin line %d: content = %q, want %q", i, msg.Message.Content, want)
			}
		}
	})
}

// Verify ClaudeAgent satisfies claude.Agent at compile time.
var _ claude.Agent = (*ClaudeAgent)(nil)

// setUpFakeClaude configures the test binary (exe) as a fake Claude subprocess
// via env vars. Returns a ClaudeAgent pointing at that binary.
// Env vars are restored automatically by t.Setenv cleanup.
func setUpFakeClaude(t *testing.T, exe string, exitCode int, stdout, stderr string) *ClaudeAgent {
	t.Helper()
	dir := t.TempDir()
	stdoutFile := filepath.Join(dir, "stdout.txt")
	if err := os.WriteFile(stdoutFile, []byte(stdout), 0644); err != nil {
		t.Fatalf("write stdout file: %v", err)
	}
	t.Setenv("_FAKE_CLAUDE", "1")
	t.Setenv("_FAKE_CLAUDE_STDOUT_FILE", stdoutFile)
	if exitCode != 0 {
		t.Setenv("_FAKE_CLAUDE_EXIT", fmt.Sprintf("%d", exitCode))
	}
	if stderr != "" {
		t.Setenv("_FAKE_CLAUDE_STDERR", stderr)
	}
	return &ClaudeAgent{Executable: exe}
}

func containsArg(args []string, target string) bool {
	for _, a := range args {
		if a == target {
			return true
		}
	}
	return false
}
