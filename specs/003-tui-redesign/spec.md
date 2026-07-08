# Feature Specification: Panel-Based TUI Redesign

**Feature Branch**: `003-tui-redesign`
**Created**: 2026-02-26
**Status**: Draft
**Input**: Redesign the RalphSpec TUI in the likeness of lazygit and lazydocker — a multi-panel, keyboard-driven terminal dashboard for monitoring and controlling the spec-driven AI coding loop and The Regent supervisor.

## Clarifications

### Session 2026-02-28

- Q: How should `store.Reader` handle malformed/truncated JSONL lines (e.g., after a Regent kill mid-write)? → A: Skip malformed lines silently, log warning to stderr.
- Q: Should the existing TUI code be replaced wholesale or incrementally refactored? → A: Clean replacement — delete old TUI files, write new from scratch. Old code is recoverable from git history.
- Q: What retention policy should the store apply to session log files in `.ralph/logs/`? → A: Keep last N session logs (default 20), delete oldest on startup. N is configurable via `ralph.toml`.
- Q: What should happen when the terminal is smaller than the 80×24 minimum? → A: Show a centered "Terminal too small — resize to at least 80×24" message instead of rendering the layout.
- Q: Should `LoopState` transitions be strictly defined or open? → A: Strict state machine — define allowed transitions explicitly.

---

## Context

### Current State

The existing TUI (`internal/tui/`) is a single-panel log viewer built with bubbletea + lipgloss. It consists of:

- A `Model` struct consuming `loop.LogEntry` events from a channel
- A header bar (project name, branch, iteration, cost)
- A scrollable log of timestamped entries (tool use, git ops, Regent messages, errors)
- A footer bar (last commit, scroll hints, quit)

All information flows through a single flat stream. There is no way to inspect individual specs, review past iterations, examine Regent decisions, or view git history without leaving the TUI.

### Desired State

A multi-panel dashboard inspired by lazygit and lazydocker where:

- The left sidebar shows navigable lists (specs, iterations) that drive context in the main panel
- The main panel renders context-dependent content (live agent output, spec content, diffs, plan)
- A secondary panel provides always-visible supporting info (Regent log, git log, tests, cost)
- All panels respond to keyboard navigation with discoverable keybindings
- The TUI feels like a control room — status at a glance, intervene only when needed

### Reference Architecture

lazygit and lazydocker use `gocui` for panel management. RalphSpec's constitution mandates `bubbletea` + `lipgloss` (Technical Constraints, §IV). This spec implements the lazygit/lazydocker UX patterns (side list panels, tabbed main view, context-sensitive keybindings, panel focus cycling) using bubbletea's Elm architecture and lipgloss for layout and styling.

Key patterns to port from lazydocker:

| lazydocker concept | RalphSpec equivalent |
|---|---|
| `Panels` struct with `SideListPanel[T]` per resource | `panels` package with list panels for Specs and Iterations |
| `Views` struct holding `*gocui.View` references | Sub-models composed into root `Model` via bubbletea |
| `MainTab[T]` for tabbed content in main view | Tab sub-model with switchable render functions |
| `WindowMaximisation` (normal/half/full) | Panel zoom: `tab` cycles focus, `+`/`-` resizes focused panel |
| `HandleClick` + `HandleSelect` on panels | `tea.KeyMsg` dispatch per focused panel |
| `Options` view (bottom keybinding hints) | Footer component rendering context-sensitive hints |

### Constraints

- **bubbletea + lipgloss only** — no gocui, no raw ANSI (constitution §IV)
- **Existing event system extended, not changed** — the `loop.LogEntry` channel and all `LogKind` types remain as-is. A new `internal/store` package taps the event stream to persist entries to disk as JSONL. The TUI reads from both the live channel (current iteration) and the store (past iterations)
- **`--no-tui` mode preserved** — headless plain-text output must continue to work
- **No new dependencies beyond bubbles** — `charmbracelet/bubbles` (list, viewport, textinput, spinner) is permitted as it's the official bubbletea component library. All other deps require justification per constitution
- **Wiring extended minimally** — `cmd/ralph/wiring.go` orchestration gains a `store.Writer` that taps the event stream alongside the existing Regent state tracker and TUI forwarding. The forwarding goroutine appends each entry to the store before sending to the TUI channel. No structural change to the channel topology
- **Agent interface unchanged** — `internal/claude/`, `internal/loop/`, `internal/regent/` packages are not modified
- **Clean TUI replacement** — The existing `internal/tui/` files (`model.go`, `styles.go`, `model_test.go`) are deleted and replaced by the new multi-panel architecture. No incremental migration or parallel implementation. Old code is recoverable from git history

---

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Live Loop Monitoring (Priority: P1)

The operator starts `ralph build` and watches the TUI. The main panel streams Claude's tool calls in real time with auto-scroll. The header bar shows branch, iteration count, and running cost. When Claude completes an iteration, the iterations list in the sidebar updates. The Regent's activity appears in the secondary panel.

**Why this priority**: This is the core use case — it replaces the current single-panel experience and is the minimum viable TUI redesign.

**Independent Test**: Run `ralph build --max 2` against a project with specs. Verify: header updates per iteration, live output streams in main panel, iterations panel populates, Regent messages appear in secondary panel, footer shows context-sensitive keybindings.

**Acceptance Scenarios**:

1. **Given** Ralph is idle, **When** the operator presses `b`, **Then** the build loop starts, the header shows `● BUILDING`, the main panel begins streaming live agent output with auto-scroll, and the footer updates to show `x:stop  f:follow  /:search`.
2. **Given** a build is running and Claude emits a `tool_use` event, **Then** the main panel appends a timestamped, color-coded line (blue for reads, green for writes, yellow for bash) and auto-scrolls to bottom if follow mode is on.
3. **Given** an iteration completes, **Then** the iterations list in the left sidebar gains a new entry showing `#N mode ✓` with cost and duration, and the header's iteration counter and cost update.
4. **Given** the Regent detects a hang and kills the loop, **Then** the secondary panel's Regent tab shows `🛡️ Hang detected — no output for 5m — killing loop` with timestamp, and the header flashes `⟳ REGENT RESTART`.

---

### User Story 2 — Spec Navigation (Priority: P2)

The operator navigates the specs panel in the left sidebar, selects a spec, and views its content rendered in the main panel. They can create a new spec or open one in `$EDITOR` without leaving the TUI.

**Why this priority**: Specs are the input to the factory. Being able to review them alongside the running loop is a key usability improvement over the current CLI-only `ralph spec list`.

**Independent Test**: Run `ralph build` (or just launch the TUI in idle state). Navigate to specs panel, select a spec, verify content renders in main panel. Press `n` to create a new spec, press `e` to open in editor.

**Acceptance Scenarios**:

1. **Given** the specs panel is focused, **When** the operator presses `j`/`k`, **Then** the selection cursor moves through the spec list, and the main panel updates to show the selected spec's content (rendered markdown or raw text).
2. **Given** a spec is selected, **When** the operator presses `e`, **Then** the TUI suspends (`tea.Suspend`), `$EDITOR` opens with the spec file, and on editor exit the TUI resumes with the spec content refreshed.
3. **Given** the specs panel is focused, **When** the operator presses `n`, **Then** a text input appears prompting for the spec name, and on enter `ralph spec new <name>` runs and the spec list refreshes.
4. **Given** the spec list contains specs from `specs/` subdirectories, **Then** each entry shows the spec's relative path and status indicator (✅ complete, 🔄 in-progress, ⬜ not started).

---

### User Story 3 — Iteration Drill-Down (Priority: P2)

The operator selects a past iteration from the iterations panel and reviews its full agent output and cost in the main panel's tabs. Past iteration output is read from the JSONL session log on disk, so it survives Regent crash/restart cycles and is available even for iterations that completed before the current TUI instance started.

**Why this priority**: Post-hoc review of what the agent did is essential for trust. Without it, the dark factory is opaque. The Regent regularly kills and restarts Ralph — without persistent logs, all pre-restart iteration output is lost.

**Independent Test**: After running a multi-iteration build, navigate to the iterations panel. Select a completed iteration. Verify the main panel shows tabs for `[Output]` and `[Summary]`. Verify output contains the full tool-use log for that iteration. Kill the TUI, relaunch with `ralph status`, verify iteration history is still accessible.

**Acceptance Scenarios**:

1. **Given** the iterations panel is focused and a completed iteration is selected, **When** the operator presses `enter`, **Then** the main panel switches to that iteration's output tab showing the full tool-use log read from the JSONL session log.
2. **Given** an iteration is selected in the main panel, **When** the operator presses `]` to switch tabs, **Then** the main panel cycles to the Summary tab showing cost, duration, exit subtype, and commit hash.
3. **Given** the iterations panel is focused, **When** the current iteration is running (●), **Then** selecting it switches the main panel back to live output mode with auto-scroll.
4. **Given** the Regent killed and restarted Ralph mid-session, **When** the operator navigates to a pre-restart iteration, **Then** the full tool-use log is available because it was persisted to the JSONL session log before the restart.

---

### User Story 4 — Panel Navigation & Focus Management (Priority: P1)

The operator moves between panels using keyboard shortcuts. The focused panel is visually highlighted. Keybinding hints in the footer update to reflect the focused panel's available actions.

**Why this priority**: Without panel navigation, the multi-panel layout is unusable. This is foundational.

**Independent Test**: Launch the TUI. Press `tab` to cycle focus. Verify border highlighting changes. Verify footer hints update per panel. Press `1`/`2`/`3`/`4` to jump to specific panels.

**Acceptance Scenarios**:

1. **Given** the TUI is running, **When** the operator presses `tab`, **Then** focus cycles: specs → iterations → main → secondary → specs. The focused panel's border color changes to the accent color. The footer updates with the focused panel's keybindings.
2. **Given** any panel is focused, **When** the operator presses `1`, **Then** focus jumps to specs. `2` → iterations. `3` → main. `4` → secondary.
3. **Given** the main panel is focused, **When** the operator presses `[` or `]`, **Then** the active tab within the main panel cycles (Output → Summary, or Regent → Git → Tests → Cost in the secondary panel).
4. **Given** a small terminal (< 100 columns), **Then** the layout degrades gracefully: sidebar collapses to minimum width (20 chars), secondary panel stacks below main panel, content truncates with horizontal scroll.

---

### User Story 5 — Loop Control from TUI (Priority: P3)

The operator starts, stops, and restarts the loop directly from the TUI without restarting the binary.

**Why this priority**: Currently `ralph build` starts the loop immediately. Being able to start in an idle/dashboard state, then launch the loop, adds flexibility. However, the current fire-and-forget model still works, so this is lower priority.

**Independent Test**: Launch `ralph` (no subcommand) to enter dashboard mode. Press `b` to start build. Press `x` to stop. Press `R` to restart. Verify loop state transitions are reflected in header and panels.

**Acceptance Scenarios**:

1. **Given** the TUI is in idle state (no loop running), **When** the operator presses `b`, **Then** a build loop starts with the current `ralph.toml` config, the header shows `● BUILDING`, and the main panel begins streaming output.
2. **Given** a loop is running, **When** the operator presses `x`, **Then** the loop's context is cancelled, the current iteration finishes or is interrupted, the header shows `✓ IDLE`, and the iterations panel shows the last iteration's status.
3. **Given** the TUI is in idle state, **When** the operator presses `R` (shift+r), **Then** a roam loop starts.

---

### User Story 6 — Secondary Panel Tabs (Priority: P3)

The secondary panel (bottom-right) provides tabbed access to Regent log, git log, test output, and cost breakdown.

**Why this priority**: Regent visibility is already present in the current TUI as inline log entries. Separating it into a dedicated panel with additional tabs adds value but is not blocking.

**Independent Test**: During or after a build, focus the secondary panel. Switch between tabs. Verify each tab renders the correct content.

**Acceptance Scenarios**:

1. **Given** the secondary panel is focused, **When** the operator presses `]`, **Then** tabs cycle: Regent → Git → Tests → Cost.
2. **Given** the Regent tab is active and the Regent performs a rollback, **Then** the tab shows the rollback event with timestamp, commit hash, and test failure reason.
3. **Given** the Cost tab is active, **Then** it shows a per-iteration cost table: iteration number, mode, cost, duration, and a running total at the bottom.

---

### Edge Cases

- What happens when the JSONL session log grows very large (thousands of iterations)? → The TUI builds an in-memory index of byte offsets per iteration on startup (scan for `LogIterStart` markers). Individual iteration logs are read on demand via `ReadAt`, not loaded into memory all at once. Session log files are per-session, so a single file covers one `ralph build` invocation including Regent restarts.
- What happens when the `.ralph/logs/` directory doesn't exist? → The store creates it on first write (`os.MkdirAll`). Read operations return empty results rather than errors.
- What happens when `.ralph/logs/` accumulates many session files? → On startup, the store lists existing session logs, sorts by filename (which embeds timestamp), and deletes the oldest files exceeding the retention limit (default 20, configurable via `log_retention` in `ralph.toml`).
- What happens when two Ralph instances write to the same session log? → They don't. Each session gets a unique filename based on PID + timestamp (`<unix-ts>-<pid>.jsonl`). The Regent restarts Ralph within the same process, so the PID and session ID remain stable across restarts.
- What happens when a terminal resize occurs? → All panels recalculate dimensions on `tea.WindowSizeMsg`. Content reflows. Minimum terminal size: 80×24.
- What happens when the Regent kills Ralph mid-write and the JSONL session log contains a truncated final line? → `store.Reader` skips malformed lines silently during index building and iteration reads, logging a warning to stderr. Since `Append` calls `file.Sync()` after each complete line, only the in-flight write at kill time can be partial — all prior lines are guaranteed intact.
- What happens when the event channel fills faster than the TUI renders? → The existing non-blocking send in `loop.emit()` and the 128-capacity buffer remain. The TUI drains as fast as bubbletea's event loop runs. Events dropped by the buffer are lost (acceptable — they are also written to Regent state).
- What happens when no specs exist? → Specs panel shows "No specs. Press n to create one."
- What happens when `$EDITOR` is unset and the operator presses `e`? → Show inline message "Set $EDITOR to open specs" in the footer. Do not crash.
- What happens when the TUI is launched via `ralph build` (not dashboard mode)? → Loop starts immediately. All panels populate as events arrive. Dashboard mode (`ralph` with no subcommand) is a future enhancement (User Story 5) — until implemented, the TUI always starts with a running loop.

---

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: TUI MUST render a multi-panel layout with left sidebar (specs panel, iterations panel), main panel (top-right), secondary panel (bottom-right), header bar, and footer bar.
- **FR-002**: Panels MUST support keyboard focus cycling via `tab`/`shift+tab` and direct jump via `1`/`2`/`3`/`4`.
- **FR-003**: The focused panel MUST be visually distinct (accent-colored border) and the footer MUST show context-sensitive keybindings for the focused panel.
- **FR-004**: Specs panel MUST list all specs discovered from `specs/` directory with status indicators, support `j`/`k` navigation, and drive main panel content on selection.
- **FR-005**: Iterations panel MUST list all iterations for the current session (most recent first) with status icons (● running, ✓ complete, ✗ failed), cost, and duration.
- **FR-006**: Main panel MUST support tabs (switchable via `[`/`]`): live Output (auto-scroll during active build), Spec Content (when spec selected), Iteration Output (when past iteration selected).
- **FR-007**: Secondary panel MUST support tabs: Regent Log, Git Log, Tests, Cost Summary.
- **FR-008**: Live output view MUST auto-scroll (follow mode), toggleable with `f`. MUST support `ctrl+u`/`ctrl+d` for page scroll, `j`/`k` for line scroll when follow is off.
- **FR-009**: Header bar MUST display: project name, current branch, iteration counter (N/max), running cost, and state indicator (● PLANNING, ● BUILDING, ✓ IDLE, ⟳ REGENT RESTART, ✗ FAILED).
- **FR-010**: All existing `LogKind` types MUST render in the TUI: `LogInfo`, `LogIterStart`, `LogToolUse`, `LogIterComplete`, `LogError`, `LogGitPull`, `LogGitPush`, `LogDone`, `LogStopped`, `LogRegent`.
- **FR-011**: Tool-use lines MUST be color-coded per existing `styles.go` scheme: reads=blue, writes=green, bash=yellow, errors=red, Regent=orange.
- **FR-012**: `--no-tui` flag MUST continue to work, producing the existing plain-text timestamped output to stdout.
- **FR-013**: Layout MUST be responsive to terminal size via `tea.WindowSizeMsg`. Minimum supported size: 80×24. When the terminal is below minimum, the TUI MUST render a centered "Terminal too small — resize to at least 80×24" message instead of the panel layout.
- **FR-014**: `tea.WithAltScreen()` MUST be used (already the case) to preserve the user's terminal on exit.
- **FR-015**: The TUI MUST support suspending to `$EDITOR` via `tea.Suspend` and resume cleanly after editor exit.
- **FR-016**: Every `LogEntry` emitted during a session MUST be persisted to a JSONL session log at `.ralph/logs/<session-id>.jsonl` before being forwarded to the TUI.
- **FR-017**: The store MUST write entries synchronously (append + flush) so that a crash or Regent kill never loses events that were emitted before the kill signal.
- **FR-018**: The store MUST support reading back all entries for a given iteration number by scanning `LogIterStart`/`LogIterComplete` boundaries without loading the full file into memory.
- **FR-019**: The store MUST expose a `Store` interface so the backing implementation can be replaced (e.g., with SQLite) without changing callers.
- **FR-020**: `--no-tui` mode MUST also write to the JSONL session log so that headless runs produce reviewable history.
- **FR-021**: Session log filenames MUST be deterministic per session (`<unix-timestamp>-<pid>.jsonl`) so that the Regent's restart cycle appends to the same file rather than creating a new one.
- **FR-022**: The `LogEntry` struct MUST be serialized with `encoding/json` using the existing field names. No schema migration is needed — the struct is the schema.
- **FR-023**: The store MUST enforce a configurable session log retention policy: keep the last N session logs (default 20), deleting the oldest on startup. N is configurable via `log_retention` in `ralph.toml`.

### Key Entities

- **Panel**: A rectangular region of the terminal with its own model, view, keybindings, and focus state. Panels are composed into the root `Model`.
- **Tab**: A named sub-view within a panel. Tabs share the panel's viewport but render different content. Switched via `[`/`]`.
- **FocusTarget**: An enum identifying which panel currently has keyboard focus. Determines keybinding dispatch and border highlighting.
- **Iteration**: A completed or in-progress loop iteration, stored in a slice on the root model, fed by `LogIterStart`/`LogIterComplete` events. Full tool-use output is persisted in the session log and read back on demand.
- **Session Log**: An append-only JSONL file at `.ralph/logs/<session-id>.jsonl` containing every `LogEntry` for a session. Survives Regent restarts within the same process. Indexed by iteration number via byte offsets for fast retrieval.

---

## Technical Design

### Package Structure

```
internal/store/
├── store.go            # Store interface: Append, Iterations, IterationLog, Close
├── store_test.go
├── jsonl.go            # JSONL implementation: append-only file writer + indexed reader
├── jsonl_test.go
└── index.go            # In-memory byte-offset index: maps iteration N → file offset range

internal/tui/
├── app.go              # Root Model: Init/Update/View, panel composition, focus management
├── app_test.go
├── focus.go            # FocusTarget enum, focus cycling logic
├── focus_test.go
├── layout.go           # Responsive layout calculator: given (w,h) → panel rects
├── layout_test.go
├── keymap.go           # Global keybindings + per-panel dispatch table
├── keymap_test.go
├── theme.go            # Colors, border styles, accent (replaces current styles.go)
├── theme_test.go
├── panels/
│   ├── specs.go        # Specs list panel: model, update, view, keybindings
│   ├── specs_test.go
│   ├── iterations.go   # Iterations list panel: model, update, view
│   ├── iterations_test.go
│   ├── main_view.go    # Main panel: tabbed content (output, spec content, iteration detail)
│   ├── main_view_test.go
│   ├── secondary.go    # Secondary panel: tabbed content (regent, git, tests, cost)
│   ├── secondary_test.go
│   ├── header.go       # Header bar renderer (stateless — receives props)
│   ├── header_test.go
│   ├── footer.go       # Footer bar renderer (context-sensitive hints)
│   └── footer_test.go
├── components/
│   ├── logview.go      # Scrollable log viewport with follow mode (wraps bubbles/viewport)
│   ├── logview_test.go
│   ├── tabbar.go       # Tab bar component: renders tab titles, handles [/] switching
│   └── tabbar_test.go
└── msg.go              # Custom tea.Msg types shared across panels
```

### Root Model Composition

```go
type Model struct {
    // Sub-models (each implements its own Update/View)
    specs      panels.SpecsPanel
    iterations panels.IterationsPanel
    mainView   panels.MainView
    secondary  panels.SecondaryPanel

    // Layout
    width, height int
    focus         FocusTarget

    // Loop state (fed from events)
    events    <-chan loop.LogEntry
    store     store.Reader // read-only handle for past iteration drill-down
    mode      string
    branch    string
    iteration int
    maxIter   int
    totalCost float64
    commit    string
    state     LoopState // idle, planning, building, failed, regentRestart

    // Config
    accentColor string
    done        bool
    err         error
}
```

#### LoopState Transitions

`LoopState` follows a strict state machine. Invalid transitions are no-ops (logged as warnings).

```
idle ──→ planning ──→ building ──→ idle (success)
  │         │            │
  │         │            ├──→ failed ──→ idle (user restart)
  │         │            │
  │         │            └──→ regentRestart ──→ building
  │         │
  │         └──→ failed ──→ idle
  │
  └──→ building ──→ (same as above)
```

| From | To | Trigger |
|------|----|---------|
| `idle` | `planning` | User presses `p` or smart-run starts planning |
| `idle` | `building` | User presses `b` or `ralph build` starts |
| `planning` | `building` | Plan phase completes, build begins |
| `planning` | `failed` | Plan phase errors |
| `building` | `idle` | All iterations complete successfully |
| `building` | `failed` | Unrecoverable error or user presses `x` |
| `building` | `regentRestart` | Regent kills loop (hang/crash/test failure) |
| `regentRestart` | `building` | Regent relaunches loop |
| `failed` | `idle` | User acknowledges or presses `b`/`R` to restart |

The root `Update` dispatches `tea.KeyMsg` to the focused panel's update function. Global keys (`tab`, `1-4`, `q`, `?`) are handled before dispatch. `logEntryMsg` is broadcast to all sub-models that need it (iterations panel, main view output, secondary Regent tab, header state). When the iterations panel requests a past iteration's output, the main view reads from `store.Reader` rather than memory.

### Session Log Store (`internal/store/`)

#### Interface

```go
// Writer is used by wiring.go to persist events as they flow through.
type Writer interface {
    Append(entry loop.LogEntry) error
    Close() error
}

// Reader is used by the TUI to retrieve past iteration data.
type Reader interface {
    Iterations() ([]IterationSummary, error)
    IterationLog(n int) ([]loop.LogEntry, error)
    SessionSummary() (SessionSummary, error)
}

// Store combines both interfaces. Wiring creates one; passes Writer to
// the forwarding goroutine and Reader to the TUI.
type Store interface {
    Writer
    Reader
}

type IterationSummary struct {
    Number   int
    Mode     string
    CostUSD  float64
    Duration float64
    Subtype  string    // "success", "error_max_turns", etc.
    Commit   string
    StartAt  time.Time
    EndAt    time.Time
}

type SessionSummary struct {
    SessionID    string
    StartedAt    time.Time
    TotalCost    float64
    Iterations   int
    LastCommit   string
    Branch       string
}
```

#### JSONL Implementation

File location: `.ralph/logs/<unix-timestamp>-<pid>.jsonl`

Each line is a JSON-serialized `loop.LogEntry` using `encoding/json`. The `LogEntry` struct is the schema — no wrapper, no envelope.

```
{"Kind":1,"Timestamp":"2026-02-27T14:23:01Z","Message":"── iteration 1 ──","Iteration":1,"MaxIter":10,"Branch":"feat/tui","Mode":"build"}
{"Kind":2,"Timestamp":"2026-02-27T14:23:02Z","ToolName":"Read","ToolInput":"app/main.go",...}
{"Kind":2,"Timestamp":"2026-02-27T14:23:03Z","ToolName":"Write","ToolInput":"app/service.go",...}
{"Kind":3,"Timestamp":"2026-02-27T14:23:07Z","Iteration":1,"CostUSD":0.14,"Duration":4.2,"Subtype":"success"}
{"Kind":1,"Timestamp":"2026-02-27T14:23:08Z","Message":"── iteration 2 ──","Iteration":2,...}
...
```

**Write path** (`Append`):
1. `json.Marshal` the entry
2. Write line + `\n` to the open file handle
3. `file.Sync()` to flush to disk (ensures durability across Regent kills)

The sync-per-write cost is acceptable because events arrive at human-readable frequency (~1-10/sec during active Claude runs), not high throughput.

**Read path** (`IterationLog`):
1. On first read (or after detecting new bytes via `file.Stat`), scan the file line-by-line building an in-memory index: `map[int]offsetRange` where `offsetRange` is `{startByte, endByte}` for each iteration's `LogIterStart` to `LogIterComplete` span.
2. To read iteration N: `file.ReadAt` from `index[N].startByte` to `index[N].endByte`, split lines, `json.Unmarshal` each. Malformed lines (e.g., truncated by a mid-write kill) are skipped with a warning to stderr.
3. Index is cached and extended incrementally — only scan new bytes past the last known offset.

This avoids loading the full log for large sessions while keeping the implementation simple (no B-tree, no WAL, just sequential scan + offset bookkeeping).

**Session ID**: `fmt.Sprintf("%d-%d", startTime.Unix(), os.Getpid())`. The Regent restarts Ralph within the same process, so PID is stable. The wiring code creates the store once at session start and passes it through.

#### Integration Point (wiring.go)

The existing forwarding goroutine in `runWithRegentTUI` gains one line:

```go
go func() {
    defer close(forwardDone)
    for entry := range loopEvents {
        rgt.UpdateState(entry)
        sessionStore.Append(entry)  // ← new: persist before forwarding
        select {
        case tuiEvents <- entry:
        default:
        }
    }
}()
```

The same pattern applies to `runWithRegent` (no-TUI), `runWithStateTracking`, and `runWithTUIAndState`. All four wiring paths get store integration.

### Layout Algorithm

`layout.go` computes panel rectangles from `(width, height)`:

```
sidebarWidth  = max(24, min(35, width * 25 / 100))
mainWidth     = width - sidebarWidth - 1  (1 for border)
headerHeight  = 1
footerHeight  = 1
bodyHeight    = height - headerHeight - footerHeight
specsHeight   = bodyHeight * 40 / 100
itersHeight   = bodyHeight - specsHeight
mainHeight    = bodyHeight * 65 / 100
secondaryHeight = bodyHeight - mainHeight
```

When `width < 80` or `height < 24`: render a centered "Terminal too small — resize to at least 80×24" message. No panel layout is attempted.

### Event Flow (store added to existing wiring)

```
Claude process → claude.Event channel → loop.Loop.Run() → loop.LogEntry channel
    ↓ (loopEvents)
cmd/ralph/wiring.go: forwardDone goroutine
    ├── regent.UpdateState(entry)          [existing]
    ├── sessionStore.Append(entry)         [NEW — persist to JSONL]
    └── tuiEvents <- entry                 [existing]
        ↓
tui.Model.Update()
    ├── broadcast to sub-models (live)     [existing]
    └── store.Reader for drill-down        [NEW — past iteration reads]
```

The `tui.New()` signature changes to accept a `store.Reader` and config/options. `wiring.go` creates the `store.Store`, passes `Writer` to the forwarding goroutine and `Reader` to the TUI constructor.

### New Dependency

```
github.com/charmbracelet/bubbles  # list, viewport, textinput, spinner
```

Justification: official bubbletea component library from Charm. Provides battle-tested viewport (scrolling), list (selectable items), textinput (spec creation), and spinner (loading states). Avoids reimplementing these from scratch.

---

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Operator can identify loop state (running, idle, failed), current iteration, branch, and cost within 2 seconds of glancing at the TUI.
- **SC-002**: Navigating from live output to a specific spec takes at most 3 keystrokes (`1` to focus specs, `j`/`k` to select, `enter` to view).
- **SC-003**: All existing `LogKind` event types render correctly in the new TUI with no information loss compared to the current single-panel view.
- **SC-004**: `--no-tui` mode produces identical output to the current implementation (regression test via captured output comparison).
- **SC-005**: TUI renders correctly at 80×24 minimum terminal size with no panics or layout overflow.
- **SC-006**: `go test ./internal/tui/...` passes with ≥80% coverage on layout calculation, focus cycling, event dispatch, and panel rendering.
- **SC-007**: No new dependencies beyond `charmbracelet/bubbles`. The JSONL store uses only `encoding/json` and `os` from the standard library.
- **SC-008**: Binary size increase is ≤2MB from adding bubbles.
- **SC-009**: After a Regent kill/restart cycle, all pre-restart iteration logs are accessible in the iterations panel within 1 second of TUI launch.
- **SC-010**: Session log write latency (Append + Sync) adds <5ms per event to the forwarding path.
- **SC-011**: `go test ./internal/store/...` passes with ≥80% coverage on write, read, index building, and edge cases (empty file, partial writes, concurrent read/write).

---

## Constitution Check

| Principle | Compliance |
|---|---|
| I. Spec-Driven | This spec drives the implementation. No code without spec. |
| II. Supervised Autonomy | Regent integration preserved — secondary panel surfaces all Regent activity. JSONL store ensures iteration history survives Regent kill/restart cycles. |
| III. Test-Gated Commits | Table-driven tests for layout, focus, event dispatch, store write/read/index. ≥80% coverage target. |
| IV. Idiomatic Go | bubbletea + lipgloss only. bubbles is the sole new dep. Store uses stdlib only (`encoding/json`, `os`). Small packages, clear interfaces, explicit state. |
| V. Observable Loops | Every `LogKind` renders. Header shows state at a glance. Regent has its own tab. Session log makes all iteration output reviewable, not just the current one. |
