# Tasks: TUI Overhaul & UX Fixes

**Input**: Design documents from `specs/008-tui-overhaul/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md

**Tests**: Table-driven tests required per constitution (Principle III). Include test tasks for non-trivial changes.

**Organization**: Tasks grouped by user story. US-013/US-007/US-008 are quick wins bundled into Phase 2 (foundational). Remaining stories are independent phases.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to
- Include exact file paths in descriptions

---

## Phase 1: Setup

**Purpose**: Branch, dependency, config scaffolding

- [ ] T001 Create branch `008-tui-overhaul` from current HEAD
- [ ] T002 Add `github.com/charmbracelet/glamour` dependency via `go get github.com/charmbracelet/glamour`
- [ ] T003 Add `Focus string` field to `BuildConfig` in `internal/config/config.go` with toml tag `focus`
- [ ] T004 Add `focus = ""` under `[build]` in `ralph.toml` example config

---

## Phase 2: Foundational Quick Wins (US-013, US-007, US-008)

**Purpose**: Trivial fixes that unblock everything else and improve immediate UX

**CRITICAL**: These are one-line or few-line changes with high impact.

- [ ] T005 [US13] Change default focus from `FocusMain` to `FocusSpecs` in `internal/tui/app.go` New() constructor (line ~102)
- [ ] T006 [US13] Update test assertions in `internal/tui/app_test.go` that assert initial focus is FocusMain

- [ ] T007 [US8] Add `initGitInfoCmd()` tea.Cmd to `Init()` in `internal/tui/app.go` that reads branch and last commit via `exec.Command("git", ...)`
- [ ] T008 [US8] Add `gitInfoMsg` type to `internal/tui/msg.go` with Branch and LastCommit fields
- [ ] T009 [US8] Handle `gitInfoMsg` in `Update()` in `internal/tui/app.go` to populate `m.branch` and `m.lastCommit`
- [ ] T010 [US8] Add test for `gitInfoMsg` handling in `internal/tui/app_test.go`

- [ ] T011 [US7] Add `initIterationsCmd()` tea.Cmd to `Init()` in `internal/tui/app.go` that calls `storeReader.Iterations()` when non-nil
- [ ] T012 [US7] Add `iterationsLoadedMsg` type to `internal/tui/msg.go` with Summaries field
- [ ] T013 [US7] Handle `iterationsLoadedMsg` in `Update()` in `internal/tui/app.go` to populate `iterationsPanel` via AddIteration loop
- [ ] T014 [US7] Add test for iterations pre-loading in `internal/tui/app_test.go`

**Checkpoint**: TUI starts with Specs focused, git info in header, previous iterations visible

---

## Phase 3: User Story 12 — Roam Focus Flag (Priority: P1) MVP

**Goal**: `ralph build --roam --focus "UI/UX"` constrains roam to a specific topic

**Independent Test**: Run `ralph build --roam --focus "test"` and verify the prompt contains the focus directive

### Implementation

- [ ] T015 [US12] Add `Focus string` field to `Loop` struct in `internal/loop/loop.go`
- [ ] T016 [US12] Extend `augmentPrompt()` signature to accept `focus string` parameter in `internal/loop/loop.go`
- [ ] T017 [US12] Implement focus directive appending in `augmentPrompt()`: when focus is non-empty, append "Focus your work on: <topic>. Prioritize changes related to this area over other improvements."
- [ ] T018 [US12] Update `augmentPrompt()` call site in `Loop.Run()` in `internal/loop/loop.go` to pass `l.Focus`
- [ ] T019 [P] [US12] Add `--focus` string flag to `buildCmd()`, `loopBuildCmd()`, `loopRunCmd()` in `cmd/ralph/commands.go`
- [ ] T020 [US12] Wire focus flag through `executeLoop()` and `executeRun()` in `cmd/ralph/execute.go` to set `lp.Focus`
- [ ] T021 [US12] Read `cfg.Build.Focus` as default when `--focus` flag is empty in `cmd/ralph/execute.go`
- [ ] T022 [P] [US12] Add table-driven tests for `augmentPrompt()` with focus parameter in `internal/loop/loop_test.go`
- [ ] T023 [P] [US12] Add test for `--focus` flag parsing in `cmd/ralph/commands_test.go`

**Checkpoint**: `--focus` flag works end-to-end, prompt contains focus directive

---

## Phase 4: User Story 1 — Interactive Speckit Commands (Priority: P2)

**Goal**: `ralph clarify` and `ralph specify` complete full interactive sessions

**Independent Test**: Run `ralph clarify` and verify Claude enters interactive mode, asks questions, waits for answers

### Implementation

- [ ] T024 [US1] Add `interactive bool` parameter to `executeSpeckit()` in `cmd/ralph/execute.go`
- [ ] T025 [US1] When `interactive=true`, spawn `claude --verbose` without `-p` flag, with inherited stdio in `executeSpeckit()`
- [ ] T026 [US1] Update `clarifyCmd()` and `specifyCmd()` in `cmd/ralph/speckit_cmds.go` to pass `interactive: true`
- [ ] T027 [US1] Update remaining speckit commands (plan, tasks, etc.) in `cmd/ralph/speckit_cmds.go` to pass `interactive: false`
- [ ] T028 [P] [US1] Add test for interactive vs non-interactive mode in `cmd/ralph/speckit_cmds_test.go`

**Checkpoint**: `ralph clarify` stays interactive; `ralph plan` still uses `-p` mode

---

## Phase 5: User Story 5 — Stable Content Panels (Priority: P2)

**Goal**: Opening a spec or iteration log stays visible; streaming output doesn't displace it

**Independent Test**: Open a spec in Main panel, wait for new output events, verify spec content remains in Spec tab

### Implementation

- [ ] T029 [US5] Refactor `MainView` in `internal/tui/panels/main_view.go`: replace single `logview` with `outputLog`, `specLog`, `iterationLog`, `summaryLog` (four LogView instances)
- [ ] T030 [US5] Update `NewMainView()` to initialize all four LogView instances in `internal/tui/panels/main_view.go`
- [ ] T031 [US5] Update `AppendLine()` to always append to `outputLog` only in `internal/tui/panels/main_view.go`
- [ ] T032 [US5] Update `ShowSpec()` to write to `specLog` (not outputLog) in `internal/tui/panels/main_view.go`
- [ ] T033 [US5] Update `ShowIterationLog()` to write to `iterationLog` in `internal/tui/panels/main_view.go`
- [ ] T034 [US5] Update `SetIterationSummary()` to write to `summaryLog` in `internal/tui/panels/main_view.go`
- [ ] T035 [US5] Update `View()` to render the active tab's LogView in `internal/tui/panels/main_view.go`
- [ ] T036 [US5] Update `Update()` to delegate scroll keys to the active tab's LogView in `internal/tui/panels/main_view.go`
- [ ] T037 [US5] Update `SetSize()` to resize all four LogViews in `internal/tui/panels/main_view.go`
- [ ] T038 [US5] Update `ShowWorktreeLog()` to write to `outputLog` in `internal/tui/panels/main_view.go`
- [ ] T039 [P] [US5] Add tests verifying AppendLine doesn't affect specLog content in `internal/tui/panels/main_view_test.go`

**Checkpoint**: Spec/iteration content persists independently from streaming output

---

## Phase 6: User Story 2 — Panel Titles and Numbers (Priority: P3)

**Goal**: Each panel displays "[N] Title" label, accent-colored when focused

**Independent Test**: Launch TUI, verify each panel shows its number and title; focused panel title is highlighted

### Implementation

- [ ] T040 [US2] Add `renderPanelTitle(number int, title string, focused bool, th Theme) string` helper to `internal/tui/theme.go`
- [ ] T041 [US2] Update `View()` in `internal/tui/app.go` to prepend panel title to each panel's Render() content using the new helper
- [ ] T042 [US2] Adjust `innerDims()` usage or panel content height by -1 to account for title row in `internal/tui/app.go`
- [ ] T043 [P] [US2] Add test for `renderPanelTitle()` in `internal/tui/theme_test.go`
- [ ] T044 [P] [US2] Update layout tests to account for title row height adjustment in `internal/tui/layout_test.go`

**Checkpoint**: All panels show "[1] Specs", "[2] Iterations", "[3] Output", "[4] Secondary", "[5] Worktrees"

---

## Phase 7: User Story 6 — Layout Correctness (Priority: P3)

**Goal**: TUI renders within terminal bounds with no overflow or misalignment

**Independent Test**: Launch TUI at 80x24, 120x40, 200x60 — no clipping, no off-screen content

### Implementation

- [ ] T045 [US6] Audit `Calculate()` in `internal/tui/layout.go` — verify panel rect sums match terminal dimensions exactly (width: sidebarW + rightW = width; height: header + specs + iters = height, header + main + sec = height)
- [ ] T046 [US6] Ensure header content in `internal/tui/panels/header.go` truncates to fit width (replace emoji with ASCII if needed for width consistency)
- [ ] T047 [US6] Ensure footer content in `internal/tui/panels/footer.go` truncates gracefully when width is tight
- [ ] T048 [US6] Ensure TabBar in `internal/tui/components/tabbar.go` constrains output to `t.width` via lipgloss `.Width(t.width)` or `.MaxWidth(t.width)`
- [ ] T049 [US6] Ensure each panel's View() in `internal/tui/panels/` constrains output to `p.width` and `p.height`
- [ ] T050 [P] [US6] Add layout regression tests for 80x24, 120x40, 200x60 in `internal/tui/layout_test.go`

**Checkpoint**: TUI fits perfectly in all tested terminal sizes

---

## Phase 8: User Story 3 — Specs as Traversable Tree (Priority: P3)

**Goal**: Specs panel shows collapsible tree with spec.md/plan.md/tasks.md per directory

**Independent Test**: Launch TUI, navigate specs, expand a directory, select plan.md, see its content in Main panel

### Implementation

- [ ] T051 [US3] Define `specTreeNode` struct in `internal/tui/panels/specs.go` with sf, children ([]string), expanded fields
- [ ] T052 [US3] Add `buildTree(specs []spec.SpecFile, workDir string) []specTreeNode` that discovers .md children per directory in `internal/tui/panels/specs.go`
- [ ] T053 [US3] Refactor `NewSpecsPanel()` to build tree nodes instead of flat list items in `internal/tui/panels/specs.go`
- [ ] T054 [US3] Implement custom `flattenTree()` that produces visible rows (dirs + expanded children) in `internal/tui/panels/specs.go`
- [ ] T055 [US3] Implement `View()` rendering: directories with expand/collapse indicator, children indented with file icon in `internal/tui/panels/specs.go`
- [ ] T056 [US3] Implement `Update()` navigation: j/k moves cursor in flattened view; enter on dir toggles expand; enter on child emits SpecSelectedMsg with child file path in `internal/tui/panels/specs.go`
- [ ] T057 [US3] Update `SpecSelectedMsg` to include the specific file path (not just the dir-level spec) in `internal/tui/panels/specs.go`
- [ ] T058 [US3] Update `handleSpecSelected()` in `internal/tui/app.go` to read and display the selected file (spec.md, plan.md, or tasks.md)
- [ ] T059 [US3] Pass `workDir` to `NewSpecsPanel()` so `buildTree()` can stat child files in `internal/tui/panels/specs.go`
- [ ] T060 [P] [US3] Add tests for tree building, flattening, expand/collapse in `internal/tui/panels/specs_test.go`

**Checkpoint**: Specs panel is a navigable tree with expand/collapse

---

## Phase 9: User Story 11 — Markdown Rendering with Glamour (Priority: P3)

**Goal**: Spec content in Main panel is rendered as formatted markdown

**Independent Test**: Select a spec file, verify headers/code blocks/lists render with terminal formatting

### Implementation

- [ ] T061 [US11] Add `renderMarkdown(content string, width int) string` helper to `internal/tui/app.go` using glamour
- [ ] T062 [US11] Call `renderMarkdown()` in `handleSpecSelected()` before passing content to `ShowSpec()` in `internal/tui/app.go`
- [ ] T063 [US11] Handle glamour rendering errors by falling back to raw text in `renderMarkdown()`
- [ ] T064 [P] [US11] Add test for `renderMarkdown()` in `internal/tui/app_test.go`

**Checkpoint**: Spec content displays with formatted markdown

---

## Phase 10: User Story 4 — Correct Spec Status Detection (Priority: P3)

**Goal**: Running ralph from a different project directory shows correct spec statuses

**Independent Test**: Call `spec.List("/path/to/project")` from a different CWD, verify statuses match file presence

### Implementation

- [ ] T065 [US4] Audit `spec.List()` in `internal/spec/spec.go` — verify all paths use `dir` argument, not CWD
- [ ] T066 [US4] Audit `detectDirStatus()` — verify `absDir` is constructed from the `dir` argument in `internal/spec/spec.go`
- [ ] T067 [US4] If bug found: fix path resolution; if not: add defensive comment confirming correctness
- [ ] T068 [P] [US4] Add integration test: create temp dir with completed specs, call `List(tempDir)` from different CWD, verify StatusTasked in `internal/spec/spec_test.go`

**Checkpoint**: Spec status detection works correctly regardless of CWD

---

## Phase 11: User Story 10 — Functional Cost Tracking (Priority: P4)

**Goal**: Cost tab shows per-iteration breakdown; header shows running total

**Independent Test**: Run a build loop, verify Cost tab updates after each iteration, header cost increments

### Implementation

- [ ] T069 [US10] Trace cost data flow: verify `LogIterComplete` events carry `CostUSD` and `TotalCost` fields in `internal/loop/loop.go`
- [ ] T070 [US10] Verify `handleLogEntry()` populates `m.totalCost` from `entry.TotalCost` in `internal/tui/app.go`
- [ ] T071 [US10] Verify `secondary.AddIteration()` receives summaries with non-zero CostUSD in `internal/tui/app.go`
- [ ] T072 [US10] If cost is always zero: trace from `claude.EventResult.CostUSD` through `loop.emit()` to `LogIterComplete` and fix the gap
- [ ] T073 [P] [US10] Add test verifying cost accumulation in `internal/tui/app_test.go`

**Checkpoint**: Cost tab shows real per-iteration costs; header total is accurate

---

## Phase 12: User Story 9 — Footer Detail View (Priority: P4)

**Goal**: Selecting an iteration or output line shows detail in Secondary panel

**Independent Test**: Select an iteration in panel 2, verify Secondary panel shows full summary (cost, duration, commit, subtype)

### Implementation

- [ ] T074 [US9] On `IterationSelectedMsg`, populate Secondary panel's active tab with detailed iteration summary in `internal/tui/app.go`
- [ ] T075 [US9] Add `ShowDetail(lines []string)` method to `SecondaryPanel` that replaces active tab content in `internal/tui/panels/secondary.go`
- [ ] T076 [US9] Render detail lines: iteration number, mode, cost, duration, exit subtype, commit hash, timestamps in `internal/tui/app.go`
- [ ] T077 [P] [US9] Add test for detail view rendering in `internal/tui/panels/secondary_test.go`

**Checkpoint**: Selecting an iteration shows detail in the secondary panel

---

## Phase 13: Polish & Cross-Cutting Concerns

**Purpose**: Final cleanup and validation

- [ ] T078 Run `go vet ./...` and fix any warnings
- [ ] T079 Run `go test ./...` and ensure all tests pass
- [ ] T080 Run `golangci-lint run` and fix lint issues
- [ ] T081 Update CLAUDE.md approved deps list to include `glamour`
- [ ] T082 Test TUI at 80x24, 120x40, 200x60 terminal sizes manually
- [ ] T083 Verify `ralph build --roam --focus "UI/UX"` end-to-end
- [ ] T084 Verify `ralph clarify` enters interactive mode

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: No dependencies — start immediately
- **Phase 2 (Foundational)**: Depends on Phase 1 — BLOCKS all user stories
- **Phases 3-12 (User Stories)**: All depend on Phase 2 completion
  - Phase 5 (US5: per-tab buffers) should complete before Phase 8/9 (tree view/markdown) for best UX
  - All other phases are independent and can run in parallel
- **Phase 13 (Polish)**: Depends on all phases complete

### User Story Dependencies

- **US-013 (default focus)**: Independent, no deps
- **US-007 (load iterations)**: Independent, no deps
- **US-008 (git info)**: Independent, no deps
- **US-012 (focus flag)**: Independent, no deps
- **US-001 (interactive speckit)**: Independent, no deps
- **US-005 (per-tab buffers)**: Independent, no deps — but should complete early since US-003/US-011 build on it
- **US-002 (panel titles)**: Independent, no deps
- **US-006 (layout)**: Independent, no deps
- **US-003 (spec tree)**: Benefits from US-005 (per-tab buffers) to display tree-selected files properly
- **US-011 (glamour)**: Benefits from US-005 (per-tab buffers) for spec display
- **US-004 (spec status)**: Independent, no deps
- **US-010 (cost)**: Independent, no deps
- **US-009 (detail view)**: Independent, no deps

### Parallel Opportunities

- T003 + T004 (config changes) — different files
- T005 + T007 + T011 (Phase 2 quick wins) — different concerns
- T019 + T022 + T023 (focus flag impl + tests) — different files
- T039 + T043 + T044 + T050 + T060 + T064 + T068 + T073 + T077 (all test tasks) — independent
- Phases 3, 4, 6, 10, 11, 12 can all proceed in parallel after Phase 2

---

## Parallel Example: Phase 2 Quick Wins

```bash
# All three foundational changes can proceed simultaneously:
Task: "T005 — Change default focus to FocusSpecs in app.go"
Task: "T007 — Add initGitInfoCmd to Init() in app.go"
Task: "T011 — Add initIterationsCmd to Init() in app.go"
# Note: T007 and T011 both modify Init() in app.go — coordinate merge
```

---

## Implementation Strategy

### MVP First (Phase 1 + 2 + 3)

1. Complete Phase 1: Setup (branch, dep, config)
2. Complete Phase 2: Quick wins (default focus, git info, iterations)
3. Complete Phase 3: `--focus` flag
4. **STOP and VALIDATE**: TUI starts correctly, focus flag works
5. Ship as v0.1.57

### Incremental Delivery

1. Setup + Foundational + Focus flag → Quick value (MVP)
2. Add US-001 (interactive speckit) + US-005 (per-tab buffers) → Major UX fix
3. Add US-002 (titles) + US-006 (layout) → Visual polish
4. Add US-003 (tree) + US-011 (glamour) → Spec browsing overhaul
5. Add US-004 + US-010 + US-009 → Remaining fixes
6. Polish → Ship as v0.2.0

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- Each user story is independently completable and testable
- Commit after each task or logical group
- Total: 84 tasks across 13 phases
- Per-story counts: US13=2, US8=4, US7=4, US12=9, US1=5, US5=11, US2=5, US6=6, US3=10, US11=4, US4=4, US10=5, US9=4, Polish=7
