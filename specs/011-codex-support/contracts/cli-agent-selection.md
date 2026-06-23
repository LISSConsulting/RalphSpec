# Contract: CLI Agent Selection

## Scope

Defines user-facing behavior for commands that delegate work to an AI coding agent.

## Agent Selection Inputs

| Input | Applies To | Expected Behavior |
|-------|------------|-------------------|
| `ralph.toml` project default | All agent-delegating commands | Used when no explicit run override is provided |
| `--agent claude` | Current invocation | Uses Claude for this run only |
| `--agent codex` | Current invocation | Uses Codex for this run only after availability validation |
| No explicit selection | Current invocation | Uses the saved project default, then the existing built-in default |

## Command Coverage

The same selection behavior applies to:

- Plan loop commands
- Build loop commands
- Smart run commands
- Top-level build aliases
- Worktree or parallel command paths that delegate agent work
- Any future user-facing command that delegates agent work

## Required Outcomes

- The effective agent is shown before or during execution.
- Unknown agent values fail before work starts.
- Codex availability failures fail before work starts.
- Run overrides do not mutate saved project defaults.
- Non-Codex runs preserve previous behavior.

## Error Contract

Invalid agent selection:

```text
agent.type must be one of claude,codex
```

Codex unavailable:

```text
codex agent unavailable: <diagnosis>; install or log into Codex CLI, or use --agent claude
```

Silent fallback to another agent is not allowed.
