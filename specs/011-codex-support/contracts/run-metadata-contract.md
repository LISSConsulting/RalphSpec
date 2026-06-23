# Contract: Run Metadata and Observability

## Scope

Defines required agent attribution across live events, persisted state, summaries, and status output.

## Required Metadata Fields

| Field | Required For | Purpose |
|-------|--------------|---------|
| `agent` | All agent-backed events and records | Identifies the producing agent |
| `agent_source` | Run/session records | Explains whether selection came from role, run override, project default, or built-in default |
| `workflow` | Run/session records | Identifies the launched workflow |
| `role` | Multi-role records | Identifies the role-specific agent assignment |
| `run_id` | Run/session records | Connects live and historical data |
| `parent_run_id` | Parallel or mixed-agent records | Groups related child sessions |
| `worktree_path` | Worktree sessions | Attributes work to an isolated workspace |
| `state` | Live and historical records | Reports lifecycle status |
| `last_error` | Failed or blocked records | Preserves actionable failure diagnostics |

## Status Requirements

- Live console output must include or otherwise expose the active agent.
- TUI status surfaces must show the active or producing agent.
- `ralph status` must report the active or most recent agent when state is available.
- Historical summaries must not infer agent identity from current configuration.
- Parallel event fan-in must retain both session identity and agent identity.

## Regression Requirements

- Existing Claude-only history remains readable when optional new metadata is absent.
- New Codex records include agent attribution in all newly written state and summary files.
- Mixed-agent records preserve per-role attribution instead of overwriting with one global agent.
