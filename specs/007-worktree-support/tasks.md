# Tasks: Git Worktree Support via Worktrunk

**Input**: Design documents from `specs/007-worktree-support/`
**Prerequisites**: plan.md (required), spec.md (required), research.md, data-model.md, contracts/

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Configuration, worktrunk adapter package, and shared types

- [x] T001 Add WorktreeConfig struct to internal/config/config.go with fields: Enabled (bool), MaxParallel (int), AutoMerge (bool), MergeTarget (string), PathTemplate (string) and TOML tags; add to top-level Config struct; add validation (MaxParallel >= 1); add defaults in Load()
- [x] T002 Add `[worktree]` section to ralph.toml example config with all fields and comments
- [x] T003 [P] Add WorktreeConfig validation tests to internal/config/config_test.go — table-driven: missing max_parallel defaults to 5, invalid max_parallel < 1 errors, empty merge_target accepted
- [x] T004 Create internal/worktree/worktree.go — define WorktreeOps interface (Detect, Switch, List, Merge, Remove), WorktreeInfo struct, Runner struct with Dir field, and NewRunner constructor
- [x] T005 [P] Create internal/worktree/detect.go — implement Runner.Detect(): check for `wt` on PATH (and `git-wt` on Windows); validate output of `--version` contains "worktrunk"; return clear error with install instructions if missing
- [x] T006 [P] Create internal/worktree/switch.go — implement Runner.Switch(branch, create): invoke `wt switch -c <branch>` or `wt switch <branch>`; parse worktree path from stdout (after `@ `); return (path, error)
- [x] T007 [P] Create internal/worktree/list.go — implement Runner.List(): invoke `wt list --json`; parse JSON into []WorktreeInfo; fall back to `git worktree list --porcelain` if --json unsupported
- [x] T008 [P] Create internal/worktree/merge.go — implement Runner.Merge(branch, target): invoke `wt merge <target>` from worktree; return error on conflict/failure
- [x] T009 [P] Create internal/worktree/remove.go — implement Runner.Remove(branch): invoke `wt remove <branch>`; return error if branch has running agent
- [x] T010 Create internal/worktree/worktree_test.go — subprocess faking via _FAKE_WT=1 env pattern with init() registration; table-driven tests for Detect (found/not-found/windows-git-wt), Switch (create-new/reuse-existing/error), List (json-output/empty), Merge (success/conflict), Remove (success/error)

**Checkpoint**: Worktrunk CLI adapter complete with full test coverage. Config supports [worktree] section.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Orchestrator package and TaggedLogEntry — required by all user stories

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [x] T011 Create internal/orchestrator/types.go — define AgentState enum (Creating, Running, Completed, Failed, Stopped, Merging, Merged, MergeFailed, Removed), WorktreeAgent struct (Branch, WorktreePath, SpecName, SpecDir, State, Iterations, TotalCost, Events chan, StopCh chan, Error), TaggedLogEntry struct (Branch, Entry)
- [x] T012 Create internal/orchestrator/orchestrator.go — Orchestrator struct with mutex-guarded agents map, MaxParallel, AutoMerge, MergeTarget, WorktreeOps, MergedEvents chan; implement New() constructor, ActiveAgents(), RunningCount(), AgentByBranch()
- [x] T013 Implement fan-in multiplexer in internal/orchestrator/fanin.go — startFanIn() goroutine that reads from each agent's Events channel, wraps in TaggedLogEntry with branch name, and forwards to MergedEvents; handles agent channel close gracefully
- [x] T014 Implement Orchestrator.Launch() in internal/orchestrator/orchestrator.go — validate not at max_parallel, validate no duplicate branch, call WorktreeOps.Switch(create=true), create WorktreeAgent, wire Loop with worktree Dir, start Loop.Run() in goroutine, register in fan-in, update state on completion/failure
- [x] T015 Implement Orchestrator.Stop(branch) and StopAll() in internal/orchestrator/orchestrator.go — close agent's StopCh; update state to Stopped
- [x] T016 Implement Orchestrator.Merge(branch) in internal/orchestrator/orchestrator.go — validate agent is Completed/Stopped (reject if Running), call WorktreeOps.Merge(), update state to Merged or MergeFailed
- [x] T017 Implement Orchestrator.Clean(branch) in internal/orchestrator/orchestrator.go — validate agent is not Running, call WorktreeOps.Remove(), update state to Removed, delete from agents map
- [x] T018 Create internal/orchestrator/orchestrator_test.go — table-driven tests: Launch (success, duplicate-branch-rejected, max-parallel-rejected), Stop (running→stopped, not-running-error), Merge (completed→merged, running-rejected, conflict→merge-failed), Clean (completed→removed, running-rejected), ActiveAgents (filters removed), fan-in (events tagged correctly, channel close handled)

**Checkpoint**: Orchestrator manages full agent lifecycle. All user stories can now build on this.

---

## Phase 3: User Story 1 — Single Worktree Build (Priority: P1) 🎯 MVP

**Goal**: `ralph build --worktree` runs a loop in an isolated worktree

**Independent Test**: Run `ralph build -w --no-tui --max 1` and verify worktree created, loop runs inside it, events stream to stdout with branch prefix

### Implementation for User Story 1

- [x] T019 [US1] Add `--worktree` / `-w` bool flag to buildCmd, loopBuildCmd, loopPlanCmd, loopRunCmd in cmd/ralph/commands.go — pass value through to execute functions
- [x] T020 [US1] Add worktree flag parameter to executeLoop() and executeRun() signatures in cmd/ralph/execute.go; thread through from command handlers
- [x] T021 [US1] Implement worktree setup in executeLoop() in cmd/ralph/execute.go — when --worktree is set: create worktree.NewRunner(dir), call Detect() (error if missing), call Switch(branch, create=true), override loop Dir to worktree path, log worktree creation; on completion log worktree path and branch
- [x] T022 [US1] Add branch-name prefix to non-TUI log output in cmd/ralph/format.go — when running in worktree mode, prepend `[branch]` to each log line for distinguishability (FR-010)
- [x] T023 [US1] Add tests for --worktree flag handling in cmd/ralph/execute_test.go — table-driven: worktree-created-and-loop-runs, wt-not-found-errors, existing-worktree-reused, log-lines-include-branch-prefix
- [x] T024 [US1] Create cmd/ralph/worktree_cmds.go — add `ralph worktree` parent command with subcommands: `list`, `merge`, `clean`; register under root command in commands.go
- [x] T025 [P] [US1] Implement worktreeListCmd in cmd/ralph/worktree_cmds.go — create worktree.Runner, call List(), format as table (Branch, Status, Iter, Cost, Spec); support --json flag
- [x] T026 [P] [US1] Implement worktreeMergeCmd in cmd/ralph/worktree_cmds.go — accept optional branch arg, --target flag, --no-remove flag; validate preconditions (not running); call worktree.Runner.Merge(); print result
- [x] T027 [P] [US1] Implement worktreeCleanCmd in cmd/ralph/worktree_cmds.go — accept optional branch arg, --all flag, --force flag; validate preconditions; call worktree.Runner.Remove(); print result
- [x] T028 [US1] Add tests for worktree subcommands in cmd/ralph/worktree_cmds_test.go — table-driven: list-shows-worktrees, list-json-format, merge-success, merge-running-rejected, clean-success, clean-all

**Checkpoint**: `ralph build -w` works end-to-end. `ralph worktree list/merge/clean` commands available. MVP complete.

---

## Phase 4: User Story 2 — Parallel Agents from Dashboard (Priority: P2)

**Goal**: TUI dashboard launches multiple agents in worktrees via keybind

**Independent Test**: Launch dashboard, press W on two different specs, verify both agents appear with independent status

### Implementation for User Story 2

- [x] T029 [US2] Create internal/tui/panels/worktrees.go — new WorktreesPanel component: list of WorktreeAgent entries with columns (Branch, State, Iterations, Cost); j/k navigation; selected item tracking; renders agent state with status icons
- [x] T030 [US2] Add WorktreesPanel to TUI layout in internal/tui/app.go — add as fifth panel or replace/augment existing panel when worktree mode is enabled; wire into panel focus cycle (tab/shift+tab and number keys)
- [x] T031 [US2] Wire Orchestrator into TUI in internal/tui/app.go — accept Orchestrator in tui.New() (optional, nil when worktree disabled); subscribe to MergedEvents channel; dispatch TaggedLogEntry updates to appropriate panels
- [x] T032 [US2] Implement `W` keybind in internal/tui/app.go — when Specs panel focused and spec selected: call Orchestrator.Launch(selectedSpec, ModeBuild); show error message if at max or duplicate branch
- [x] T033 [US2] Implement worktree-specific keybinds in internal/tui/app.go — when WorktreesPanel focused: `x` calls Orchestrator.Stop(branch), `M` calls Orchestrator.Merge(branch), `D` calls Orchestrator.Clean(branch); show result/error in status bar
- [x] T034 [US2] Update Main panel to show selected worktree's log in internal/tui/app.go — when user selects a worktree in WorktreesPanel and presses enter, switch Main panel to show that agent's log stream (filter TaggedLogEntry by branch)
- [x] T035 [US2] Wire Orchestrator creation in cmd/ralph/wiring.go — when [worktree] enabled in config: create worktree.Runner, create Orchestrator with config values, pass to tui.New()
- [x] T036 [US2] Add tests for WorktreesPanel in internal/tui/panels/worktrees_test.go — table-driven: renders-empty, renders-agents-with-status, navigation-j-k, selected-item-correct
- [x] T037 [US2] Add tests for Orchestrator TUI wiring in cmd/ralph/wiring_test.go — verify orchestrator created when [worktree] enabled; nil when disabled; keybinds dispatch correctly

**Checkpoint**: Dashboard supports multi-agent worktree management. Parallel agent workflows operational.

---

## Phase 5: User Story 3 — Merge and Cleanup (Priority: P3)

**Goal**: Explicit merge command + auto-merge on completion + cleanup

**Independent Test**: Complete a build in worktree, run `ralph worktree merge`, verify branch merged and worktree removed

### Implementation for User Story 3

- [x] T038 [US3] Implement auto-merge trigger in internal/orchestrator/orchestrator.go — when agent transitions to Completed and AutoMerge is true: run test command (if configured), if tests pass call Merge(branch), if tests fail log warning and skip merge
- [x] T039 [US3] Wire Regent test command into Orchestrator completion handler in internal/orchestrator/orchestrator.go — on agent completion: if regent.test_command configured, exec test in worktree Dir; gate auto-merge on test result
- [x] T040 [US3] Add auto-merge tests to internal/orchestrator/orchestrator_test.go — table-driven: auto-merge-on-success, auto-merge-skipped-on-test-failure, auto-merge-disabled-no-action, merge-conflict-preserves-worktree
- [x] T041 [US3] Send notification on merge/failure in internal/orchestrator/orchestrator.go — when merge completes or fails, emit LogEntry that triggers notification hook (reuse existing notify infrastructure)

**Checkpoint**: Auto-merge and explicit merge both functional. Worktree lifecycle fully managed.

---

## Phase 6: User Story 4 — Worktree Status in TUI (Priority: P4)

**Goal**: Dashboard shows real-time status of all worktree agents

**Independent Test**: Launch dashboard with 3 worktrees, verify all appear with correct status and update in real-time

### Implementation for User Story 4

- [x] T042 [US4] Enhance WorktreesPanel rendering in internal/tui/panels/worktrees.go — add status icons (🔨 running, ✅ completed, ❌ failed, ⏹ stopped, 🔀 merging), iteration count, cost column; empty-state hint when no worktrees
- [x] T043 [US4] Implement real-time status updates in internal/tui/app.go — on receiving TaggedLogEntry from MergedEvents: update matching WorktreeAgent's iteration count and cost; trigger panel re-render
- [x] T044 [US4] Implement log aggregation for dashboard in internal/orchestrator/orchestrator.go — WorktreePaths() method returns all active worktree paths; dashboard store reader scans each path's .ralph/logs/ for session history
- [x] T045 [US4] Add tests for real-time status updates in internal/tui/panels/worktrees_test.go — verify status transitions render correctly, cost accumulates, iteration count increments

**Checkpoint**: Full observability into parallel agent activity from the dashboard.

---

## Phase 7: User Story 5 — Multi-Worktree Regent (Priority: P5)

**Goal**: Regent supervises each worktree agent independently

**Independent Test**: Run two agents, simulate hang in one, verify Regent kills only the hung agent

### Implementation for User Story 5

- [x] T046 [US5] Refactor Regent to support multiple supervised agents in internal/regent/regent.go — accept a map or slice of supervised processes instead of a single one; per-agent hang timeout tracking, crash detection, and rollback
- [x] T047 [US5] Wire per-agent Regent in internal/orchestrator/orchestrator.go — on Launch(), create a Regent instance (or register agent with shared Regent) for the new worktree agent; on agent stop/complete, deregister from Regent
- [x] T048 [US5] Implement per-worktree rollback in internal/regent/regent.go — when test command fails for a specific worktree agent: create git.Runner with that worktree's Dir, revert the last commit in that worktree only
- [x] T049 [US5] Add tests for multi-agent Regent in internal/regent/state_test.go — table-driven: hang-detected-kills-one-agent-not-others, crash-restarts-one-agent, test-failure-rollback-one-worktree, max-retries-stops-agent

**Checkpoint**: Per-worktree supervision operational. Failures isolated between agents.

---

## Phase 8: Polish & Cross-Cutting Concerns

**Purpose**: Documentation, integration testing, and cleanup

- [x] T050 [P] Update README.md — add worktree section: quick start with `ralph build -w`, parallel agents from dashboard, merge/clean commands, `[worktree]` config reference
- [x] T051 [P] Update ralph.toml example config — add `[worktree]` section with all fields and comments
- [x] T052 [P] Update TUI keyboard reference in README.md — add W, M, D keybinds; document WorktreesPanel
- [x] T053 Run quickstart.md validation — verify all commands from quickstart.md work end-to-end
- [x] T054 Run `go vet ./...` and `golangci-lint run` — fix any warnings in new code
- [x] T055 Verify all existing tests pass — `go test ./...` must be green with zero regressions

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately
- **Foundational (Phase 2)**: Depends on Phase 1 (worktree adapter + config)
- **User Stories (Phase 3+)**: All depend on Phase 2 (Orchestrator)
  - US1 (Phase 3): Can start after Phase 2
  - US2 (Phase 4): Depends on US1 (--worktree flag and worktree commands must exist)
  - US3 (Phase 5): Depends on US1 (merge/clean commands) and Phase 2 (Orchestrator)
  - US4 (Phase 6): Depends on US2 (WorktreesPanel must exist)
  - US5 (Phase 7): Depends on Phase 2 (Orchestrator) and existing Regent
- **Polish (Phase 8)**: Depends on all user stories being complete

### User Story Dependencies

- **US1 (P1)**: Foundational only — no other story dependencies
- **US2 (P2)**: Depends on US1 (worktree commands exist, --worktree flag works)
- **US3 (P3)**: Depends on US1 (merge command) + Orchestrator (auto-merge trigger)
- **US4 (P4)**: Depends on US2 (WorktreesPanel exists)
- **US5 (P5)**: Depends on Orchestrator only (can start after Phase 2, parallel with US2-US4)

### Within Each User Story

- Types/models before services
- Services before CLI commands
- CLI commands before TUI integration
- Core implementation before tests (tests validate, not drive, in this feature)

### Parallel Opportunities

- Phase 1: T005, T006, T007, T008, T009 are all parallel (different files in internal/worktree/)
- Phase 1: T003 parallel with T004-T010 (different packages)
- Phase 3: T025, T026, T027 parallel (different command implementations)
- Phase 5: US5 can start after Phase 2, parallel with US3 and US4
- Phase 8: T050, T051, T052 all parallel (different files)

---

## Parallel Example: Phase 1

```
# All worktree adapter methods can be implemented in parallel:
T005: detect.go (wt detection)
T006: switch.go (create/switch worktree)
T007: list.go (list worktrees)
T008: merge.go (merge worktree)
T009: remove.go (remove worktree)
```

## Parallel Example: User Story 1

```
# Worktree subcommands can be implemented in parallel:
T025: worktreeListCmd (list worktrees)
T026: worktreeMergeCmd (merge worktree)
T027: worktreeCleanCmd (clean worktree)
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (config + worktree adapter)
2. Complete Phase 2: Foundational (Orchestrator core)
3. Complete Phase 3: User Story 1 (`ralph build -w` + worktree commands)
4. **STOP and VALIDATE**: Test `ralph build --worktree --no-tui --max 1` end-to-end
5. Ship as first increment — single-worktree builds are immediately useful

### Incremental Delivery

1. Setup + Foundational → Worktree adapter and Orchestrator ready
2. US1 → `ralph build -w` works → Ship (MVP!)
3. US2 → Dashboard multi-agent → Ship
4. US3 → Merge/auto-merge → Ship
5. US4 → TUI status → Ship
6. US5 → Multi-Regent → Ship
7. Polish → Docs, lint, validation → Final ship

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- Each user story is independently testable after its phase completes
- Commit after each task or logical group
- The loop package requires NO changes — each agent is a Loop instance with a different Dir
- The store package requires NO changes — it already creates .ralph/logs/ per Dir
- Worktrunk is invoked as a subprocess only — no Go dependency added
