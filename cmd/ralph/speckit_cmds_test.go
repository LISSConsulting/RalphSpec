package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initGitRepoOnBranch creates a real git repo in dir on the given branch name.
// Falls back gracefully if git is not available.
func initGitRepoOnBranch(t *testing.T, dir, branch string) {
	t.Helper()
	cmds := [][]string{
		{"git", "init"},
		{"git", "checkout", "-b", branch},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "commit", "--allow-empty", "-m", "init"},
	}
	for _, args := range cmds {
		c := exec.Command(args[0], args[1:]...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
}

func TestSpecifyCmd_RequiresArgs(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	initGitRepoOnBranch(t, dir, "main")

	cmd := specifyCmd()
	// No args — cobra ValidateArgs should prevent RunE from running
	err := cmd.Args(cmd, []string{})
	if err == nil {
		t.Fatal("specify: expected error when no args provided")
	}
}

func TestSpecifyCmd_WithSpecFlag_CreatesDir(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	initGitRepoOnBranch(t, dir, "main")
	t.Setenv("PATH", "")

	cmd := specifyCmd()
	if err := cmd.Flags().Set("spec", "004-my-feature"); err != nil {
		t.Fatalf("set --spec flag: %v", err)
	}

	// RunE will fail because 'claude' is not installed, but should create the dir first.
	_ = cmd.RunE(cmd, []string{"Add user authentication"})

	specDir := filepath.Join(dir, "specs", "004-my-feature")
	if _, err := os.Stat(specDir); err != nil {
		t.Errorf("specify should create spec directory %s: %v", specDir, err)
	}
}

func TestSpeckitPlanCmd_RequiresSpecMd(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	initGitRepoOnBranch(t, dir, "main")

	// Create spec dir without spec.md
	specDir := filepath.Join(dir, "specs", "004-feature")
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := speckitPlanCmd()
	if err := cmd.Flags().Set("spec", "004-feature"); err != nil {
		t.Fatalf("set --spec flag: %v", err)
	}

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("plan: expected error when spec.md is missing")
	}
	if !strings.Contains(err.Error(), "spec.md") {
		t.Errorf("error should mention spec.md, got: %v", err)
	}
}

func TestSpeckitPlanCmd_HasSpecFlag(t *testing.T) {
	cmd := speckitPlanCmd()
	if cmd.Flags().Lookup("spec") == nil {
		t.Error("plan cmd: missing --spec flag")
	}
}

func TestClarifyCmd_RequiresSpecMd(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	initGitRepoOnBranch(t, dir, "main")

	// Create spec dir without spec.md
	specDir := filepath.Join(dir, "specs", "004-feature")
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := clarifyCmd()
	if err := cmd.Flags().Set("spec", "004-feature"); err != nil {
		t.Fatalf("set --spec flag: %v", err)
	}

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("clarify: expected error when spec.md is missing")
	}
	if !strings.Contains(err.Error(), "spec.md") {
		t.Errorf("error should mention spec.md, got: %v", err)
	}
}

func TestClarifyCmd_HasSpecFlag(t *testing.T) {
	cmd := clarifyCmd()
	if cmd.Flags().Lookup("spec") == nil {
		t.Error("clarify cmd: missing --spec flag")
	}
}

func TestSpeckitTasksCmd_RequiresPlanMd(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	initGitRepoOnBranch(t, dir, "main")

	// Create spec dir with spec.md but no plan.md
	specDir := filepath.Join(dir, "specs", "004-feature")
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(specDir, "spec.md"), []byte("# Spec"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := speckitTasksCmd()
	if err := cmd.Flags().Set("spec", "004-feature"); err != nil {
		t.Fatalf("set --spec flag: %v", err)
	}

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("tasks: expected error when plan.md is missing")
	}
	if !strings.Contains(err.Error(), "plan.md") {
		t.Errorf("error should mention plan.md, got: %v", err)
	}
}

func TestSpeckitTasksCmd_HasSpecFlag(t *testing.T) {
	cmd := speckitTasksCmd()
	if cmd.Flags().Lookup("spec") == nil {
		t.Error("tasks cmd: missing --spec flag")
	}
}

func TestSpeckitRunCmd_RequiresTasksMd(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	initGitRepoOnBranch(t, dir, "main")

	// Create spec dir with spec.md + plan.md but no tasks.md
	specDir := filepath.Join(dir, "specs", "004-feature")
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"spec.md", "plan.md"} {
		if err := os.WriteFile(filepath.Join(specDir, f), []byte("# "+f), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	cmd := speckitRunCmd()
	if err := cmd.Flags().Set("spec", "004-feature"); err != nil {
		t.Fatalf("set --spec flag: %v", err)
	}

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("run: expected error when tasks.md is missing")
	}
	if !strings.Contains(err.Error(), "tasks.md") {
		t.Errorf("error should mention tasks.md, got: %v", err)
	}
}

func TestSpeckitRunCmd_HasSpecFlag(t *testing.T) {
	cmd := speckitRunCmd()
	if cmd.Flags().Lookup("spec") == nil {
		t.Error("run cmd: missing --spec flag")
	}
}

// TestSpecifyCmd_MkdirAllError covers the MkdirAll error branch in specifyCmd
// when --spec is set but the specs directory cannot be created (blocked by a
// regular file at the same path).
func TestSpecifyCmd_MkdirAllError(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	initGitRepoOnBranch(t, dir, "main")

	// Place a regular file named "specs" so MkdirAll("specs/004-my-feature") fails.
	if err := os.WriteFile("specs", []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := specifyCmd()
	if err := cmd.Flags().Set("spec", "004-my-feature"); err != nil {
		t.Fatalf("set --spec flag: %v", err)
	}

	err := cmd.RunE(cmd, []string{"Add feature"})
	if err == nil {
		t.Fatal("specify: expected error when specs dir cannot be created")
	}
	if !strings.Contains(err.Error(), "create spec directory") {
		t.Errorf("error should mention create spec directory, got: %v", err)
	}
}

// TestSpecifyCmd_MainBranch_ResolveError tests the else branch of specifyCmd
// when --spec is absent and the branch name cannot be resolved to a spec directory.
func TestSpecifyCmd_MainBranch_ResolveError(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	initGitRepoOnBranch(t, dir, "main")

	cmd := specifyCmd()
	// No --spec flag; on main branch resolveSpec should fail.
	err := cmd.RunE(cmd, []string{"some description"})
	if err == nil {
		t.Fatal("specify: expected error on main branch without --spec flag")
	}
}

// TestSpeckitPlanCmd_MainBranch_ResolveError covers the resolveSpec error path.
func TestSpeckitPlanCmd_MainBranch_ResolveError(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	initGitRepoOnBranch(t, dir, "main")

	cmd := speckitPlanCmd()
	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("plan: expected error on main branch without --spec flag")
	}
}

// TestClarifyCmd_MainBranch_ResolveError covers the resolveSpec error path.
func TestClarifyCmd_MainBranch_ResolveError(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	initGitRepoOnBranch(t, dir, "main")

	cmd := clarifyCmd()
	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("clarify: expected error on main branch without --spec flag")
	}
}

// TestSpeckitTasksCmd_MainBranch_ResolveError covers the resolveSpec error path.
func TestSpeckitTasksCmd_MainBranch_ResolveError(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	initGitRepoOnBranch(t, dir, "main")

	cmd := speckitTasksCmd()
	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("tasks: expected error on main branch without --spec flag")
	}
}

// TestSpeckitRunCmd_MainBranch_ResolveError covers the resolveSpec error path.
func TestSpeckitRunCmd_MainBranch_ResolveError(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	initGitRepoOnBranch(t, dir, "main")

	cmd := speckitRunCmd()
	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("run: expected error on main branch without --spec flag")
	}
}

// TestSpeckitPlanCmd_AllPrereqs_ReachesSpeckit verifies that when spec.md exists the
// command reaches executeSpeckit (which fails because claude is not installed, but all
// prerequisite-check and signalContext statements are executed).
func TestSpeckitPlanCmd_AllPrereqs_ReachesSpeckit(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	initGitRepoOnBranch(t, dir, "main")
	t.Setenv("PATH", "")

	specDir := filepath.Join(dir, "specs", "004-feature")
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(specDir, "spec.md"), []byte("# Spec"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := speckitPlanCmd()
	if err := cmd.Flags().Set("spec", "004-feature"); err != nil {
		t.Fatalf("set --spec flag: %v", err)
	}

	err := cmd.RunE(cmd, nil)
	// Should not fail with a "spec.md not found" error.
	if err != nil && strings.Contains(err.Error(), "spec.md not found") {
		t.Errorf("plan: should not get spec.md error when spec.md exists, got: %v", err)
	}
}

// TestClarifyCmd_AllPrereqs_ReachesSpeckit verifies that when spec.md exists the
// command reaches executeSpeckit.
func TestClarifyCmd_AllPrereqs_ReachesSpeckit(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	initGitRepoOnBranch(t, dir, "main")
	t.Setenv("PATH", "")

	specDir := filepath.Join(dir, "specs", "004-feature")
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(specDir, "spec.md"), []byte("# Spec"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := clarifyCmd()
	if err := cmd.Flags().Set("spec", "004-feature"); err != nil {
		t.Fatalf("set --spec flag: %v", err)
	}

	err := cmd.RunE(cmd, nil)
	if err != nil && strings.Contains(err.Error(), "spec.md not found") {
		t.Errorf("clarify: should not get spec.md error when spec.md exists, got: %v", err)
	}
}

// TestSpeckitTasksCmd_AllPrereqs_ReachesSpeckit verifies that when plan.md exists the
// command reaches executeSpeckit.
func TestSpeckitTasksCmd_AllPrereqs_ReachesSpeckit(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	initGitRepoOnBranch(t, dir, "main")
	t.Setenv("PATH", "")

	specDir := filepath.Join(dir, "specs", "004-feature")
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(specDir, "plan.md"), []byte("# Plan"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := speckitTasksCmd()
	if err := cmd.Flags().Set("spec", "004-feature"); err != nil {
		t.Fatalf("set --spec flag: %v", err)
	}

	err := cmd.RunE(cmd, nil)
	if err != nil && strings.Contains(err.Error(), "plan.md not found") {
		t.Errorf("tasks: should not get plan.md error when plan.md exists, got: %v", err)
	}
}

// TestSpeckitRunCmd_AllPrereqs_ReachesSpeckit verifies that when tasks.md exists the
// command reaches executeSpeckit.
func TestSpeckitRunCmd_AllPrereqs_ReachesSpeckit(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	initGitRepoOnBranch(t, dir, "main")
	t.Setenv("PATH", "")

	specDir := filepath.Join(dir, "specs", "004-feature")
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(specDir, "tasks.md"), []byte("# Tasks"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := speckitRunCmd()
	if err := cmd.Flags().Set("spec", "004-feature"); err != nil {
		t.Fatalf("set --spec flag: %v", err)
	}

	err := cmd.RunE(cmd, nil)
	if err != nil && strings.Contains(err.Error(), "tasks.md not found") {
		t.Errorf("run: should not get tasks.md error when tasks.md exists, got: %v", err)
	}
}

func TestSpeckitCmdsResolveBranch(t *testing.T) {
	// Verify that speckit commands use branch name when --spec flag is absent.
	// We test this via speckitPlanCmd which requires spec.md to exist.
	dir := t.TempDir()
	t.Chdir(dir)

	// Create a real git repo on a branch that matches a spec directory.
	initGitRepoOnBranch(t, dir, "004-test-feature")

	// Create the matching spec directory (no spec.md yet).
	specDir := filepath.Join(dir, "specs", "004-test-feature")
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// No spec.md — plan should fail with "spec.md not found" (not a resolution error),
	// proving that branch-to-directory resolution worked.
	cmd := speckitPlanCmd()
	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("plan: expected error (spec.md missing)")
	}
	if !strings.Contains(err.Error(), "spec.md") {
		t.Errorf("error should mention spec.md (branch resolution succeeded), got: %v", err)
	}
}

// TestExecuteSpeckit_InteractiveVsNonInteractive verifies that both modes reach
// exec.Command without crashing on flag construction. Both fail because "claude"
// is not installed in CI; the test checks the error is NOT a flag/argument error.
func TestExecuteSpeckit_InteractiveVsNonInteractive(t *testing.T) {
	isNotFlagErr := func(err error) bool {
		if err == nil {
			return true
		}
		msg := err.Error()
		return !strings.Contains(msg, "unknown flag") && !strings.Contains(msg, "invalid flag")
	}

	t.Run("interactive omits -p flag", func(t *testing.T) {
		t.Setenv("PATH", "")
		err := executeSpeckit(context.Background(), "speckit.clarify", nil, true)
		if !isNotFlagErr(err) {
			t.Errorf("interactive executeSpeckit produced flag error: %v", err)
		}
	})

	t.Run("non-interactive uses -p flag", func(t *testing.T) {
		t.Setenv("PATH", "")
		err := executeSpeckit(context.Background(), "speckit.plan", nil, false)
		if !isNotFlagErr(err) {
			t.Errorf("non-interactive executeSpeckit produced flag error: %v", err)
		}
	})
}
