# Tasks: Quota-Aware Goal Automation

**Input**: Design documents from `/specs/014-quota-aware-automation/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Tests**: Regression and contract tests are required because this feature changes terminal outcomes, supervision, scheduling, and completion behavior.

## Phase 1: Specification and Contracts

- [x] T001 Define normalized terminal outcome and quota contracts in `specs/014-quota-aware-automation/contracts/agent-outcome-contract.md`
- [x] T002 Define deterministic test-plan discovery and CLI contract in `specs/014-quota-aware-automation/contracts/test-plan-contract.md`
- [x] T003 Document Claude and Codex quota and terminal behavior in `specs/014-quota-aware-automation/research.md`
- [x] T004 Define ludicrous mode evidence and safety rules in `specs/014-quota-aware-automation/data-model.md`

## Phase 2: Core Reliability

- [x] T005 [P] Add terminal outcome types, precedence, and classifier in `internal/claude/events.go`
- [x] T006 [P] Make Claude subprocess/parser emit normalized terminal outcomes in `internal/claude/claude.go` and tests
- [x] T007 [P] Make Codex subprocess/parser emit normalized terminal outcomes in `internal/codex/agent.go` and tests
- [x] T008 Propagate terminal failures and clear prior success in `internal/loop/loop.go` with regression tests
- [x] T009 Add typed retry/block/pause/stop decisions in `internal/regent/regent.go` with state tests
- [x] T010 Persist and display explicit Regent lifecycle status in `internal/regent/state.go` and `cmd/ralph/execute.go`

## Phase 3: Quota Scheduling

- [x] T011 [P] Add validated quota configuration in `internal/config/config.go`, scaffold, and tests
- [x] T012 [P] Implement snapshots, decisions, bounded waits, and shared gate in `internal/quota/`
- [x] T013 [P] Implement Codex app-server `account/rateLimits/read` probe in `internal/codex/quota.go` and tests
- [x] T014 [P] Ingest documented Claude status-line quota snapshot files in `internal/claude/quota.go` and tests
- [x] T015 Gate each loop iteration with observable quota decisions in `internal/loop/loop.go`
- [x] T016 Gate parallel worker launch centrally in `internal/orchestrator/orchestrator.go`

## Phase 4: Supported Harness Scope

- [x] T017 Remove the experimental OMP and OpenCode adapters and contract tests
- [x] T018 Restrict configuration and scaffolding to Claude Code and Codex
- [x] T019 Restrict CLI and orchestrator agent resolution to Claude Code and Codex
- [x] T020 Preserve consistent CLI, dashboard, status, and history attribution for both supported harnesses

## Phase 5: Test Discovery

- [x] T021 [P] Implement manifest traversal, ignore rules, evidence, precedence, and deduplication in `internal/testplan/discover.go`
- [x] T022 [P] Add package, Python, Go, Rust, workspace, aggregate, malformed, and mixed fixture tests in `internal/testplan/discover_test.go`
- [x] T023 Implement immutable multi-step plan execution in `internal/testplan/run.go` and tests
- [x] T024 Add `ralph tests detect|run` text/JSON preview commands in `cmd/ralph/commands.go` and tests
- [x] T025 Snapshot automatic plans before agent work and use them for Regent/ludicrous gates

## Phase 6: Ludicrous Mode

- [x] T026 Add configuration, scaffold, `--ludicrous`, and positive-max override behavior
- [x] T027 Inject scope-preserving objective-evidence instructions into iteration prompts
- [x] T028 Require tasks, clean tree, successful outcome, passing plan, and confirming no-change iteration in ludicrous completion tests
- [x] T029 Persist and display effective ludicrous mode and completion evidence

## Phase 7: Verification and Cleanup

- [x] T030 Run targeted terminal, quota, harness, discovery, and ludicrous regression tests
- [x] T031 Smoke-test CLI help, `tests detect`, and fake one-iteration harness workflows
- [x] T032 Run `go test ./...`
- [x] T033 Run `go vet ./...`
- [x] T034 Review the complete change for defects, stale paths, docs, and scaffolding

## Dependencies

- Phase 2 blocks all behavioral phases.
- T011 and T012 block T013–T016.
- T017 and T018 can proceed after terminal outcome types exist.
- T021–T024 can proceed independently after configuration shape is settled; T025 requires loop/Regent wiring.
- Phase 6 requires terminal outcomes and test-plan results.
- Phase 7 follows all implementation phases.

## Parallel Opportunities

- T005–T007 modify separate adapter/type files after the shared event shape is fixed.
- T011–T014 cover independent config/policy/provider slices.
- T017 and T018 are independent harness adapters.
- T021–T023 separate discovery fixtures and execution.
