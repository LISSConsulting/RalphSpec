# Feature Specification: Git Worktree Support via Worktrunk

**Feature Branch**: `007-worktree-support`
**Created**: 2026-03-07
**Status**: Draft
**Input**: User description: "Git worktree support via worktrunk. Ralph should integrate with worktrunk to enable parallel AI agent workflows using git worktrees."

## Clarifications

### Session 2026-03-07

- Q: Where should parallel agents write their session logs? → A: Hybrid — agents log locally in their worktree's `.ralph/logs/`, dashboard aggregates by scanning all known worktree paths.
- Q: What happens if a developer launches a second agent on a branch that already has an active agent? → A: Reject — prevent launching a duplicate; display clear error message.
- Q: How should stale worktrees be cleaned up when auto-merge is off? → A: Manual via CLI command or TUI keybind; also cleaned up automatically on merge (Ralph supports an explicit merge command). Ralph never silently removes worktrees otherwise.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Launch a Single Build in a New Worktree (Priority: P1)

A developer wants to run a Ralph build loop in an isolated worktree so their main working directory stays clean for manual work. They run `ralph build --worktree` (or `ralph build -w`). Ralph invokes worktrunk to create a new worktree for the active spec's branch, starts the build loop inside that worktree, and streams events back to the TUI. When the loop completes, the developer reviews the results and decides whether to merge or discard.

**Why this priority**: This is the foundational capability. Every subsequent story (parallel agents, auto-merge, Regent supervision) depends on Ralph being able to run a single loop inside a worktree. Without this, nothing else works.

**Independent Test**: Can be fully tested by running `ralph build --worktree` on a project with worktrunk installed, verifying that a new worktree is created, the build loop runs inside it, and events stream to the terminal. Delivers isolated builds without polluting the developer's working directory.

**Acceptance Scenarios**:

1. **Given** a project with worktrunk installed and an active spec on branch `007-worktree-support`, **When** the developer runs `ralph build --worktree`, **Then** Ralph creates a new worktree via `wt switch -c <branch>`, runs the build loop inside that worktree directory, and streams log events to the TUI or stdout.
2. **Given** Ralph is running a build in a worktree, **When** the build loop completes (spec complete or max iterations), **Then** Ralph logs a completion message including the worktree path and branch name, and the worktree remains available for review.
3. **Given** a project without worktrunk installed, **When** the developer runs `ralph build --worktree`, **Then** Ralph emits a clear error message explaining that worktrunk (`wt`) is required and how to install it.
4. **Given** Ralph is running a build in a worktree with `--no-tui`, **When** the loop runs, **Then** log lines include the worktree branch name as context so the developer can distinguish output from multiple concurrent runs.

---

### User Story 2 - Launch Multiple Parallel Agents from the Dashboard (Priority: P2)

A developer opens the Ralph TUI dashboard and wants to run multiple specs in parallel. They select a spec from the Specs panel and press a keybind to launch it in a new worktree. Each launched agent appears in the dashboard with its own status. The developer can launch up to N agents simultaneously (configurable), each working on a different spec in its own worktree.

**Why this priority**: This is the core value proposition of worktree support — transforming Ralph from sequential to parallel. It builds directly on Story 1's single-worktree capability.

**Independent Test**: Can be tested by launching the dashboard, starting two builds on different specs via keybind, and verifying both appear in the TUI with independent status updates. Delivers the ability to work on multiple features simultaneously.

**Acceptance Scenarios**:

1. **Given** the developer is in the TUI dashboard with the Specs panel focused, **When** they select a spec and press `W` (launch in worktree), **Then** Ralph creates a new worktree for that spec's branch and starts a build loop inside it, and the Iterations panel shows the new agent's activity.
2. **Given** two agents are running in separate worktrees, **When** both emit log events, **Then** the TUI displays events from each agent distinguishably (e.g., prefixed by branch name or in separate views).
3. **Given** the maximum parallel agent limit (default: 5) has been reached, **When** the developer tries to launch another agent, **Then** Ralph displays a message indicating the limit and does not launch a new agent.
4. **Given** an agent in a worktree completes its loop, **When** the developer views the dashboard, **Then** the completed agent shows a "done" status and the worktree remains until explicitly removed.

---

### User Story 3 - Merge and Clean Up Worktrees (Priority: P3)

A developer has a completed build in a worktree. They can merge it in two ways: (a) explicitly via `ralph worktree merge` or a TUI keybind, or (b) automatically if `auto_merge = true` in `ralph.toml`. Either path invokes worktrunk's `wt merge` to squash-merge the branch and clean up the worktree. The developer can also discard worktrees via `ralph worktree clean` or a TUI keybind.

**Why this priority**: Merge and cleanup close the loop — without them, worktrees accumulate and branches linger. The explicit merge command is the safe default; auto-merge is the power-user opt-in.

**Independent Test**: Can be tested by completing a build in a worktree, then running `ralph worktree merge` and verifying the branch is merged and worktree removed. Separately, test auto-merge by enabling the config and verifying automatic merge on completion. Delivers full agent lifecycle management.

**Acceptance Scenarios**:

1. **Given** a completed build in a worktree on branch `007-worktree-support`, **When** the developer runs `ralph worktree merge`, **Then** Ralph invokes `wt merge <target>` to squash-merge the branch, logs the merge result, and the worktree is cleaned up.
2. **Given** `[worktree] auto_merge = true` is set and a build loop completes with spec-complete status, **When** the Regent confirms tests pass, **Then** Ralph automatically invokes `wt merge <target>`, logs the result, and cleans up the worktree.
3. **Given** auto-merge is enabled but the Regent's test command fails after loop completion, **When** the merge would occur, **Then** Ralph skips the merge, logs a warning that tests failed, and leaves the worktree intact for manual review.
4. **Given** auto-merge is disabled (default), **When** a build loop completes in a worktree, **Then** Ralph logs completion but takes no merge action — the worktree and branch remain for manual review or explicit merge.
5. **Given** a merge conflict occurs during `wt merge`, **When** the merge fails, **Then** Ralph logs the conflict error, leaves the worktree intact, and optionally notifies via the notification webhook.
6. **Given** completed and failed worktrees exist, **When** the developer runs `ralph worktree clean`, **Then** Ralph removes the selected worktrees and their branches via worktrunk.
7. **Given** a worktree has an active running agent, **When** the developer tries to merge or clean it, **Then** Ralph rejects the operation with a clear error — the agent must be stopped first.

---

### User Story 4 - Worktree Status in TUI Dashboard (Priority: P4)

The developer opens the Ralph TUI dashboard and sees a view of all active worktrees with their current status — which spec each is working on, iteration count, cost, and whether the agent is running, idle, or completed. This gives at-a-glance visibility into all parallel work.

**Why this priority**: Observability is essential for managing parallel agents but is not blocking for the core workflow. A developer can manage worktrees via the command line even without TUI status.

**Independent Test**: Can be tested by launching the dashboard with multiple worktrees active, verifying each appears with correct status information. Delivers visibility into parallel agent activity.

**Acceptance Scenarios**:

1. **Given** three worktrees are active (two running, one completed), **When** the developer opens the dashboard, **Then** all three worktrees appear with their status (running/completed), spec name, iteration count, and accumulated cost.
2. **Given** a worktree's agent finishes while the dashboard is open, **When** the loop completes, **Then** the dashboard updates the worktree's status in real-time from "running" to "completed".
3. **Given** no worktrees are active, **When** the developer opens the dashboard, **Then** the worktree view is empty or shows a hint about launching worktrees.

---

### User Story 5 - Regent Supervises All Active Worktrees (Priority: P5)

The Regent supervisor monitors all active worktree agents, not just the primary loop. If any worktree agent crashes, hangs, or fails tests, the Regent intervenes for that specific worktree — restarting, rolling back, or killing as appropriate — without affecting other running agents.

**Why this priority**: Per-worktree supervision is important for reliability at scale, but a single-worktree workflow (Story 1) already benefits from the existing Regent. This story extends supervision to the parallel case.

**Independent Test**: Can be tested by running two agents in worktrees, simulating a hang in one, and verifying the Regent kills and restarts only the hung agent while the other continues unaffected.

**Acceptance Scenarios**:

1. **Given** two agents are running in separate worktrees, **When** one agent hangs (no output for `hang_timeout_seconds`), **Then** the Regent kills and restarts only the hung agent; the other agent continues unaffected.
2. **Given** an agent in a worktree produces a commit that fails the test command, **When** the Regent detects the failure, **Then** the Regent rolls back the commit in that worktree only.
3. **Given** an agent in a worktree crashes, **When** the Regent detects the crash, **Then** the Regent restarts the agent in the same worktree with exponential backoff, up to `max_retries`.

---

### Edge Cases

- What happens when the developer runs `ralph build --worktree` but `wt` is not installed or not on PATH? Ralph checks for worktrunk availability at startup and exits with a clear installation message.
- What happens when the target worktree path already exists (e.g., from a previous interrupted run)? Ralph detects the existing worktree and reuses it (via `wt switch` without `-c`) rather than failing.
- What happens when the developer cancels a worktree agent via Ctrl+C in the TUI? Ralph sends a graceful stop signal to that specific agent's loop; the worktree and branch are preserved for later resumption.
- What happens when a worktree build is running and the developer also runs `ralph build` in the main directory? Both run independently — the main directory build and worktree builds are separate processes with separate git state.
- What happens when disk space runs out while creating a worktree? Worktrunk handles this — Ralph surfaces the error from `wt switch` and does not start the loop.
- What happens when the developer manually deletes a worktree directory while an agent is running in it? The agent's git operations fail; the Regent detects the crash and does not attempt to restart (the worktree is gone).
- What happens when `wt merge` is invoked but the target branch has diverged? Worktrunk's merge rebases first; if conflicts arise, the merge fails and Ralph logs the error, leaving the worktree intact.
- What happens when `--worktree` is combined with `--roam`? Ralph creates a worktree for the roam sweep, running the improvement sweep in isolation. This is valid — roam can operate in a worktree.
- What happens when the developer tries to launch a second agent on the same branch? Ralph rejects the launch with a clear error message ("Agent already running on branch X"). Only one agent per branch is allowed to prevent git conflicts within the same worktree.
- What happens to completed worktrees when auto-merge is off? They remain on disk until the developer explicitly merges (`ralph worktree merge`) or cleans up (`ralph worktree clean`) via CLI or TUI keybind. Ralph never silently removes worktrees.
- What happens when the developer runs `ralph worktree merge` on a worktree whose agent is still running? Ralph rejects the merge with an error — the agent must be stopped or completed first.

## Requirements *(mandatory)*

### Functional Requirements

**Worktree Lifecycle (P1)**:

- **FR-001**: Ralph MUST detect whether worktrunk (`wt`) is available on PATH before attempting any worktree operation, and emit a clear error with installation instructions if missing.
- **FR-002**: The `--worktree` (short: `-w`) flag MUST be available on `ralph build`, `ralph loop build`, and `ralph loop run` commands.
- **FR-003**: When `--worktree` is set, Ralph MUST create a new worktree via worktrunk (`wt switch -c <branch>`) and run the loop inside that worktree's directory.
- **FR-004**: If a worktree for the target branch already exists, Ralph MUST switch to it (`wt switch <branch>`) instead of failing.
- **FR-005**: The loop's working directory (`Loop.Dir`) MUST be set to the worktree path, not the original repository root, so all file operations (prompt reading, spec resolution) happen inside the worktree.
- **FR-006**: When a worktree loop completes, Ralph MUST log the completion including the worktree branch and path.

**Parallel Agents (P2)**:

- **FR-007**: The TUI dashboard MUST support launching multiple concurrent build loops, each in its own worktree, via a keybind on the Specs panel.
- **FR-008**: Ralph MUST enforce a configurable maximum number of parallel agents (default: 5), rejecting new launches when the limit is reached.
- **FR-009**: Each parallel agent MUST have its own independent loop state, event stream, and store writer. Each agent writes session logs to its own worktree's `.ralph/logs/` directory.
- **FR-010**: Log events from parallel agents MUST be distinguishable by branch name in both TUI and non-TUI output.
- **FR-025**: Ralph MUST reject launching a new agent on a branch that already has an active agent running, displaying a clear error message (e.g., "Agent already running on branch X").

**Log Aggregation (P2)**:

- **FR-023**: The TUI dashboard MUST aggregate session logs from all known worktree paths by scanning each worktree's `.ralph/logs/` directory.
- **FR-024**: The dashboard MUST track the set of active worktree paths so it knows which directories to scan for log aggregation.

**Merge & Cleanup (P3)**:

- **FR-026**: Ralph MUST provide a `ralph worktree merge` command that merges a completed worktree's branch via `wt merge <target>` and removes the worktree on success.
- **FR-027**: Ralph MUST provide a `ralph worktree clean` command that removes one or more completed/failed worktrees (branch + worktree directory) via worktrunk.
- **FR-028**: The TUI dashboard MUST support a keybind to merge a selected completed worktree (invoking `wt merge`) and a keybind to remove/clean a selected worktree.
- **FR-029**: Ralph MUST NOT automatically remove worktrees except when: (a) auto-merge is enabled and merge succeeds, or (b) the developer explicitly invokes merge or clean.

**Auto-Merge (P3)**:

- **FR-011**: Ralph MUST support an `auto_merge` option in the `[worktree]` config section (default: `false`).
- **FR-012**: When `auto_merge` is enabled and a worktree loop completes with spec-complete status AND tests pass, Ralph MUST invoke `wt merge <target_branch>` to merge and clean up.
- **FR-013**: When `auto_merge` is enabled but tests fail after loop completion, Ralph MUST skip the merge and log a warning.
- **FR-014**: When `wt merge` fails (e.g., conflict), Ralph MUST log the error and preserve the worktree for manual resolution.
- **FR-015**: The target branch for auto-merge MUST be configurable via `[worktree] merge_target` (default: the branch from which the worktree was created).

**TUI Status (P4)**:

- **FR-016**: The TUI dashboard MUST display a list of all active worktrees with their status (running, completed, failed), spec name, iteration count, and cost.
- **FR-017**: Worktree status MUST update in real-time as agents emit events.
- **FR-018**: The developer MUST be able to select a worktree in the TUI to view its detailed log in the Main panel.

**Multi-Worktree Regent (P5)**:

- **FR-019**: The Regent MUST supervise each worktree agent independently — crash recovery, hang detection, and test-gated rollback MUST apply per-worktree.
- **FR-020**: A failure in one worktree agent MUST NOT affect other running agents.

**Configuration (P1)**:

- **FR-021**: `ralph.toml` MUST support a `[worktree]` configuration section with fields: `enabled` (bool, default false), `max_parallel` (int, default 5), `auto_merge` (bool, default false), `merge_target` (string, default ""), and `path_template` (string, default "" — uses worktrunk's default).
- **FR-022**: The `path_template` config option, if set, MUST be passed to worktrunk to control where worktrees are created on disk.

### Key Entities

- **WorktreeAgent**: Represents a single Claude agent running inside a git worktree. Tracks the branch name, worktree path, loop state (running/completed/failed), iteration count, cost, and associated spec. Owned by the orchestrator. Each agent has its own session log in `<worktree>/.ralph/logs/`.
- **Orchestrator**: Manages the lifecycle of multiple WorktreeAgents. Handles creation, supervision delegation to the Regent, status aggregation for the TUI (by scanning each worktree's log directory), and auto-merge on completion. Lives in the TUI dashboard or a new top-level coordinator.
- **WorktreeConfig**: The `[worktree]` section of `ralph.toml`. Controls whether worktree support is active, parallelism limits, auto-merge behavior, and path templates.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A developer can launch a build loop in an isolated worktree with a single command (`ralph build -w`) and see results without their main working directory being affected.
- **SC-002**: A developer can run up to 5 parallel agents from the TUI dashboard, each working on a different spec simultaneously, reducing wall-clock time for multi-spec projects proportionally.
- **SC-003**: With auto-merge enabled, completed worktree builds that pass tests are merged and cleaned up without manual intervention.
- **SC-004**: All existing single-agent workflows (`ralph build`, `ralph loop run`, dashboard mode) continue to work identically when `[worktree] enabled = false` (default) — zero regressions.
- **SC-005**: Worktree agent failures (crash, hang, test failure) are isolated — a failure in one agent does not affect any other running agent.
- **SC-006**: The TUI dashboard provides at-a-glance visibility into all parallel agents' status, refreshing in real-time as events arrive.

## Assumptions

- Worktrunk (`wt`) is installed separately by the developer. Ralph does not install or manage worktrunk — it detects its presence and uses it as an external dependency.
- Worktrunk's `wt switch -c` creates both the branch and worktree atomically. Ralph relies on this behavior rather than orchestrating separate git branch + worktree commands.
- Worktrunk's `wt merge` handles squash, rebase, and cleanup. Ralph delegates the full merge lifecycle to worktrunk rather than implementing its own merge logic.
- The developer's machine has sufficient disk space for multiple worktrees. Each worktree is a full working copy (though git shares object storage).
- The `ANTHROPIC_API_KEY` or Claude subscription supports concurrent sessions. Ralph does not rate-limit against the Claude API — cost management is the developer's responsibility.
- The `[worktree]` feature is opt-in. All existing behavior is preserved when the feature is not enabled. The `--worktree` flag activates it for single commands; `[worktree] enabled = true` activates it for dashboard mode.
- Shell integration for worktrunk (`wt config shell install`) is the developer's responsibility. Ralph invokes `wt` as a subprocess and does not need shell integration (no directory-changing needed — Ralph reads the worktree path from wt's output).
- On Windows, the developer has resolved the `wt` alias conflict with Windows Terminal (either by using `git-wt` or disabling the Windows Terminal alias). Ralph will check for both `wt` and `git-wt` on Windows.
