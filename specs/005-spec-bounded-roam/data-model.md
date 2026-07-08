# Data Model: Spec-Bounded Loop with --roam Flag

**Date**: 2026-03-02

## Entities

### CompletionState (local to Loop.Run)

Tracks the two-signal completion detection within the iteration loop. This is NOT a struct — it's local variables in `Run()`.

| Variable | Type | Description |
|----------|------|-------------|
| prevSubtype | string | Result subtype from the previous iteration ("success", "error_max_turns", etc.) |

**State transitions**:
- After each iteration: `prevSubtype` is set to the iteration's result subtype
- Between iterations: if `prevSubtype == "success"` AND current iteration produced no commits → spec/sweep complete
- `prevSubtype` resets naturally when `Run()` returns

**Per-iteration commit detection**: `iteration()` captures `LastCommit()` before Claude runs and compares after. Returns a `commitsProduced bool`.

### Extended Structs

#### BuildConfig (internal/config/config.go)

```go
type BuildConfig struct {
    PromptFile    string `toml:"prompt_file"`
    MaxIterations int    `toml:"max_iterations"`
    Roam          bool   `toml:"roam"`           // NEW: enable cross-spec improvement sweep
}
```

#### Loop (internal/loop/loop.go)

```go
type Loop struct {
    // ... existing fields ...
    Roam bool   // NEW: when true, use sweep prompt augmentation and emit LogSweepComplete
    Spec string // NEW: resolved spec name for prompt augmentation (empty = no spec context)
    SpecDir string // NEW: resolved spec directory path for prompt augmentation
}
```

#### LogKind (internal/loop/event.go)

```go
const (
    // ... existing kinds ...
    LogSpecComplete  LogKind = iota // NEW: spec work detected as complete (default mode)
    LogSweepComplete                // NEW: sweep work detected as complete (roam mode)
)
```

**Note**: Two separate log kinds allow the TUI and notification system to distinguish between "single spec done" and "entire sweep done" without parsing message text.

#### LogEntry (internal/loop/event.go)

No new fields. The existing `Message`, `Branch`, and `Commit` fields carry sufficient context. The log kind distinguishes the event type.

### iteration() Return Signature Change

Current: `func (l *Loop) iteration(...) (float64, error)`
New: `func (l *Loop) iteration(...) (float64, string, bool, error)`

| Return | Type | Description |
|--------|------|-------------|
| cost | float64 | Iteration cost in USD |
| subtype | string | Result subtype ("success", "error_max_turns", etc.) |
| commitsProduced | bool | Whether HEAD changed during this iteration |
| err | error | Iteration error |

### git.Runner.CreateAndCheckout (internal/git/git.go)

```go
// CreateAndCheckout creates a new branch and checks it out.
// Equivalent to: git checkout -b <name>
func (r *Runner) CreateAndCheckout(name string) error
```

**Not added to GitOps interface** — only called by `executeLoop()` which has `*git.Runner` directly.

## Relationships

```
cmd/ralph/execute.go                    internal/loop/loop.go
┌─────────────────────┐                ┌─────────────────────────┐
│ executeLoop()       │                │ Loop.Run()              │
│  ├─ --roam check    │                │  ├─ read prompt file    │
│  ├─ --spec conflict │                │  ├─ augment prompt      │
│  ├─ sweep branch    │──creates──►    │  │   (Spec/SpecDir/Roam)│
│  │   creation       │                │  ├─ iteration loop      │
│  ├─ set Loop.Roam   │                │  │   ├─ capture HEAD    │
│  ├─ set Loop.Spec   │                │  │   ├─ run Claude      │
│  └─ call Run()      │                │  │   ├─ compare HEAD    │
│                     │                │  │   └─ return subtype  │
│ executeRun()        │                │  ├─ completion check    │
│  └─ same roam logic │                │  │   (prevSubtype +     │
└─────────────────────┘                │  │    commitsProduced)  │
                                       │  └─ emit LogSpec/Sweep  │
internal/git/git.go                    │      Complete           │
┌─────────────────────┐                └─────────────────────────┘
│ Runner              │
│  └─ CreateAndCheckout│──used by──► executeLoop() only
└─────────────────────┘

internal/config/config.go
┌─────────────────────┐
│ BuildConfig         │
│  └─ RoamConfig      │──read by──► executeLoop(), executeRun()
└─────────────────────┘
```
