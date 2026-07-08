# Contract: CLI and Config Agent Selection

## Project Configuration

```toml
[agent]
type = "claude"

[claude]
model = "sonnet"
max_turns = 0
danger_skip_permissions = true

[codex]
model = ""
```

## Supported Values

| Setting | Allowed values | Notes |
|---------|----------------|-------|
| `[agent].type` | `claude`, `codex` | Defaults to `claude` when omitted |
| `[codex].model` | any non-empty string or empty | Empty defers to Codex CLI default |

Unknown agent values are configuration errors.

## Supported Commands

The initial release supports `--agent <name>` on single-run loop entry points:

| Command | Supported values | Behavior |
|---------|------------------|----------|
| `ralph build --agent <name>` | `claude`, `codex` | Overrides project default for one build run |
| `ralph loop build --agent <name>` | `claude`, `codex` | Overrides project default for one build run |
| `ralph loop build --agent <name>` | `claude`, `codex` | Overrides project default for one loop run |
| `ralph loop run --agent <name>` | `claude`, `codex` | Uses the selected agent for the composed single-run plan/build flow |

## Unsupported Combinations

| Invocation | Expected result |
|------------|-----------------|
| `ralph build --worktree --agent codex` | Fail before startup with an unsupported-workflow error and Claude fallback guidance |
| Dashboard-triggered loop starts with effective agent `codex` | Fail before startup with an unsupported-workflow error and direct no-TUI guidance |
| Orchestrator/worktree-managed Codex launches | Fail before startup with an unsupported-workflow error |

## Precedence Rules

1. `--agent` flag, if present
2. `[agent].type` project default
3. Built-in default: `claude`

## Visibility Requirements

- The resolved effective agent must appear in structured loop events.
- Plain no-TUI output must visibly identify the effective agent during the run.
- Session summaries and Regent state must persist the effective agent.
- User-facing live output/status views must show the effective agent for the run.

## Error Contract

Codex startup failures must be actionable. Examples:

- `codex agent unavailable: codex executable not found on PATH; install or log into Codex CLI, or use --agent claude`
- `codex agent unsupported for worktree mode; use --agent claude or omit --worktree`
- `codex agent unsupported for dashboard mode; start Codex with ralph build --agent codex --no-tui or use claude in the dashboard`
- `config validation: agent.type must be one of claude,codex`
