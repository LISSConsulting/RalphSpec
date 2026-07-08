# Feature Specification: Spec-Bounded Loop with --roam Flag

**Feature Branch**: `005-spec-bounded-roam`
**Created**: 2026-03-02
**Status**: Draft
**Input**: User description: "Ralph should respect spec boundaries by default; --roam flag enables cross-spec autonomy"

## Clarifications

### Session 2026-03-02

- Q: Is --max a global budget or per-spec in roam mode? → A: Global — counter continues across the entire roam session.
- Q: Should roam skip "done" specs or visit all? → A: Visit all. Roam is an improvement sweep, not feature development. Ralph checks code against specs, finds gaps, missing tests, and fixes. No specs are skipped.
- Q: Should roam switch between spec branches or run on a single branch? → A: Single branch. Roam creates a `sweep/YYYY-MM-DD` branch from develop. All specs are visible on this branch. No branch-switching during the sweep.
- Q: Should spec-boundary enforcement include structural checks? → A: No. Prompt augmentation + completion detection only for v1. No structural verification of commit scope.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Default Loop Stops at Spec Boundary (Priority: P1)

A developer runs `ralph build --max 10 --no-tui` on branch `005-my-feature`. Ralph works through iterations on the spec. After iteration 4, Claude reports success and the next iteration produces no new commits. Ralph detects that the spec's work is complete and gracefully stops the loop, even though 6 iterations remain in the budget.

**Why this priority**: This is the core behavior change. Without it, Ralph crosses spec boundaries silently, producing work on the wrong branch that the developer must untangle. Every Ralph user benefits from this guardrail.

**Independent Test**: Can be fully tested by running a loop with a mock agent that reports success and produces no commits — verify the loop exits early with a "spec complete" message.

**Acceptance Scenarios**:

1. **Given** Ralph is running on branch `005-my-feature` with `--max 10`, **When** Claude's result subtype is `"success"` AND the following iteration produces no new commits, **Then** the loop emits a "spec complete" log entry and exits gracefully with no error.
2. **Given** Ralph is running on branch `005-my-feature` with `--max 10`, **When** Claude's result subtype is `"success"` but the next iteration DOES produce new commits, **Then** the loop continues normally (Claude found more work within the spec).
3. **Given** Ralph is running on branch `005-my-feature` with `--max 5`, **When** all 5 iterations produce commits, **Then** the loop completes normally at max iterations (no early exit — work is still happening).
4. **Given** Ralph is running with `--max 1`, **When** the single iteration completes with success and no commits, **Then** the loop exits normally at max iterations (single-iteration runs should not trigger spec-complete logic since there is no "next iteration" to confirm).

---

### User Story 2 - Roam Mode Sweeps All Specs (Priority: P2)

A developer runs `ralph build --roam`. Ralph creates a `sweep/YYYY-MM-DD` branch from develop and begins an improvement sweep. Claude has visibility into all specs on this branch and works through them — checking code against each spec, finding gaps, missing tests, and fixes. When Claude reports success and produces no further commits, the sweep is complete. The developer reviews the sweep results and PRs the branch back to develop.

**Why this priority**: This enables the "wandering journeyman" workflow where Ralph autonomously sweeps the entire codebase for improvements. It builds on Story 1's completion detection to know when the sweep is done.

**Independent Test**: Can be tested by running a roam loop on a branch with multiple spec directories, a mock agent that produces some fixes then reports success with no commits — verify Ralph creates the sweep branch, runs iterations, and stops when idle.

**Acceptance Scenarios**:

1. **Given** a developer runs `ralph build --roam`, **When** Ralph starts, **Then** Ralph creates a `sweep/YYYY-MM-DD` branch from the current branch (typically develop), logs the branch creation, and begins looping.
2. **Given** Ralph is running in roam mode, **When** Claude reports success and the next iteration produces no new commits, **Then** Ralph emits a "sweep complete" log entry and exits gracefully.
3. **Given** Ralph is running in roam mode with `--max 10`, **When** 10 iterations are consumed, **Then** Ralph stops at the iteration limit regardless of sweep progress. `--max` is a global budget.
4. **Given** Ralph is running in roam mode, **When** the sweep branch already exists (`sweep/YYYY-MM-DD` from an earlier run today), **Then** Ralph appends a sequence number (e.g., `sweep/YYYY-MM-DD-2`) or reuses the existing branch.

---

### User Story 3 - Prompt Guardrails Keep Claude in Scope (Priority: P3)

A developer runs `ralph build` (default, no `--roam`). When Ralph launches Claude, the prompt includes the active spec's context — its name, directory, and a directive to focus only on that spec's work. Claude naturally stays within the spec's boundaries because the prompt guides it. Ralph's completion detection (Story 1) provides the safety net.

**Why this priority**: Prompt guardrails reduce wasted iterations by keeping Claude focused. This enhances quality rather than enabling new capability, so it's lower priority than the completion detection.

**Independent Test**: Can be tested by verifying that the prompt passed to Claude includes the active spec's name and scope directive when a spec is resolved for the current branch.

**Acceptance Scenarios**:

1. **Given** Ralph resolves the active spec as `005-my-feature`, **When** Ralph constructs the prompt for Claude, **Then** the prompt includes the active spec's name, directory path, and a directive to stay within that spec's scope.
2. **Given** Ralph is running on a branch with no matching spec, **When** Ralph constructs the prompt, **Then** no spec-boundary directive is added (backwards-compatible with unscoped work).
3. **Given** Ralph is running in roam mode on a sweep branch, **When** Ralph constructs the prompt, **Then** the prompt includes a directive to sweep all specs for improvements (no single-spec focus).

---

### Edge Cases

- What happens when Ralph is on `main`/`master` branch WITHOUT `--roam`? No spec resolution — Ralph runs as today with no spec-boundary enforcement (backwards-compatible).
- What happens when `--roam` is used with `--max` and the budget runs out mid-sweep? Ralph stops at the iteration limit — `--max` is a global budget that always wins.
- What happens when Claude reports `"error_max_turns"` instead of `"success"`? This is NOT spec/sweep completion — Ralph continues iterating (Claude hit its internal turn limit but may have more work).
- What happens when `--roam` is combined with `ralph loop run`? Ralph uses ROAM.md for the loop run instead of BUILD.md.
- What happens when `--roam` is combined with `--spec`? Ralph errors with a clear message — explicit spec override and roaming are mutually exclusive.
- What happens when the sweep branch creation fails (e.g., develop doesn't exist)? Ralph logs the error and exits. The developer must ensure a valid base branch.
- What happens when the sweep branch already exists? Ralph appends a sequence number or reuses the existing branch.

## Requirements *(mandatory)*

### Functional Requirements

**Spec Completion Detection (P1)**:

- **FR-001**: The loop MUST detect spec completion when Claude's result subtype is `"success"` AND the subsequent iteration produces no new git commits (two-signal confirmation).
- **FR-002**: The loop MUST track whether each iteration produced commits by comparing HEAD before and after each iteration.
- **FR-003**: When spec completion is detected and `--roam` is NOT set, the loop MUST emit a `LogSpecComplete` entry with a "spec complete" message and exit gracefully (return nil).
- **FR-004**: Spec completion detection MUST require two consecutive signals: first a `"success"` result subtype, then a no-commit iteration. A single no-commit iteration without a prior success MUST NOT trigger completion.

**Roam Mode (P2)**:

- **FR-005**: When `--roam` is set, Ralph MUST create a `sweep/YYYY-MM-DD` branch from the current branch before starting the loop.
- **FR-006**: In roam mode, the `--max` iteration budget is global — the counter does NOT reset during the sweep.
- **FR-007**: In roam mode, when completion is detected (success + no commits), Ralph MUST emit a "sweep complete" log entry and exit gracefully.
- **FR-008**: The `--roam` flag MUST be available on `ralph build`, `ralph run`, and `ralph loop build/run` commands.
- **FR-009**: The `--roam` flag MUST also be configurable via `ralph.toml` (e.g., `[build] roam = true`) so it can be set as a project default.
- **FR-010**: The `--roam` flag MUST NOT be compatible with `--spec`. If both are provided, Ralph MUST error with a clear message.
- **FR-011**: The git runner MUST support creating and checking out a new branch from a base.

**Prompt Augmentation (P3)**:

- **FR-012**: The loop MUST inject the active spec's name and scope directive into the prompt when a spec is resolved for the current branch (non-roam mode).
- **FR-013**: In roam mode, the loop MUST inject a sweep directive into the prompt instructing Claude to check all specs for improvements.
- **FR-014**: Spec-boundary enforcement is prompt-only for v1. No structural verification of commit scope.

### Key Entities

- **Spec Completion State**: Tracks whether the previous iteration's result subtype was `"success"` and whether the current iteration produced commits. Used in both default mode (to stop the loop) and roam mode (to end the sweep).
- **Sweep Branch**: A branch created by roam mode (`sweep/YYYY-MM-DD`) from the current branch (typically develop). Contains all specs. PRed back to develop after review.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Ralph stops within 2 iterations of spec work being complete (one success + one no-commit confirmation), rather than consuming the full iteration budget.
- **SC-002**: In roam mode, Ralph creates a sweep branch, runs an improvement sweep, and stops when idle — all without human intervention during the sweep.
- **SC-003**: All existing `ralph build` and `ralph run` invocations without `--roam` behave identically to current behavior when specs have remaining work (no regressions).
- **SC-004**: The prompt sent to Claude includes the active spec's scope context in default mode, and a sweep directive in roam mode.

## Assumptions

- Branch naming follows the existing convention: `NNN-short-name` matching `specs/NNN-short-name/` directory.
- The project uses a develop (or main) branch that contains all spec directories. Roam's sweep branch is created from this integration branch.
- The BUILD.md prompt file is controlled by the project. Ralph injects spec/sweep context into the prompt at runtime rather than requiring manual prompt edits.
- The two-signal completion detection (success + no commits) is a pragmatic heuristic. It may occasionally produce false positives or false negatives. This is acceptable because the developer reviews via PR.
- Roam mode is intended for improvement sweeps (finding gaps, missing tests, fixes across specs), not for feature development. Feature development happens on spec-specific branches.
