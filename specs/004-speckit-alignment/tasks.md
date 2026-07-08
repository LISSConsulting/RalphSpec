# Tasks: Spec Kit Alignment

**Input**: Design documents from `specs/004-speckit-alignment/`
**Prerequisites**: plan.md (required), spec.md (required), research.md, data-model.md, contracts/cli-commands.md, quickstart.md

**Tests**: Included inline — constitution mandates test-gated commits and 80% coverage target.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: Setup

**Purpose**: No project initialization needed — extending an existing Go project. This phase prepares shared infrastructure changes.

- [x] T001 Verify existing tests pass before any changes — run `go test ./...` and `go vet ./...`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Refactor the `internal/spec/` data model to support directory-based features and artifact-presence status. All user stories depend on these type changes.

- [x] T002 Add new status constants (StatusSpecified, StatusPlanned, StatusTasked) with Symbol() and String() methods in `internal/spec/spec.go` — keep existing StatusDone/StatusInProgress/StatusNotStarted for legacy compatibility
- [x] T003 Add Dir (string) and IsDir (bool) fields to SpecFile struct in `internal/spec/spec.go` — Dir holds relative path to feature directory, IsDir distinguishes directory-based from flat-file specs
- [x] T004 Update existing tests in `internal/spec/spec_test.go` to account for new struct fields (Dir, IsDir) — ensure all existing assertions still pass with zero-value defaults for flat-file specs

**Checkpoint**: Foundation ready — type system supports both directory-based and flat-file specs

---

## Phase 3: User Story 1 — Spec Kit Directory Discovery (Priority: P1) 🎯 MVP

**Goal**: Ralph discovers `specs/NNN-name/` directories as single features with artifact-presence status, instead of treating each `.md` as a separate spec.

**Independent Test**: Create a spec kit directory structure and verify `ralph spec list` shows one entry per feature directory with correct status.

### Implementation for User Story 1

- [x] T005 [US1] Rewrite List() in `internal/spec/spec.go` — when entry is a directory, emit one SpecFile with Name=dirName, Dir=relative dir path, Path=dir/spec.md, IsDir=true, Status from detectDirStatus(); when entry is a flat .md file, keep existing behavior with IsDir=false
- [x] T006 [US1] Add detectDirStatus() function in `internal/spec/spec.go` — check file existence: tasks.md→StatusTasked, plan.md→StatusPlanned, spec.md→StatusSpecified, else StatusNotStarted
- [x] T007 [P] [US1] Add table-driven tests for directory-based discovery in `internal/spec/spec_test.go` — test cases: dir with only spec.md (specified), dir with spec+plan (planned), dir with spec+plan+tasks (tasked), empty dir (not_started), dir with extra files (still correct status)
- [x] T008 [P] [US1] Add test for mixed flat+directory discovery in `internal/spec/spec_test.go` — specs/ contains both a flat .md file and a directory; verify both are returned with correct types
- [x] T009 [US1] Remove spec.New() function and embedded template from `internal/spec/spec.go` — delete New(), openEditor(), and the spec-template.md embed; ralph specify replaces this functionality
- [x] T010 [US1] Remove specNewCmd() from `cmd/ralph/commands.go` — remove the function and its registration in specCmd(); specCmd() should only register specListCmd()
- [x] T011 [US1] Update formatSpecList() in `cmd/ralph/commands.go` — display Dir path for directory specs (instead of .md file path), use new status symbols (📋📐✅)
- [x] T012 [US1] Update TUI specs panel in `internal/tui/panels/specs.go` — update specItem.Title() and specItem.Description() to use new status symbols and show Dir for directory-based specs

**Checkpoint**: `ralph spec list` shows one entry per feature directory with artifact-presence status. TUI specs panel reflects the new model.

---

## Phase 4: User Story 4 — Repurpose Existing plan/run Commands (Priority: P2)

**Goal**: Move existing Claude loop commands under `ralph loop` parent, freeing `plan` and `run` for speckit.

**Independent Test**: Verify `ralph loop build` and `ralph loop run` invoke the Claude loop identically to the old top-level commands.

### Implementation for User Story 4

- [x] T013 [US4] Create loopCmd() parent command in `cmd/ralph/commands.go` — Use: "loop", Short: "Autonomous Claude loop commands"
- [x] T014 [US4] Move existing buildCmd() and runCmd() loop bodies into loopBuildCmd() and loopRunCmd() in `cmd/ralph/commands.go` — same RunE logic, same flags (--max, --no-tui)
- [x] T015 [US4] Update rootCmd() in `cmd/ralph/main.go` — remove old top-level plan/run loop behavior, register loopCmd with build/run subcommands, keep top-level build as alias (call same executeLoop function), keep status/init/spec unchanged
- [x] T016 [US4] Add tests verifying loop subcommand registration in `cmd/ralph/commands_test.go` — verify loopCmd has build/run subcommands, verify top-level build still exists, verify old top-level plan/run loop commands are gone

**Checkpoint**: `ralph loop build` and `ralph loop run` work. Top-level `ralph build` unchanged. Old loop-backed `ralph plan` and `ralph run` are removed (not yet replaced by speckit).

---

## Phase 5: User Story 3 — Active Spec Resolution (Priority: P2)

**Goal**: Resolve active spec directory from git branch name or `--spec` flag.

**Independent Test**: Check out a feature branch and verify Resolve() returns the correct spec directory.

### Implementation for User Story 3

- [x] T017 [US3] Create Resolve() function in `internal/spec/resolve.go` — signature: `Resolve(dir, specFlag, branch string) (ActiveSpec, error)`; resolution order: specFlag→branch→error; check specs/<name>/ directory exists with os.Stat
- [x] T018 [US3] Define ActiveSpec struct in `internal/spec/resolve.go` — fields: Name string, Dir string (absolute), Branch string, Explicit bool
- [x] T019 [P] [US3] Add table-driven tests for Resolve() in `internal/spec/resolve_test.go` — test cases: branch match, --spec override, no match (error), empty branch (detached HEAD error), specify command with missing dir (create dir behavior delegated to caller)
- [x] T020 [P] [US3] Add test for main/master branch error case in `internal/spec/resolve_test.go` — verify clear error message suggesting --spec flag

**Checkpoint**: Resolve() correctly maps branch names to spec directories. Error messages guide users to `--spec` flag.

---

## Phase 6: User Story 2 — Speckit Command Mapping (Priority: P1)

**Goal**: Top-level `specify`, `plan`, `clarify`, `tasks`, `run` commands invoke Claude Code speckit skills.

**Independent Test**: Run each command and verify Claude Code is spawned with the correct slash command.

### Implementation for User Story 2

- [x] T021 [US2] Create executeSpeckit() function in `cmd/ralph/execute.go` — spawns `claude -p "/<skill> <args>" --verbose` with inherited stdin/stdout/stderr; returns Claude's exit code as error; accepts context for cancellation
- [x] T022 [US2] Create specifyCmd() in `cmd/ralph/speckit_cmds.go` — Use: "specify", Args: cobra.MinimumNArgs(1), Flags: --spec; resolves active spec, creates dir if missing, calls executeSpeckit("speckit.specify", description)
- [x] T023 [P] [US2] Create speckitPlanCmd() and clarifyCmd() in `cmd/ralph/speckit_cmds.go` — both require spec.md to exist; plan: executeSpeckit("speckit.plan"); clarify: executeSpeckit("speckit.clarify")
- [x] T024 [P] [US2] Create speckitTasksCmd() in `cmd/ralph/speckit_cmds.go` — requires plan.md to exist; executeSpeckit("speckit.tasks")
- [x] T025 [US2] Create speckitRunCmd() in `cmd/ralph/speckit_cmds.go` — requires tasks.md to exist; executeSpeckit("speckit.implement"); Flags: --spec
- [x] T026 [US2] Register all speckit commands in rootCmd() in `cmd/ralph/main.go` — top-level: specify, plan (speckit), clarify, tasks, run (speckit)
- [x] T027 [US2] Add tests for speckit commands in `cmd/ralph/speckit_cmds_test.go` — verify: specifyCmd requires args, plan/clarify require spec.md, tasks requires plan.md, run requires tasks.md; verify --spec flag is wired; verify executeSpeckit builds correct claude command

**Checkpoint**: All five speckit commands registered and functional. Full workflow: specify → clarify → plan → tasks → run.

---

## Phase 7: User Story 5 — ROAM.md and BUILD.md Update (Priority: P3)

**Goal**: Update autonomous loop prompt files to understand spec kit directory structure.

**Independent Test**: Run `ralph build --roam` and verify the agent reads spec kit directories correctly.

### Implementation for User Story 5

- [x] T028 [P] [US5] Update ROAM.md — replace generic `specs/` scanning instruction with spec kit directory awareness: read `spec.md`, `plan.md`, `tasks.md` from each `specs/NNN-name/` directory; reference artifact-presence status model
- [x] T029 [P] [US5] Update BUILD.md — replace CHRONICLE.md-centric task picking with spec kit directory awareness: read `tasks.md` within active spec directory, reference `spec.md` and `plan.md` for context

**Checkpoint**: Autonomous loop agents understand spec kit directory structure when invoked via `ralph build --roam` or `ralph loop build`.

---

## Phase 8: Polish & Cross-Cutting Concerns

**Purpose**: Final validation and cleanup across all stories.

- [x] T030 Run `go test ./...` and verify all tests pass (existing + new)
- [x] T031 Run `go vet ./...` and fix any warnings
- [x] T032 Run `golangci-lint run` and fix any lint issues (ifElseChain, errcheck, etc.)
- [x] T033 Verify quickstart.md workflow end-to-end — test each command in sequence on a feature branch
- [x] T034 Remove dead code — any orphaned functions, unused imports, or stale test helpers from the refactor

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — verify baseline
- **Foundational (Phase 2)**: Depends on Phase 1 — type changes block all stories
- **US1 (Phase 3)**: Depends on Phase 2 — uses new SpecFile fields and status types
- **US4 (Phase 4)**: Depends on Phase 3 — T010 removes specNewCmd from commands.go before US4 restructures the same file
- **US3 (Phase 5)**: Depends on Phase 2 — uses SpecFile types; no dependency on US1 or US4
- **US2 (Phase 6)**: Depends on US1 (discovery), US3 (resolution), US4 (freed command names)
- **US5 (Phase 7)**: No code dependencies — can run in parallel with US2 (different files)
- **Polish (Phase 8)**: Depends on all stories being complete

### User Story Dependencies

```
Phase 2 (Foundational)
  ├──→ US1 (Phase 3) ──→ US4 (Phase 4) ──┐
  │                                        ├──→ US2 (Phase 6) ──→ Polish (Phase 8)
  └──→ US3 (Phase 5) ────────────────────┘
                                    US5 (Phase 7) ──→ Polish (Phase 8)
```

### Within Each Phase

- Models/types before functions that use them
- Functions before commands that call them
- Implementation before tests (tests written alongside, committed together)

### Parallel Opportunities

- **T007 + T008**: Both are test files, different test functions — can run in parallel
- **T019 + T020**: Both are resolve_test.go functions, but different test cases — can be written in parallel
- **T023 + T024**: Different speckit commands in same file — can be written in parallel
- **T028 + T029**: ROAM.md and BUILD.md are independent files — can run in parallel
- **US3 (Phase 5) + US5 (Phase 7)**: Touch completely different files — can run in parallel with each other (and with US4)

---

## Parallel Example: User Story 1

```
# After Phase 2 completes, these can run in parallel:
T007 [P] [US1] Table-driven tests for directory discovery  (internal/spec/spec_test.go)
T008 [P] [US1] Test for mixed flat+directory discovery      (internal/spec/spec_test.go)

# After T005-T006 complete:
T009 [US1] Remove spec.New()           (internal/spec/spec.go)
T010 [US1] Remove specNewCmd()         (cmd/ralph/commands.go)
```

## Parallel Example: User Story 2

```
# After T021-T022 complete, these can run in parallel:
T023 [P] [US2] speckitPlanCmd + clarifyCmd   (cmd/ralph/speckit_cmds.go)
T024 [P] [US2] speckitTasksCmd              (cmd/ralph/speckit_cmds.go)
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (verify baseline)
2. Complete Phase 2: Foundational (type refactor)
3. Complete Phase 3: User Story 1 (directory discovery)
4. **STOP and VALIDATE**: `ralph spec list` shows correct output for spec kit directories
5. This alone delivers value — Ralph understands the spec kit layout

### Incremental Delivery

1. Setup + Foundational → Type system ready
2. Add US1 → Directory discovery works → `ralph spec list` validates (MVP!)
3. Add US4 → Old commands moved under `ralph loop` → backward compat preserved
4. Add US3 → Active spec resolution from branch → ergonomic targeting
5. Add US2 → Full speckit command mapping → complete workflow
6. Add US5 → Prompt files updated → autonomous loop also spec-kit-aware
7. Polish → All tests, vet, lint green

### Single Developer Strategy (recommended)

Execute phases sequentially: 1 → 2 → 3 → 4 → 5 → 6 → 7 → 8. Commit after each phase checkpoint.

---

## Notes

- [P] tasks = different files, no dependencies on incomplete tasks in same phase
- [Story] label maps task to specific user story for traceability
- Each user story is independently testable after its phase completes
- Constitution mandates: table-driven tests, no global mutable state, explicit error wrapping
- Existing patterns to follow: `init()` for test fakes, `t.TempDir()` for isolation, `t.Run` subtests
- Known lint rules: ifElseChain (≥3 branches → switch), errcheck (fmt.Fprint, defer Close), singleCaseSwitch
