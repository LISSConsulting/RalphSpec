# Feature Specification: Complete Codex Support

**Feature Branch**: `011-codex-support`  
**Created**: 2026-06-20  
**Status**: Draft  
**Input**: User description: "Implement/complete full Codex support."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Use Codex Across All Ralph Workflows (Priority: P1)

A developer who prefers Codex wants to use it anywhere Ralph delegates work to an AI coding agent, including specification-driven planning, task execution, interactive dashboard actions, and automated build loops. The developer selects Codex once for the project or for a run and receives the same Ralph supervision, progress visibility, and completion handling available with other supported agents.

**Why this priority**: Full Codex support is only complete when developers can rely on Codex throughout Ralph's normal workflow instead of encountering partial support or unsupported command paths.

**Independent Test**: Select Codex and complete each user-facing Ralph workflow that delegates agent work, verifying that every workflow starts, reports progress, completes, and records results without requiring a different agent.

**Acceptance Scenarios**:

1. **Given** a developer has selected Codex as the active agent and Codex is available, **When** they start any Ralph workflow that delegates work to an AI coding agent, **Then** Ralph uses Codex and preserves the normal workflow outcome for that action.
2. **Given** a workflow can be started from more than one entry point, **When** the developer starts it from the command line or dashboard, **Then** Codex is used consistently and the developer receives equivalent status and completion feedback.
3. **Given** a Codex-backed workflow completes with changes, results, or validation output, **When** Ralph records the run, **Then** the run clearly identifies Codex as the producing agent.

---

### User Story 2 - Run Advanced Codex Modes Safely (Priority: P2)

A developer wants Codex to participate in Ralph's advanced modes, including parallel work, isolated worktree-based runs, review or remediation loops, and other multi-agent activities. Ralph makes the active agent choice explicit, keeps concurrent runs isolated, and prevents one Codex session from interfering with another run or project.

**Why this priority**: Earlier Codex support was limited to main flows. Completing support requires removing those practical limitations while maintaining Ralph's safety guarantees.

**Independent Test**: Start supported advanced workflows with Codex selected, including at least one concurrent or isolated run, and verify isolation, status reporting, completion handling, and failure containment.

**Acceptance Scenarios**:

1. **Given** a developer starts a parallel or worktree-based Ralph workflow with Codex selected, **When** multiple agent sessions run at the same time, **Then** each session uses Codex only where selected and keeps its workspace, logs, results, and completion state separate.
2. **Given** a multi-agent workflow includes roles that may use different supported agents, **When** the developer chooses Codex for one or more roles, **Then** Ralph shows the selected agent per role and does not silently substitute another agent.
3. **Given** one Codex-backed advanced run fails, **When** other runs are active, **Then** Ralph contains the failure to the affected run and keeps unaffected runs understandable and recoverable.

---

### User Story 3 - Configure, Inspect, and Troubleshoot Codex (Priority: P3)

A developer or operator wants to understand whether Codex is available, how Ralph will use it, and what to do when a Codex-backed run cannot start or complete. Ralph exposes Codex in configuration, dashboard, run history, documentation, and error messages so support needs are clear without source-code inspection.

**Why this priority**: Complete support must be operable. Developers need clear selection, validation, documentation, and troubleshooting for Codex to be trusted in daily use.

**Independent Test**: Review Codex configuration and documentation, run health checks for available and unavailable Codex environments, and verify live and historical views expose Codex status and actionable failures.

**Acceptance Scenarios**:

1. **Given** Codex is not installed, not authenticated, or otherwise unavailable, **When** a developer selects Codex and starts a run, **Then** Ralph stops before doing work and explains the missing prerequisite and next corrective action.
2. **Given** a developer reviews configuration or dashboard status, **When** Codex is available or selected, **Then** Ralph clearly shows its availability and active selection.
3. **Given** a developer reads Ralph's user documentation, **When** they follow the Codex setup and usage instructions, **Then** they can select Codex and run a supported workflow without needing undocumented steps.

---

### Edge Cases

- What happens when a project does not choose Codex explicitly? Ralph continues using the existing default agent so current projects do not change unexpectedly.
- What happens when Codex is selected for one run while another run uses a different supported agent? Each run honors its own explicit selection and displays it independently.
- What happens when Codex becomes unavailable after being selected but before work starts? Ralph fails fast with an actionable setup or availability message.
- What happens when Codex fails during a long-running or parallel workflow? Ralph reports the failed Codex session, preserves available diagnostics, and keeps unaffected sessions isolated.
- What happens when a saved Codex preference is invalid or stale? Ralph rejects the preference with a clear validation message instead of silently falling back to another agent.
- What happens when historical runs include mixed agents? Ralph shows the producing agent for each run so operators can interpret results correctly.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Ralph MUST support Codex as a first-class selectable agent for every user-facing workflow that delegates work to an AI coding agent.
- **FR-002**: Ralph MUST allow Codex to be selected as a saved project default and as a per-run override.
- **FR-003**: Ralph MUST define and display a clear precedence rule where an explicit per-run Codex selection overrides the saved project default for that run only.
- **FR-004**: Ralph MUST preserve existing behavior for projects and runs that do not opt into Codex.
- **FR-005**: Ralph MUST show the active agent before or during each run and in historical run records.
- **FR-006**: Ralph MUST apply the same supervision, progress reporting, validation, completion handling, cancellation, and recovery expectations to Codex-backed runs as to other supported agents.
- **FR-007**: Ralph MUST validate that Codex is available and usable before starting any Codex-backed workflow.
- **FR-008**: Ralph MUST provide actionable error messages when Codex cannot be started, is unavailable, is not authenticated, or has invalid configuration.
- **FR-009**: Ralph MUST reject invalid agent selections with a clear validation error rather than silently falling back to a different agent.
- **FR-010**: Ralph MUST support Codex in parallel, isolated, and multi-role workflows where Ralph already supports agent delegation.
- **FR-011**: Ralph MUST keep concurrent Codex-backed sessions isolated so their workspaces, logs, status, and results remain attributable to the correct run.
- **FR-012**: Ralph MUST allow mixed-agent workflows only when the selected workflow explicitly supports multiple agent roles, and it MUST show the chosen agent for each role.
- **FR-013**: Ralph MUST record enough Codex run metadata for live monitoring, historical review, troubleshooting, and auditability.
- **FR-014**: Ralph MUST document Codex prerequisites, selection options, supported workflows, troubleshooting steps, and expected behavior in mixed-agent or concurrent runs.
- **FR-015**: Ralph MUST expose Codex availability and selected-agent state through normal operator-facing status surfaces.

### Key Entities *(include if feature involves data)*

- **Agent Selection**: The chosen supported agent for a project, run, workflow, or role. Determines which agent Ralph delegates to while preserving the rest of Ralph's workflow behavior.
- **Project Agent Default**: The saved agent preference used when a run starts without an explicit override.
- **Run Agent Override**: A temporary agent choice that applies only to one invocation or dashboard action.
- **Agent Role Assignment**: The selected agent for a specific role in a workflow that can involve more than one agent participant.
- **Agent Run Record**: The run metadata that identifies which agent performed the work, where status and diagnostics can be found, and what outcome occurred.
- **Codex Availability State**: The operator-facing status indicating whether Codex appears ready for use, unavailable, or misconfigured.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A developer with Codex available can select Codex and successfully complete 100% of Ralph workflows that delegate work to an AI coding agent during acceptance testing.
- **SC-002**: 100% of existing projects that do not opt into Codex continue to use their previous agent behavior during regression testing.
- **SC-003**: Operators can identify the active or producing agent for 100% of live, completed, and historical runs through normal Ralph status or review surfaces.
- **SC-004**: In first-time setup testing, at least 90% of developers can resolve a missing or invalid Codex setup using Ralph's displayed guidance and documentation without source-code inspection.
- **SC-005**: In concurrent workflow testing, 100% of Codex-backed sessions keep their work, logs, status, and final outcome attributable to the correct run.
- **SC-006**: A developer can switch between Codex and another supported agent for a single run in under 2 minutes without changing unrelated project settings.

## Assumptions

- Codex support remains additive and opt-in; existing defaults remain unchanged unless a project or run explicitly selects Codex.
- Developers using Codex have responsibility for any external account, authentication, or access requirements, while Ralph is responsible for detecting readiness and explaining setup failures.
- Any workflow that already delegates work to a supported AI coding agent is in scope unless it is intentionally documented as unavailable before planning.
- Ralph should present a consistent operator experience across supported agents even when the underlying agent capabilities differ.
