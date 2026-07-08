# Tasks: Codex Agent Support

**Input**: Design documents from `specs/010-codex-support/`
**Prerequisites**: plan.md (required), spec.md (required), research.md, data-model.md, contracts/, quickstart.md

**Tests**: Included. The plan and constitution require table-driven Go tests plus `go test ./...` and `go vet ./...` validation.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g. US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Establish config surfaces and Codex package scaffolding used by all stories.

- [X] T001 Add `AgentConfig` and `CodexConfig` plus defaults and validation rules to `internal/config/config.go`
- [X] T002 [P] Update generated config scaffolding for `[agent]` and `[codex]` in `internal/config/scaffold.go` and `ralph.toml`
- [X] T003 [P] Add table-driven config and scaffold coverage for agent settings in `internal/config/config_test.go` and `internal/config/scaffold_test.go`
- [X] T004 Create Codex adapter package skeleton in `internal/codex/agent.go`, `internal/codex/parser.go`, and `internal/codex/parser_test.go`

**Checkpoint**: Config and package scaffolding exist; all user stories can build on the same selection and adapter surfaces.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Add shared agent metadata and agent-resolution plumbing required before any user story can be completed.

**⚠️ CRITICAL**: No user story work should begin until this phase is complete.

- [X] T005 Add `Agent` metadata to `internal/loop/event.go`, `internal/store/store.go`, and `internal/regent/state.go`
- [X] T006 [P] Add metadata round-trip coverage in `internal/store/jsonl_test.go` and `internal/regent/state_test.go`
- [X] T007 Implement effective-agent resolution and shared unsupported-flow guards in `cmd/ralph/execute.go`
- [X] T008 [P] Add `--agent` flag plumbing to supported commands in `cmd/ralph/commands.go` and registration coverage in `cmd/ralph/commands_test.go`
- [X] T009 [P] Make startup and loop messaging agent-agnostic in `internal/loop/loop.go` and `cmd/ralph/main.go`

**Checkpoint**: Effective-agent selection, metadata fields, and command surfaces are in place for all stories.

---

## Phase 3: User Story 1 - Run Ralph with Codex for Main Flows (Priority: P1) 🎯 MVP

**Goal**: A developer can run Codex in Ralph's supported single-run `plan`, `build`, and `run` flows with the same supervision and git lifecycle as Claude.

**Independent Test**: Configure or override a run to use Codex, execute `ralph loop build --agent codex --no-tui` or `ralph build --agent codex --no-tui`, and verify the session starts, streams structured progress, and completes through the normal loop flow.

### Tests for User Story 1

> Write these tests first. They must fail before implementation.

- [X] T010 [P] [US1] Write Codex subprocess and parser tests in `internal/codex/agent_test.go` and `internal/codex/parser_test.go`
- [X] T011 [P] [US1] Write supported-flow execution tests for Codex in `cmd/ralph/execute_test.go`

### Implementation for User Story 1

- [X] T012 [US1] Implement Codex JSONL event translation in `internal/codex/parser.go`
- [X] T013 [US1] Implement the `codex exec --json --dangerously-bypass-approvals-and-sandbox --cd` runner in `internal/codex/agent.go`
- [X] T014 [US1] Wire the Codex adapter into supported single-run paths in `cmd/ralph/execute.go`
- [X] T015 [US1] Emit effective-agent metadata in live loop output from `internal/loop/loop.go` and `cmd/ralph/format.go`

**Checkpoint**: Codex can drive Ralph's main single-run plan/build workflows end to end. MVP complete.

---

## Phase 4: User Story 2 - Choose the Agent Per Project or Run (Priority: P2)

**Goal**: A developer can set a project default agent and override it per run, with the selected agent visible in stored and live status surfaces.

**Independent Test**: Set one project to `agent.type = "codex"`, start a run without overrides, then start another run with `--agent claude` or `--agent codex` and verify precedence plus visibility in status/output.

### Tests for User Story 2

- [X] T016 [P] [US2] Write project-default and flag-precedence tests in `internal/config/config_test.go` and `cmd/ralph/execute_test.go`
- [X] T017 [P] [US2] Write visibility and persisted-state tests in `internal/store/jsonl_test.go`, `internal/regent/state_test.go`, and `internal/tui/app_test.go`

### Implementation for User Story 2

- [X] T018 [P] [US2] Implement project-default and per-run precedence handling in `internal/config/config.go` and `cmd/ralph/execute.go`
- [X] T019 [P] [US2] Persist effective-agent identity in `internal/store/jsonl.go` and `internal/regent/state.go`
- [X] T020 [US2] Surface the selected agent in user-visible status output in `cmd/ralph/execute.go`, `internal/tui/app.go`, and `internal/tui/panels/header.go`

**Checkpoint**: Agent choice is durable, overrideable, and visible in both live and recorded run context.

---

## Phase 5: User Story 3 - Recover Quickly from Setup or Compatibility Problems (Priority: P3)

**Goal**: Codex startup failures and unsupported workflow selections fail early with clear, actionable guidance.

**Independent Test**: Attempt to start Codex when the binary is missing or when Codex is selected for an unsupported workflow such as `--worktree`, and verify Ralph exits before work starts with an actionable error.

### Tests for User Story 3

- [X] T021 [P] [US3] Write unavailable-binary and unsupported-workflow tests in `cmd/ralph/execute_test.go` and `cmd/ralph/wiring_test.go`
- [X] T022 [P] [US3] Write Codex startup failure coverage in `internal/codex/agent_test.go`

### Implementation for User Story 3

- [X] T023 [P] [US3] Validate Codex executable availability and startup failure handling in `internal/codex/agent.go`
- [X] T024 [P] [US3] Reject Codex for worktree and dashboard/orchestrator flows in `cmd/ralph/execute.go` and `cmd/ralph/wiring.go`
- [X] T025 [US3] Return actionable agent-selection and startup errors in `cmd/ralph/commands.go`, `cmd/ralph/main.go`, and `cmd/ralph/execute.go`

**Checkpoint**: Misconfiguration and unsupported scope errors are clear, early, and safe.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Documentation, regression coverage, and final validation across all stories.

- [X] T026 [P] Update Codex setup and selection docs in `README.md` and `ralph.toml`
- [X] T027 [P] Validate example flows and expected results in `specs/010-codex-support/quickstart.md` and `specs/010-codex-support/contracts/cli-agent-selection.md`
- [X] T028 [P] Add regression coverage for unchanged Claude-default behavior in `cmd/ralph/execute_test.go` and `internal/loop/loop_test.go`
- [X] T029 Run final validation with `go test ./...` and `go vet ./...` from the repository root

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies; can start immediately
- **Foundational (Phase 2)**: Depends on Phase 1; blocks all user stories
- **US1 (Phase 3)**: Depends on Phase 2
- **US2 (Phase 4)**: Depends on US1 because persistence and visibility build on a working Codex single-run path
- **US3 (Phase 5)**: Depends on US1 because failure handling wraps the Codex adapter and supported-flow execution path
- **Polish (Phase 6)**: Depends on all desired user stories being complete

### User Story Dependencies

- **US1 (P1)**: Starts after Foundational; no dependencies on other stories
- **US2 (P2)**: Depends on US1 runtime support and Foundational selection plumbing
- **US3 (P3)**: Depends on US1 runtime support; can proceed independently of US2 after US1 is complete

### Within Each User Story

- Tests must be written and fail before implementation
- Parser/adapter work before command integration
- Persistence before UI/status visibility
- Error detection before user-facing error copy polish

### Parallel Opportunities

- Phase 1: T002 and T003 can run in parallel
- Phase 2: T006, T008, and T009 can run in parallel after T005/T007 planning is clear
- US1: T010 and T011 can run in parallel
- US2: T016 and T017 can run in parallel; T018 and T019 can run in parallel after tests exist
- US3: T021 and T022 can run in parallel; T023 and T024 can run in parallel after tests exist
- Phase 6: T026, T027, and T028 can run in parallel

---

## Parallel Example: User Story 1

```text
# Parallel test work before Codex runtime implementation:
T010: Write Codex subprocess and parser tests in internal/codex/agent_test.go and internal/codex/parser_test.go
T011: Write supported-flow execution tests for Codex in cmd/ralph/execute_test.go
```

## Parallel Example: User Story 2

```text
# Parallel precedence and visibility work after US1 lands:
T016: Write project-default and flag-precedence tests in internal/config/config_test.go and cmd/ralph/execute_test.go
T017: Write visibility and persisted-state tests in internal/store/jsonl_test.go, internal/regent/state_test.go, and internal/tui/app_test.go
T018: Implement precedence handling in internal/config/config.go and cmd/ralph/execute.go
T019: Persist agent identity in internal/store/jsonl.go and internal/regent/state.go
```

## Parallel Example: User Story 3

```text
# Parallel failure-path work after Codex runtime exists:
T021: Write unavailable-binary and unsupported-workflow tests in cmd/ralph/execute_test.go and cmd/ralph/wiring_test.go
T022: Write Codex startup failure coverage in internal/codex/agent_test.go
T023: Validate Codex executable availability and startup failure handling in internal/codex/agent.go
T024: Reject Codex for worktree and dashboard/orchestrator flows in cmd/ralph/execute.go and cmd/ralph/wiring.go
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational
3. Complete Phase 3: User Story 1
4. **Stop and validate**: Run a Codex-backed `plan` or `build` flow independently
5. Ship/demo the Codex single-run path as the first increment

### Incremental Delivery

1. Setup + Foundational: Config, metadata, and command surfaces ready
2. Add US1: Codex main flows run end to end
3. Add US2: Project default and per-run override become durable and visible
4. Add US3: Setup and unsupported-flow failures become actionable
5. Finish Polish: Docs, regression coverage, and full validation

### Parallel Team Strategy

1. One engineer completes Setup and Foundational tasks first
2. After Phase 2, one engineer can own Codex runtime (US1) while another prepares precedence/visibility tests (US2)
3. Once US1 lands, a separate engineer can implement unsupported-flow protections and startup error handling (US3)

---

## Task Count Summary

| Phase | Scope | Tasks |
|-------|-------|-------|
| Phase 1 | Setup | 4 |
| Phase 2 | Foundational | 5 |
| Phase 3 | US1 | 6 |
| Phase 4 | US2 | 5 |
| Phase 5 | US3 | 5 |
| Phase 6 | Polish | 4 |
| **Total** |  | **29** |

## Notes

- Every task follows the required checklist format with checkbox, task ID, optional `[P]`, optional story label, and explicit file path
- The suggested MVP scope is **User Story 1 only** after Setup and Foundational phases
- US2 and US3 remain independently valuable follow-on increments once the Codex runtime path exists
