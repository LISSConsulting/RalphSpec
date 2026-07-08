# Research: Spec-Bounded Loop with --roam Flag

**Feature**: 005-spec-bounded-roam | **Date**: 2026-03-02

## R1: Two-Signal Completion Detection Strategy

**Decision**: Track completion with a `prevSubtype string` local variable in `Run()` and a per-iteration commit check via `LastCommit()` hash comparison before/after Claude runs.

**Rationale**: FR-004 requires two consecutive signals: a `"success"` result subtype followed by a no-commit iteration. This is a simple state machine:

```
[normal] --success subtype--> [sawSuccess]
[sawSuccess] --no commits--> COMPLETE (emit LogSpecComplete or LogSweepComplete)
[sawSuccess] --has commits--> [normal] (Claude found more work, reset)
[normal] --non-success subtype--> [normal]
```

The `prevSubtype` variable carries state across iterations within a single `Run()` call. It resets naturally when `Run()` returns.

**Edge case (FR-004)**: Single-iteration runs (`--max 1`) cannot trigger completion — there is no "next iteration" to confirm. The state machine handles this naturally: the first iteration may set `prevSubtype = "success"`, but the loop exits at `i > maxIter` before a second iteration can confirm.

**Alternatives considered**:
- Claude self-report only: Unreliable — Claude says "done" then finds more work. Rejected.
- No-commit only: Would trigger on "thinking" iterations where Claude analyzes without committing. Rejected.
- tasks.md parsing: Brittle, not all specs use tasks.md. Rejected.
- Three-signal (success + no-commit + no-commit): Wastes an extra iteration. Rejected for v1.
- `git rev-list --count`: Over-engineered; hash comparison suffices. Rejected.

## R2: Commit Detection via HEAD Comparison

**Decision**: Compare `LastCommit()` output (short hash) before and after Claude runs within `iteration()`.

**Rationale**: `LastCommit()` already exists on `GitOps` interface (returns `%h %s` format). HEAD changes if and only if new commits exist. The "before" snapshot is taken after git pull (post-rebase), so rebases don't produce false positives.

**Implementation**: `iteration()` currently returns `(float64, error)` — cost and error. It will now also return a `bool` for commits-produced. The return signature becomes `(float64, string, bool, error)` where the string is the result subtype and the bool is commits-produced.

**Alternatives considered**:
- Track push events: Only works with auto_push. Rejected.
- `git diff --stat`: Shows uncommitted changes, not commits. Rejected.
- New `CommitCount()` method: Unnecessary when hash comparison works. Rejected.

## R3: Sweep Branch Architecture (Roam Mode)

**Decision**: Roam creates a single `sweep/YYYY-MM-DD` branch from the current branch. Claude runs on this one branch with all specs visible. No branch switching during the sweep.

**Rationale**: The spec is explicit: "Single branch. Roam creates a `sweep/YYYY-MM-DD` branch from develop. All specs are visible on this branch. No branch-switching during the sweep." Roam is an improvement sweep, not feature development. Claude checks code against specs, finds gaps, missing tests, and fixes — all from one branch.

**Branch collision handling**: On collision (branch already exists), append `-N` suffix starting at `-2`, up to `-10`. Creating a new branch is safer than reusing — preserves clean PR diffs.

**Implementation**:
```go
// In executeLoop(), before building the Loop:
base := fmt.Sprintf("sweep/%s", time.Now().Format("2006-01-02"))
name := base
for i := 2; i <= 10; i++ {
    if err := gitRunner.CreateAndCheckout(name); err == nil { break }
    name = fmt.Sprintf("%s-%d", base, i)
}
```

**Alternatives considered**:
- Spec-to-spec branch switching: Contradicts the spec clarification. Rejected.
- Reusing existing branch: Merges old sweep work, harder PR review. Rejected.
- Timestamps (HH-MM-SS): Too fine-grained. Rejected.

## R4: GitOps Interface — No Extension Needed

**Decision**: Add `CreateAndCheckout(name string) error` to `git.Runner` only. Do NOT extend the `GitOps` interface in `loop.go`.

**Rationale**: The loop doesn't create branches — that's orchestration done by the command layer (`executeLoop()`). `executeLoop()` already constructs `git.NewRunner(dir)` and can call `gitRunner.CreateAndCheckout()` directly. Adding to `GitOps` would pollute the loop's interface with a method it never calls.

**Method**:
```go
func (r *Runner) CreateAndCheckout(name string) error {
    _, err := r.run("checkout", "-b", name)
    if err != nil {
        return fmt.Errorf("git create branch %s: %w", name, err)
    }
    return nil
}
```

**Alternatives considered**:
- Extending `GitOps`: Violates interface segregation; loop never calls it. Rejected.
- `CheckoutOrCreate` (try existing, then create): Roam always creates new branches; "try existing" path is dead code. Rejected.
- Separate `BranchCreator` interface: Over-engineered for one call site. Rejected.

## R5: Prompt Augmentation Strategy

**Decision**: Augment the prompt string in `Loop.Run()` after `os.ReadFile()`, before the iteration loop. Append a `\n\n## Spec Context\n` section. The augmented prompt is passed to all iterations (it doesn't change mid-loop).

**Rationale**: The prompt file (BUILD.md) is a static template. Dynamic injection means no disk mutation, no per-spec prompt files, and fresh context. The `spec.Resolve()` function handles branch→spec resolution.

**Prompt templates**:

Default mode (spec resolved):
```
## Spec Context

You are working on spec "005-my-feature" in directory specs/005-my-feature/.
Focus ONLY on work defined in this spec. Do not modify code outside this spec's scope.
Read spec.md, plan.md, and tasks.md from this directory for your work items.
```

Roam mode:
```
## Spec Context

You are performing an improvement sweep across ALL specs in specs/.
Check each spec directory for gaps, missing tests, code quality issues, and fixes.
This is not feature development — focus on quality improvements and consistency.
```

No spec resolved (backwards-compatible):
```
(no augmentation — raw prompt file passed unchanged)
```

**Alternatives considered**:
- Modify BUILD.md on disk: Risky, gets committed. Rejected.
- Per-spec BUILD.md: Duplicates content, maintenance burden. Rejected.
- `PromptMiddleware` interface: Over-engineered for one augmentation point. Rejected.
- Per-iteration injection: Unnecessary overhead; spec context is static within a run. Rejected.

## R6: Config Integration

**Decision**: Add dedicated roam configuration. CLI `--roam` flag overrides config.

**Rationale**: Config-level default lets projects opt into roam permanently. CLI override gives per-invocation control. Roam has its own prompt file.

**`ralph.toml` example**:
```toml
[roam]
enabled = false  # default; set true for always-roam
prompt_file = "ROAM.md"
max_iterations = 10
```

**Validation**: `--roam` and `--spec` are mutually exclusive (FR-010). Checked in `executeLoop()` before TUI/Regent setup.
