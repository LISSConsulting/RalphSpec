# Implementation Plan: Codex Agent Support

**Branch**: `010-codex-support` | **Date**: 2026-04-18 | **Spec**: `specs/010-codex-support/spec.md`
**Input**: Feature specification from `specs/010-codex-support/spec.md`

## Summary

Add Codex as an opt-in agent for Ralph's main single-run plan/build workflows by introducing explicit agent selection, a Codex CLI adapter, and persisted agent metadata across loop logs, session summaries, Regent state, and user-visible status output. Preserve Claude as the default agent, reuse the existing loop/Regent/store plumbing, and fail fast for unsupported Codex workflows such as worktree orchestration and dashboard-launched parallel flows.

## Technical Context

**Language/Version**: Go 1.24  
**Primary Dependencies**: cobra, BurntSushi/toml, bubbletea, lipgloss, existing `internal/claude` event/interface types, external Codex CLI binary  
**Storage**: `ralph.toml`, JSONL session logs in `.ralph/logs/`, Regent state in `.ralph/regent-state.json`  
**Testing**: `go test ./...`, `go vet ./...`, table-driven unit tests for config, command parsing, adapter parsing, and selection precedence  
**Target Platform**: darwin/arm64, darwin/amd64, linux/amd64, windows/amd64  
**Project Type**: CLI tool with optional TUI  
**Performance Goals**: Agent selection and availability validation complete before the first iteration begins; unsupported Codex flows fail immediately with actionable errors; agent metadata is visible in live and recorded runs with no extra operator steps  
**Constraints**: Preserve existing Claude defaults; add no new Go dependencies unless parser requirements force it; keep Codex initial scope to single-run plan/build workflows; reject unsupported worktree/orchestrator/dashboard flows explicitly; maintain existing Regent, git, and session-log behavior across agents  
**Scale/Scope**: One project-level default agent, one per-run override, one active single-run loop at a time; persisted agent identity for session and iteration history

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Spec-Driven | PASS | Spec 010 and clarification session define scope, selection model, and unsupported workflows. |
| II. Supervised Autonomy | PASS | Plan preserves the Regent path and requires Codex to flow through the same supervision hooks as Claude. |
| III. Test-Gated Commits | PASS | Work includes config, command, adapter, state, and visibility tests; no bypass of existing test-gated flow. |
| IV. Idiomatic Go | PASS | Plan prefers a small new internal adapter package plus existing interfaces; no new external Go dependency is planned. |
| V. Observable Loops | PASS | Agent identity becomes part of log/state/session metadata, removing silent ambiguity in mixed-agent histories. |

## Project Structure

### Documentation (this feature)

```text
specs/010-codex-support/
├── spec.md
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   └── cli-agent-selection.md
└── tasks.md
```

### Source Code (affected files)

```text
cmd/ralph/
├── commands.go              # Add per-run --agent override on supported loop commands
├── execute.go               # Resolve effective agent, create adapter, reject unsupported Codex flows
├── execute_test.go          # Selection precedence and unsupported-flow coverage
├── main.go                  # Remove Claude-only startup messaging from generic loop paths
└── wiring.go                # Surface agent identity in single-run TUI paths; reject dashboard Codex launches

internal/claude/
├── claude.go                # Keep shared agent interface/events for initial release

internal/codex/
├── agent.go                 # Codex CLI adapter implementing the shared agent interface
├── parser.go                # Map `codex exec --json` JSONL events into shared loop events
└── parser_test.go           # Parser and startup/failure coverage

internal/config/
├── config.go                # Add [agent] and [codex] config sections plus validation/defaults
├── config_test.go           # Supported values, defaults, and validation errors
└── scaffold.go              # Scaffold new config keys into generated ralph.toml

internal/loop/
├── loop.go                  # Remove Claude-specific messages/config access; emit agent-aware entries
├── event.go                 # Add agent identity to structured log entries
└── loop_test.go             # Agent-agnostic loop messaging and metadata coverage

internal/store/
├── store.go                 # Add agent identity to summaries
├── jsonl.go                 # Persist and return agent-aware session data
└── jsonl_test.go            # Session/iteration metadata coverage

internal/regent/
├── state.go                 # Persist effective agent in regent state
└── state_test.go            # State round-trip coverage

internal/tui/
├── app.go                   # Show selected agent in live session context
└── panels/
    └── header.go            # Render active agent in header/status area

README.md                    # Document Codex as supported and describe selection/setup
ralph.toml                   # New [agent] and [codex] settings in examples/tests
```

**Structure Decision**: Preserve the existing command/loop/store/regent architecture. Add one small `internal/codex/` adapter package rather than renaming broad shared plumbing now. Keep the shared agent/event interface in `internal/claude/` for this release to minimize churn, while making loop execution and user-visible metadata agent-agnostic.

## Phase 0: Research

### R-001: Agent Selection Contract

**Decision**: Use a project-level `[agent]` section with `type = "claude" | "codex"` plus a repeatable per-run `--agent` flag on supported loop entry points. Per-run selection overrides the project default for that run only.

**Rationale**: This matches the clarified spec, keeps existing projects unchanged by default, and makes precedence explicit and testable in one place.

**Alternatives considered**:
- Reusing `[project]` for agent selection: mixes unrelated concerns and weakens validation.
- Environment-variable-only selection: not discoverable or durable enough for project defaults.
- Separate commands such as `ralph codex build`: duplicates command surface and fragments tests.

### R-002: Codex Runtime Invocation

**Decision**: Run Codex through `codex exec --json --full-auto --cd <dir> [--model <model>] <prompt>` and translate the emitted JSONL stream into the existing shared event model.

**Rationale**: `codex exec` is the non-interactive Codex path, `--json` provides machine-readable events, `--cd` aligns the subprocess working root with Ralph's loop directory, and `--full-auto` enables autonomous execution without resorting to the more dangerous sandbox-bypass flag.

**Alternatives considered**:
- Interactive `codex` TUI mode: not appropriate for Ralph's autonomous loop.
- `--dangerously-bypass-approvals-and-sandbox`: lower friction but violates the repo's safety posture.
- Direct API integration: larger dependency and auth surface than needed for the first release.

### R-003: Shared Interface Strategy

**Decision**: Keep the existing `internal/claude.Agent` interface and shared event types for the initial release, and add `internal/codex.Agent` as another implementation of that interface.

**Rationale**: The interface seam already exists and the loop depends on it. A broad rename from `internal/claude` to a new generic package would create documentation and code churn that is unnecessary for initial Codex support.

**Alternatives considered**:
- Renaming `internal/claude` to a new generic package immediately: cleaner long-term, but high churn across the codebase and constitution references.
- Duplicating loop execution for Codex: would split logic for logging, git, and Regent behavior.

### R-004: Persisted Agent Identity

**Decision**: Add an `Agent` field to `loop.LogEntry`, `store.IterationSummary`, `store.SessionSummary`, and `regent.State`, and surface it in status/TUI output.

**Rationale**: The spec requires operators to distinguish live and historical runs by agent. Persisting the effective agent in the primary data structures is more reliable than inferring it later from config or branch state.

**Alternatives considered**:
- Inferring the agent from current config: incorrect for historical sessions after config changes.
- Showing agent only in transient console output: fails the observability requirement for stored sessions.

### R-005: Unsupported Workflow Handling

**Decision**: Explicitly reject Codex in workflows outside the initial release scope: worktree mode, orchestrator-managed parallel agents, and dashboard-launched loop starts. Speckit commands remain separate from this feature.

**Rationale**: The current worktree/orchestrator code still hard-codes Claude. An explicit error is safer and clearer than a partial or silent fallback.

**Alternatives considered**:
- Silent fallback to Claude: violates the clarified selection model and obscures operator intent.
- Partial support in some advanced flows: hard to test and easy to regress.

### R-006: Codex Config Scope

**Decision**: Add a minimal `[codex]` section with `model = ""` for the initial release and rely on Codex CLI defaults for sandbox/approval behavior beyond Ralph's fixed `--full-auto` invocation.

**Rationale**: This keeps the first release small while still allowing developers to pick a Codex model explicitly when needed.

**Alternatives considered**:
- Mirroring every Codex CLI option in `ralph.toml`: too large for the initial release.
- No Codex config section at all: makes model selection harder and limits future extension.

## Phase 1: Data Model & Contracts

### Data Model

See `specs/010-codex-support/data-model.md`.

### Contracts

See `specs/010-codex-support/contracts/cli-agent-selection.md`.

### Quickstart

See `specs/010-codex-support/quickstart.md`.

## Post-Design Constitution Check

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Spec-Driven | PASS | Design artifacts trace directly to spec requirements and clarifications. |
| II. Supervised Autonomy | PASS | Both adapters converge on the same loop, store, and Regent paths. |
| III. Test-Gated Commits | PASS | Planned test matrix covers selection, parser behavior, unsupported-flow rejection, and state persistence. |
| IV. Idiomatic Go | PASS | One new internal package, no planned external dependency additions, explicit validation/errors. |
| V. Observable Loops | PASS | Agent identity is stored and rendered in both live and persisted views. |

## Complexity Tracking

No constitution violations requiring justification.
