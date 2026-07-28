# Implementation Plan: Stdin Steering for Running Agents

**Branch**: `012-stdin-steer` | **Date**: 2026-07-28 | **Spec**: [spec.md](./spec.md)

## Technical Approach

Extend the existing packages in place. No new dependencies (textinput ships in
`charmbracelet/bubbles`, already vendored via viewport).

### Flow

```
TUI 'i' → textinput → requestSteer(msg) closure
        → non-blocking send → steerCh (buffered 32)
                                  │
        ┌─────────────────────────┼──────────────────────────┐
        ▼ (always)                ▼                          ▼ (stdin_steer=true)
  loop.drainSteers()        LogSteer events emitted     RunOptions.Steer → adapter
  before each iteration →   → TUI render + JSONL        → agent stdin writer
  appended to prompt                                    goroutine (best-effort)
```

### Changes

**internal/loop/event.go** — add `LogSteer` at the end of the `LogKind` const
block (append-only keeps persisted values stable).

**internal/loop/loop.go**
- `Loop.Steer <-chan string` field (nil = feature off).
- `drainSteers() []string` — non-blocking receive loop.
- `Run` calls it before each `iteration`; messages emit `LogSteer` and are
  joined onto the prompt as `## User steering\n- <msg>` lines.
- `iteration` sets `RunOptions.Steer = l.Steer` when
  `l.Config.Agent.StdinSteer` is true.

**internal/claude/claude.go** — `RunOptions.Steer <-chan string`.

**internal/loop/runner.go** (ClaudeAgent)
- When `opts.Steer == nil`: current behavior, byte-identical.
- When non-nil: args become `["-p", "--output-format", "stream-json",
  "--input-format", "stream-json", "--verbose", ...]` (bare `-p`, SDK pipe
  mode); open `cmd.StdinPipe()`; goroutine writes the initial prompt as a
  stream-JSON user message, then each steer message; closes the pipe after
  `cmd.Wait()` returns. Write errors are swallowed.

**internal/codex/agent.go** — same shape: open stdin pipe when steer non-nil,
write `<msg>\n` lines, swallow errors, close on exit. Args unchanged (Codex
`exec` has no stdin protocol yet).

**internal/config/config.go** — `AgentConfig.StdinSteer bool` (`stdin_steer`),
default false; scaffold template documents it.

**internal/tui**
- `tui.New(...)` gains a trailing `requestSteer func(string)` parameter (nil =
  `i` key disabled).
- Model: `steerMode bool`, `steerInput textinput.Model`.
- `handleKey`: when `steerMode`, route all keys to the input; Enter submits
  (non-blocking send; full buffer → transient "steer buffer full" feedback) and
  exits; Esc cancels. `i` enters steer mode when `requestSteer != nil`.
- Footer is replaced by `steer> <input>` while in steer mode; help overlay
  documents `i`.
- `theme.RenderLogLine` renders `LogSteer` as `🧭 <msg>` in accent style.

**cmd/ralph/wiring.go** — each of the three TUI run paths creates
`steerCh := make(chan string, 32)`, assigns `lp.Steer`, and passes a
`requestSteer` closure into `tui.New`. Dashboard's `loopController` owns one
steer channel reused across runs.

## Testing

- loop: fake agent captures the prompt; queued message appears in iteration 2
  prompt and emits `LogSteer`.
- runner: `buildArgs` gains `--input-format stream-json` only with steer; stdin
  writer emits valid user-message JSON for prompt + steer (helper-process
  pattern already used in runner tests).
- codex: steer channel drains without deadlock; args unchanged.
- config: `stdin_steer` parses, defaults false.
- tui: `i`/Enter/Esc state machine; footer swap; full-buffer drop.
- Gates: `gofmt`, `go vet ./...`, `go test ./internal/...` groups, `just ci`
  before release.

## Constitution Check

- **I. Spec-Driven**: PASS — implements `specs/012-stdin-steer/spec.md`.
- **II. Supervised Autonomy**: PASS — steering is human input, not autonomy
  removal; Regent restart semantics unchanged.
- **III. Test-Gated**: PASS — tests listed above run before commit.
- **IV. Idiomatic Go**: PASS — channels, existing adapters, no new deps.
- **V. Observable Loops**: PASS — `LogSteer` is persisted and rendered.
