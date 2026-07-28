package codex

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/LISSConsulting/RalphSpec/internal/claude"
)

func init() {
	if os.Getenv("_FAKE_CODEX") != "1" {
		return
	}
	if f := os.Getenv("_FAKE_CODEX_STDOUT_FILE"); f != "" {
		if data, err := os.ReadFile(f); err == nil {
			_, _ = os.Stdout.Write(data)
		}
	}
	if s := os.Getenv("_FAKE_CODEX_STDERR"); s != "" {
		_, _ = fmt.Fprint(os.Stderr, s)
	}
	if os.Getenv("_FAKE_CODEX_SLEEP") == "1" {
		time.Sleep(time.Minute)
	}
	code := 0
	if s := os.Getenv("_FAKE_CODEX_EXIT"); s != "" {
		_, _ = fmt.Sscan(s, &code)
	}
	os.Exit(code)
}

func TestNewAgent(t *testing.T) {
	agent := NewAgent()
	if agent.Executable != "codex" {
		t.Errorf("expected executable %q, got %q", "codex", agent.Executable)
	}
}

func TestBuildArgs(t *testing.T) {
	agent := &Agent{}
	args := agent.buildArgs("test prompt", claude.RunOptions{Dir: "/tmp/project", Model: "gpt-5"})

	for _, want := range []string{"exec", "--json", "--dangerously-bypass-approvals-and-sandbox", "--cd", "/tmp/project", "--model", "gpt-5", "test prompt"} {
		if !containsArg(args, want) {
			t.Fatalf("args %v missing %q", args, want)
		}
	}
	if containsArg(args, "--full-auto") {
		t.Fatalf("args %v should not use --full-auto because it still applies Codex sandbox constraints", args)
	}

	// Live steer must not alter the Codex command line — steering is a
	// best-effort stdin write, not a CLI flag.
	steerArgs := agent.buildArgs("test prompt", claude.RunOptions{Dir: "/tmp/project", Model: "gpt-5", Steer: make(chan string)})
	if strings.Join(args, " ") != strings.Join(steerArgs, " ") {
		t.Errorf("steer must not change codex args: %v vs %v", args, steerArgs)
	}
}

func TestAgentRun(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}

	t.Run("streams events from subprocess", func(t *testing.T) {
		output := strings.Join([]string{
			`{"type":"message","text":"thinking"}`,
			`{"type":"tool_call","tool_name":"read_file","tool_input":{"path":"main.go"}}`,
			`{"type":"result","cost_usd":0.10,"duration_ms":2500,"subtype":"success"}`,
		}, "\n")
		agent := setUpFakeCodex(t, exe, 0, output, "")

		ch, err := agent.Run(context.Background(), "test prompt", claude.RunOptions{Dir: t.TempDir()})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var events []claude.Event
		for ev := range ch {
			events = append(events, ev)
		}

		if len(events) != 3 {
			t.Fatalf("expected 3 events, got %d", len(events))
		}
		if events[0].Type != claude.EventText {
			t.Fatalf("expected text event, got %#v", events[0])
		}
		if events[1].Type != claude.EventToolUse || events[1].ToolName != "read_file" {
			t.Fatalf("unexpected tool event: %#v", events[1])
		}
		if events[2].Type != claude.EventResult || events[2].Subtype != "success" {
			t.Fatalf("unexpected result event: %#v", events[2])
		}
	})

	t.Run("non-zero exit includes stderr in error", func(t *testing.T) {
		agent := setUpFakeCodex(t, exe, 1, "", "login required")

		ch, err := agent.Run(context.Background(), "test", claude.RunOptions{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var last claude.Event
		for ev := range ch {
			last = ev
		}
		if last.Type != claude.EventError {
			t.Fatalf("expected final error event, got %#v", last)
		}
		if !strings.Contains(last.Error, "login required") {
			t.Fatalf("expected stderr in error, got %q", last.Error)
		}
	})

	t.Run("invalid executable returns start error", func(t *testing.T) {
		agent := &Agent{Executable: filepath.Join(t.TempDir(), "missing-codex")}
		_, err := agent.Run(context.Background(), "test", claude.RunOptions{})
		if err == nil {
			t.Fatal("expected error for invalid executable")
		}
		if !strings.Contains(err.Error(), "codex agent: start:") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("steer channel drains without deadlock", func(t *testing.T) {
		output := `{"type":"result","cost_usd":0.01,"duration_ms":50,"subtype":"success"}`
		agent := setUpFakeCodex(t, exe, 0, output, "")

		steerCh := make(chan string, 4)
		steerCh <- "keep going"
		ch, err := agent.Run(context.Background(), "test", claude.RunOptions{Steer: steerCh})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for range ch {
		}
		close(steerCh)
	})
}

func TestCheckAvailable(t *testing.T) {
	t.Run("missing binary returns actionable error", func(t *testing.T) {
		t.Setenv("PATH", "")
		err := CheckAvailable("")
		if err == nil {
			t.Fatal("expected missing-binary error")
		}
		if !strings.Contains(err.Error(), "codex executable not found on PATH") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("present binary succeeds", func(t *testing.T) {
		dir := t.TempDir()
		name := writeFakeExecutable(t, dir, "codex")
		prependPath(t, dir)
		if err := CheckAvailable(name); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func setUpFakeCodex(t *testing.T, exe string, exitCode int, stdout, stderr string) *Agent {
	t.Helper()
	dir := t.TempDir()
	stdoutFile := filepath.Join(dir, "stdout.txt")
	if err := os.WriteFile(stdoutFile, []byte(stdout), 0644); err != nil {
		t.Fatalf("write stdout file: %v", err)
	}
	t.Setenv("_FAKE_CODEX", "1")
	t.Setenv("_FAKE_CODEX_STDOUT_FILE", stdoutFile)
	if exitCode != 0 {
		t.Setenv("_FAKE_CODEX_EXIT", fmt.Sprintf("%d", exitCode))
	}
	if stderr != "" {
		t.Setenv("_FAKE_CODEX_STDERR", stderr)
	}
	return &Agent{Executable: exe}
}

func writeFakeExecutable(t *testing.T, dir, base string) string {
	t.Helper()
	name := base
	content := "#!/bin/sh\nexit 0\n"
	if runtime.GOOS == "windows" {
		name += ".cmd"
		content = "@echo off\r\nexit /b 0\r\n"
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		t.Fatalf("write fake executable: %v", err)
	}
	return name
}

func prependPath(t *testing.T, dir string) {
	t.Helper()
	oldPath := os.Getenv("PATH")
	sep := ":"
	if runtime.GOOS == "windows" {
		sep = ";"
	}
	if oldPath == "" {
		t.Setenv("PATH", dir)
		return
	}
	t.Setenv("PATH", dir+sep+oldPath)
}

func containsArg(args []string, target string) bool {
	for _, a := range args {
		if a == target {
			return true
		}
	}
	return false
}

var _ claude.Agent = (*Agent)(nil)
