package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initTestRepo creates a temporary git repo with one commit and returns
// its path. It configures local user.name and user.email so commits work.
func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "checkout", "-b", "main"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %s (%v)", args, out, err)
		}
	}

	// Create a file and make an initial commit
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", "initial commit"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %s (%v)", args, out, err)
		}
	}

	return dir
}

func TestCurrentBranch(t *testing.T) {
	dir := initTestRepo(t)
	r := NewRunner(dir)

	branch, err := r.CurrentBranch()
	if err != nil {
		t.Fatal(err)
	}
	if branch != "main" {
		t.Errorf("got %q, want %q", branch, "main")
	}
}

func TestHasUncommittedChanges(t *testing.T) {
	dir := initTestRepo(t)
	r := NewRunner(dir)

	t.Run("clean repo", func(t *testing.T) {
		has, err := r.HasUncommittedChanges()
		if err != nil {
			t.Fatal(err)
		}
		if has {
			t.Error("expected no uncommitted changes")
		}
	})

	t.Run("dirty repo", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("dirty"), 0644); err != nil {
			t.Fatal(err)
		}
		has, err := r.HasUncommittedChanges()
		if err != nil {
			t.Fatal(err)
		}
		if !has {
			t.Error("expected uncommitted changes")
		}
	})
}

func TestLastCommit(t *testing.T) {
	dir := initTestRepo(t)
	r := NewRunner(dir)

	last, err := r.LastCommit()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(last, "initial commit") {
		t.Errorf("expected commit message in output, got %q", last)
	}
}

func TestStashAndPop(t *testing.T) {
	dir := initTestRepo(t)
	r := NewRunner(dir)

	// Create a dirty file
	dirtyPath := filepath.Join(dir, "dirty.txt")
	if err := os.WriteFile(dirtyPath, []byte("stash me"), 0644); err != nil {
		t.Fatal(err)
	}
	// Need to track the file first for stash to pick it up
	cmd := exec.Command("git", "add", "dirty.txt")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %s (%v)", out, err)
	}

	created, err := r.Stash()
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("expected stash entry to be created")
	}

	// File should be gone from working tree
	has, _ := r.HasUncommittedChanges()
	if has {
		t.Error("expected clean after stash")
	}

	if err := r.StashPop(); err != nil {
		t.Fatal(err)
	}

	// File should be back
	has, _ = r.HasUncommittedChanges()
	if !has {
		t.Error("expected dirty after stash pop")
	}
}

func TestStashUntrackedOnlyDoesNotCreateEntry(t *testing.T) {
	dir := initTestRepo(t)
	r := NewRunner(dir)

	untrackedPath := filepath.Join(dir, "untracked.txt")
	if err := os.WriteFile(untrackedPath, []byte("stash me"), 0644); err != nil {
		t.Fatal(err)
	}

	created, err := r.Stash()
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("expected no stash entry for untracked-only changes")
	}
	if _, err := os.Stat(untrackedPath); err != nil {
		t.Fatalf("expected untracked file to remain in worktree: %v", err)
	}
}

func TestStashNoChanges(t *testing.T) {
	dir := initTestRepo(t)
	r := NewRunner(dir)
	// Repo is clean — Stash should succeed without error.
	created, err := r.Stash()
	if err != nil {
		t.Fatalf("Stash() on clean repo returned unexpected error: %v", err)
	}
	if created {
		t.Fatal("expected no stash entry for clean repo")
	}
}

func TestRevert(t *testing.T) {
	dir := initTestRepo(t)
	r := NewRunner(dir)

	// Make a second commit to revert
	if err := os.WriteFile(filepath.Join(dir, "bad.txt"), []byte("bad change"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "add", "bad.txt"},
		{"git", "commit", "-m", "bad commit"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %s (%v)", args, out, err)
		}
	}

	// Get the SHA of HEAD
	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	cmd.Dir = dir
	shaOut, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	sha := strings.TrimSpace(string(shaOut))

	if err := r.Revert(sha); err != nil {
		t.Fatal(err)
	}

	// bad.txt should no longer exist after revert
	if _, err := os.Stat(filepath.Join(dir, "bad.txt")); !os.IsNotExist(err) {
		t.Error("expected bad.txt to be removed by revert")
	}
}

func TestNewRunner(t *testing.T) {
	r := NewRunner("/tmp/test")
	if r.Dir != "/tmp/test" {
		t.Errorf("got %q, want %q", r.Dir, "/tmp/test")
	}
}

// initTestRepoWithRemote creates a local repo + bare remote and returns
// (workDir, remoteDir). The local repo has "origin" pointing at the bare remote.
func initTestRepoWithRemote(t *testing.T) (string, string) {
	t.Helper()

	// Create bare remote
	remoteDir := t.TempDir()
	for _, args := range [][]string{
		{"git", "init", "--bare"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = remoteDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %s (%v)", args, out, err)
		}
	}

	// Create working repo
	workDir := initTestRepo(t)

	// Add remote and push initial commit
	for _, args := range [][]string{
		{"git", "remote", "add", "origin", remoteDir},
		{"git", "push", "-u", "origin", "main"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = workDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %s (%v)", args, out, err)
		}
	}

	return workDir, remoteDir
}

func TestPull(t *testing.T) {
	t.Run("pull with no changes", func(t *testing.T) {
		workDir, _ := initTestRepoWithRemote(t)
		r := NewRunner(workDir)

		if err := r.Pull("main"); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("pull picks up remote changes", func(t *testing.T) {
		workDir, remoteDir := initTestRepoWithRemote(t)

		// Clone a second copy, commit something, push to remote
		secondDir := filepath.Join(t.TempDir(), "clone")
		cmd := exec.Command("git", "clone", remoteDir, secondDir)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("clone failed: %s (%v)", out, err)
		}
		for _, args := range [][]string{
			{"git", "config", "user.email", "other@test.com"},
			{"git", "config", "user.name", "Other"},
			{"git", "checkout", "main"},
		} {
			cmd := exec.Command(args[0], args[1:]...)
			cmd.Dir = secondDir
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%v failed: %s (%v)", args, out, err)
			}
		}
		if err := os.WriteFile(filepath.Join(secondDir, "remote.txt"), []byte("from remote"), 0644); err != nil {
			t.Fatal(err)
		}
		for _, args := range [][]string{
			{"git", "add", "remote.txt"},
			{"git", "commit", "-m", "remote change"},
			{"git", "push", "origin", "main"},
		} {
			cmd := exec.Command(args[0], args[1:]...)
			cmd.Dir = secondDir
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("[%v] failed: %s (%v)", args, out, err)
			}
		}

		// Now pull from original working dir
		r := NewRunner(workDir)
		if err := r.Pull("main"); err != nil {
			t.Fatal(err)
		}

		// remote.txt should now exist locally
		if _, err := os.Stat(filepath.Join(workDir, "remote.txt")); os.IsNotExist(err) {
			t.Error("expected remote.txt after pull")
		}
	})
}

func TestPush(t *testing.T) {
	t.Run("push new commit", func(t *testing.T) {
		workDir, _ := initTestRepoWithRemote(t)
		r := NewRunner(workDir)

		// Make a local commit
		if err := os.WriteFile(filepath.Join(workDir, "local.txt"), []byte("local"), 0644); err != nil {
			t.Fatal(err)
		}
		for _, args := range [][]string{
			{"git", "add", "local.txt"},
			{"git", "commit", "-m", "local change"},
		} {
			cmd := exec.Command(args[0], args[1:]...)
			cmd.Dir = workDir
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%v failed: %s (%v)", args, out, err)
			}
		}

		if err := r.Push("main"); err != nil {
			t.Fatal(err)
		}
	})
}

func TestPull_conflictFallsBackToMerge(t *testing.T) {
	workDir, remoteDir := initTestRepoWithRemote(t)

	// Clone a second copy, make a conflicting change, push
	secondDir := filepath.Join(t.TempDir(), "clone")
	cmd := exec.Command("git", "clone", remoteDir, secondDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("clone failed: %s (%v)", out, err)
	}
	for _, args := range [][]string{
		{"git", "config", "user.email", "other@test.com"},
		{"git", "config", "user.name", "Other"},
		{"git", "checkout", "main"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = secondDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %s (%v)", args, out, err)
		}
	}
	if err := os.WriteFile(filepath.Join(secondDir, "README.md"), []byte("remote change\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "add", "README.md"},
		{"git", "commit", "-m", "remote conflicting change"},
		{"git", "push", "origin", "main"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = secondDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("[%v] failed: %s (%v)", args, out, err)
		}
	}

	// Edit same file locally with conflicting content
	if err := os.WriteFile(filepath.Join(workDir, "README.md"), []byte("local conflicting change\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "add", "README.md"},
		{"git", "commit", "-m", "local conflicting change"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = workDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %s (%v)", args, out, err)
		}
	}

	// Pull — rebase will conflict, abort, merge will also conflict
	r := NewRunner(workDir)
	err := r.Pull("main")
	if err == nil {
		t.Fatal("expected error from conflicting pull")
	}
	if !strings.Contains(err.Error(), "merge also failed") {
		t.Errorf("expected merge-also-failed error, got: %v", err)
	}
}

func TestPush_nonFastForward(t *testing.T) {
	workDir, remoteDir := initTestRepoWithRemote(t)

	// Advance remote from a second clone
	secondDir := filepath.Join(t.TempDir(), "clone")
	cmd := exec.Command("git", "clone", remoteDir, secondDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("clone failed: %s (%v)", out, err)
	}
	for _, args := range [][]string{
		{"git", "config", "user.email", "other@test.com"},
		{"git", "config", "user.name", "Other"},
		{"git", "checkout", "main"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = secondDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %s (%v)", args, out, err)
		}
	}
	if err := os.WriteFile(filepath.Join(secondDir, "remote.txt"), []byte("advance remote"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "add", "remote.txt"},
		{"git", "commit", "-m", "advance remote"},
		{"git", "push", "origin", "main"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = secondDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("[%v] failed: %s (%v)", args, out, err)
		}
	}

	// Make a local divergent commit
	if err := os.WriteFile(filepath.Join(workDir, "local.txt"), []byte("local"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "add", "local.txt"},
		{"git", "commit", "-m", "local diverge"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = workDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %s (%v)", args, out, err)
		}
	}

	// Push should fail — remote is ahead, non-fast-forward
	r := NewRunner(workDir)
	err := r.Push("main")
	if err == nil {
		t.Fatal("expected error from non-fast-forward push")
	}
	if !strings.Contains(err.Error(), "git push main") {
		t.Errorf("expected push error message, got: %v", err)
	}
}

func TestErrorPaths(t *testing.T) {
	// A Runner pointing at a non-existent directory triggers error
	// branches in every method.
	r := NewRunner("/nonexistent/git/path")

	t.Run("CurrentBranch", func(t *testing.T) {
		_, err := r.CurrentBranch()
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "git current branch") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("HasUncommittedChanges", func(t *testing.T) {
		_, err := r.HasUncommittedChanges()
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "git status") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("LastCommit", func(t *testing.T) {
		_, err := r.LastCommit()
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "git last commit") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("Stash", func(t *testing.T) {
		_, err := r.Stash()
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "git stash") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("StashPop", func(t *testing.T) {
		err := r.StashPop()
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "git stash pop") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("Revert", func(t *testing.T) {
		err := r.Revert("abc1234")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "git revert abc1234") {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestStashPopNoEntries(t *testing.T) {
	dir := initTestRepo(t)
	r := NewRunner(dir)
	// No stash entries exist — StashPop should return nil (not an error).
	if err := r.StashPop(); err != nil {
		t.Fatalf("StashPop() on empty stash returned unexpected error: %v", err)
	}
}

func TestHasRemoteBranch(t *testing.T) {
	t.Run("branch exists on remote", func(t *testing.T) {
		workDir, _ := initTestRepoWithRemote(t)
		r := NewRunner(workDir)
		if !r.HasRemoteBranch("main") {
			t.Error("expected HasRemoteBranch to return true for main")
		}
	})

	t.Run("branch does not exist on remote", func(t *testing.T) {
		workDir, _ := initTestRepoWithRemote(t)
		r := NewRunner(workDir)
		if r.HasRemoteBranch("nonexistent-branch") {
			t.Error("expected HasRemoteBranch to return false for nonexistent branch")
		}
	})
}

func TestDiffFromRemote(t *testing.T) {
	t.Run("no diff when in sync", func(t *testing.T) {
		workDir, _ := initTestRepoWithRemote(t)
		r := NewRunner(workDir)

		diff, err := r.DiffFromRemote("main")
		if err != nil {
			t.Fatal(err)
		}
		if diff {
			t.Error("expected no diff when in sync")
		}
	})

	t.Run("error when remote branch does not exist", func(t *testing.T) {
		workDir, _ := initTestRepoWithRemote(t)
		r := NewRunner(workDir)

		_, err := r.DiffFromRemote("nonexistent-branch")
		if err == nil {
			t.Fatal("expected error for nonexistent remote branch")
		}
		if !strings.Contains(err.Error(), "git diff from remote") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("diff when local has new commit", func(t *testing.T) {
		workDir, _ := initTestRepoWithRemote(t)
		r := NewRunner(workDir)

		if err := os.WriteFile(filepath.Join(workDir, "new.txt"), []byte("new"), 0644); err != nil {
			t.Fatal(err)
		}
		for _, args := range [][]string{
			{"git", "add", "new.txt"},
			{"git", "commit", "-m", "new commit"},
		} {
			cmd := exec.Command(args[0], args[1:]...)
			cmd.Dir = workDir
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%v failed: %s (%v)", args, out, err)
			}
		}

		diff, err := r.DiffFromRemote("main")
		if err != nil {
			t.Fatal(err)
		}
		if !diff {
			t.Error("expected diff when local has new commit")
		}
	})
}
