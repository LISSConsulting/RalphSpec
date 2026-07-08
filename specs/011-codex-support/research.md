# Research: Complete Codex Support

## R-001: Unified Agent Resolution

**Decision**: Use one shared agent resolution path for command-line loops, roam runs, dashboard-started runs, worktree orchestration, and mixed-role workflows. The effective agent is resolved from an explicit per-run selection first, then the saved project default, then the existing default agent.

**Rationale**: Current support already accepts `claude` and `codex` in configuration and CLI overrides, but advanced paths can bypass that resolution. A shared resolver prevents drift, preserves opt-in behavior, and gives every workflow the same validation and error semantics.

**Alternatives considered**:
- Keep separate resolution logic per command path. Rejected because it would keep dashboard and worktree behavior inconsistent.
- Add Codex-specific commands. Rejected because users need agent selection, not duplicate workflow commands.
- Infer agent from environment variables. Rejected because it is less visible and harder to audit than explicit config and run overrides.

## R-002: Shared Agent Factory for Advanced Paths

**Decision**: Replace direct construction of Claude-backed agents in dashboard and orchestrator paths with a shared factory that accepts the effective agent, working directory, role/run context, and configured options.

**Rationale**: Worktree and dashboard flows currently need the same agent construction guarantees as normal loops: availability validation, actionable startup errors, no silent fallback, and metadata attribution. A shared factory keeps those guarantees in one place.

**Alternatives considered**:
- Patch only worktree creation to instantiate Codex. Rejected because dashboard and future mixed-role paths would remain divergent.
- Move all agent code into a new top-level package. Rejected for this feature because the existing shared interface can be extended with less churn.
- Keep Codex unsupported for advanced paths. Rejected because the feature explicitly requires full support.

## R-003: Codex Availability and Startup Validation

**Decision**: Validate Codex availability before starting any Codex-backed workflow, including dashboard and parallel runs, and report setup, authentication, and invalid selection failures before doing work.

**Rationale**: Failing early avoids partial work, keeps errors attributable to the correct run, and satisfies the requirement that unsupported or unavailable Codex states are actionable.

**Alternatives considered**:
- Let the first Codex invocation fail naturally. Rejected because it can leave noisy or partial run state.
- Validate Codex only when saving configuration. Rejected because availability can change between configuration and execution.
- Silently fall back to Claude. Rejected by the spec and prior design because it obscures user intent.

## R-004: Concurrent Session Isolation

**Decision**: Treat each Codex-backed concurrent run as an independent agent session with its own workspace, lifecycle state, logs, event fan-in tag, cancellation path, Regent supervision, and final result metadata.

**Rationale**: Worktree and parallel workflows are safe only when the operator can attribute status and failures to the correct run. The existing orchestrator model already tracks per-agent state and can carry effective agent metadata through the same boundaries.

**Alternatives considered**:
- Share one Codex process across concurrent runs. Rejected because it risks cross-run interference and unclear cancellation.
- Disable concurrent Codex after adding dashboard support. Rejected because the feature requires advanced Codex modes.
- Serialize Codex runs globally. Rejected because it would break expected parallel workflow behavior.

## R-005: Metadata and Observability Contract

**Decision**: Persist and display the effective agent in loop log entries, session summaries, iteration summaries, Regent state, orchestrator worktree entries, dashboard panels, status output, and historical run review.

**Rationale**: Project defaults can change after a run finishes, so history cannot infer agent identity from current config. Live dashboards also need per-role and per-run attribution for mixed-agent workflows.

**Alternatives considered**:
- Show agent only in live log prefixes. Rejected because historical review would be incomplete.
- Store agent only at session level. Rejected because multi-role and worktree runs need per-run or per-role attribution.
- Infer from command arguments. Rejected because dashboard actions and defaults may not have explicit arguments.

## R-006: Mixed-Agent Workflow Boundary

**Decision**: Allow mixed-agent workflows only where Ralph already models distinct run or role assignments. Each role must resolve and display its own effective agent; workflows that do not model roles should use one effective agent for the whole run.

**Rationale**: This supports full Codex participation without introducing ambiguous partial substitution. It also keeps the UX predictable: a workflow either has one selected agent or explicit per-role selections.

**Alternatives considered**:
- Permit arbitrary per-step agent changes. Rejected because it complicates attribution and recovery.
- Ban mixed-agent workflows entirely. Rejected because the spec requires Codex support for multi-role activities where supported.
- Use Codex automatically for some roles. Rejected because it violates explicit selection and no-silent-substitution requirements.

## R-007: Documentation and Compatibility

**Decision**: Update user-facing documentation to describe Codex prerequisites, project default configuration, per-run overrides, dashboard/worktree behavior, mixed-agent attribution, troubleshooting, and regression-safe fallback to the existing default when Codex is not selected.

**Rationale**: Complete support includes operability. Users should be able to set up, inspect, and troubleshoot Codex without reading source code.

**Alternatives considered**:
- Rely on command help only. Rejected because setup and troubleshooting need examples and context.
- Document Codex as experimental with hidden flags. Rejected because the feature requires first-class support.
- Omit advanced workflow documentation. Rejected because advanced paths are the main completion gap.
