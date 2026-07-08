# Data Model: Complete Codex Support

## AgentSelection

Represents how Ralph determines the effective agent for a workflow, run, or role.

| Field | Type | Description |
|-------|------|-------------|
| `ProjectDefault` | string | Saved project-level agent preference from configuration |
| `RunOverride` | string | Optional per-run or dashboard-selected override |
| `RoleOverride` | string | Optional selection for a distinct multi-agent role |
| `EffectiveAgent` | string | Resolved supported agent value used for execution |
| `Source` | string | Resolution source: `role-override`, `run-override`, `project-default`, or `built-in-default` |
| `Workflow` | string | Workflow being launched, such as build, roam run, dashboard action, or worktree run |

**Validation rules**:
- Supported effective values are `claude` and `codex`.
- A role override is valid only when the workflow models explicit roles.
- A run override applies only to the current invocation or dashboard action.
- Unknown values fail validation before work begins.
- Invalid selections never fall back silently to another agent.

## AgentProfile

Describes configured options and runtime readiness for a supported agent.

| Field | Type | Description |
|-------|------|-------------|
| `Type` | string | Supported agent identifier |
| `DisplayName` | string | Operator-facing name |
| `Model` | string | Optional model preference when supported by the selected agent |
| `Available` | bool | Whether the agent appears ready to start |
| `AvailabilityMessage` | string | Actionable setup or status message |

**Validation rules**:
- Codex-backed runs validate availability before execution starts.
- Availability is evaluated at run start rather than assumed from saved configuration.
- Operator-facing availability messages must identify the missing prerequisite or corrective action.

## AgentRunRecord

Persisted metadata for a run or role-specific agent session.

| Field | Type | Description |
|-------|------|-------------|
| `RunID` | string | Stable identifier for the run or session |
| `ParentRunID` | string | Optional parent identifier for parallel or multi-role workflows |
| `Workflow` | string | Workflow or command that launched the run |
| `Role` | string | Optional role name in a mixed-agent workflow |
| `Agent` | string | Effective agent that produced the run |
| `AgentSource` | string | Source of the effective agent selection |
| `Spec` | string | Feature/spec context when applicable |
| `Branch` | string | Git branch associated with the run |
| `WorktreePath` | string | Isolated workspace path when applicable |
| `StartedAt` | timestamp | Run start time |
| `FinishedAt` | timestamp | Run completion time, if complete |
| `State` | string | Lifecycle state |
| `LastError` | string | Most recent actionable failure, if any |
| `TotalCostUSD` | number | Reported running cost when available |

**Relationships**:
- One workflow may create one or more `AgentRunRecord` entries.
- A mixed-agent workflow may have one parent run and multiple role-specific child runs.
- A worktree/parallel run has a distinct `WorktreePath` and branch.

## CodexAvailabilityState

Operator-facing readiness state for Codex before a run starts.

| Field | Type | Description |
|-------|------|-------------|
| `Selected` | bool | Whether Codex is requested for the current context |
| `CheckedAt` | timestamp | Time readiness was evaluated |
| `Status` | string | `ready`, `missing`, `not-authenticated`, `misconfigured`, or `unknown-error` |
| `Message` | string | Human-readable diagnosis and next action |
| `Blocking` | bool | Whether the current workflow must stop |

**State transitions**:

```text
NotSelected -> NotChecked
Selected -> Checking -> Ready -> Running
Selected -> Checking -> RejectedMissing
Selected -> Checking -> RejectedAuth
Selected -> Checking -> RejectedMisconfigured
Running -> Completed
Running -> Failed
Running -> Cancelled
```

## WorktreeAgentSession

Represents one isolated agent session managed by the orchestrator.

| Field | Type | Description |
|-------|------|-------------|
| `Branch` | string | Worktree branch name |
| `Spec` | string | Spec being implemented or reviewed |
| `Agent` | string | Effective agent for this session |
| `State` | string | Creating, running, stopped, failed, completed, merging, merged, or removed |
| `WorktreePath` | string | Isolated workspace path |
| `LogStream` | string | Session-specific log or event stream identifier |
| `RegentState` | string | Supervision state for this session |
| `Error` | string | Actionable failure for this session only |

**Validation rules**:
- Each session must carry its own effective agent value.
- Session failures must not mutate unrelated session state.
- Logs and merged events must remain attributable to the correct session and agent.
