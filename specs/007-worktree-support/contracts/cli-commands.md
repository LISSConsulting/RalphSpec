# CLI Contract: Worktree Commands

**Feature**: 007-worktree-support
**Date**: 2026-03-07

## New Commands

### `ralph worktree list`

Display all active worktrees and their agent status.

```
$ ralph worktree list

  Branch                  Status      Iter   Cost    Spec
  007-worktree-support    running     3      $0.36   007-worktree-support
  008-notifications       completed   7      $0.84   008-notifications
  009-api-cleanup         failed      2      $0.18   009-api-cleanup

  3 worktrees (1 running, 1 completed, 1 failed)
```

**Flags**: `--json` — output as JSON array for scripting.

**Exit codes**: 0 (success), 1 (error).

### `ralph worktree merge [branch]`

Merge a completed worktree's branch into the target branch via `wt merge` and clean up.

```
$ ralph worktree merge 008-notifications
Merging 008-notifications → develop... done
Worktree removed.
```

**Arguments**:
- `branch` (optional) — branch name to merge. If omitted and only one completed worktree exists, use it. If multiple, error with list.

**Flags**:
- `--target <branch>` — override merge target (default: config `merge_target` or worktree's base branch)
- `--no-remove` — merge but keep the worktree

**Preconditions**:
- Agent on target branch must not be running (error: "Agent still running on branch X — stop it first")
- Branch must be in completed or stopped state

**Exit codes**: 0 (merged), 1 (error/conflict).

### `ralph worktree clean [branch|--all]`

Remove worktree(s) and their branches without merging.

```
$ ralph worktree clean 009-api-cleanup
Removed worktree for 009-api-cleanup.

$ ralph worktree clean --all
Removed 2 worktrees (008-notifications, 009-api-cleanup).
```

**Arguments**:
- `branch` (optional) — specific branch to clean

**Flags**:
- `--all` — remove all completed and failed worktrees (does not touch running agents)
- `--force` — also remove stopped agents (not running, but not explicitly completed)

**Preconditions**:
- Agent must not be in running state (error: "Agent still running on branch X — stop it first")

**Exit codes**: 0 (removed), 1 (error).

## Modified Commands

### `ralph build` / `ralph loop build` / `ralph loop run`

**New flag**: `--worktree` / `-w`

```
$ ralph build --worktree
Creating worktree for 007-worktree-support...
Worktree created at ../RalphSpec.007-worktree-support
Starting build loop in worktree...
```

**Behavior**:
1. Detect worktrunk (`wt`) on PATH; error if missing
2. Create worktree via `wt switch -c <branch>` (or reuse existing via `wt switch <branch>`)
3. Set loop working directory to worktree path
4. Run loop as normal inside worktree
5. On completion, log result + worktree path; worktree remains for review

**Combines with**: `--roam`, `--max N`, `--no-tui`, `--no-color` (all existing flags work)

## TUI Dashboard Keybinds

New keybinds when worktree support is enabled:

| Key | Context | Action |
|-----|---------|--------|
| `W` | Specs panel, spec selected | Launch build in new worktree for selected spec |
| `M` | Worktrees panel, completed agent selected | Merge worktree (invoke `wt merge`) |
| `D` | Worktrees panel, completed/failed agent selected | Discard/clean worktree |
| `x` | Worktrees panel, running agent selected | Stop selected agent (graceful) |

## Configuration

### `ralph.toml` — new `[worktree]` section

```toml
[worktree]
enabled = false          # enable worktree support in dashboard mode
max_parallel = 5         # max concurrent worktree agents
auto_merge = false       # auto-merge on successful completion + test pass
merge_target = ""        # target branch for merge (empty = worktree's base branch)
path_template = ""       # worktrunk path template (empty = worktrunk default)
```
