package main

import (
	"strings"
	"testing"
	"time"

	"github.com/LISSConsulting/RalphSpec/internal/loop"
)

// TestLineFormatter_PlainMode covers the no-color path, migrating the
// former TestFormatLogLine tests and verifying no ANSI escapes are present.
func TestLineFormatter_PlainMode(t *testing.T) {
	ts := time.Date(2026, 2, 23, 14, 23, 1, 0, time.UTC)
	f := lineFormatter{color: false}

	tests := []struct {
		name  string
		entry loop.LogEntry
		want  string
	}{
		{
			name: "info entry — timestamp and message",
			entry: loop.LogEntry{
				Kind:      loop.LogInfo,
				Timestamp: ts,
				Message:   "starting iteration 3",
			},
			want: "[14:23:01]  starting iteration 3",
		},
		{
			name: "tool use entry — aligned tool name",
			entry: loop.LogEntry{
				Kind:      loop.LogToolUse,
				Timestamp: ts,
				ToolName:  "Edit",
				ToolInput: `C:\Temp\bob.txt`,
			},
			want: `[14:23:01]  Edit       C:\Temp\bob.txt`,
		},
		{
			name: "regent entry — shield prefix",
			entry: loop.LogEntry{
				Kind:      loop.LogRegent,
				Timestamp: ts,
				Message:   "Ralph exited (exit 1) — retrying in 30s",
			},
			want: "[14:23:01]  🛡️  Regent: Ralph exited (exit 1) — retrying in 30s",
		},
		{
			name: "error entry — no special prefix",
			entry: loop.LogEntry{
				Kind:      loop.LogError,
				Timestamp: ts,
				Message:   "claude exited with error",
			},
			want: "[14:23:01]  claude exited with error",
		},
		{
			name: "agent entry — agent prefix included",
			entry: loop.LogEntry{
				Kind:      loop.LogInfo,
				Timestamp: ts,
				Agent:     "codex",
				Message:   "starting iteration 3",
			},
			want: "[14:23:01]  [codex] starting iteration 3",
		},
		{
			name: "regent entry — includes agent prefix when present",
			entry: loop.LogEntry{
				Kind:      loop.LogRegent,
				Timestamp: ts,
				Agent:     "codex",
				Message:   "retrying",
			},
			want: "[14:23:01]  🛡️  Regent: [codex] retrying",
		},
		{
			name: "git push entry — no special prefix",
			entry: loop.LogEntry{
				Kind:      loop.LogGitPush,
				Timestamp: ts,
				Message:   "⬇ pushed to origin/main",
			},
			want: "[14:23:01]  ⬇ pushed to origin/main",
		},
		{
			name: "done entry — no special prefix",
			entry: loop.LogEntry{
				Kind:      loop.LogDone,
				Timestamp: ts,
				Message:   "loop finished (5 iterations, $1.42)",
			},
			want: "[14:23:01]  loop finished (5 iterations, $1.42)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := f.format(tt.entry)
			if got != tt.want {
				t.Errorf("format() = %q, want %q", got, tt.want)
			}
			if strings.Contains(got, "\x1b[") {
				t.Errorf("plain mode should not contain ANSI escapes: %q", got)
			}
		})
	}
}

func TestLineFormatter_PlainMode_ToolUseAlignment(t *testing.T) {
	ts := time.Date(2026, 2, 23, 14, 23, 1, 0, time.UTC)
	f := lineFormatter{color: false}

	tests := []struct {
		name  string
		entry loop.LogEntry
		want  string
	}{
		{name: "PowerShell", entry: loop.LogEntry{Kind: loop.LogToolUse, Timestamp: ts, ToolName: "PowerShell", ToolInput: "Get-Item ."}, want: "[14:23:01]  PowerShell Get-Item ."},
		{name: "Edit", entry: loop.LogEntry{Kind: loop.LogToolUse, Timestamp: ts, ToolName: "Edit", ToolInput: `C:\Temp\bob.txt`}, want: `[14:23:01]  Edit       C:\Temp\bob.txt`},
		{name: "Write", entry: loop.LogEntry{Kind: loop.LogToolUse, Timestamp: ts, ToolName: "Write", ToolInput: `C:\Windows\system32\drivers\etc\hosts`}, want: `[14:23:01]  Write      C:\Windows\system32\drivers\etc\hosts`},
		{name: "Grep", entry: loop.LogEntry{Kind: loop.LogToolUse, Timestamp: ts, ToolName: "Grep", ToolInput: "stop*"}, want: "[14:23:01]  Grep       stop*"},
		{name: "Cmd", entry: loop.LogEntry{Kind: loop.LogToolUse, Timestamp: ts, ToolName: "Cmd", ToolInput: "dir"}, want: "[14:23:01]  Cmd        dir"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := f.format(tt.entry); got != tt.want {
				t.Fatalf("format() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestLineFormatter_ColorMode_ReturnsNonEmpty verifies the color path produces
// a non-empty string containing the timestamp for each LogKind.
// ANSI escape presence is not asserted because lipgloss strips color on
// non-TTY output (which is always the case in test environments).
func TestLineFormatter_ColorMode_ReturnsNonEmpty(t *testing.T) {
	ts := time.Date(2026, 2, 23, 14, 23, 1, 0, time.UTC)
	f := lineFormatter{color: true}

	entries := []struct {
		name  string
		entry loop.LogEntry
	}{
		{"info", loop.LogEntry{Kind: loop.LogInfo, Timestamp: ts, Message: "hello"}},
		{"error", loop.LogEntry{Kind: loop.LogError, Timestamp: ts, Message: "oops"}},
		{"regent", loop.LogEntry{Kind: loop.LogRegent, Timestamp: ts, Message: "retrying"}},
		{"done", loop.LogEntry{Kind: loop.LogDone, Timestamp: ts, Message: "finished"}},
		{"tooluse", loop.LogEntry{Kind: loop.LogToolUse, Timestamp: ts, ToolName: "Read", ToolInput: "main.go"}},
		{"iterstart", loop.LogEntry{Kind: loop.LogIterStart, Timestamp: ts, Iteration: 1}},
		{"itercomplete", loop.LogEntry{Kind: loop.LogIterComplete, Timestamp: ts, Iteration: 1, CostUSD: 0.05, Duration: 10.5}},
		{"stopped", loop.LogEntry{Kind: loop.LogStopped, Timestamp: ts, Message: "stopped"}},
		{"gitpull", loop.LogEntry{Kind: loop.LogGitPull, Timestamp: ts, Message: "pulled"}},
		{"gitpush", loop.LogEntry{Kind: loop.LogGitPush, Timestamp: ts, Message: "pushed"}},
	}

	for _, tt := range entries {
		t.Run(tt.name, func(t *testing.T) {
			got := f.format(tt.entry)
			if got == "" {
				t.Errorf("color mode format() should not return empty string for %s", tt.name)
			}
			if !strings.Contains(got, "14:23:01") {
				t.Errorf("format() should contain timestamp, got: %q", got)
			}
		})
	}
}

// TestLineFormatter_PlainMode_NoANSI verifies that no ANSI escape codes are
// emitted across all LogKind values in plain (no-color) mode.
func TestLineFormatter_PlainMode_NoANSI(t *testing.T) {
	ts := time.Date(2026, 2, 23, 14, 23, 1, 0, time.UTC)
	f := lineFormatter{color: false}

	entries := []loop.LogEntry{
		{Kind: loop.LogInfo, Timestamp: ts, Message: "info"},
		{Kind: loop.LogError, Timestamp: ts, Message: "error"},
		{Kind: loop.LogRegent, Timestamp: ts, Message: "regent"},
		{Kind: loop.LogDone, Timestamp: ts, Message: "done"},
		{Kind: loop.LogToolUse, Timestamp: ts, ToolName: "Read", ToolInput: "f.go"},
		{Kind: loop.LogIterStart, Timestamp: ts, Iteration: 1},
		{Kind: loop.LogIterComplete, Timestamp: ts, Iteration: 1},
		{Kind: loop.LogStopped, Timestamp: ts, Message: "stopped"},
		{Kind: loop.LogGitPull, Timestamp: ts, Message: "pulled"},
		{Kind: loop.LogGitPush, Timestamp: ts, Message: "pushed"},
	}

	for _, entry := range entries {
		got := f.format(entry)
		if strings.Contains(got, "\x1b[") {
			t.Errorf("plain mode contains ANSI for kind %v: %q", entry.Kind, got)
		}
	}
}
