# Feature Specification: Quota-Aware Goal Automation

**Feature Branch**: `014-quota-aware-automation`  
**Created**: 2026-08-10  
**Status**: Draft  
**Input**: User description: "Implement quota-aware agent scheduling for Claude Code and Codex; correct terminal error propagation; discover and run tests from project metadata; and add a ludicrous mode that persists toward verified goal completion without bypassing safety, quota, or operator stop controls."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Stop Wasting Exhausted Quota (Priority: P1)

An operator runs Ralph for a long task. Ralph distinguishes terminal agent failures from ordinary streamed warnings, checks available provider quota when the selected harness exposes it, and avoids launching work that cannot complete before the relevant quota resets.

**Why this priority**: Misclassified quota failures currently consume retries and can be recorded as successful completion, directly losing work and operator trust.

**Independent Test**: Feed successful, quota-exhausted, authentication-failed, transient, and terminal-failure agent outcomes through a supervised run and verify that each produces the correct run state and retry decision.

**Acceptance Scenarios**:

1. **Given** an agent terminates with a quota-exhausted outcome and a known reset time, **When** Ralph receives the terminal outcome, **Then** the run is paused or stopped according to policy and is never marked complete.
2. **Given** a provider reports remaining quota below the configured reserve, **When** Ralph is about to start another iteration, **Then** it does not launch that iteration and reports the applicable quota window and reset.
3. **Given** an agent emits a recoverable warning and later succeeds, **When** the iteration ends, **Then** Ralph records success without treating the warning as a terminal failure.
4. **Given** parallel work shares one provider quota, **When** the quota reserve is reached, **Then** no additional workers are launched.

---

### User Story 2 - Use Any Supported Coding Harness (Priority: P2)

An operator can select Claude Code or Codex as the coding harness and receives consistent events, terminal outcomes, status, history, and supervision behavior.

**Why this priority**: Quota-aware scheduling is only useful when harnesses are first-class and return a common outcome contract.

**Independent Test**: Run each harness adapter against a deterministic fake executable and verify command construction, streamed event conversion, terminal error classification, cancellation, and working-directory behavior.

**Acceptance Scenarios**:

1. **Given** any supported harness is configured, **When** Ralph starts an iteration, **Then** it invokes that harness and attributes all live and historical events to it.
2. **Given** a configured harness is unavailable, **When** Ralph validates the run, **Then** it fails before starting work with an actionable message and does not silently switch harnesses.
3. **Given** a harness lacks a provider-neutral quota query, **When** Ralph schedules work, **Then** it uses the configured conservative policy and terminal response classification rather than fabricating quota availability.

---

### User Story 3 - Discover the Project Test Gate (Priority: P2)

An operator can ask Ralph to inspect project metadata, preview a deterministic test plan, and use that plan for post-iteration verification without hand-writing a shell command for every project.

**Why this priority**: Test-gated autonomy is ineffective when the default configuration has no test command, especially in mixed-language repositories.

**Independent Test**: Create fixture projects representing package scripts, Python test tooling, Go modules, Rust packages/workspaces, and nested mixed-language projects; verify the detected commands, working directories, confidence, precedence, and deduplication.

**Acceptance Scenarios**:

1. **Given** an explicit test command, **When** test discovery is requested, **Then** the explicit command remains authoritative.
2. **Given** a package manifest declares a test script and package manager, **When** Ralph detects tests, **Then** it reports the matching package-manager test command and its working directory.
3. **Given** a workspace-level aggregate test gate covers child projects, **When** Ralph builds the plan, **Then** it does not also schedule duplicate child commands.
4. **Given** no high-confidence test command exists, **When** automatic testing is enabled, **Then** Ralph reports that discovery is inconclusive and does not guess a command.

---

### User Story 4 - Pursue the Goal in Ludicrous Mode (Priority: P3)

An operator can enable ludicrous mode when Ralph should continue pursuing the stated goal until objective completion evidence exists. The mode increases persistence, not authority: safety checks, quota reserves, test gates, explicit scope, and operator stop controls remain binding.

**Why this priority**: Long-running goal work can stop after a locally successful iteration even when declared tasks or verification remain incomplete.

**Independent Test**: Run a multi-iteration fixture where the agent reports success before all declared tasks and verification are complete; verify normal mode may finish under its standard rule while ludicrous mode continues until every objective signal is satisfied.

**Acceptance Scenarios**:

1. **Given** ludicrous mode and unchecked declared tasks, **When** an iteration reports success with no commit, **Then** Ralph continues rather than declaring completion.
2. **Given** ludicrous mode and a failing required test gate, **When** an iteration ends, **Then** Ralph continues recovery or stops with a verified blocker; it never reports success.
3. **Given** ludicrous mode and all declared tasks complete, a clean worktree, a successful terminal outcome, and passing required tests, **When** a confirming iteration produces no further changes, **Then** Ralph reports verified goal completion.
4. **Given** an operator stop, exhausted quota under fail-closed policy, authentication failure, or safety restriction, **When** ludicrous mode is active, **Then** the restriction wins immediately.

### Edge Cases

- A quota snapshot is missing, stale, or contains only one of several provider windows.
- A reset timestamp is in the past, malformed, or farther away than the configured maximum wait.
- A subprocess emits both an error event and a later successful terminal result.
- A previous iteration succeeded immediately before the current iteration terminates with an error and creates no commit.
- Multiple agents share one credential while reporting quota updates at different times.
- Project metadata is malformed, nested below ignored build/vendor directories, or changed during the run.
- A Python project has a `pyproject.toml` but no declared test runner.
- A task runner offers several plausible aggregate gates without identifying one as canonical.
- Ludicrous mode has no machine-verifiable task list or test plan.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Ralph MUST represent the terminal outcome of every agent iteration separately from non-terminal streamed warnings.
- **FR-002**: Ralph MUST NOT mark an iteration, specification, roam run, or supervised run successful after a terminal agent error.
- **FR-003**: Ralph MUST classify terminal outcomes at least as success, quota exhausted, authentication required, transient provider failure, context exhausted, cancelled, or terminal agent failure.
- **FR-004**: A classified quota outcome MUST carry a reset time and quota window when the provider supplies them.
- **FR-005**: Ralph MUST support a configurable quota reserve and a policy that either waits within an operator-defined bound or fails closed.
- **FR-006**: Ralph MUST NOT silently change agents, credentials, or models when quota is low or exhausted.
- **FR-007**: Ralph MUST query fresh quota before an iteration when the selected harness exposes a documented quota interface.
- **FR-008**: Ralph MUST treat unavailable quota data as unknown rather than unlimited.
- **FR-009**: Parallel scheduling MUST honor a shared quota gate before launching each additional worker.
- **FR-010**: Regent MUST persist whether a run is running, paused for quota, blocked for operator action, passed, failed, or stopped.
- **FR-011**: Quota pauses MUST NOT consume ordinary crash retry attempts.
- **FR-012**: Authentication failures MUST stop retries and identify the required operator action.
- **FR-013**: Ralph MUST support Claude Code and Codex as explicit agent selections without silent fallback.
- **FR-014**: Each supported harness MUST provide consistent live events, cancellation, working-directory selection, and terminal outcome reporting.
- **FR-015**: Harness-specific quota capabilities MUST be exposed through one optional common quota snapshot contract.
- **FR-016**: Operators MUST be able to preview a discovered test plan without executing it.
- **FR-017**: Explicitly configured test commands MUST take precedence over discovered commands.
- **FR-018**: Automatic test discovery MUST support package test scripts, declared Python test runners, Go modules, and Rust packages/workspaces.
- **FR-019**: Each discovered test step MUST include its command, working directory, evidence source, and confidence.
- **FR-020**: Automatic execution MUST run only high-confidence steps and MUST fail the gate if any required step fails or cannot start.
- **FR-021**: Discovery MUST avoid duplicate child test steps when a recognized aggregate workspace gate covers them.
- **FR-022**: Ralph MUST snapshot the selected test plan before agent work begins.
- **FR-023**: Operators MUST be able to enable ludicrous mode through configuration and a run-specific command option.
- **FR-024**: Ludicrous mode MUST make iteration count unbounded unless a run-specific positive limit is explicitly supplied.
- **FR-025**: Ludicrous mode MUST inject a goal-persistence instruction that requires objective completion evidence and continued blocker resolution within scope.
- **FR-026**: Ludicrous mode MUST require all declared tasks complete, a clean worktree, a successful terminal result, and a passing required test gate before completion when those signals are available.
- **FR-027**: Ludicrous mode MUST preserve permissions, safety controls, quota policy, test gates, explicit scope, and operator cancellation.
- **FR-028**: Live status and persisted history MUST identify quota decisions, selected test plans, agent type, ludicrous mode, pauses, blockers, and final outcomes.

### Key Entities

- **Terminal Outcome**: Final result of one harness invocation, including category, message, provider, optional reset time, and optional quota window.
- **Quota Snapshot**: Provider-reported usage windows, remaining or used percentage, reset time, freshness, and optional spend-control state.
- **Quota Policy**: Operator choices for reserve, wait behavior, maximum wait, unknown-data behavior, and concurrency gating.
- **Test Plan**: Immutable run-start collection of test steps with command, directory, source, confidence, and aggregate coverage.
- **Run State**: Supervised lifecycle state including active, paused, blocked, stopped, passed, and failed.
- **Goal Mode**: Normal or ludicrous completion policy applied to a run.

## Assumptions

- Quota-aware scheduling controls new work; it does not attempt to throttle token generation inside an already-running provider request.
- Quota data may be unknown when Claude's operator-managed snapshot is unavailable, so unknown-data policy and terminal error classification remain necessary.
- Ludicrous mode means persistent, evidence-driven work and does not authorize broader scope, destructive actions, hidden fallback, or bypassing safety controls.
- Test discovery is opt-in and preserves the current empty test-command behavior unless automatic discovery is explicitly selected.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: In all terminal-error fixtures, zero runs are recorded as successful and zero previous-success signals cause false completion.
- **SC-002**: For providers with quota reporting, Ralph prevents 100% of new iterations whose reported remaining quota is below the configured reserve.
- **SC-003**: Quota pauses consume zero ordinary Regent retry attempts and resume or stop at the configured policy boundary.
- **SC-004**: All four supported harnesses pass the same terminal-outcome and cancellation contract suite.
- **SC-005**: Test discovery produces the expected high-confidence plan for every supported manifest fixture and executes no medium- or low-confidence guess.
- **SC-006**: In mixed-language fixtures, aggregate detection introduces zero duplicate test executions.
- **SC-007**: Ludicrous mode continues through every premature-success fixture and reports completion only after all available objective signals pass.
- **SC-008**: Operator cancellation stops normal and ludicrous runs within one supervision cycle.
