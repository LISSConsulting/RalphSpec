# Data Model: Codex Agent Support

## AgentSelection

Represents how Ralph chooses the effective agent for a run.

| Field | Type | Description |
|-------|------|-------------|
| `ProjectDefault` | `string` | Saved project-level agent from `[agent] type` |
| `Override` | `string` | Optional per-run `--agent` value |
| `Effective` | `string` | Resolved agent for the run |
| `Source` | `string` | Resolution source: `project-default` or `flag` |

**Validation rules**:
- Supported values are `claude` and `codex`.
- Empty override means use `ProjectDefault`.
- Unknown values fail validation before loop startup.

## CodexConfig

Minimal configuration for the Codex adapter.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `Model` | `string` | `""` | Optional Codex model override; empty means use Codex CLI default |

## AgentRunRecord

Persisted session-level metadata describing which agent produced a run.

| Field | Type | Description |
|-------|------|-------------|
| `Agent` | `string` | Effective agent for the session |
| `Mode` | `string` | Loop mode (`plan`, `build`, or supported composed flow) |
| `Branch` | `string` | Git branch for the run |
| `LastCommit` | `string` | Latest commit observed during the session |
| `StartedAt` | `time` | Session start time |
| `FinishedAt` | `time` | Session end time |
| `TotalCostUSD` | `float64` | Accumulated reported cost |
| `Passed` | `bool` | Final success state recorded by Regent/state tracking |

**Backed by**:
- `loop.LogEntry.Agent`
- `store.IterationSummary.Agent`
- `store.SessionSummary.Agent`
- `regent.State.Agent`

## CodexSessionState

Ephemeral validation state used before loop startup.

| Field | Type | Description |
|-------|------|-------------|
| `Selected` | `string` | Requested effective agent |
| `Available` | `bool` | Whether the selected binary is present and runnable |
| `SupportedFlow` | `bool` | Whether the current Ralph command/path supports the selected agent |
| `ValidationError` | `string` | Actionable failure reason when startup is rejected |

## State Transitions

```text
Selected -> Validated -> Running -> Completed
                    \-> RejectedUnsupported
                    \-> RejectedUnavailable
                    \-> Failed
```

- `RejectedUnsupported`: Codex chosen for worktree, dashboard-started, or orchestrator-managed flow.
- `RejectedUnavailable`: Codex binary missing, login/setup invalid, or startup command fails validation.
- `Failed`: Loop started but agent run returned an execution error.
