# Implementation Plan: Complete Codex Support

**Branch**: `011-codex-support` | **Date**: 2026-06-20 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `specs/011-codex-support/spec.md`

## Summary

Complete Codex support by making Codex a first-class selectable agent across every Ralph path that delegates to an AI coding agent, including command-line loops, smart runs, dashboard-started runs, worktree/parallel orchestration, mixed-role workflows, status surfaces, run history, and documentation. The technical approach is to retain the existing Go CLI architecture, extend shared agent resolution and metadata propagation, replace direct Claude construction in advanced paths with configured agent factories, and add regression coverage for both Codex and non-Codex behavior.

## Technical Context

**Language/Version**: Go 1.24+  
**Primary Dependencies**: Cobra command routing, Bubble Tea/Lip Gloss TUI, BurntSushi TOML config, existing Regent, loop, store, worktree, orchestrator, Claude, and Codex packages  
**Storage**: `ralph.toml` configuration plus `.ralph/` run logs, session summaries, and Regent state files  
**Testing**: `go test ./...`, `go vet ./...`, table-driven unit tests, command behavior tests, TUI/orchestrator state tests where applicable  
**Target Platform**: Cross-platform CLI for darwin/arm64, darwin/amd64, linux/amd64, and windows/amd64  
**Project Type**: Single Go CLI/TUI application  
**Performance Goals**: Agent selection and availability checks add no noticeable delay to non-Codex runs; dashboard and status updates remain responsive during concurrent agent sessions  
**Constraints**: Preserve opt-in behavior, avoid silent fallback, keep concurrent work isolated, avoid unnecessary dependencies, preserve existing Claude behavior, support Windows paths and process handling  
**Scale/Scope**: All Ralph workflows that delegate agent work, including main loop commands, smart runs, dashboard actions, worktree/parallel orchestration, mixed-role workflows, live status, historical records, and documentation

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **I. Spec-Driven**: PASS. This plan implements `specs/011-codex-support/spec.md` and preserves the prior `010-codex-support` work as an earlier baseline.
- **II. Supervised Autonomy**: PASS. Codex-backed runs remain under Ralph/Regent supervision; advanced Codex runs must preserve restart, revert, cancellation, and escalation behavior.
- **III. Test-Gated Commits**: PASS. The implementation plan requires regression tests and `go test ./...` plus `go vet ./...` before completion.
- **IV. Idiomatic Go**: PASS. Changes stay in existing packages, use explicit error returns, table-driven tests, and avoid new dependencies unless justified.
- **V. Observable Loops**: PASS. Active agent identity, events, failures, and historical metadata are explicit requirements for live and recorded runs.

## Project Structure

### Documentation (this feature)

```text
specs/011-codex-support/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── cli-agent-selection.md
│   ├── dashboard-agent-contract.md
│   └── run-metadata-contract.md
└── tasks.md
```

### Source Code (repository root)

```text
cmd/ralph/
├── commands.go          # CLI flags and command entry points
├── execute.go           # agent resolution, loop setup, smart run, speckit invocation
└── *_test.go            # command and execution behavior tests

internal/config/
├── config.go            # agent and Codex configuration contract
└── *_test.go

internal/claude/
└── *.go                 # existing shared agent interface and Claude adapter

internal/codex/
└── *.go                 # Codex adapter and availability checks

internal/loop/
├── *.go                 # shared loop execution, events, metadata, cancellation
└── *_test.go

internal/orchestrator/
├── *.go                 # worktree/parallel agent orchestration
└── *_test.go

internal/regent/
├── *.go                 # supervised run state and rollback behavior
└── *_test.go

internal/store/
├── *.go                 # persisted run and iteration summaries
└── *_test.go

internal/tui/
├── *.go                 # dashboard run launch, status, logs, worktree panel
└── *_test.go

internal/worktree/
└── *.go                 # isolated workspace support

README.md                # user-facing Codex setup and workflow documentation
```

**Structure Decision**: Use the existing single Go CLI/TUI layout. No new top-level application, service, or dependency layer is required. Codex completion should centralize agent construction/resolution so command, dashboard, and orchestrator paths share behavior instead of duplicating agent-specific branches.

## Complexity Tracking

No constitution violations or complexity exceptions are required.

## Phase 0: Research

See [research.md](./research.md). All technical context unknowns are resolved; no `NEEDS CLARIFICATION` items remain.

## Phase 1: Design & Contracts

- Data model: [data-model.md](./data-model.md)
- CLI agent selection contract: [contracts/cli-agent-selection.md](./contracts/cli-agent-selection.md)
- Dashboard agent contract: [contracts/dashboard-agent-contract.md](./contracts/dashboard-agent-contract.md)
- Run metadata contract: [contracts/run-metadata-contract.md](./contracts/run-metadata-contract.md)
- Quickstart and acceptance guide: [quickstart.md](./quickstart.md)

## Post-Design Constitution Check

- **I. Spec-Driven**: PASS. Design artifacts trace directly to requirements FR-001 through FR-015.
- **II. Supervised Autonomy**: PASS. Orchestrator and TUI plans preserve Regent-supervised execution and isolated failure handling.
- **III. Test-Gated Commits**: PASS. Contracts and quickstart define regression checks for Codex and non-Codex paths.
- **IV. Idiomatic Go**: PASS. Design reuses existing packages and avoids new dependencies.
- **V. Observable Loops**: PASS. Metadata, dashboard status, and log contracts require agent attribution and actionable failures across live and historical surfaces.
