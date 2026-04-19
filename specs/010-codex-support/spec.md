# Feature Specification: Codex Agent Support

**Feature Branch**: `010-codex-support`  
**Created**: 2026-04-18  
**Status**: Draft  
**Input**: User description: "add support for codex"

## Clarifications

### Session 2026-04-18

- Q: What is the initial Codex support scope? → A: Main runs first: project/run selection plus normal plan/build flows.
- Q: How should developers choose Codex? → A: Both: project default plus explicit per-run override.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Run Ralph with Codex for Main Flows (Priority: P1)

A developer wants to use Codex as Ralph's active coding agent for Ralph's main plan and build workflows without changing the rest of Ralph's supervision, git, and review flow. They select Codex, start a run, and Ralph completes the session while showing progress the same way it does for the existing default agent.

**Why this priority**: This is the core user value of the feature. If a developer cannot successfully run Ralph with Codex in the main workflow, Codex support does not exist in any meaningful sense.

**Independent Test**: Configure a project to use Codex, start a plan or build run, and verify the session begins, progress is visible, and the run completes with the same end-to-end workflow expectations as the existing agent path.

**Acceptance Scenarios**:

1. **Given** a developer has selected Codex as the active agent and Codex is available for use, **When** they start a Ralph plan or build run, **Then** Ralph starts the session with Codex and streams progress, results, and completion state through the normal Ralph workflow.
2. **Given** a Codex-backed run produces changes that require Ralph's normal validation steps, **When** the run completes an iteration, **Then** Ralph applies the same supervision, git, and test-gated behavior used for other supported agents.

---

### User Story 2 - Choose the Agent Per Project or Run (Priority: P2)

A developer wants to decide which supported agent Ralph should use for a given project or invocation. They can keep the existing default agent for one project, choose Codex for another, or override the choice for a single run without rewriting the rest of their workflow.

**Why this priority**: Codex support is only practical if developers can intentionally select it. Clear agent selection also avoids accidental behavior changes for existing Claude-based projects.

**Independent Test**: Set one project or run to use Codex and another to use the existing default agent, then verify each run uses the selected agent and reports that choice clearly before work begins.

**Acceptance Scenarios**:

1. **Given** a project is configured to use Codex by default, **When** a developer starts Ralph without additional overrides, **Then** Ralph uses Codex for that run.
2. **Given** a project defaults to another supported agent, **When** a developer explicitly chooses Codex for a single run, **Then** Ralph uses Codex only for that run and leaves the project's default setting unchanged.
3. **Given** a developer opens Ralph's dashboard or reviews run output, **When** a run is active or completed, **Then** the selected agent is visible so the developer can distinguish Codex sessions from other agent sessions.

---

### User Story 3 - Recover Quickly from Setup or Compatibility Problems (Priority: P3)

A developer tries to use Codex before it is installed, authenticated, or configured correctly. Instead of failing silently or with a generic subprocess error, Ralph explains what is wrong and what the developer needs to fix before retrying.

**Why this priority**: Agent support is incomplete if failures are opaque. Clear setup and compatibility feedback reduces support burden and makes the feature usable by developers who are trying Codex for the first time.

**Independent Test**: Attempt to start a Codex-backed run in an environment where Codex is unavailable or misconfigured and verify Ralph exits early with an actionable message that identifies the problem and the next step.

**Acceptance Scenarios**:

1. **Given** a developer selects Codex but Codex is not available in the environment, **When** they start a run, **Then** Ralph stops before beginning work and shows an actionable setup error.
2. **Given** a developer selects Codex with an unsupported or invalid agent setting, **When** Ralph validates the run configuration, **Then** Ralph rejects the run with a clear explanation of what must be corrected.

---

### Edge Cases

- What happens when a project does not choose an agent explicitly? Ralph continues using the existing default agent so current projects do not change behavior unexpectedly.
- What happens when a developer selects Codex for a run while another project still uses the existing default agent? Each run uses its own explicit selection without leaking that choice across projects.
- What happens when Codex becomes unavailable after selection but before work starts? Ralph fails fast with a clear error before entering the main iteration loop.
- What happens when a developer reviews historical runs from multiple agents? Ralph shows which agent produced each run so results remain understandable.
- What happens when a developer attempts to use Codex in a workflow outside the initial release scope? Ralph clearly reports that Codex is not yet available for that workflow instead of silently falling back or partially running.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Ralph MUST support Codex as a first-class selectable agent for Ralph's main plan and build workflows in the initial release.
- **FR-002**: Ralph MUST allow Codex to be selected as the saved default agent for a project.
- **FR-003**: Ralph MUST allow Codex to be selected for an individual run without changing the project's saved default agent.
- **FR-004**: Ralph MUST preserve existing behavior for projects that do not opt into Codex.
- **FR-005**: Ralph MUST display the selected agent before or during a run so the operator can verify which agent is active.
- **FR-006**: Ralph MUST keep Ralph's existing supervision, logging, git, and post-run validation behavior consistent regardless of whether the active agent is Codex or the existing default agent.
- **FR-007**: Ralph MUST validate that Codex is available and usable before starting a Codex-backed run.
- **FR-008**: Ralph MUST provide actionable error messages when Codex cannot be started, is not configured correctly, or is otherwise unavailable.
- **FR-009**: Ralph MUST record which agent produced a run so live views and historical run review remain understandable.
- **FR-010**: Ralph MUST clearly reject attempts to use Codex in workflows that are outside the initial release scope.
- **FR-013**: Ralph MUST keep unsupported Codex workflows out of scope for the initial release, including parallel-agent, worktree, and other advanced run modes until they are explicitly added in a later feature.
- **FR-011**: Ralph MUST reject invalid agent selections with a clear validation error rather than silently falling back to a different agent.
- **FR-012**: Ralph MUST document Codex as a supported agent, including how a developer selects it and what prerequisites are required.
- **FR-014**: Ralph MUST define a clear precedence rule where an explicit per-run agent selection overrides the project's saved default for that run only.

### Key Entities *(include if feature involves data)*

- **Agent Selection**: The developer's chosen agent for a project or a single run. Determines which supported agent Ralph will launch while preserving the rest of the Ralph workflow.
- **Project Agent Default**: The saved agent preference used when a run starts without an explicit override.
- **Agent Profile**: The saved configuration associated with a supported agent, including any agent-specific defaults or validation rules required before a run can start.
- **Agent Run Record**: The session metadata that identifies which agent performed a given run so operators can interpret live output, history, and troubleshooting information correctly.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A developer who has Codex available can switch a project to Codex and start a successful Ralph plan or build run in under 2 minutes without changing unrelated workflow settings.
- **SC-002**: In acceptance testing, 100% of existing projects that do not opt into Codex continue to use their prior default agent with no behavior change.
- **SC-003**: In acceptance testing, 90% of first-time Codex users can identify and correct missing setup from Ralph's error guidance without needing to inspect source code or external support.
- **SC-004**: Operators can identify the active agent for 100% of live and recorded runs from Ralph's normal UI or run output.

## Assumptions

- Codex support is additive and opt-in; the current default agent remains the default unless a project or run explicitly selects Codex.
- Developers using Codex have already satisfied Codex account, authentication, and local installation requirements outside Ralph.
- Ralph should provide a consistent user-facing workflow across supported agents even if the underlying agent tools differ.
- Codex support in this feature is intentionally limited to the main plan and build workflows; parallel-agent, worktree, and other advanced Codex run modes are deferred.
