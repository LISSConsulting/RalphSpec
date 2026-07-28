# Feature Specification: Stdin Steering for Running Agents

**Branch**: `012-stdin-steer` | **Date**: 2026-07-28 | **Status**: Draft

## Summary

Allow the operator to send natural-language steering messages to a running Ralph
loop from the TUI. Messages are delivered in two tiers:

1. **Nudge (always on, both agents)** — queued messages are appended to the next
   iteration's prompt under a clearly delimited "User steering" section. Works
   with any agent because both Claude and Codex take the prompt as an argument.
2. **Live steer (opt-in, `[agent] stdin_steer = true`)** — messages are written
   directly to the running agent process's stdin while it is mid-turn. Claude
   runs in SDK pipe mode (`-p --input-format stream-json`) and receives user
   messages as stream-JSON; Codex receives plain-text lines on a best-effort
   basis (its `exec` mode currently ignores stdin).

## User Stories

### US1 — Queue a steering message from the TUI (P1)

As an operator watching a loop, I want to press `i`, type a message, and press
Enter so that the next iteration's prompt includes my instruction.

**Acceptance**:

1. Given a running loop (or idle dashboard), when I press `i`, an input line
   replaces the footer and all other keys are routed to it.
2. Given text in the input line, when I press Enter, the message is queued
   without blocking the TUI, the footer confirms "steer queued", and the input
   closes. Esc closes the input without sending.
3. Given a queued message, when the next iteration starts, the agent prompt
   contains a `## User steering` section listing all queued messages in order.

### US2 — See steering activity in the log (P2)

As an operator, I want consumed steering messages to appear as a distinct log
kind so I can correlate agent behavior changes with my input, and so they are
persisted in the session JSONL history.

**Acceptance**:

1. When the loop drains queued messages, one `LogSteer` entry per message is
   emitted before the iteration starts, rendered with a compass icon.
2. Steer entries are appended to the session JSONL log like every other kind.

### US3 — Live mid-turn steering behind an opt-in flag (P3)

As an advanced operator, I want to enable `[agent] stdin_steer = true` so that
messages are written to the running agent's stdin mid-turn instead of waiting
for the next iteration.

**Acceptance**:

1. Given `stdin_steer = true` and agent type `claude`, when the loop starts an
   iteration, Claude is spawned as `-p --output-format stream-json
   --input-format stream-json --verbose`, the iteration prompt is written as
   the first stdin user message, and the pipe remains open for the turn.
2. Given a message queued mid-turn, when the steer channel receives it, Ralph
   writes it to the agent stdin as a stream-JSON user message
   (`{"type":"user","message":{"role":"user","content":"<text>"}}`) within one
   second, without disturbing the event parsing goroutine.
3. Given agent type `codex`, when `stdin_steer = true`, messages are written as
   plain-text lines; a failed or ignored write never fails the iteration.
4. Given `stdin_steer = false` (default), agent invocation is byte-identical to
   the current behavior (prompt via `-p`, no stdin pipe).

## Functional Requirements

- **FR-001**: The loop MUST accept an optional `Steer <-chan string`. When nil,
  behavior is unchanged.
- **FR-002**: Before each iteration the loop MUST drain all pending steer
  messages non-blockingly, emit one `LogSteer` event per message, and append
  them to that iteration's prompt under `## User steering`.
- **FR-003**: `LogSteer` MUST be a new `LogKind` persisted by the JSONL store
  without schema changes.
- **FR-004**: The TUI MUST provide an `i` keybinding opening a single-line
  steer input; Enter queues the message (non-blocking, drop with warning when
  the buffer is full), Esc cancels.
- **FR-005**: When `[agent] stdin_steer = true`, the loop MUST pass the steer
  channel to the agent adapter via `RunOptions.Steer`.
- **FR-006**: The Claude adapter, given a non-nil `RunOptions.Steer`, MUST use
  SDK pipe mode: bare `-p`, `--input-format stream-json`, prompt sent as the
  first stdin user message, subsequent steer messages written as they arrive.
- **FR-007**: The Codex adapter, given a non-nil `RunOptions.Steer`, MUST open
  a stdin pipe and write steer messages as plain-text lines, treating write
  errors as non-fatal.
- **FR-008**: Default configuration (`stdin_steer = false`) MUST produce
  byte-identical agent command lines to previous releases.

## Non-Goals

- Worktree/orchestrator agents (each has its own stdin; a per-agent steer UI is
  a follow-up).
- A `ralph send` CLI for headless `--no-tui` runs (no interactive input source).
- Session history loading and `gg`/`G` scrolling (separate features).

## Risks

- Claude CLI input-format changes could break the opt-in path; mitigated by the
  flag defaulting off and by the nudge path covering steering universally.
- Writing to a dead process's stdin returns EPIPE; handled as non-fatal.
