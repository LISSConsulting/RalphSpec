# Data Model: Quota-Aware Goal Automation

## TerminalOutcome

Final state of one harness invocation.

| Field | Type | Rules |
|---|---|---|
| `Kind` | enum | `success`, `quota_exhausted`, `authentication_required`, `transient_provider_failure`, `context_exhausted`, `cancelled`, `agent_failure` |
| `Message` | string | Original provider/adapter diagnostic; empty only for success |
| `Provider` | string | Provider when known; otherwise empty |
| `QuotaWindow` | string | Window label when supplied by provider |
| `RetryAt` | timestamp | Optional valid future reset |

Invariants:

- Exactly one terminal outcome is selected for an invocation.
- Any terminal error observed after a result overrides the result.
- `success` cannot carry an error message.
- A non-success outcome prevents completion and is returned to supervision.

## QuotaSnapshot

One fresh or persisted observation from a harness quota capability.

| Field | Type | Rules |
|---|---|---|
| `Agent` | string | Selected harness |
| `Provider` | string | Provider/account domain when known |
| `ObservedAt` | timestamp | Probe completion time |
| `Windows` | list of `QuotaWindow` | May be empty |
| `SpendControlReached` | optional boolean | Absent means unknown |

### QuotaWindow

| Field | Type | Rules |
|---|---|---|
| `Name` | string | Stable display label |
| `UsedPercent` | float | $0 \le p \le 100$ after clamping |
| `ResetAt` | timestamp | Optional |
| `Duration` | duration | Optional provider window length |

Derived `RemainingPercent = 100 - UsedPercent`. Scheduling evaluates the minimum remaining percentage.

## QuotaDecision

| Field | Type | Rules |
|---|---|---|
| `Action` | enum | `allow`, `wait`, `block` |
| `Reason` | string | Operator-facing evidence |
| `ResumeAt` | timestamp | Required for `wait` |
| `Snapshot` | optional `QuotaSnapshot` | Exact input used by decision |

## QuotaPolicy

| Field | Type | Default |
|---|---|---|
| `Enabled` | bool | `false` |
| `ReservePercent` | float | `10` |
| `ExhaustedPolicy` | enum `fail_closed`, `wait` | `fail_closed` |
| `UnknownPolicy` | enum `allow`, `fail_closed` | `allow` |
| `MaxWaitSeconds` | integer | `3600` |

Validation:

- Reserve is within $[0,100]$.
- Maximum wait is non-negative.
- Unknown values are never converted to 100% remaining.
- Wait requires a future reset within maximum wait; otherwise it becomes block.

## TestPlan

Immutable run-start verification plan.

| Field | Type | Rules |
|---|---|---|
| `Steps` | list of `TestStep` | Deterministically ordered and deduplicated |
| `Explicit` | bool | True when sourced from configured command |
| `DetectedAt` | timestamp | Snapshot time |
| `Diagnostics` | list of string | Malformed/inconclusive evidence |

### TestStep

| Field | Type | Rules |
|---|---|---|
| `Command` | string | Exact command passed to platform shell |
| `Dir` | string | Absolute or run-root-relative working directory |
| `Source` | string | Manifest/recipe path and evidence |
| `Confidence` | enum `high`, `medium`, `low` | Only high runs automatically |
| `Aggregate` | bool | Suppresses covered child steps |

## TestPlanResult

| Field | Type | Rules |
|---|---|---|
| `Passed` | bool | True only if every required high-confidence step passed |
| `Steps` | list of `TestStepResult` | Execution order |

A start failure or non-zero result fails the plan. No discovered high-confidence steps is inconclusive, not passing evidence.

## RunState

Existing Regent state extended with:

| Field | Type | Rules |
|---|---|---|
| `Status` | enum | `running`, `paused_quota`, `blocked_action`, `passed`, `failed`, `stopped` |
| `BlockReason` | string | Required for paused/blocked states |
| `ResumeAt` | timestamp | Optional quota reset |
| `Ludicrous` | bool | Effective run mode |
| `TestPlan` | optional summary | Run-start snapshot identity and steps |
| `Quota` | optional decision summary | Last preflight/terminal quota decision |

Compatibility: old state files without `Status` derive their display state from existing `passed`, timestamps, and error count.

## GoalCompletionEvidence

Ephemeral loop state:

- last terminal outcome is success;
- current worktree is clean;
- all present task checkboxes are checked;
- required test plan exists and its latest result passed, or no plan is available;
- current iteration made no commit and no dirty changes;
- previous iteration established success evidence.

Only ludicrous mode uses the full conjunction. Normal mode retains the existing two-signal completion rule after terminal error propagation is fixed.
