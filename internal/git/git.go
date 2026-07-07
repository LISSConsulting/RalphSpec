// Package git provides helpers for git operations during the Ralph loop.
package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// Runner executes git commands in a working directory.
type Runner struct {
	Dir string // working directory for git commands
}

// NewRunner creates a Runner for the given directory.
func NewRunner(dir string) *Runner {
	return &Runner{Dir: dir}
}

// CurrentBranch returns the name of the current git branch.
func (r *Runner) CurrentBranch() (string, error) {
	out, err := r.run("branch", "--show-current")
	if err != nil {
		return "", fmt.Errorf("git current branch: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// HasUncommittedChanges returns true if the working tree or index has changes.
func (r *Runner) HasUncommittedChanges() (bool, error) {
	out, err := r.run("status", "--porcelain")
	if err != nil {
		return false, fmt.Errorf("git status: %w", err)
	}
	return strings.TrimSpace(out) != "", nil
}

// Pull performs a git pull with rebase. If rebase conflicts, it aborts and
// falls back to a merge pull.
func (r *Runner) Pull(branch string) error {
	_, err := r.run("pull", "--rebase", "origin", branch)
	if err == nil {
		return nil
	}

	// Rebase failed — abort and fall back to merge
	_, abortErr := r.run("rebase", "--abort")
	if _, mergeErr := r.run("pull", "--no-rebase", "origin", branch); mergeErr != nil {
		if abortErr != nil {
			return fmt.Errorf("git pull (rebase failed, abort failed: %v, merge also failed): %w", abortErr, mergeErr)
		}
		return fmt.Errorf("git pull (rebase failed, merge also failed): %w", mergeErr)
	}
	return nil
}

// Push pushes the branch to origin. If the branch has no upstream, it sets one.
func (r *Runner) Push(branch string) error {
	_, err := r.run("push", "origin", branch)
	if err == nil {
		return nil
	}
	// Try setting upstream
	if _, upErr := r.run("push", "-u", "origin", branch); upErr != nil {
		return fmt.Errorf("git push %s: %w", branch, upErr)
	}
	return nil
}

// Stash saves tracked uncommitted changes to the stash.
// It returns false when no stash entry was created, such as an untracked-only
// dirty tree. Ralph must not pop a stash in that case or it may pop an older one.
func (r *Runner) Stash() (bool, error) {
	out, err := r.run("stash", "push", "-m", "ralph-pre-pull-stash")
	if err != nil {
		// Some git versions exit non-zero when there is nothing to stash.
		if strings.Contains(err.Error(), "No local changes to save") {
			return false, nil
		}
		return false, fmt.Errorf("git stash: %w", err)
	}
	if strings.Contains(out, "No local changes to save") {
		return false, nil
	}
	return true, nil
}

// StashPop restores the most recent stash entry.
// Returns nil when there are no stash entries (e.g., stash push was a no-op
// because only untracked files were present).
func (r *Runner) StashPop() error {
	_, err := r.run("stash", "pop")
	if err != nil {
		if strings.Contains(err.Error(), "No stash entries found") {
			return nil
		}
		return fmt.Errorf("git stash pop: %w", err)
	}
	return nil
}

// LastCommit returns the short SHA and message of the most recent commit.
func (r *Runner) LastCommit() (string, error) {
	out, err := r.run("log", "-1", "--format=%h %s")
	if err != nil {
		return "", fmt.Errorf("git last commit: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// Revert reverts the commit at the given SHA without opening an editor.
func (r *Runner) Revert(sha string) error {
	_, err := r.run("revert", sha, "--no-edit")
	if err != nil {
		return fmt.Errorf("git revert %s: %w", sha, err)
	}
	return nil
}

// HasRemoteBranch returns true if origin/<branch> exists.
func (r *Runner) HasRemoteBranch(branch string) bool {
	_, err := r.run("rev-parse", "--verify", fmt.Sprintf("origin/%s", branch))
	return err == nil
}

// DiffFromRemote returns true if HEAD differs from origin/<branch>.
// It distinguishes real diffs (exit code 1) from errors like a missing
// remote tracking branch (which produce "fatal:" in stderr).
func (r *Runner) DiffFromRemote(branch string) (bool, error) {
	_, err := r.run("diff", "--quiet", fmt.Sprintf("origin/%s", branch), "HEAD")
	if err == nil {
		return false, nil
	}
	// git diff --quiet exits 1 for real diffs, but other failures (e.g.,
	// missing remote ref) include "fatal:" in the error message.
	if strings.Contains(err.Error(), "fatal:") {
		return false, fmt.Errorf("git diff from remote: %w", err)
	}
	return true, nil
}

// run executes a git command and returns its combined output.
func (r *Runner) run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = strings.TrimSpace(stdout.String())
		}
		return "", fmt.Errorf("%s: %w", errMsg, err)
	}
	return stdout.String(), nil
}
