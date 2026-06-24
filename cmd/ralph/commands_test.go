package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/LISSConsulting/RalphSpec/internal/config"
	"github.com/LISSConsulting/RalphSpec/internal/regent"
	"github.com/LISSConsulting/RalphSpec/internal/spec"
)

func isWindows() bool { return runtime.GOOS == "windows" }

func TestFormatSpecList(t *testing.T) {
	tests := []struct {
		name     string
		specs    []spec.SpecFile
		contains []string
		excludes []string
	}{
		{
			name:     "empty — no specs message",
			specs:    nil,
			contains: []string{"No specs found"},
		},
		{
			name: "single done spec (flat file)",
			specs: []spec.SpecFile{
				{Name: "test-spec", Path: "specs/test-spec.md", Status: spec.StatusDone},
			},
			contains: []string{"Specs", "─────", "✅", "specs/test-spec.md", "done"},
		},
		{
			name: "directory-based spec uses Dir path",
			specs: []spec.SpecFile{
				{Name: "004-feature", Path: "specs/004-feature/spec.md", Dir: "specs/004-feature", IsDir: true, Status: spec.StatusPlanned},
			},
			contains: []string{"Specs", "─────", "📐", "specs/004-feature", "planned"},
			excludes: []string{"spec.md"},
		},
		{
			name: "multiple specs with mixed statuses",
			specs: []spec.SpecFile{
				{Name: "spec-one", Path: "specs/spec-one.md", Status: spec.StatusDone},
				{Name: "spec-two", Path: "specs/spec-two.md", Status: spec.StatusInProgress},
				{Name: "new-feature", Path: "specs/new-feature.md", Status: spec.StatusNotStarted},
			},
			contains: []string{
				"Specs", "─────",
				"✅", "specs/spec-one.md", "done",
				"🔄", "specs/spec-two.md", "in progress",
				"⬜", "specs/new-feature.md", "not started",
			},
		},
		{
			name:     "empty slice — same as nil",
			specs:    []spec.SpecFile{},
			contains: []string{"No specs found"},
			excludes: []string{"Specs", "─────"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatSpecList(tt.specs)
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

func TestFormatScaffoldResult(t *testing.T) {
	tests := []struct {
		name     string
		created  []string
		contains []string
		excludes []string
	}{
		{
			name:     "nothing created — already exists message",
			created:  nil,
			contains: []string{"All files already exist"},
			excludes: []string{"Created"},
		},
		{
			name:     "empty slice — same as nil",
			created:  []string{},
			contains: []string{"All files already exist"},
		},
		{
			name:     "single file created",
			created:  []string{"ralph.toml"},
			contains: []string{"Created ralph.toml"},
			excludes: []string{"already exist"},
		},
		{
			name:    "multiple files created",
			created: []string{"ralph.toml", "PLAN.md", "BUILD.md", "specs/"},
			contains: []string{
				"Created ralph.toml",
				"Created PLAN.md",
				"Created BUILD.md",
				"Created specs/",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatScaffoldResult(tt.created)
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

// captureStderr captures output written to os.Stderr during fn.
func captureStderr(fn func()) string {
	r, w, _ := os.Pipe()
	old := os.Stderr
	os.Stderr = w
	fn()
	_ = w.Close()
	os.Stderr = old
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

// TestAPIKeyWarning_KeySet verifies the warning appears on stderr when
// ANTHROPIC_API_KEY is set.
func TestAPIKeyWarning_KeySet(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-test-key-12345")

	root := rootCmd()
	got := captureStderr(func() {
		if root.PersistentPreRunE != nil {
			_ = root.PersistentPreRunE(root, nil)
		}
	})

	if !strings.Contains(got, "ANTHROPIC_API_KEY") {
		t.Errorf("expected warning to mention ANTHROPIC_API_KEY, got: %q", got)
	}
	if !strings.Contains(got, "WARNING") {
		t.Errorf("expected 'WARNING' in output, got: %q", got)
	}
}

// TestAPIKeyWarning_KeyUnset verifies no warning is printed when
// ANTHROPIC_API_KEY is not set.
func TestAPIKeyWarning_KeyUnset(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")

	root := rootCmd()
	got := captureStderr(func() {
		if root.PersistentPreRunE != nil {
			_ = root.PersistentPreRunE(root, nil)
		}
	})

	if strings.Contains(got, "ANTHROPIC_API_KEY") {
		t.Errorf("should not print warning when env var is unset, got: %q", got)
	}
}

// TestAPIKeyWarning_NoColor verifies --no-color suppresses ANSI escapes.
func TestAPIKeyWarning_NoColor(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-test-key-12345")

	root := rootCmd()
	if err := root.PersistentFlags().Set("no-color", "true"); err != nil {
		t.Fatalf("set --no-color: %v", err)
	}

	got := captureStderr(func() {
		if root.PersistentPreRunE != nil {
			_ = root.PersistentPreRunE(root, nil)
		}
	})

	if strings.Contains(got, "\x1b[") {
		t.Errorf("--no-color mode should not emit ANSI escapes, got: %q", got)
	}
	if !strings.Contains(got, "ANTHROPIC_API_KEY") {
		t.Errorf("warning text should still appear with --no-color, got: %q", got)
	}
}

func TestRootCmdStructure(t *testing.T) {
	root := rootCmd()

	if root.Use != "ralph" {
		t.Errorf("root Use = %q, want %q", root.Use, "ralph")
	}

	// --no-tui and --no-color persistent flags must exist
	noTUI := root.PersistentFlags().Lookup("no-tui")
	if noTUI == nil {
		t.Fatal("missing --no-tui persistent flag")
	}
	noColor := root.PersistentFlags().Lookup("no-color")
	if noColor == nil {
		t.Fatal("missing --no-color persistent flag")
	}

	// Collect all top-level subcommand names
	subs := map[string]bool{}
	for _, sub := range root.Commands() {
		subs[sub.Name()] = true
	}

	// Speckit workflow commands must be present
	for _, want := range []string{"specify", "plan", "clarify", "tasks", "run"} {
		if !subs[want] {
			t.Errorf("missing top-level speckit command %q", want)
		}
	}

	// Loop and project management commands
	for _, want := range []string{"build", "loop", "status", "init", "spec"} {
		if !subs[want] {
			t.Errorf("missing top-level command %q", want)
		}
	}

	// Old top-level plan/run commands should be GONE (now under loop)
	// Note: "plan" and "run" now refer to speckit commands, not the old loop commands
}

func TestLoopCmdStructure(t *testing.T) {
	root := rootCmd()

	var lp *cobra.Command
	for _, sub := range root.Commands() {
		if sub.Name() == "loop" {
			lp = sub
			break
		}
	}
	if lp == nil {
		t.Fatal("missing 'loop' subcommand")
	}

	loopSubs := map[string]bool{}
	for _, sub := range lp.Commands() {
		loopSubs[sub.Name()] = true
	}

	for _, want := range []string{"plan", "build", "run"} {
		if !loopSubs[want] {
			t.Errorf("loop: missing subcommand %q", want)
		}
	}
}

func TestLoopCmdsHaveMaxFlag(t *testing.T) {
	root := rootCmd()

	// Find the loop command
	var lp *cobra.Command
	for _, sub := range root.Commands() {
		if sub.Name() == "loop" {
			lp = sub
			break
		}
	}
	if lp == nil {
		t.Fatal("missing loop subcommand")
	}

	for _, name := range []string{"plan", "build", "run"} {
		t.Run(name, func(t *testing.T) {
			for _, sub := range lp.Commands() {
				if sub.Name() == name {
					if sub.Flags().Lookup("max") == nil {
						t.Errorf("loop %s: missing --max flag", name)
					}
					return
				}
			}
			t.Fatalf("loop subcommand %q not found", name)
		})
	}

	// Top-level build also has --max flag
	for _, sub := range root.Commands() {
		if sub.Name() == "build" {
			if sub.Flags().Lookup("max") == nil {
				t.Error("top-level build: missing --max flag")
			}
			return
		}
	}
}

func TestLoopCmdsHaveAgentFlag(t *testing.T) {
	root := rootCmd()

	var loopCmd *cobra.Command
	for _, sub := range root.Commands() {
		if sub.Name() == "loop" {
			loopCmd = sub
			break
		}
	}
	if loopCmd == nil {
		t.Fatal("missing loop subcommand")
	}

	for _, name := range []string{"plan", "build", "run"} {
		t.Run(name, func(t *testing.T) {
			for _, sub := range loopCmd.Commands() {
				if sub.Name() == name {
					if sub.Flags().Lookup("agent") == nil {
						t.Errorf("loop %s: missing --agent flag", name)
					}
					return
				}
			}
			t.Fatalf("loop subcommand %q not found", name)
		})
	}

	for _, sub := range root.Commands() {
		if sub.Name() == "build" {
			if sub.Flags().Lookup("agent") == nil {
				t.Error("top-level build: missing --agent flag")
			}
			return
		}
	}
}

func TestSpecCmdSubcommands(t *testing.T) {
	root := rootCmd()

	for _, sub := range root.Commands() {
		if sub.Name() != "spec" {
			continue
		}
		specSubs := map[string]bool{}
		for _, child := range sub.Commands() {
			specSubs[child.Name()] = true
		}
		if !specSubs["list"] {
			t.Error("spec: missing subcommand 'list'")
		}
		if specSubs["new"] {
			t.Error("spec: 'new' subcommand should be removed (use 'ralph specify' instead)")
		}
		return
	}
	t.Fatal("missing spec subcommand")
}

// --- End-to-end command execution tests ---
// These use t.Chdir (Go 1.24) to test command RunE handlers with real I/O.

func TestInitCmdExecution(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	cmd := initCmd()
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("initCmd RunE: %v", err)
	}

	// Verify core files were created
	for _, name := range []string{"ralph.toml", "PLAN.md", "BUILD.md"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("expected %s to exist: %v", name, err)
		}
	}
}

func TestInitCmdIdempotent(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	cmd1 := initCmd()
	if err := cmd1.RunE(cmd1, nil); err != nil {
		t.Fatalf("first initCmd RunE: %v", err)
	}

	// Second run should succeed without creating anything
	cmd2 := initCmd()
	if err := cmd2.RunE(cmd2, nil); err != nil {
		t.Fatalf("second initCmd RunE: %v", err)
	}
}

func TestInitCmd_ScaffoldError(t *testing.T) {
	// Trigger ScaffoldProject returning an error by creating .gitignore as a
	// directory. Pre-create all files that scaffold checks before .gitignore so
	// the function progresses past them and reaches the .gitignore read step.
	dir := t.TempDir()
	t.Chdir(dir)
	if _, err := config.InitFile(dir); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"PLAN.md":  "x",
		"BUILD.md": "x",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "specs"), 0755); err != nil {
		t.Fatalf("MkdirAll specs: %v", err)
	}
	// .gitignore as a directory → os.ReadFile returns a non-IsNotExist error.
	if err := os.MkdirAll(filepath.Join(dir, ".gitignore"), 0755); err != nil {
		t.Fatalf("MkdirAll .gitignore: %v", err)
	}

	cmd := initCmd()
	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when ScaffoldProject fails")
	}
	if !strings.Contains(err.Error(), ".gitignore") {
		t.Errorf("error should mention .gitignore, got: %v", err)
	}
}

func TestStatusCmdExecution(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	now := time.Now()
	state := regent.State{
		RalphPID:     123,
		Iteration:    5,
		Branch:       "main",
		TotalCostUSD: 1.50,
		StartedAt:    now.Add(-10 * time.Minute),
		FinishedAt:   now,
		Passed:       true,
	}
	if err := regent.SaveState(dir, state); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	cmd := statusCmd()
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("statusCmd RunE: %v", err)
	}
}

func TestStatusCmdNoState(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	// No state file — should succeed (LoadState returns empty state)
	cmd := statusCmd()
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("statusCmd RunE: %v", err)
	}
}

func TestSpecListCmdExecution(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	// Create specs directory with a test spec
	specsDir := filepath.Join(dir, "specs")
	if err := os.MkdirAll(specsDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(specsDir, "test-spec.md"), []byte("# Test"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cmd := specListCmd()
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("specListCmd RunE: %v", err)
	}
}

func TestSpecListCmdNoSpecs(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	// No specs directory — should succeed
	cmd := specListCmd()
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("specListCmd RunE: %v", err)
	}
}

func TestSpecListCmd_SpecsNotDir(t *testing.T) {
	if testing.Short() {
		t.Skip("platform-specific test")
	}
	dir := t.TempDir()
	t.Chdir(dir)

	// On Windows, ReadDir on a regular file may return an IsNotExist-like error.
	// This test covers the non-IsNotExist error path which is Unix-only.
	if isWindows() {
		t.Skip("ReadDir on a regular file returns IsNotExist on Windows")
	}

	// Create a regular file named "specs" so ReadDir returns a non-IsNotExist error.
	if err := os.WriteFile(filepath.Join(dir, "specs"), []byte("not a dir"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cmd := specListCmd()
	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when specs/ is a regular file, not a directory")
	}
}

// ---- RunE handler tests for loop plan / build / run commands ----

func TestLoopPlanCmdRunE_NoConfig(t *testing.T) {
	t.Chdir(t.TempDir())

	cmd := loopPlanCmd()
	if err := cmd.Flags().Set("max", "1"); err != nil {
		t.Fatalf("set --max flag: %v", err)
	}

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when ralph.toml not found")
	}
	if !strings.Contains(err.Error(), "ralph.toml") {
		t.Errorf("error should mention ralph.toml, got: %v", err)
	}
}

func TestLoopBuildCmdRunE_NoConfig(t *testing.T) {
	t.Chdir(t.TempDir())

	cmd := loopBuildCmd()
	if err := cmd.Flags().Set("max", "1"); err != nil {
		t.Fatalf("set --max flag: %v", err)
	}

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when ralph.toml not found")
	}
	if !strings.Contains(err.Error(), "ralph.toml") {
		t.Errorf("error should mention ralph.toml, got: %v", err)
	}
}

func TestLoopRunCmdRunE_NoConfig(t *testing.T) {
	t.Chdir(t.TempDir())

	cmd := loopRunCmd()
	if err := cmd.Flags().Set("max", "1"); err != nil {
		t.Fatalf("set --max flag: %v", err)
	}

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when ralph.toml not found")
	}
	if !strings.Contains(err.Error(), "ralph.toml") {
		t.Errorf("error should mention ralph.toml, got: %v", err)
	}
}

func TestBuildCmdRunE_NoConfig(t *testing.T) {
	t.Chdir(t.TempDir())

	cmd := buildCmd()
	if err := cmd.Flags().Set("max", "1"); err != nil {
		t.Fatalf("set --max flag: %v", err)
	}

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when ralph.toml not found")
	}
	if !strings.Contains(err.Error(), "ralph.toml") {
		t.Errorf("error should mention ralph.toml, got: %v", err)
	}
}

// TestBuildCmd_FocusFlag verifies the --focus flag is registered on build, loop build,
// and loop run commands and parsed correctly.
func TestBuildCmd_FocusFlag(t *testing.T) {
	cmds := []struct {
		name string
		cmd  *cobra.Command
	}{
		{"build", buildCmd()},
		{"loop build", loopBuildCmd()},
		{"loop run", loopRunCmd()},
	}
	for _, tc := range cmds {
		t.Run(tc.name, func(t *testing.T) {
			f := tc.cmd.Flags().Lookup("focus")
			if f == nil {
				t.Fatalf("%s: --focus flag not registered", tc.name)
			}
			if err := tc.cmd.Flags().Set("focus", "UI/UX"); err != nil {
				t.Fatalf("%s: set --focus: %v", tc.name, err)
			}
			got, _ := tc.cmd.Flags().GetString("focus")
			if got != "UI/UX" {
				t.Errorf("%s: focus=%q, want %q", tc.name, got, "UI/UX")
			}
		})
	}
}

// TestRootCmd_NoSubcommand_CallsDashboard exercises the rootCmd RunE body
// (return executeDashboard()) by executing the root command with no subcommand.
// Without ralph.toml present, executeDashboard fails early at config.Load,
// making this test safe to run in any environment (no TTY required).
func TestRootCmd_NoSubcommand_CallsDashboard(t *testing.T) {
	t.Chdir(t.TempDir())

	root := rootCmd()
	root.SetArgs([]string{})
	root.SilenceErrors = true
	root.SilenceUsage = true

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error when ralph.toml not found")
	}
	if !strings.Contains(err.Error(), "ralph.toml") {
		t.Errorf("error should mention ralph.toml, got: %v", err)
	}
}
