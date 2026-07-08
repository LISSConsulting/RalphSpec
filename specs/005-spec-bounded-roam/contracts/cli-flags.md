# CLI Contract: --roam Flag

**Date**: 2026-03-02

## Flag Definition

| Flag | Type | Default | Scope |
|------|------|---------|-------|
| `--roam` | bool | false | `ralph build`, `ralph loop build`, `ralph loop run` |

**Note**: `--roam` is not on the speckit `ralph run` command, which delegates to Claude's `/speckit.implement`.

## Behavior Matrix

| Command | --roam | Spec resolved | Behavior |
|---------|--------|---------------|----------|
| `ralph build` | no | yes | Build with prompt augmentation; stop when spec complete |
| `ralph build` | no | no (main/master) | Run full iteration budget, no prompt augmentation (backwards-compatible) |
| `ralph build` | no | no (other branch) | Run full iteration budget, no prompt augmentation (backwards-compatible) |
| `ralph build` | yes | — | Create `sweep/YYYY-MM-DD` branch, sweep all specs, stop when idle |
| `ralph build --roam --max 10` | yes | — | Same as above, hard stop at 10 iterations |
| `ralph build --roam --spec X` | — | — | ERROR: `--roam and --spec are mutually exclusive` |
| `ralph loop build --roam` | yes | — | Same as `ralph build --roam` |
| `ralph loop run --roam` | yes | — | Roam using the dedicated ROAM.md prompt |

## Config Override

```toml
[roam]
enabled = false  # default; set true for always-roam
prompt_file = "ROAM.md"
```

CLI `--roam` flag takes precedence over config value.

## Sweep Branch Naming

| Scenario | Branch name |
|----------|-------------|
| First sweep today | `sweep/2026-03-02` |
| Branch exists | `sweep/2026-03-02-2` |
| That also exists | `sweep/2026-03-02-3` |
| Up to | `sweep/2026-03-02-10` (error if all taken) |

## Exit Conditions

| Condition | Log kind | Message pattern |
|-----------|----------|-----------------|
| Max iterations reached | `LogDone` | "Loop complete — N iterations done, total cost: $X.XX" |
| Spec complete (no roam) | `LogSpecComplete` | "Spec complete — NNN-name (N iterations)" |
| Sweep complete (roam) | `LogSweepComplete` | "Sweep complete (N iterations, $X.XX)" |
| Graceful stop (Ctrl+C) | `LogStopped` | "Stop requested — exiting after this iteration" |
| Hard stop (Ctrl+C x2) | — | context cancelled |
| Error | error return | "loop: iteration N: ..." |

## Prompt Augmentation

| Mode | Spec resolved | Prompt suffix |
|------|---------------|---------------|
| Default | Yes | Spec-boundary directive (name + directory + focus instruction) |
| Default | No | None (raw prompt file, backwards-compatible) |
| Roam | — | Sweep directive (check all specs for improvements) |
