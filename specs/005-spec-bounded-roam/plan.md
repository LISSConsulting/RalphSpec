# Implementation Plan: Spec-Bounded Loop with --roam Flag

**Branch**: `005-spec-bounded-roam` | **Date**: 2026-03-02 | **Spec**: `specs/005-spec-bounded-roam/spec.md`
**Input**: Feature specification from `/specs/005-spec-bounded-roam/spec.md`

## Summary

Ralph must respect spec boundaries by default: when the active spec's work is complete (Claude reports success + no new commits on the next iteration), the loop exits early. A new `--roam` flag enables cross-spec improvement sweeps on a `sweep/YYYY-MM-DD` branch. Prompt augmentation guides Claude to stay within the active spec (default) or sweep all specs (roam mode).

## Technical Context

**Language/Version**: Go 1.24
**Primary Dependencies**: cobra (CLI), BurntSushi/toml (config), bubbletea + lipgloss (TUI), bubbles (list/viewport/textinput)
**Storage**: JSONL session logs (`internal/store`)
**Testing**: `go test ./...` with table-driven tests, `go vet ./...`
**Target Platform**: darwin/arm64, darwin/amd64, linux/amd64, windows/amd64
**Project Type**: CLI tool
**Performance Goals**: N/A (CLI tool, no latency-sensitive paths)
**Constraints**: No new dependencies; stdlib + approved deps only
**Scale/Scope**: Single-user CLI; iteration count typically < 100

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Spec-Driven | PASS | Feature originates from `specs/005-spec-bounded-roam/spec.md` with 14 FRs, 3 user stories, edge cases |
| II. Supervised Autonomy | PASS | Regent continues to supervise; roam mode does not bypass supervision |
| III. Test-Gated Commits | PASS | Regent rollback still applies in roam mode; no changes to test gating |
| IV. Idiomatic Go | PASS | No new deps; uses existing patterns (interfaces, small packages, explicit errors) |
| V. Observable Loops | PASS | New log kinds (`LogSpecComplete`, `LogSweepComplete`) ensure completion events are visible in TUI and logs |

**Gate result: PASS — no violations.**

## Project Structure

### Documentation (this feature)

```text
specs/005-spec-bounded-roam/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
└── tasks.md             # Phase 2 output (/speckit.tasks command)
```

### Source Code (repository root)

```text
internal/
├── loop/
│   ├── loop.go          # MODIFY: add completion detection, prompt augmentation, roam orchestration
│   └── event.go         # MODIFY: add LogSpecComplete, LogSweepComplete log kinds
├── git/
│   └── git.go           # MODIFY: add CreateAndCheckout method
├── config/
│   └── config.go        # MODIFY: add Roam field to BuildConfig, validation
└── spec/
    └── resolve.go       # READ ONLY: use existing Resolve() for prompt augmentation

cmd/ralph/
├── commands.go          # MODIFY: add --roam flag to build/loop build/loop run commands
└── execute.go           # MODIFY: add roam pre-flight (branch creation, --spec conflict check)
```

**Structure Decision**: All changes are within existing packages. No new packages needed. The loop package gains the most complexity (completion state machine + prompt augmentation). The git package gets one new method. Config gets one new field.

## Complexity Tracking

> No constitution violations — this section is empty.

---

## Phase 0: Research

### R1: Two-Signal Completion Detection Strategy

**Decision**: Track completion state with two fields on the Loop struct: `prevSubtype string` (the result subtype from the previous iteration) and a per-iteration commit check (compare `LastCommit()` before and after Claude runs).

**Rationale**: The spec requires two consecutive signals (FR-004): first a `"success"` result subtype, then a no-commit iteration. This is a simple state machine with two states: "saw success" and "confirmed idle". Using `LastCommit()` (already on `GitOps` interface) before and after the Claude agent run within `iteration()` gives a commit-produced boolean without adding new git methods.

**Alternatives considered**:
- Counting all commits via `git rev-list --count`: Over-engineered. `LastCommit()` hash comparison is simpler and sufficient.
- Checking `DiffFromRemote()`: Wrong signal — it checks remote diff, not whether Claude made new commits.
- Adding a `CommitCount()` method: Unnecessary when hash comparison works.

### R2: Sweep Branch Naming and Collision Handling

**Decision**: Use `sweep/YYYY-MM-DD` format. On collision, append `-N` suffix (starting at `-2`). Implementation: try `git checkout -b sweep/YYYY-MM-DD`, if it fails with "already exists", try `-2`, `-3`, up to a reasonable limit (10).

**Rationale**: The spec says "Ralph appends a sequence number (e.g., `sweep/YYYY-MM-DD-2`) or reuses the existing branch." Creating a new branch is safer than reusing — it preserves a clean diff for review. The suffix approach is simple and predictable.

**Alternatives considered**:
- Reusing the existing branch: Merges old sweep work with new, making PR review harder.
- Using timestamps (HH-MM-SS): Too fine-grained; the date-based naming is more readable.
- Random suffixes: Not predictable for the user.

### R3: Prompt Augmentation Injection Point

**Decision**: Augment the prompt string in `Loop.Run()` after reading the prompt file and before passing it to `iteration()`. The loop will call `spec.Resolve()` to get the active spec (if any), then append a spec-boundary directive or sweep directive to the prompt string.

**Rationale**: The prompt is currently a raw file read passed unchanged. Injecting at the `Run()` level (not `iteration()`) means the augmentation happens once per loop run, not per iteration. The spec context doesn't change mid-loop. This keeps `iteration()` clean.

**Alternatives considered**:
- Modifying the prompt file on disk: Destructive and error-prone.
- Injecting per-iteration: Unnecessary overhead; spec context is static within a run.
- Adding a `PromptMiddleware` interface: Over-engineered for a single augmentation point.

### R4: --roam and --spec Mutual Exclusion

**Decision**: Validate in `executeLoop()` / `executeRun()` before building the loop. If both `--roam` and resolved spec (via `--spec` flag) are set, return a clear error. The `--roam` flag is checked at the command level, not in `Loop.Run()`.

**Rationale**: FR-010 requires a clear error when both are provided. Validating early (before TUI/Regent setup) gives the user immediate feedback. The loop itself doesn't need to know about the flag conflict — it receives either roam=true or a spec, never both.

**Alternatives considered**:
- Validating inside `Loop.Run()`: Too late; TUI and Regent are already initialized.
- Making `--roam` imply `--spec ""`: Implicit behavior is confusing.

### R5: Roam Mode Integration with Loop.Run()

**Decision**: Add a `Roam bool` field to the `Loop` struct. When `Roam` is true:
1. `Run()` skips spec resolution (no single-spec prompt augmentation).
2. `Run()` injects a sweep directive into the prompt instead.
3. Completion detection emits `LogSweepComplete` instead of `LogSpecComplete`.
4. The sweep branch is created in `executeLoop()` / `executeRun()` before `Loop.Run()` is called.

**Rationale**: The sweep branch creation is a pre-flight step (git operation) that belongs in the command layer. The loop itself just needs to know "I'm in roam mode" for prompt augmentation and log entry selection. Keeping git branch creation outside the loop avoids adding branch-creation logic to the loop package.

**Alternatives considered**:
- Creating the sweep branch inside `Loop.Run()`: Mixes git orchestration with loop logic. The loop doesn't create branches today.
- Passing a `RoamConfig` struct: Over-engineered for a single boolean.

### R6: GitOps Interface Extension

**Decision**: Do NOT extend the `GitOps` interface in `loop.go`. Instead, add `CreateAndCheckout(name string) error` to `git.Runner` directly. The sweep branch creation happens in `cmd/ralph/execute.go` which already has access to `*git.Runner` (not through the `GitOps` interface). The loop doesn't need to create branches.

**Rationale**: The `GitOps` interface is the loop's view of git. The loop doesn't create branches — that's orchestration done by the command layer. Adding methods to an interface that only one call site needs violates interface segregation. The command layer already constructs `git.NewRunner(dir)` and can call `gitRunner.CreateAndCheckout()` directly.

**Alternatives considered**:
- Extending `GitOps` with `CreateAndCheckout`: Pollutes the loop's interface; loop never calls it.
- Creating a separate `BranchCreator` interface: Over-engineered for one call site.

---

## Phase 1: Design

*See `data-model.md` for entity definitions, `contracts/cli-flags.md` for CLI contract, and `quickstart.md` for usage examples.*

## Constitution Re-Check (Post-Design)

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Spec-Driven | PASS | All design artifacts trace to spec FRs. No scope creep. |
| II. Supervised Autonomy | PASS | Regent supervision unaffected. Roam mode runs under Regent like default mode. |
| III. Test-Gated Commits | PASS | Completion detection is orthogonal to test gating. Regent rollback applies equally in roam. |
| IV. Idiomatic Go | PASS | No new deps. `iteration()` return change is a local refactor. `CreateAndCheckout` follows existing `Runner` method pattern. `Loop.Roam`/`Loop.Spec`/`Loop.SpecDir` are simple struct fields. |
| V. Observable Loops | PASS | `LogSpecComplete` and `LogSweepComplete` log kinds ensure all completion events are visible. Sweep branch creation is logged via `LogInfo`. |

**Post-design gate: PASS — no violations. Ready for task breakdown.**

## Generated Artifacts

| Artifact | Path | Status |
|----------|------|--------|
| Plan | `specs/005-spec-bounded-roam/plan.md` | Complete |
| Research | `specs/005-spec-bounded-roam/research.md` | Complete |
| Data Model | `specs/005-spec-bounded-roam/data-model.md` | Complete |
| CLI Contract | `specs/005-spec-bounded-roam/contracts/cli-flags.md` | Complete |
| Quickstart | `specs/005-spec-bounded-roam/quickstart.md` | Complete |
| Tasks | `specs/005-spec-bounded-roam/tasks.md` | Pending (`/speckit.tasks`) |
