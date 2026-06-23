# Contract: Dashboard Agent Behavior

## Scope

Defines dashboard expectations for selecting, launching, monitoring, and troubleshooting Codex-backed runs.

## Required Dashboard Surfaces

| Surface | Requirement |
|---------|-------------|
| Run launch action | Uses the resolved project default or explicit dashboard selection |
| Active run display | Shows the effective agent for the running workflow |
| Worktree/parallel panel | Shows each session's effective agent and state |
| Log view | Attributes entries to the correct run and agent |
| Error display | Shows actionable Codex setup or execution failures |
| Historical/session review | Preserves the producing agent after completion |

## Behavior Rules

- Dashboard-started Codex runs must not be rejected solely because they were launched from the dashboard.
- Dashboard actions must use the same agent resolution and validation behavior as command-line invocations.
- Parallel sessions must keep agent identity, workspace, logs, errors, and final state separate.
- Mixed-agent views must show each role or session's selected agent rather than a single ambiguous global value.
- Cancelling or failing one Codex-backed session must not cancel or mark unrelated sessions failed unless the user requested a global stop.

## Acceptance Checks

- Start a dashboard build with Codex selected and confirm the active run shows Codex.
- Start a worktree or parallel session with Codex selected and confirm the worktree panel shows Codex for that session.
- Induce a Codex availability failure and confirm the dashboard displays the diagnosis and next action.
- Review a completed dashboard-launched run and confirm the producing agent remains visible.
