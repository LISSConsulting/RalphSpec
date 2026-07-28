package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/LISSConsulting/RalphSpec/internal/config"
	"github.com/LISSConsulting/RalphSpec/internal/git"
	"github.com/LISSConsulting/RalphSpec/internal/loop"
	"github.com/LISSConsulting/RalphSpec/internal/regent"
)

// TestFormatLogLine is superseded by TestLineFormatter_PlainMode in format_test.go.

func TestClassifyResult(t *testing.T) {
	now := time.Now()
	past := now.Add(-10 * time.Minute)

	tests := []struct {
		name  string
		state regent.State
		want  statusResult
	}{
		{
			name:  "no state — zero PID and zero iteration",
			state: regent.State{},
			want:  statusNoState,
		},
		{
			name: "running — started but not finished",
			state: regent.State{
				RalphPID:  123,
				Iteration: 3,
				StartedAt: past,
			},
			want: statusRunning,
		},
		{
			name: "stale unfinished state becomes fail",
			state: regent.State{
				RalphPID:  123,
				Iteration: 3,
				StartedAt: past,
			},
			want: statusFail,
		},
		{
			name: "pass — finished with Passed true",
			state: regent.State{
				RalphPID:   123,
				Iteration:  5,
				StartedAt:  past,
				FinishedAt: now,
				Passed:     true,
			},
			want: statusPass,
		},
		{
			name: "fail with consecutive errors",
			state: regent.State{
				RalphPID:        123,
				Iteration:       2,
				StartedAt:       past,
				FinishedAt:      now,
				ConsecutiveErrs: 3,
			},
			want: statusFailWithErrors,
		},
		{
			name: "plain fail — finished but not passed, no consecutive errors",
			state: regent.State{
				RalphPID:   123,
				Iteration:  1,
				StartedAt:  past,
				FinishedAt: now,
			},
			want: statusFail,
		},
		{
			name: "passed wins over consecutive errors",
			state: regent.State{
				RalphPID:        123,
				Iteration:       4,
				StartedAt:       past,
				FinishedAt:      now,
				Passed:          true,
				ConsecutiveErrs: 2,
			},
			want: statusPass,
		},
		{
			name: "running wins over passed",
			state: regent.State{
				RalphPID:  123,
				Iteration: 2,
				StartedAt: past,
				Passed:    true,
			},
			want: statusRunning,
		},
		{
			name: "non-zero PID with zero iteration is no-state",
			state: regent.State{
				RalphPID: 123,
			},
			want: statusNoState,
		},
		{
			name: "zero PID with non-zero iteration is no-state",
			state: regent.State{
				Iteration: 5,
			},
			want: statusNoState,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pidRunning := func(pid int) bool {
				return (tt.name == "running — started but not finished" || tt.name == "running wins over passed") && pid == 123
			}
			got := classifyResultWithPIDCheck(tt.state, pidRunning)
			if got != tt.want {
				t.Errorf("classifyResult() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestFormatStatus(t *testing.T) {
	now := time.Date(2026, 2, 23, 15, 0, 0, 0, time.UTC)
	started := now.Add(-10 * time.Minute)
	finished := now.Add(-1 * time.Minute)
	lastOutput := now.Add(-30 * time.Second)

	tests := []struct {
		name     string
		state    regent.State
		contains []string
		excludes []string
	}{
		{
			name:     "no state — empty state shows prompt",
			state:    regent.State{},
			contains: []string{"No state found"},
			excludes: []string{"Ralph Status"},
		},
		{
			name: "running — shows elapsed duration and last output",
			state: regent.State{
				RalphPID:     123,
				Agent:        "codex",
				Iteration:    3,
				Branch:       "feat/test",
				Mode:         "build",
				LastCommit:   "abc1234",
				TotalCostUSD: 0.42,
				StartedAt:    started,
				LastOutputAt: lastOutput,
			},
			contains: []string{
				"Ralph Status",
				"Branch:",
				"feat/test",
				"Agent:",
				"codex",
				"Mode:",
				"build",
				"Last commit:",
				"abc1234",
				"Iteration:",
				"3",
				"$0.42",
				"10m0s (running)",
				"30s ago",
				"Result:",
				"running",
			},
		},
		{
			name: "stale unfinished state shows fail not running",
			state: regent.State{
				RalphPID:     999999,
				Iteration:    3,
				Branch:       "feat/test",
				Mode:         "build",
				TotalCostUSD: 0.42,
				StartedAt:    started,
				LastOutputAt: lastOutput,
			},
			contains: []string{
				"Ralph Status",
				"Duration:",
				"10m0s",
				"Result:",
				"fail",
			},
			excludes: []string{"(running)", "Last output:", "running"},
		},
		{
			name: "pass — shows duration and pass result",
			state: regent.State{
				RalphPID:     123,
				Iteration:    5,
				Branch:       "main",
				TotalCostUSD: 1.50,
				StartedAt:    started,
				FinishedAt:   finished,
				Passed:       true,
			},
			contains: []string{
				"Ralph Status",
				"main",
				"Iteration:",
				"5",
				"$1.50",
				"9m0s",
				"Result:",
				"pass",
			},
			excludes: []string{"running", "fail", "Last output:"},
		},
		{
			name: "fail with consecutive errors — shows error count",
			state: regent.State{
				RalphPID:        123,
				Iteration:       2,
				TotalCostUSD:    0.30,
				StartedAt:       started,
				FinishedAt:      finished,
				ConsecutiveErrs: 3,
			},
			contains: []string{
				"Ralph Status",
				"fail (3 consecutive errors)",
			},
			excludes: []string{"pass", "running"},
		},
		{
			name: "plain fail — finished but not passed, no errors",
			state: regent.State{
				RalphPID:   123,
				Iteration:  1,
				StartedAt:  started,
				FinishedAt: finished,
			},
			contains: []string{
				"Ralph Status",
				"Result:",
				"fail",
			},
			excludes: []string{"pass", "running", "consecutive"},
		},
		{
			name: "optional fields omitted when empty",
			state: regent.State{
				RalphPID:   123,
				Iteration:  1,
				StartedAt:  started,
				FinishedAt: finished,
			},
			excludes: []string{"Branch:", "Mode:", "Last commit:", "Last output:"},
		},
		{
			name: "running without last output — omits last output line",
			state: regent.State{
				RalphPID:  123,
				Iteration: 1,
				StartedAt: started,
			},
			contains: []string{"running"},
			excludes: []string{"Last output:"},
		},
		{
			name: "zero cost displays as $0.00",
			state: regent.State{
				RalphPID:   123,
				Iteration:  1,
				StartedAt:  started,
				FinishedAt: finished,
			},
			contains: []string{"$0.00"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pidRunning := func(pid int) bool {
				return (tt.name == "running — shows elapsed duration and last output" || tt.name == "running without last output — omits last output line") && pid == 123
			}
			got := formatStatusWithPIDCheck(tt.state, now, pidRunning)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output should contain %q\ngot:\n%s", want, got)
				}
			}
			for _, exclude := range tt.excludes {
				if strings.Contains(got, exclude) {
					t.Errorf("output should NOT contain %q\ngot:\n%s", exclude, got)
				}
			}
		})
	}
}

func TestResolveAgent(t *testing.T) {
	cfg := &config.Config{Agent: config.AgentConfig{Type: config.AgentCodex}}

	got, err := resolveAgent(cfg, config.AgentClaude)
	if err != nil {
		t.Fatalf("resolveAgent override error: %v", err)
	}
	if got != config.AgentClaude {
		t.Fatalf("override got %q, want %q", got, config.AgentClaude)
	}

	got, err = resolveAgent(&config.Config{}, "")
	if err != nil {
		t.Fatalf("resolveAgent default error: %v", err)
	}
	if got != config.AgentClaude {
		t.Fatalf("default got %q, want %q", got, config.AgentClaude)
	}

	if _, err := resolveAgent(&config.Config{}, "nope"); err == nil {
		t.Fatal("expected unknown-agent error")
	}
}

func TestHarnessExecutable(t *testing.T) {
	cfg := config.Defaults()
	if got := harnessExecutable(&cfg, config.AgentClaude); got != "claude" {
		t.Errorf("default claude harness = %q", got)
	}
	if got := harnessExecutable(&cfg, config.AgentCodex); got != "codex" {
		t.Errorf("default codex harness = %q", got)
	}

	cfg.Harness.Claude = "claude-kimi"
	cfg.Harness.Codex = "codex-minimax"
	if got := harnessExecutable(&cfg, config.AgentClaude); got != "claude-kimi" {
		t.Errorf("override claude harness = %q", got)
	}
	if got := harnessExecutable(&cfg, config.AgentCodex); got != "codex-minimax" {
		t.Errorf("override codex harness = %q", got)
	}
}

func TestBuildAgent_HarnessOverride(t *testing.T) {
	cfg := config.Defaults()
	cfg.Harness.Claude = "claude-kimi"

	agent, err := buildAgent(&cfg, config.AgentClaude)
	if err != nil {
		t.Fatalf("buildAgent: %v", err)
	}
	ca, ok := agent.(*loop.ClaudeAgent)
	if !ok {
		t.Fatalf("expected *loop.ClaudeAgent, got %T", agent)
	}
	if ca.Executable != "claude-kimi" {
		t.Errorf("Executable = %q, want claude-kimi", ca.Executable)
	}
}

func TestValidateAgentFlow(t *testing.T) {
	if err := validateAgentFlow(config.AgentCodex, true, false); err != nil {
		t.Fatalf("codex worktree should be supported, got %v", err)
	}
	if err := validateAgentFlow(config.AgentCodex, false, true); err != nil {
		t.Fatalf("codex dashboard should be supported, got %v", err)
	}
	if err := validateAgentFlow(config.AgentClaude, true, true); err != nil {
		t.Fatalf("claude should remain supported, got %v", err)
	}
}

func TestSetupLoop_AgentPrecedence(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeExecTestFile(t, dir, "ralph.toml", `[agent]
type = "codex"

[build]
prompt_file = "BUILD.md"
max_iterations = 1

[git]
auto_pull_rebase = false
auto_push = false

[regent]
enabled = false
`)
	writeFakeCodexRunner(t, dir, `{"type":"result","cost_usd":0.01,"duration_ms":1,"subtype":"success"}`, "", 0)
	prependCLIPath(t, dir)

	setup, err := setupLoop(true, false, true, "")
	if err != nil {
		t.Fatalf("setupLoop default agent: %v", err)
	}
	defer setup.cancel()
	defer setup.cleanup()
	if setup.agentType != config.AgentCodex {
		t.Fatalf("default setup agent = %q, want %q", setup.agentType, config.AgentCodex)
	}

	setup2, err := setupLoop(true, false, true, config.AgentClaude)
	if err != nil {
		t.Fatalf("setupLoop override agent: %v", err)
	}
	defer setup2.cancel()
	defer setup2.cleanup()
	if setup2.agentType != config.AgentClaude {
		t.Fatalf("override setup agent = %q, want %q", setup2.agentType, config.AgentClaude)
	}
}

func TestExecuteLoop_CodexUnavailable(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeExecTestFile(t, dir, "ralph.toml", testConfigNoRegent())
	t.Setenv("PATH", "")

	err := executeLoop(loop.ModeBuild, 1, true, false, "", false, false, config.AgentCodex)
	if err == nil {
		t.Fatal("expected codex unavailable error")
	}
	if !strings.Contains(err.Error(), "codex agent unavailable") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "use --agent claude") {
		t.Fatalf("expected actionable guidance, got: %v", err)
	}
}

func TestExecuteLoop_CodexWorktreeAccepted(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeExecTestFile(t, dir, "ralph.toml", testConfigNoRegent())
	writeFakeCLIExecutable(t, dir, "codex")
	prependCLIPath(t, dir)

	err := executeLoop(loop.ModeBuild, 1, true, false, "", false, true, config.AgentCodex)
	if err == nil {
		t.Fatal("expected worktree setup error without worktrunk, not codex rejection")
	}
	if strings.Contains(err.Error(), "codex agent unsupported for worktree mode") {
		t.Fatalf("unexpected codex rejection: %v", err)
	}
}

func TestExecuteLoop_CodexBuildSuccess(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	t.Chdir(dir)
	writeExecTestFile(t, dir, "ralph.toml", testConfigNoRegent())
	writeExecTestFile(t, dir, "BUILD.md", "# Build\n")
	writeFakeCodexRunner(t, dir, strings.Join([]string{
		`{"type":"message","text":"building"}`,
		`{"type":"result","cost_usd":0.10,"duration_ms":25,"subtype":"success"}`,
	}, "\n"), "", 0)
	prependCLIPath(t, dir)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err := executeLoop(loop.ModeBuild, 1, true, false, "", true, false, config.AgentCodex)
	_ = w.Close()
	os.Stdout = oldStdout
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	if err != nil {
		t.Fatalf("expected successful codex build run, got %v", err)
	}
	if !strings.Contains(buf.String(), "[codex]") {
		t.Fatalf("expected live output to contain codex prefix, got %q", buf.String())
	}
	state, loadErr := regent.LoadState(dir)
	if loadErr != nil {
		t.Fatalf("LoadState: %v", loadErr)
	}
	if state.Agent != config.AgentCodex {
		t.Fatalf("state.Agent = %q, want %q", state.Agent, config.AgentCodex)
	}
}

func TestExecuteRun_CodexBuildSuccess(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	t.Chdir(dir)
	writeExecTestFile(t, dir, "ralph.toml", `[agent]
type = "codex"
`+testConfigNoRegent())
	writeExecTestFile(t, dir, "CHRONICLE.md", "# Chronicle\n\nready\n")
	writeExecTestFile(t, dir, "BUILD.md", "# Build\n")
	writeFakeCodexRunner(t, dir, `{"type":"result","cost_usd":0.10,"duration_ms":25,"subtype":"success"}`, "", 0)
	prependCLIPath(t, dir)

	err := executeRun(1, true, false, "", true, false, "")
	if err != nil {
		t.Fatalf("expected successful codex run, got %v", err)
	}
	state, loadErr := regent.LoadState(dir)
	if loadErr != nil {
		t.Fatalf("LoadState: %v", loadErr)
	}
	if state.Agent != config.AgentCodex {
		t.Fatalf("state.Agent = %q, want %q", state.Agent, config.AgentCodex)
	}
}

func TestExecuteLoop_DefaultClaudeRegression(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeExecTestFile(t, dir, "ralph.toml", testConfigNoRegent())

	setup, err := setupLoop(true, false, true, "")
	if err != nil {
		t.Fatalf("setupLoop: %v", err)
	}
	defer setup.cancel()
	defer setup.cleanup()
	if setup.agentType != config.AgentClaude {
		t.Fatalf("default agent = %q, want %q", setup.agentType, config.AgentClaude)
	}
}

// ---- Integration tests for executeLoop and executeRun ----
//
// These tests exercise the full orchestration path through config loading,
// wiring, and loop setup. They fail before any claude invocation
// (config not found, invalid config, or prompt file missing), so they
// work without a real Claude binary installed.

// testConfigNoRegent returns a minimal ralph.toml with regent disabled and
// git ops turned off so tests don't attempt network operations.
func testConfigNoRegent() string {
	return `[build]
prompt_file = "BUILD.md"
max_iterations = 1

[git]
auto_pull_rebase = false
auto_push = false

[regent]
enabled = false
`
}

// testConfigWithRegent returns a ralph.toml with regent enabled but
// max_retries=0 so it fails fast after one error without backoff.
func testConfigWithRegent() string {
	return `[build]
prompt_file = "BUILD.md"
max_iterations = 1

[git]
auto_pull_rebase = false
auto_push = false

[regent]
enabled = true
max_retries = 0
retry_backoff_seconds = 0
hang_timeout_seconds = 0
rollback_on_test_failure = false
`
}

// writeExecTestFile writes content to dir/name, creating parent directories.
func writeExecTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll %s: %v", name, err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile %s: %v", name, err)
	}
}

func writeFakeCLIExecutable(t *testing.T, dir, base string) string {
	t.Helper()
	name := base
	content := "#!/bin/sh\nexit 0\n"
	if runtime.GOOS == "windows" {
		name += ".cmd"
		content = "@echo off\r\nexit /b 0\r\n"
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		t.Fatalf("WriteFile %s: %v", name, err)
	}
	return name
}

func writeFakeWorktrunkExecutable(t *testing.T, dir string) {
	t.Helper()
	names := []string{"wt", "git-wt"}
	content := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo \"worktrunk 1.0.0\"; exit 0; fi\nexit 0\n"
	if runtime.GOOS == "windows" {
		names = []string{"git-wt.cmd", "wt.cmd"}
		content = "@echo off\r\nif \"%1\"==\"--version\" echo worktrunk 1.0.0\r\nexit /b 0\r\n"
	}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0755); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}
}

func writeFakeCodexRunner(t *testing.T, dir, stdout, stderr string, exitCode int) {
	t.Helper()
	stdoutPath := filepath.Join(dir, "codex-stdout.txt")
	if err := os.WriteFile(stdoutPath, []byte(stdout), 0644); err != nil {
		t.Fatalf("WriteFile codex stdout: %v", err)
	}
	stderrPath := filepath.Join(dir, "codex-stderr.txt")
	if err := os.WriteFile(stderrPath, []byte(stderr), 0644); err != nil {
		t.Fatalf("WriteFile codex stderr: %v", err)
	}
	if runtime.GOOS == "windows" {
		script := "@echo off\r\ntype \"%~dp0codex-stdout.txt\"\r\ntype \"%~dp0codex-stderr.txt\" 1>&2\r\nexit /b " + strconv.Itoa(exitCode) + "\r\n"
		if err := os.WriteFile(filepath.Join(dir, "codex.cmd"), []byte(script), 0755); err != nil {
			t.Fatalf("WriteFile codex.cmd: %v", err)
		}
		return
	}
	script := "#!/bin/sh\ncat \"$(dirname \"$0\")/codex-stdout.txt\"\nif [ -s \"$(dirname \"$0\")/codex-stderr.txt\" ]; then cat \"$(dirname \"$0\")/codex-stderr.txt\" >&2; fi\nexit " + strconv.Itoa(exitCode) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "codex"), []byte(script), 0755); err != nil {
		t.Fatalf("WriteFile codex: %v", err)
	}
}

func prependCLIPath(t *testing.T, dir string) {
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

func TestExecuteLoop_ConfigNotFound(t *testing.T) {
	// Isolated temp dir with no ralph.toml anywhere in its ancestor tree.
	t.Chdir(t.TempDir())

	err := executeLoop(loop.ModeBuild, 1, true, false, "", false, false, "")
	if err == nil {
		t.Fatal("expected error when ralph.toml not found")
	}
	if !strings.Contains(err.Error(), "ralph.toml") {
		t.Errorf("error should mention ralph.toml, got: %v", err)
	}
}

func TestExecuteLoop_ConfigInvalid(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	// Empty roam.prompt_file fails Validate().
	writeExecTestFile(t, dir, "ralph.toml", "[build]\nprompt_file = \"b.md\"\n[roam]\nprompt_file = \"\"\n")

	err := executeLoop(loop.ModeBuild, 1, true, false, "", false, false, "")
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "config validation") {
		t.Errorf("error should mention config validation, got: %v", err)
	}
}

func TestExecuteLoop_RegentDisabled_RoamPromptMissing(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	t.Chdir(dir)
	writeExecTestFile(t, dir, "ralph.toml", testConfigNoRegent())
	// ROAM.md intentionally absent — pre-flight fails reading it.

	err := executeLoop(loop.ModeBuild, 1, true, true, "", false, false, "")
	if err == nil {
		t.Fatal("expected error when prompt file missing")
	}
	if !strings.Contains(err.Error(), "ROAM.md") {
		t.Errorf("error should mention ROAM.md, got: %v", err)
	}
}

func TestExecuteLoop_RegentEnabled_RoamPromptMissing(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	t.Chdir(dir)
	writeExecTestFile(t, dir, "ralph.toml", testConfigWithRegent())
	// ROAM.md intentionally absent.
	// Pre-flight check returns an error before Regent is initialised.

	err := executeLoop(loop.ModeBuild, 1, true, true, "", false, false, "")
	if err == nil {
		t.Fatal("expected error when prompt file missing")
	}
	if !strings.Contains(err.Error(), "ROAM.md") {
		t.Errorf("error should mention ROAM.md, got: %v", err)
	}
}

func TestExecuteLoop_BuildMode_PromptMissing(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	t.Chdir(dir)
	writeExecTestFile(t, dir, "ralph.toml", testConfigNoRegent())
	// BUILD.md intentionally absent — covers default case in mode switch.

	err := executeLoop(loop.ModeBuild, 1, true, false, "", false, false, "")
	if err == nil {
		t.Fatal("expected error when build prompt file missing")
	}
	if !strings.Contains(err.Error(), "BUILD.md") {
		t.Errorf("error should mention BUILD.md, got: %v", err)
	}
}

func TestExecuteLoop_RegentDisabled_PromptExists_GitFails(t *testing.T) {
	// Prompt file present (pre-flight passes) but no git repo, so loop fails at
	// CurrentBranch(). Covers the noTUI/non-regent branch in executeLoop.
	dir := t.TempDir()
	// Deliberately no initGitRepo — git ops will fail.
	t.Chdir(dir)
	writeExecTestFile(t, dir, "ralph.toml", testConfigNoRegent())
	writeExecTestFile(t, dir, "BUILD.md", "# Build\n")

	err := executeLoop(loop.ModeBuild, 1, true, false, "", false, false, "")
	// Loop fails at git CurrentBranch — must be an error but not a prompt-file error.
	if err == nil {
		t.Fatal("expected error from git operations")
	}
	if strings.Contains(err.Error(), "prompt file") {
		t.Errorf("should not be a prompt-file error, got: %v", err)
	}
}

func TestExecuteLoop_RegentEnabled_PromptExists_GitFails(t *testing.T) {
	// Prompt file present (pre-flight passes) but no git repo, so loop fails at
	// CurrentBranch(). Regent gives up after 0 retries.
	// Covers the noTUI/regent branch in executeLoop.
	dir := t.TempDir()
	// Deliberately no initGitRepo — git ops will fail.
	t.Chdir(dir)
	writeExecTestFile(t, dir, "ralph.toml", testConfigWithRegent())
	writeExecTestFile(t, dir, "BUILD.md", "# Build\n")

	err := executeLoop(loop.ModeBuild, 1, true, false, "", false, false, "")
	// Regent gives up after 0 retries — must be an error.
	if err == nil {
		t.Fatal("expected error — Regent should give up after 0 retries")
	}
	if strings.Contains(err.Error(), "prompt file") {
		t.Errorf("should not be a prompt-file error, got: %v", err)
	}
}

// ---- executeRun integration tests ----

func TestExecuteRun_ConfigNotFound(t *testing.T) {
	t.Chdir(t.TempDir())

	err := executeRun(1, true, false, "", false, false, "")
	if err == nil {
		t.Fatal("expected error when ralph.toml not found")
	}
	if !strings.Contains(err.Error(), "ralph.toml") {
		t.Errorf("error should mention ralph.toml, got: %v", err)
	}
}

func TestExecuteRun_BuildPromptMissing(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	t.Chdir(dir)
	writeExecTestFile(t, dir, "ralph.toml", testConfigNoRegent())
	// BUILD.md absent → build loop fails reading it.

	err := executeRun(1, true, false, "", false, false, "")
	if err == nil {
		t.Fatal("expected error when build prompt file missing")
	}
	if !strings.Contains(err.Error(), "BUILD.md") {
		t.Errorf("error should mention BUILD.md, got: %v", err)
	}
}

func TestExecuteRun_WithChronicle_BuildPromptMissing(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	t.Chdir(dir)
	writeExecTestFile(t, dir, "ralph.toml", testConfigNoRegent())
	// Non-empty CHRONICLE.md should not affect prompt selection.
	writeExecTestFile(t, dir, "CHRONICLE.md", "# Plan\n\nSome content.\n")
	// BUILD.md absent → build loop fails reading it.

	err := executeRun(1, true, false, "", false, false, "")
	if err == nil {
		t.Fatal("expected error when build prompt file missing")
	}
	if !strings.Contains(err.Error(), "BUILD.md") {
		t.Errorf("error should mention BUILD.md, got: %v", err)
	}
}

func TestExecuteRun_ConfigInvalid(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	// Empty roam.prompt_file triggers Validate() error.
	writeExecTestFile(t, dir, "ralph.toml", "[build]\nprompt_file = \"b.md\"\n[roam]\nprompt_file = \"\"\n")

	err := executeRun(1, true, false, "", false, false, "")
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "config validation") {
		t.Errorf("error should mention config validation, got: %v", err)
	}
}

func TestExecuteRun_RegentEnabled_PromptMissing(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	t.Chdir(dir)
	writeExecTestFile(t, dir, "ralph.toml", testConfigWithRegent())
	// BUILD.md absent → build loop fails reading it.
	// Regent gives up after 0 retries and returns max-retries error.

	err := executeRun(1, true, false, "", false, false, "")
	if err == nil {
		t.Fatal("expected error — Regent should give up (max_retries=0)")
	}
}

func TestExecuteLoop_StoreUnavailable(t *testing.T) {
	// Create .ralph as a regular file so store.NewJSONL cannot create the logs
	// subdirectory (MkdirAll fails). The store failure is non-fatal — the loop
	// continues with sw=nil but fails at git CurrentBranch (no git repo).
	// This covers the fmt.Fprintf(stderr, "session log unavailable") branch.
	dir := t.TempDir()
	t.Chdir(dir)
	writeExecTestFile(t, dir, "ralph.toml", testConfigNoRegent())
	writeExecTestFile(t, dir, "BUILD.md", "# Build\n")
	if err := os.WriteFile(filepath.Join(dir, ".ralph"), []byte("x"), 0644); err != nil {
		t.Fatalf("WriteFile .ralph: %v", err)
	}

	err := executeLoop(loop.ModeBuild, 1, true, false, "", false, false, "")
	if err == nil {
		t.Fatal("expected error from git operations")
	}
}

func TestExecuteRun_StoreUnavailable(t *testing.T) {
	// Create .ralph as a regular file so store.NewJSONL cannot create the logs
	// subdirectory. Covers the same fmt.Fprintf stderr branch in executeRun.
	dir := t.TempDir()
	t.Chdir(dir)
	writeExecTestFile(t, dir, "ralph.toml", testConfigNoRegent())
	writeExecTestFile(t, dir, "CHRONICLE.md", "# Done\n\nContent.\n")
	writeExecTestFile(t, dir, "BUILD.md", "# Build\n")
	if err := os.WriteFile(filepath.Join(dir, ".ralph"), []byte("x"), 0644); err != nil {
		t.Fatalf("WriteFile .ralph: %v", err)
	}

	err := executeRun(1, true, false, "", false, false, "")
	if err == nil {
		t.Fatal("expected error from git operations")
	}
}

func TestExecuteLoop_NotificationsURLSet(t *testing.T) {
	// With notifications.url set the notify wiring runs; loop still fails at
	// git CurrentBranch() because there is no git repo in the temp dir.
	dir := t.TempDir()
	t.Chdir(dir)
	cfg := testConfigNoRegent() + "\n[notifications]\nurl = \"http://127.0.0.1:0/webhook\"\n"
	writeExecTestFile(t, dir, "ralph.toml", cfg)
	writeExecTestFile(t, dir, "BUILD.md", "# Build\n")

	err := executeLoop(loop.ModeBuild, 1, true, false, "", false, false, "")
	if err == nil {
		t.Fatal("expected error from git operations")
	}
	if strings.Contains(err.Error(), "prompt file") {
		t.Errorf("should not be a prompt-file error, got: %v", err)
	}
}

func TestExecuteRun_NotificationsURLSet(t *testing.T) {
	// With notifications.url set the notify wiring runs; executeRun still fails
	// at git operations because there is no git repo in the temp dir.
	dir := t.TempDir()
	t.Chdir(dir)
	cfg := testConfigNoRegent() + "\n[notifications]\nurl = \"http://127.0.0.1:0/webhook\"\n"
	writeExecTestFile(t, dir, "ralph.toml", cfg)
	writeExecTestFile(t, dir, "CHRONICLE.md", "# Done\n\nSome content.\n")
	writeExecTestFile(t, dir, "BUILD.md", "# Build\n")

	err := executeRun(1, true, false, "", false, false, "")
	if err == nil {
		t.Fatal("expected error from git operations")
	}
}

func TestExecuteDashboard_ConfigNotFound(t *testing.T) {
	// Isolated temp dir with no ralph.toml anywhere in its ancestor tree.
	t.Chdir(t.TempDir())

	err := executeDashboard()
	if err == nil {
		t.Fatal("expected error when ralph.toml not found")
	}
	if !strings.Contains(err.Error(), "ralph.toml") {
		t.Errorf("error should mention ralph.toml, got: %v", err)
	}
}

func TestExecuteDashboard_ConfigInvalid(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	// Empty roam.prompt_file triggers Validate() error.
	writeExecTestFile(t, dir, "ralph.toml", "[build]\nprompt_file = \"b.md\"\n[roam]\nprompt_file = \"\"\n")

	err := executeDashboard()
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "config validation") {
		t.Errorf("error should mention config validation, got: %v", err)
	}
}

func TestShowStatus_CorruptedStateFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	// Write invalid JSON to the state file — LoadState should return a parse error.
	writeExecTestFile(t, dir, ".ralph/regent-state.json", "not valid json {{{")

	err := showStatus()
	if err == nil {
		t.Fatal("expected error for corrupted state file")
	}
}

// TestSignalContext_SIGTERMCancelsContext covers the cancel() call inside the
// `case <-sigs:` branch of signalContext. Sending SIGTERM to ourselves is safe
// because signal.Notify suppresses the default termination behavior while the
// channel is registered; the signal is delivered to the channel instead.
func TestSignalContext_SIGTERMCancelsContext(t *testing.T) {
	if runtime.GOOS == "windows" {
		// On Windows, syscall.SIGTERM maps to TerminateProcess, which immediately
		// kills the process. Skipping rather than risking the test binary crash.
		t.Skip("SIGTERM cannot be sent to self safely on Windows")
	}

	ctx, cancel := signalContext()
	defer cancel()

	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("FindProcess: %v", err)
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("Signal(SIGTERM): %v", err)
	}

	select {
	case <-ctx.Done():
		// Context was cancelled by the signal handler goroutine — test passes.
	case <-time.After(2 * time.Second):
		t.Fatal("context not cancelled after sending SIGTERM to self")
	}
}

// TestSignalContextGraceful_TwoStageStop verifies the two-stage SIGINT
// behaviour: first signal closes stopCh, second signal cancels context.
func TestSignalContextGraceful_TwoStageStop(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SIGINT cannot be reliably sent to self on Windows")
	}

	ctx, cancel, stopCh := signalContextGraceful()
	defer cancel()

	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("FindProcess: %v", err)
	}

	// First SIGINT: should close stopCh but NOT cancel context.
	if err := proc.Signal(syscall.SIGINT); err != nil {
		t.Fatalf("Signal(SIGINT) #1: %v", err)
	}

	select {
	case <-stopCh:
		// stopCh closed — good.
	case <-time.After(2 * time.Second):
		t.Fatal("stopCh not closed after first SIGINT")
	}

	// Context should still be alive after first signal.
	select {
	case <-ctx.Done():
		t.Fatal("context should NOT be cancelled after first SIGINT")
	default:
	}

	// Second SIGINT: should cancel context.
	if err := proc.Signal(syscall.SIGINT); err != nil {
		t.Fatalf("Signal(SIGINT) #2: %v", err)
	}

	select {
	case <-ctx.Done():
		// Context cancelled — test passes.
	case <-time.After(2 * time.Second):
		t.Fatal("context not cancelled after second SIGINT")
	}
}

// TestSignalContextGraceful_CancelCleansUp verifies that calling the cancel
// func exits the signal goroutine without requiring any signals.
func TestSignalContextGraceful_CancelCleansUp(t *testing.T) {
	ctx, cancel, stopCh := signalContextGraceful()

	cancel()

	select {
	case <-ctx.Done():
		// Expected.
	case <-time.After(2 * time.Second):
		t.Fatal("context not cancelled by cancel()")
	}

	// stopCh should NOT be closed by cancel — only by signals.
	select {
	case <-stopCh:
		t.Fatal("stopCh should not be closed by cancel()")
	default:
	}
}

// TestSignalContextGraceful_CancelAfterFirstSignal covers the ctx.Done() branch
// inside the second select of signalContextGraceful: send one SIGINT (closes
// stopCh), then cancel the context instead of sending a second signal.
// Skipped on Windows where SIGINT cannot be sent to self reliably.
func TestSignalContextGraceful_CancelAfterFirstSignal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SIGINT cannot be reliably sent to self on Windows")
	}

	ctx, cancel, stopCh := signalContextGraceful()
	defer cancel()

	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("FindProcess: %v", err)
	}

	// First SIGINT: closes stopCh.
	if err := proc.Signal(syscall.SIGINT); err != nil {
		t.Fatalf("Signal(SIGINT): %v", err)
	}
	select {
	case <-stopCh:
	case <-time.After(2 * time.Second):
		t.Fatal("stopCh not closed after SIGINT")
	}

	// Cancel instead of a second SIGINT: the goroutine's second select should
	// hit case <-ctx.Done() and exit, leaving the context cancelled.
	cancel()
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("context not cancelled after cancel()")
	}
}

// ---- Roam orchestration tests ----

func TestExecuteLoop_Roam_StaysOnCurrentBranch(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	t.Chdir(dir)
	writeExecTestFile(t, dir, "ralph.toml", testConfigNoRegent())
	writeExecTestFile(t, dir, "BUILD.md", "# Build\n")
	writeFakeCLIExecutable(t, dir, "claude")
	prependCLIPath(t, dir)

	// Record the branch before running with roam.
	before := exec.Command("git", "branch", "--show-current")
	before.Dir = dir
	outBefore, err := before.Output()
	if err != nil {
		t.Fatalf("git branch --show-current: %v", err)
	}
	branchBefore := strings.TrimSpace(string(outBefore))

	// roam=true: should stay on the current branch (no sweep branch creation).
	_ = executeLoop(loop.ModeBuild, 1, true, true, "", false, false, "")

	after := exec.Command("git", "branch", "--show-current")
	after.Dir = dir
	outAfter, err := after.Output()
	if err != nil {
		t.Fatalf("git branch --show-current: %v", err)
	}
	branchAfter := strings.TrimSpace(string(outAfter))

	if branchAfter != branchBefore {
		t.Errorf("roam should stay on current branch %q, but switched to %q", branchBefore, branchAfter)
	}
}

// TestSetupWorktree_DetectFails verifies that setupWorktree returns an error
// when worktrunk is not found on PATH, covering the Detect() error branch.
func TestSetupWorktree_DetectFails(t *testing.T) {
	t.Setenv("PATH", "")
	dir := t.TempDir()
	setup := &loopSetup{
		dir:       dir,
		gitRunner: nil,
		lp:        &loop.Loop{},
		cleanup:   func() {},
	}
	err := setupWorktree(setup)
	if err == nil {
		t.Fatal("expected error when worktrunk not on PATH")
	}
	if !strings.Contains(err.Error(), "worktrunk") {
		t.Errorf("error should mention worktrunk, got: %v", err)
	}
}

func TestRalphWorktreeBranch(t *testing.T) {
	now := time.Date(2026, 7, 9, 12, 34, 56, 123, time.UTC)
	got := ralphWorktreeBranch("refs/heads/develop", now)
	want := "ralph/develop/20260709-123456-000000123"
	if got != want {
		t.Fatalf("ralphWorktreeBranch() = %q, want %q", got, want)
	}
}

func TestSetupWorktree_CreatesRalphBranchInsteadOfCurrentBranch(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	fakeBin := t.TempDir()
	writeFakeWorktrunkExecutable(t, fakeBin)
	prependCLIPath(t, fakeBin)

	worktreeDir := filepath.Join(t.TempDir(), "worktrees")
	setup := &loopSetup{
		cfg: &config.Config{
			Worktree: config.WorktreeConfig{WorktreeDir: worktreeDir},
		},
		dir:       dir,
		gitRunner: git.NewRunner(dir),
		lp:        &loop.Loop{},
		cleanup:   func() {},
	}
	defer func() { setup.cleanup() }()

	baseBranch, err := setup.gitRunner.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}

	if err := setupWorktree(setup); err != nil {
		t.Fatalf("setupWorktree: %v", err)
	}
	if setup.lp.Dir == dir {
		t.Fatal("expected setupWorktree to move loop dir into a separate worktree")
	}

	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = setup.lp.Dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git branch --show-current: %v", err)
	}
	worktreeBranch := strings.TrimSpace(string(out))
	if worktreeBranch == baseBranch {
		t.Fatalf("worktree reused current branch %q", baseBranch)
	}
	if !strings.HasPrefix(worktreeBranch, "ralph/"+baseBranch+"/") {
		t.Fatalf("worktree branch = %q, want ralph/%s/ prefix", worktreeBranch, baseBranch)
	}
}
