# Implementation Plan: Quota-Aware Goal Automation

**Branch**: `014-quota-aware-automation` | **Date**: 2026-08-10 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/014-quota-aware-automation/spec.md`

## Summary

Make Ralph treat agent termination as a typed outcome, apply quota-aware admission before new work, discover deterministic test gates from repository metadata, and offer a ludicrous goal mode whose completion is evidence-driven. The implementation extends existing Go interfaces and configuration, keeps explicit Claude/Codex selections authoritative, and fixes terminal error propagation before adding scheduling behavior.

## Technical Context

**Language/Version**: Go 1.24+
**Primary Dependencies**: Cobra, BurntSushi TOML, existing loop/Regent/store/TUI/worktree/orchestrator packages, external Claude Code and Codex CLIs
**Storage**: `ralph.toml`; `.ralph/regent-state.json`; JSONL run logs
**Testing**: Go `testing`; deterministic fake CLI executables; temporary manifest fixtures
**Target Platform**: Windows, Linux, and macOS terminal environments
**Project Type**: Go CLI with TUI and subprocess harness adapters
**Performance Goals**: One quota probe per admitted iteration; one immutable test discovery scan per run; no polling while an agent is active
**Constraints**: No silent provider/model/credential fallback; unknown quota is not unlimited; shell command execution remains cross-platform; operator cancellation and safety controls always win
**Scale/Scope**: Two harnesses; serial and up to configured parallel worktree workers; mixed-language repositories with nested manifests

## Constitution Check

### Pre-design

- **I. Spec-Driven — PASS**: `spec.md` defines all behavior and acceptance scenarios.
- **II. Supervised Autonomy — PASS**: terminal outcomes and quota decisions become Regent-visible state; ludicrous mode remains supervised.
- **III. Test-Gated Commits — PASS**: metadata discovery creates deterministic run-start gates and preserves explicit commands.
- **IV. Idiomatic Go — PASS**: small interfaces, typed errors, standard library subprocess/JSON/filesystem handling, table tests.
- **V. Observable Loops — PASS**: decisions, plans, harness attribution, pauses, blockers, and outcomes flow through existing logs/state.

### Post-design

- **I — PASS**: contracts trace to FR-001 through FR-028.
- **II — PASS**: quota waiting is bounded; auth and unknown fail-closed states require operator action; cancellation remains authoritative.
- **III — PASS**: only high-confidence immutable steps auto-run; failures cannot produce verified completion.
- **IV — PASS**: provider-specific probing is isolated behind an optional interface; no framework or database added.
- **V — PASS**: normalized terminal and admission events eliminate text-only hidden state.

## Project Structure

### Documentation (this feature)

```text
specs/014-quota-aware-automation/
├── spec.md
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── agent-outcome-contract.md
│   └── test-plan-contract.md
└── tasks.md
```

### Source Code (repository root)

```text
cmd/ralph/
├── commands.go            # harness/ludicrous flags and tests command
├── execute.go             # run-start quota/test-plan wiring
└── wiring.go              # Regent/TUI state integration

internal/
├── claude/                # existing adapter, normalized terminal outcome
├── codex/                 # existing adapter plus app-server quota probe
├── quota/                 # snapshots, policy, shared admission gate
├── testplan/              # metadata discovery and immutable plan execution
├── loop/                  # terminal propagation and ludicrous completion
├── regent/                # classified retry/state behavior
├── config/                # harness, quota, discovery, ludicrous settings
├── orchestrator/          # shared pre-launch gate for parallel workers
└── tui/                   # agent/mode/paused-blocked status surfaces
```

**Structure Decision**: Preserve the single Go CLI layout. Provider-specific protocol code stays with each adapter; provider-neutral policy and test discovery get focused internal packages. Existing `claude.Event`/`claude.Agent` compatibility is retained while adding normalized terminal fields, avoiding a repository-wide rename unrelated to behavior.

## Implementation Phases

### Phase 1: Reliable terminal outcomes

1. Add normalized terminal kinds and typed run errors.
2. Make Claude and Codex adapters emit one terminal outcome, including exit/cancellation diagnostics.
3. Make loop iteration fail on terminal outcomes, clear previous success, and prevent false completion.
4. Teach Regent which terminal classes retry, block, pause, or stop; persist explicit lifecycle status.

### Phase 2: Quota admission

1. Add quota snapshot/policy/gate package and validated configuration.
2. Implement Codex app-server rate-limit probe.
3. Gate every serial iteration and central parallel launch; serialize shared snapshots/decisions.
4. Implement bounded wait without consuming crash retries and fail-closed handling.

### Phase 3: Test plans

1. Implement deterministic, ignored-directory-aware manifest discovery.
2. Add explicit/aggregate/workspace precedence and deduplication.
3. Add multi-step cross-platform execution.
4. Add `ralph tests detect|run` and snapshot automatic plans at run start.

### Phase 4: Ludicrous mode

1. Add configuration and `--ludicrous` overrides.
2. Make effective max unbounded unless a positive override is supplied.
3. Inject bounded-scope goal persistence instructions.
4. Require available tasks, clean worktree, terminal success, passing plan, and confirming no-change iteration.
5. Display/persist effective mode and completion evidence.

## Complexity Tracking

No constitution violations. New packages isolate independently testable policy/protocol boundaries; they replace duplicated adapter decisions rather than adding architectural layers.
