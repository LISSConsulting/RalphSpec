package regent

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestSaveAndLoadState(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 2, 23, 14, 30, 0, 0, time.UTC)
	later := now.Add(5 * time.Minute)

	original := State{
		RalphPID:        12345,
		Iteration:       7,
		ConsecutiveErrs: 0,
		LastOutputAt:    now,
		Agent:           "codex",
		LastCommit:      "abc1234",
		TotalCostUSD:    1.42,
		Branch:          "feat/test-branch",
		Mode:            "build",
		StartedAt:       now,
		FinishedAt:      later,
		Passed:          true,
	}

	if err := SaveState(dir, original); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	loaded, err := LoadState(dir)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}

	if loaded.RalphPID != original.RalphPID {
		t.Errorf("RalphPID = %d, want %d", loaded.RalphPID, original.RalphPID)
	}
	if loaded.Iteration != original.Iteration {
		t.Errorf("Iteration = %d, want %d", loaded.Iteration, original.Iteration)
	}
	if loaded.ConsecutiveErrs != original.ConsecutiveErrs {
		t.Errorf("ConsecutiveErrs = %d, want %d", loaded.ConsecutiveErrs, original.ConsecutiveErrs)
	}
	if !loaded.LastOutputAt.Equal(original.LastOutputAt) {
		t.Errorf("LastOutputAt = %v, want %v", loaded.LastOutputAt, original.LastOutputAt)
	}
	if loaded.LastCommit != original.LastCommit {
		t.Errorf("LastCommit = %q, want %q", loaded.LastCommit, original.LastCommit)
	}
	if loaded.Agent != original.Agent {
		t.Errorf("Agent = %q, want %q", loaded.Agent, original.Agent)
	}
	if loaded.TotalCostUSD != original.TotalCostUSD {
		t.Errorf("TotalCostUSD = %f, want %f", loaded.TotalCostUSD, original.TotalCostUSD)
	}
	if loaded.Branch != original.Branch {
		t.Errorf("Branch = %q, want %q", loaded.Branch, original.Branch)
	}
	if loaded.Mode != original.Mode {
		t.Errorf("Mode = %q, want %q", loaded.Mode, original.Mode)
	}
	if !loaded.StartedAt.Equal(original.StartedAt) {
		t.Errorf("StartedAt = %v, want %v", loaded.StartedAt, original.StartedAt)
	}
	if !loaded.FinishedAt.Equal(original.FinishedAt) {
		t.Errorf("FinishedAt = %v, want %v", loaded.FinishedAt, original.FinishedAt)
	}
	if loaded.Passed != original.Passed {
		t.Errorf("Passed = %v, want %v", loaded.Passed, original.Passed)
	}
}

func TestLoadState_NoFile(t *testing.T) {
	dir := t.TempDir()
	state, err := LoadState(dir)
	if err != nil {
		t.Fatalf("LoadState with no file should not error: %v", err)
	}
	if state.RalphPID != 0 {
		t.Errorf("expected zero state, got PID=%d", state.RalphPID)
	}
}

func TestSaveState_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	ralphDir := filepath.Join(dir, stateDirName)

	// Ensure .ralph does not exist
	if _, err := os.Stat(ralphDir); !os.IsNotExist(err) {
		t.Fatal("expected .ralph to not exist initially")
	}

	if err := SaveState(dir, State{RalphPID: 1}); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	if _, err := os.Stat(ralphDir); os.IsNotExist(err) {
		t.Error("expected .ralph directory to be created")
	}
}

func TestLoadState_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, stateDirName)
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, stateFileName), []byte("{invalid"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadState(dir)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoadState_ReadError(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, stateDirName)
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create state path as a directory so ReadFile returns a non-NotExist error
	statePath := filepath.Join(stateDir, stateFileName)
	if err := os.MkdirAll(statePath, 0755); err != nil {
		t.Fatal(err)
	}

	_, err := LoadState(dir)
	if err == nil {
		t.Fatal("expected error when state path is a directory")
	}
}

func TestSaveState_MkdirError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("cannot test permission errors as root")
	}

	dir := t.TempDir()
	// Block the state directory by placing a regular file where it should be
	blockPath := filepath.Join(dir, stateDirName)
	if err := os.WriteFile(blockPath, []byte("block"), 0644); err != nil {
		t.Fatal(err)
	}

	err := SaveState(dir, State{RalphPID: 1})
	if err == nil {
		t.Fatal("expected error when state dir is blocked by a regular file")
	}
}

// TestSaveState_CreateTempError verifies that SaveState returns an error when
// os.CreateTemp fails. This is triggered by making the state directory
// read-only after it is created so that CreateTemp cannot write to it.
func TestSaveState_CreateTempError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("cannot test permission errors as root")
	}
	if runtime.GOOS == "windows" {
		t.Skip("chmod read-only directory is not reliably enforced on Windows")
	}

	dir := t.TempDir()
	stateDir := filepath.Join(dir, stateDirName)
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Make the state directory read-only: CreateTemp cannot create new files.
	if err := os.Chmod(stateDir, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(stateDir, 0755) }) // restore for cleanup

	err := SaveState(dir, State{RalphPID: 1})
	if err == nil {
		t.Fatal("expected error when state directory is read-only")
	}
}

func TestSaveState_RenameError(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, stateDirName)
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Block the destination file path by placing a directory there.
	// os.Rename(tempFile, directory) fails on all platforms.
	statePath := filepath.Join(stateDir, stateFileName)
	if err := os.MkdirAll(statePath, 0755); err != nil {
		t.Fatal(err)
	}

	err := SaveState(dir, State{RalphPID: 1})
	if err == nil {
		t.Fatal("expected error when state file path is a directory")
	}
	if !strings.Contains(err.Error(), "finalize state") {
		t.Errorf("expected 'finalize state' in error, got: %v", err)
	}
}
