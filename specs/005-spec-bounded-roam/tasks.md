# Tasks: Spec-Bounded Loop with --roam Flag

**Input**: Design documents from `/specs/005-spec-bounded-roam/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/cli-flags.md, quickstart.md

**Tests**: Included (constitution principle III mandates test-first development).

**Organization**: Tasks grouped by user story for independent implementation and testing.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: Foundational (Blocking Prerequisites)

**Purpose**: Add new log kinds used by US1 and US2. Must complete before user story implementation.

- [x] T001 Add `LogSpecComplete` and `LogSweepComplete` log kinds to the `LogKind` enum in `internal/loop/event.go`. Insert after `LogRegent`. These distinguish "single spec done" (default mode) from "sweep done" (roam mode) without parsing message text. No new `LogEntry` fields needed.

**Checkpoint**: Log kinds exist. User story implementation can begin.

---

## Phase 2: User Story 1 — Default Loop Stops at Spec Boundary (Priority: P1) 🎯 MVP

**Goal**: When Claude reports success and the next iteration produces no new commits, Ralph exits the loop early with a "spec complete" message instead of consuming the full iteration budget.

**Independent Test**: Run a loop with a mock agent that reports `"success"` and a mock git that returns the same `LastCommit()` hash before and after Claude runs. Verify the loop exits early and emits `LogSpecComplete`.

### Tests for User Story 1

> Write these tests FIRST. They MUST fail before implementation.

- [x] T002 [US1] Write table-driven tests for spec completion detection in `internal/loop/loop_test.go`. Enhance `mockGit` to support a `lastCommitSequence []string` field so `LastCommit()` returns different values on successive calls (simulating commit/no-commit). Enhance `mockAgent` to capture `lastPrompt string` from `Run()` calls (needed later for US3). Test cases covering all acceptance scenarios:
  - *success + no-commits → LogSpecComplete emitted, Run() returns nil*
  - *success + has-commits → loop continues (Claude found more work)*
  - *max iterations reached with commits every iteration → LogDone (normal exit)*
  - *single iteration (--max 1) with success + no-commits → LogDone (no early exit — no "next iteration" to confirm)*
  - *error_max_turns subtype + no-commits → loop continues (not a completion signal)*
  - *two successes in a row, second has no commits → complete on second (prevSubtype carries)*

### Implementation for User Story 1

- [x] T003 [US1] Refactor `iteration()` return signature in `internal/loop/loop.go` from `(float64, error)` to `(cost float64, subtype string, commitsProduced bool, err error)`. Inside `iteration()`: (a) capture `headBefore, _ := l.Git.LastCommit()` before calling `l.Agent.Run()`, (b) capture `headAfter, _ := l.Git.LastCommit()` after the event drain loop completes (after push), (c) extract `subtype` from the `claude.EventResult` event (already captured in the drain loop), (d) set `commitsProduced = headBefore != headAfter`, (e) return all four values. Update the single call site in `Run()` to destructure the new return values.

- [x] T004 [US1] Add completion state machine in `Run()` in `internal/loop/loop.go`. Declare `var prevSubtype string` before the iteration loop. After each `iteration()` call and before the `PostIteration` hook: if `prevSubtype == "success" && !commitsProduced`, emit a `LogSpecComplete` entry with message `fmt.Sprintf("Spec complete (%d iterations)", i)` and `return nil`. Otherwise, set `prevSubtype = subtype` and continue the loop. When `l.Roam` is true, emit `LogSweepComplete` instead (but `l.Roam` doesn't exist yet — use `LogSpecComplete` for now; US2 will add the roam branch).

**Checkpoint**: `go test ./internal/loop/...` passes. Loop exits early on success + no-commits. All existing tests still pass (they use `mockGit.lastCommit` which always returns the same value, so `commitsProduced` is always false — but `prevSubtype` is only `"success"` when the mock returns a success event, matching existing behavior).

---

## Phase 3: User Story 2 — Roam Mode Sweeps All Specs (Priority: P2)

**Goal**: `--roam` flag creates a `sweep/YYYY-MM-DD` branch and runs an improvement sweep across all specs. Uses the same completion detection as US1 but emits `LogSweepComplete`.

**Independent Test**: Run `executeLoop` with `--roam` in a temp git repo. Verify sweep branch is created, loop starts, and `LogSweepComplete` is emitted on completion.

**Depends on**: US1 (completion detection state machine must be in place).

### Tests for User Story 2

- [x] T005 [P] [US2] Write tests for `CreateAndCheckout` in `internal/git/git_test.go`. Use `initTestRepo` helper (real git repo pattern). Test cases: (a) create new branch succeeds, `CurrentBranch()` returns new name, (b) creating a branch that already exists returns an error.

- [x] T006 [P] [US2] Write tests for `Roam` config field in `internal/config/config_test.go`. Test cases: (a) `Defaults()` returns `Roam: false`, (b) TOML with `roam = true` under `[build]` parses correctly, (c) TOML with unknown key near `roam` is rejected (existing undecoded-keys behavior).

### Implementation for User Story 2

- [x] T007 [P] [US2] Add `CreateAndCheckout(name string) error` method to `git.Runner` in `internal/git/git.go`. Implementation: `_, err := r.run("checkout", "-b", name)` with error wrapping `fmt.Errorf("git create branch %s: %w", name, err)`. Do NOT add to `GitOps` interface — this method is only called by `executeLoop()` which has `*git.Runner` directly.

- [x] T008 [P] [US2] Add `Roam bool` field to `BuildConfig` in `internal/config/config.go` with TOML tag `toml:"roam"`. Update `Defaults()` to set `Roam: false`. No validation needed (bool field, default false). Update `InitFile()` template to include `roam = false` under `[build]` with comment `# enable cross-spec improvement sweep`.

- [x] T009 [US2] Add `--roam` flag to `buildCmd()`, `loopBuildCmd()`, and `loopRunCmd()` in `cmd/ralph/commands.go`. Pattern: `cmd.Flags().Bool("roam", false, "enable cross-spec improvement sweep")`. Parse with `roam, _ := cmd.Flags().GetBool("roam")`. Pass `roam` as a new parameter to `executeLoop()` and `executeRun()`.

- [x] T010 [US2] Update `executeLoop()` signature in `cmd/ralph/execute.go` to accept `roam bool` parameter. Add roam pre-flight before building the Loop struct: (a) resolve effective roam: `effectiveRoam := roam || cfg.Build.Roam`, (b) if `effectiveRoam`, create sweep branch using `gitRunner.CreateAndCheckout()` with date-based naming (`sweep/YYYY-MM-DD`) and collision retry (append `-2` through `-10`), (c) log branch creation via `fmt.Fprintf`, (d) set `lp.Roam = true` on the Loop struct. Add `Roam bool` field to the `Loop` struct in `internal/loop/loop.go`.

- [x] T011 [US2] Update `executeRun()` signature in `cmd/ralph/execute.go` to accept `roam bool` parameter. Apply the same roam pre-flight logic as `executeLoop()` and set `lp.Roam`; `lp.Run(ctx, loop.ModeBuild, maxOverride)` uses the Loop's Roam field.

- [x] T012 [US2] Update completion detection in `Run()` in `internal/loop/loop.go` to emit `LogSweepComplete` when `l.Roam` is true (instead of `LogSpecComplete`). Change the completion block from T004: `if l.Roam { l.emit(LogEntry{Kind: LogSweepComplete, Message: fmt.Sprintf("Sweep complete (%d iterations, $%.2f)", i, totalCost)}) } else { l.emit(LogEntry{Kind: LogSpecComplete, ...}) }`.

- [x] T013 [US2] Write tests for roam orchestration in `cmd/ralph/execute_test.go`. Test cases: (a) `executeLoop` with `roam=true` in a temp git repo — verify sweep branch is created (check `CurrentBranch()` starts with `sweep/`), (b) `executeLoop` with `roam=true` when sweep branch already exists — verify collision suffix (`-2`), (c) verify `--roam` flag is registered on `buildCmd()`, `loopBuildCmd()`, `loopRunCmd()` (use `cmd.Flags().Lookup("roam") != nil`). Note: full end-to-end tests will fail at Claude invocation — test up to branch creation point.

**Checkpoint**: `go test ./...` passes. `--roam` flag creates sweep branch and is wired through to the loop. `LogSweepComplete` emitted in roam completion.

---

## Phase 4: User Story 3 — Prompt Guardrails Keep Claude in Scope (Priority: P3)

**Goal**: When a spec is resolved for the current branch, the prompt sent to Claude includes a scope directive. In roam mode, the prompt includes a sweep directive instead.

**Independent Test**: Run a loop with a mock agent, verify the prompt string passed to `agent.Run()` contains the expected spec context (or sweep directive, or nothing for no-spec case).

**Can run in parallel with US2** (no dependency on roam pre-flight or sweep branch logic — only needs `Loop.Roam` field which is added in US2 T010, but the prompt augmentation logic itself is independent).

### Tests for User Story 3

- [x] T014 [US3] Write tests for prompt augmentation in `internal/loop/loop_test.go`. Uses the `mockAgent.lastPrompt` field added in T002. Test cases: (a) when `Loop.Spec` is set and `Loop.Roam` is false, prompt contains `"## Spec Context"` and the spec name and directory, (b) when `Loop.Roam` is true, prompt contains `"## Spec Context"` and `"improvement sweep"` (regardless of Spec field), (c) when `Loop.Spec` is empty and `Loop.Roam` is false, prompt is unchanged from the raw file content (backwards-compatible).

### Implementation for User Story 3

- [x] T015 [US3] Add `Spec string` and `SpecDir string` fields to the `Loop` struct in `internal/loop/loop.go` (if not already added by US2 T010 — check before adding). `Spec` holds the resolved spec name (e.g., `"005-spec-bounded-roam"`), `SpecDir` holds the absolute path to the spec directory.

- [x] T016 [US3] Implement prompt augmentation in `Run()` in `internal/loop/loop.go`. After reading the prompt file and before the iteration loop, append a `"\n\n## Spec Context\n\n"` section to the prompt string. Three cases: (a) if `l.Roam`: append sweep directive — `"You are performing an improvement sweep across ALL specs in specs/.\nCheck each spec directory for gaps, missing tests, code quality issues, and fixes.\nThis is not feature development — focus on quality improvements and consistency."`, (b) else if `l.Spec != ""`: append spec-boundary directive — `fmt.Sprintf("You are working on spec %q in directory %s/.\nFocus ONLY on work defined in this spec. Do not modify code outside this spec's scope.\nRead spec.md, plan.md, and tasks.md from this directory for your work items.", l.Spec, l.SpecDir)`, (c) else: no augmentation (raw prompt, backwards-compatible).

- [x] T017 [US3] Wire spec resolution into `executeLoop()` and `executeRun()` in `cmd/ralph/execute.go`. When `!effectiveRoam`: call `spec.Resolve(dir, "", branch)` where `branch` comes from `gitRunner.CurrentBranch()`. If resolution succeeds, set `lp.Spec = activeSpec.Name` and `lp.SpecDir = activeSpec.Dir`. If resolution fails (no matching spec, main branch, etc.), silently continue — spec augmentation is optional for backwards compatibility. Import `"github.com/LISSConsulting/RalphSpec/internal/spec"` in execute.go.

**Checkpoint**: `go test ./...` passes. Prompt includes spec context when on a feature branch, sweep directive when roaming, nothing on main/master.

---

## Phase 5: Polish & Cross-Cutting Concerns

**Purpose**: Configuration, documentation, and validation across all user stories.

- [x] T018 Update `ralph.toml` at repo root to include `roam = false` under `[build]` with comment `# enable cross-spec improvement sweep (--roam flag overrides)`.

- [x] T019 Run `go test ./...`, `go vet ./...`, and `golangci-lint run` — verify zero failures and zero warnings. Fix any lint issues (ifElseChain, errcheck, etc. per project lint rules).

- [x] T020 Verify backwards compatibility: `ralph build` without `--roam` on a non-feature branch (e.g., main) behaves identically to pre-feature behavior — no spec augmentation, no early exit, full iteration budget consumed.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Foundational (Phase 1)**: No dependencies — start immediately
- **US1 (Phase 2)**: Depends on Phase 1 (LogSpecComplete kind must exist)
- **US2 (Phase 3)**: Depends on Phase 2 (completion state machine must be in place)
- **US3 (Phase 4)**: Depends on Phase 1 only (LogSpecComplete for reference). Can run in parallel with US2 if Loop.Roam field is added first.
- **Polish (Phase 5)**: Depends on all user stories being complete

### User Story Dependencies

- **US1 (P1)**: Start after Phase 1 — no dependencies on other stories
- **US2 (P2)**: Start after US1 — uses the same completion detection state machine, adds roam-specific behavior
- **US3 (P3)**: Start after Phase 1 — independent of US1 and US2. Only shares the `Loop.Roam` field (added in US2 T010), but prompt augmentation logic can be written and tested independently.

### Within Each User Story

- Tests written first (MUST fail before implementation)
- Internal changes (loop, git, config) before command-layer wiring
- Verify tests pass after implementation

### Parallel Opportunities

Within US2:
- T005 and T006 (tests) can run in parallel (different packages)
- T007 and T008 (implementations) can run in parallel (different packages)
- T009–T012 are sequential (command layer → loop → tests)

Across stories (with team capacity):
- US3 can start in parallel with US2 after US1 is complete

---

## Parallel Example: User Story 2

```text
# These can run in parallel (different packages):
T005: "Write tests for CreateAndCheckout in internal/git/git_test.go"
T006: "Write tests for Roam config field in internal/config/config_test.go"
T007: "Add CreateAndCheckout to git.Runner in internal/git/git.go"
T008: "Add Roam to BuildConfig in internal/config/config.go"

# Then sequential (same files, dependencies):
T009 → T010 → T011 → T012 → T013
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Foundational (T001)
2. Complete Phase 2: US1 — completion detection (T002–T004)
3. **STOP and VALIDATE**: `go test ./...` passes, loop exits early on spec completion
4. This alone delivers significant value — Ralph stops wasting iterations after specs are done

### Incremental Delivery

1. Phase 1 → Foundation ready
2. US1 → Spec-bounded loop works → Test independently (MVP!)
3. US2 → Roam mode works → Test sweep branch creation + completion
4. US3 → Prompt guardrails work → Test prompt content
5. Polish → Config, lint, backwards compat verified

### Task Count Summary

| Phase | Story | Tasks | Parallel |
|-------|-------|-------|----------|
| Foundational | — | 1 | — |
| US1 (P1) | Default Loop Stops at Spec Boundary | 3 | — |
| US2 (P2) | Roam Mode Sweeps All Specs | 9 | T005∥T006, T007∥T008 |
| US3 (P3) | Prompt Guardrails Keep Claude in Scope | 4 | — |
| Polish | — | 3 | — |
| **Total** | | **20** | |

---

## Notes

- [P] tasks = different files/packages, no dependencies on incomplete tasks
- [Story] label maps task to specific user story for traceability
- `mockGit` and `mockAgent` enhancements (T002) are prerequisites for all loop tests
- Completion detection (US1) is the MVP — delivers immediate value without roam or prompt changes
- The `--spec` flag does not exist on build commands today. The `--roam + --spec` conflict guard (FR-010) is a defensive check for future compatibility — currently unreachable.
- `Loop.Roam` is set by the command layer (executeLoop/executeRun), not by the loop itself
