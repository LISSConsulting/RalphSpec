# Tasks: Complete Codex Support

**Input**: Design documents from `specs/011-codex-support/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: Included because the implementation plan and constitution require regression coverage with `go test ./...` and `go vet ./...`.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel with other [P] tasks in the same phase because it touches different files and has no dependency on incomplete tasks
- **[Story]**: User story label for traceability
- Each task includes exact repository paths

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Establish the current Codex completion baseline and identify direct Claude-only paths before story work starts.

- [X] T001 Review current agent resolution and direct Claude construction call sites in `cmd/ralph/execute.go` and `internal/orchestrator/orchestrator.go`
- [X] T002 [P] Review current dashboard agent launch and status handling in `internal/tui/app.go`
- [X] T003 [P] Review current persisted agent metadata in `internal/store/` and `internal/regent/`
- [X] T004 [P] Review current Codex adapter and availability behavior in `internal/codex/`
- [X] T005 [P] Review current CLI help and README Codex references in `cmd/ralph/commands.go` and `README.md`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Create shared primitives that every story needs before user-facing behavior is completed.

**Critical**: No user story work should begin until this phase is complete.

- [X] T006 Add failing tests for shared agent resolution precedence and invalid selection handling in `cmd/ralph/execute_test.go`
- [X] T007 Add failing tests for Codex factory availability errors and Claude fallback prevention in `cmd/ralph/execute_test.go`
- [X] T008 Refactor shared agent resolution, validation, and construction helpers in `cmd/ralph/execute.go`
- [X] T009 Add configurable working-directory support to Codex agent construction in `internal/codex/agent.go`
- [X] T010 [P] Add shared agent identity fields or helpers needed by run records in `internal/loop/loop.go`
- [X] T011 [P] Add shared agent identity fields or helpers needed by stored summaries in `internal/store/store.go`
- [X] T012 [P] Add shared agent identity fields or helpers needed by Regent state in `internal/regent/regent.go`
- [X] T013 Run targeted foundation tests with `go test ./cmd/ralph ./internal/codex ./internal/loop ./internal/store ./internal/regent`

**Checkpoint**: Agent selection, construction, and metadata primitives are ready for story implementation.

---

## Phase 3: User Story 1 - Use Codex Across All Ralph Workflows (Priority: P1) MVP

**Goal**: Codex works anywhere Ralph's main user-facing workflows delegate to an AI coding agent, including CLI loops, roam runs, dashboard-started runs, and automated build loops.

**Independent Test**: Select Codex and complete each main user-facing Ralph workflow that delegates agent work, verifying the workflow starts, reports progress, completes, and records Codex without requiring a different agent.

### Tests for User Story 1

- [X] T014 [P] [US1] Add CLI contract tests for `--agent codex` across build, roam/run, and top-level build paths in `cmd/ralph/commands_test.go`
- [X] T015 [P] [US1] Add execution tests for Codex plan/build/smart-run metadata and no silent fallback in `cmd/ralph/execute_test.go`
- [X] T016 [P] [US1] Add loop event attribution tests for Codex-backed runs in `internal/loop/loop_test.go`
- [X] T017 [P] [US1] Add TUI main workflow launch tests for Codex-selected dashboard actions in `internal/tui/app_test.go`

### Implementation for User Story 1

- [X] T018 [US1] Remove dashboard-specific Codex rejection and route dashboard-started loops through shared agent setup in `cmd/ralph/execute.go`
- [X] T019 [US1] Pass effective agent identity from command setup into loop configuration and live output in `cmd/ralph/execute.go`
- [X] T020 [US1] Preserve effective agent identity in loop events, cancellation, completion, and error paths in `internal/loop/loop.go`
- [X] T021 [US1] Update TUI run launch actions to honor project default and selected run agent in `internal/tui/app.go`
- [X] T022 [US1] Update TUI active run and log views to display Codex consistently for main workflows in `internal/tui/app.go`
- [X] T023 [US1] Persist Codex as the producing agent for main workflow summaries in `internal/store/store.go`
- [X] T024 [US1] Verify User Story 1 with `go test ./cmd/ralph ./internal/loop ./internal/store ./internal/tui`

**Checkpoint**: Main CLI and dashboard workflows can use Codex end-to-end and remain independently testable.

---

## Phase 4: User Story 2 - Run Advanced Codex Modes Safely (Priority: P2)

**Goal**: Codex participates safely in worktree, parallel, and multi-role workflows with isolated state, logs, errors, and agent attribution.

**Independent Test**: Start supported advanced workflows with Codex selected, including at least one concurrent or isolated run, and verify isolation, status reporting, completion handling, and failure containment.

### Tests for User Story 2

- [X] T025 [P] [US2] Add orchestrator tests proving Codex worktree agents use selected agent and isolated working directories in `internal/orchestrator/orchestrator_test.go`
- [X] T026 [P] [US2] Add orchestrator fan-in tests proving merged events retain branch and agent attribution in `internal/orchestrator/fanin_test.go`
- [X] T027 [P] [US2] Add TUI worktree panel tests for Codex agent display and per-session failure state in `internal/tui/panels_test.go`
- [X] T028 [P] [US2] Add command execution tests proving `--worktree --agent codex` is accepted and invalid agents still fail in `cmd/ralph/execute_test.go`

### Implementation for User Story 2

- [X] T029 [US2] Replace direct Claude worktree agent construction with shared configured agent construction in `internal/orchestrator/orchestrator.go`
- [X] T030 [US2] Add effective agent fields to worktree agent state and snapshots in `internal/orchestrator/orchestrator.go`
- [X] T031 [US2] Propagate selected agent into worktree launch requests from the dashboard in `internal/tui/app.go`
- [X] T032 [US2] Update worktree panel rendering to show each session's selected agent in `internal/tui/panels.go`
- [X] T033 [US2] Preserve per-session Codex errors without mutating unrelated sessions in `internal/orchestrator/orchestrator.go`
- [X] T034 [US2] Remove Codex worktree rejection once orchestrator support is complete in `cmd/ralph/execute.go`
- [X] T035 [US2] Verify User Story 2 with `go test ./cmd/ralph ./internal/orchestrator ./internal/tui`

**Checkpoint**: Advanced Codex sessions are isolated, attributed, and independently recoverable.

---

## Phase 5: User Story 3 - Configure, Inspect, and Troubleshoot Codex (Priority: P3)

**Goal**: Operators can configure Codex, inspect readiness and active selection, troubleshoot failures, and follow documented setup without source-code inspection.

**Independent Test**: Review Codex configuration and documentation, run health checks for available and unavailable Codex environments, and verify live and historical views expose Codex status and actionable failures.

### Tests for User Story 3

- [X] T036 [P] [US3] Add config tests for Codex defaults, invalid agent values, and project default preservation in `internal/config/config_test.go`
- [X] T037 [P] [US3] Add Codex availability diagnostics tests for missing and failing CLI behavior in `internal/codex/codex_test.go`
- [X] T038 [P] [US3] Add status output tests for active and historical agent display in `cmd/ralph/execute_test.go`
- [X] T039 [P] [US3] Add documentation smoke checks for Codex commands and troubleshooting examples in `./README.md`

### Implementation for User Story 3

- [X] T040 [US3] Improve Codex availability error classification and actionable messages in `internal/codex/agent.go`
- [X] T041 [US3] Surface Codex availability and selected-agent state in command status output in `cmd/ralph/execute.go`
- [X] T042 [US3] Surface Codex availability and selected-agent state in dashboard secondary/status panels in `internal/tui/panels/worktrees.go`
- [X] T043 [US3] Preserve backward-compatible reads of older run state without agent metadata in `internal/store/store.go`
- [X] T044 [US3] Update command help for project defaults, per-run overrides, and worktree Codex support in `cmd/ralph/commands.go`
- [X] T045 [US3] Update Codex setup, usage, dashboard, worktree, and troubleshooting documentation in `./README.md`
- [X] T046 [US3] Verify User Story 3 with `go test ./internal/config ./internal/codex ./cmd/ralph ./internal/store ./internal/tui`

**Checkpoint**: Codex is configurable, inspectable, documented, and diagnosable across normal operator surfaces.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Validate the complete feature, ensure documentation matches behavior, and remove stale partial-support assumptions.

- [X] T047 [P] Remove stale comments or tests that describe Codex dashboard/worktree support as unsupported in `cmd/ralph/execute_test.go`
- [X] T048 [P] Remove stale comments or tests that describe direct Claude-only orchestration in `internal/orchestrator/orchestrator_test.go`
- [X] T049 [P] Validate quickstart commands and update expected results in `specs/011-codex-support/quickstart.md`
- [X] T050 Run full repository tests with `go test ./...`
- [X] T051 Run full repository vet checks with `go vet ./...`
- [X] T052 Review implementation against FR-001 through FR-015 and record any follow-up gaps in `specs/011-codex-support/tasks.md`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies; can start immediately.
- **Foundational (Phase 2)**: Depends on Setup completion; blocks all user stories.
- **User Story 1 (Phase 3)**: Depends on Foundational completion; MVP scope.
- **User Story 2 (Phase 4)**: Depends on Foundational completion and benefits from US1 shared launch/metadata behavior.
- **User Story 3 (Phase 5)**: Depends on Foundational completion and can proceed alongside US1/US2 after shared metadata contracts are stable.
- **Polish (Phase 6)**: Depends on all desired stories being complete.

### User Story Dependencies

- **US1 (P1)**: Can start after Phase 2; no dependency on US2 or US3.
- **US2 (P2)**: Can start after Phase 2, but should integrate with US1 shared agent setup when available.
- **US3 (P3)**: Can start after Phase 2; documentation should be finalized after US1 and US2 behavior is settled.

### Within Each User Story

- Tests are written first and should fail before implementation.
- Shared models/helpers before service or workflow integration.
- Command/dashboard/orchestrator behavior before documentation finalization.
- Story checkpoint must pass before treating the story as complete.

### Parallel Opportunities

- T002 through T005 can run in parallel after T001 begins.
- T010 through T012 can run in parallel after T008 establishes shared resolution boundaries.
- US1 test tasks T014 through T017 can run in parallel.
- US2 test tasks T025 through T028 can run in parallel.
- US3 test tasks T036 through T039 can run in parallel.
- Polish cleanup tasks T047 through T049 can run in parallel.

---

## Parallel Example: User Story 1

```text
Task: "T014 Add CLI contract tests for --agent codex across build, roam/run, and top-level build paths in cmd/ralph/commands_test.go"
Task: "T016 Add loop event attribution tests for Codex-backed runs in internal/loop/loop_test.go"
Task: "T017 Add TUI main workflow launch tests for Codex-selected dashboard actions in internal/tui/app_test.go"
```

## Parallel Example: User Story 2

```text
Task: "T025 Add orchestrator tests proving Codex worktree agents use selected agent and isolated working directories in internal/orchestrator/orchestrator_test.go"
Task: "T026 Add orchestrator fan-in tests proving merged events retain branch and agent attribution in internal/orchestrator/fanin_test.go"
Task: "T027 Add TUI worktree panel tests for Codex agent display and per-session failure state in internal/tui/panels_test.go"
```

## Parallel Example: User Story 3

```text
Task: "T036 Add config tests for Codex defaults, invalid agent values, and project default preservation in internal/config/config_test.go"
Task: "T037 Add Codex availability diagnostics tests for missing and failing CLI behavior in internal/codex/codex_test.go"
Task: "T038 Add status output tests for active and historical agent display in cmd/ralph/status_test.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1 and Phase 2.
2. Complete Phase 3 for US1.
3. Stop and validate main CLI and dashboard Codex workflows independently.
4. Do not start advanced workflow claims until US2 is implemented.

### Incremental Delivery

1. Setup plus foundational shared resolution and metadata primitives.
2. US1 adds full Codex support to main workflows and establishes MVP value.
3. US2 adds worktree, parallel, and multi-role safety.
4. US3 adds operator-facing diagnostics, status, and documentation.
5. Polish validates `go test ./...`, `go vet ./...`, quickstart behavior, and FR-001 through FR-015 coverage.

### Parallel Team Strategy

1. Complete Phase 1 and Phase 2 as a shared baseline.
2. Assign US1 to command/loop/TUI workflow owner.
3. Assign US2 to orchestrator/worktree/TUI panel owner.
4. Assign US3 to config/status/Codex diagnostics/docs owner.
5. Rejoin for full test, vet, quickstart, and requirement traceability validation.

## Notes

- [P] tasks use different files or independent validation work.
- Every user story phase has an independently testable checkpoint.
- Preserve existing Claude behavior unless Codex is explicitly selected.
- Never silently fall back from Codex to another agent.
