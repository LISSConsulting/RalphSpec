# Feature Specification: Task Accounting

**Branch**: `013-task-accounting` | **Date**: 2026-07-28 | **Status**: Draft

## Summary

Ralph runs complete and merge spec implementations while leaving the spec's
`tasks.md` checkboxes unchecked. Root cause: the BUILD.md prompt tells the
agent to treat specs as read-only and never instructs it to check off
completed tasks, and Ralph itself never surfaces checkbox state. Fix both
levers: prompt instructions that define tasks.md as bookkeeping the agent
must maintain, and a per-iteration `## Task Accounting` prompt section with
live checkbox counts plus a TUI-visible log event.

## User Stories

### US1 — Agent checks off tasks as it completes them (P1)

As a maintainer, when a Ralph build run finishes a spec, tasks.md must reflect
which tasks were actually completed so numbering/planning decisions based on
checkbox state are sound.

**Acceptance**:

1. The scaffolded BUILD.md instructs the agent to check `[ ]` → `[x]` for each
   completed task in the same commit, and to reconcile all boxes before
   declaring the spec done. It narrows "specs are read-only" to spec.md /
   plan.md, explicitly excluding tasks.md checkboxes.
2. ROAM.md's housekeeping mission includes reconciling unchecked boxes in
   recently completed specs.

### US2 — Loop injects live task counts each iteration (P1)

As an operator, I want the agent reminded of the actual checkbox state every
iteration so the instruction in US1 is grounded in fact, not memory.

**Acceptance**:

1. Given an active spec with a tasks.md containing checkbox items, when an
   iteration starts, the iteration prompt contains a `## Task Accounting`
   section with the current `N of M` counts and the check-off instruction.
2. Given tasks.md counts change between iterations, when the next iteration
   starts, the loop emits a `LogInfo` entry `tasks: N/M complete` so the
   change is visible in the TUI and session log.
3. Given no active spec, roam mode, or a tasks.md without checkboxes, no
   section is added and no events are emitted (byte-identical behavior).

## Functional Requirements

- **FR-001**: A task counter MUST recognise `- [ ]`, `- [x]`, and `- [X]`
  items with arbitrary leading whitespace and count checked vs total.
- **FR-002**: The loop MUST re-read the active spec's tasks.md before every
  iteration (counts change as the agent works).
- **FR-003**: The accounting section MUST NOT gate spec completion; it is a
  reminder, not a blocker.
- **FR-004**: `updateExistingConfigFile` MUST backfill `[agent] stdin_steer`
  and `[harness]` into existing ralph.toml files (missed in 012).

## Non-Goals

- Verifying that checked boxes correspond to real work (trust but don't
  audit).
- Editing the other project's already-merged tasks.md files (that's a roam
  housekeeping item for that repo, enabled by US1 §2).
