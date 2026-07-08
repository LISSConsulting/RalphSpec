# CLI Command Contract: Spec Kit Alignment

**Feature**: 004-speckit-alignment
**Date**: 2026-03-01

## Command Tree (after refactor)

```
ralph
├── specify <description>    # NEW — invokes /speckit.specify
├── plan                     # REPURPOSED — invokes /speckit.plan
├── clarify                  # NEW — invokes /speckit.clarify
├── tasks                    # NEW — invokes /speckit.tasks
├── run                      # REPURPOSED — invokes /speckit.implement
├── build [--max N]          # UNCHANGED — Claude loop build mode (also under loop)
├── status                   # UNCHANGED
├── init                     # UNCHANGED
├── spec
│   └── list                 # MODIFIED — directory-aware, artifact-presence status
│                            # (spec new REMOVED)
└── loop                     # NEW parent
    ├── build [--max N]      # ALIAS — same as top-level build
    └── run [--max N]        # MOVED — autonomous loop run
```

## Speckit Commands

All speckit commands share these behaviors:
- Resolve active spec via branch name or `--spec <name>` flag
- Spawn `claude` interactively (stdin/stdout/stderr inherited)
- Exit with Claude's exit code

### `ralph specify <description>`

| Field | Value |
|-------|-------|
| **Use** | `specify <description>` |
| **Short** | Create or update a feature specification |
| **Args** | Required: feature description (string, joined from remaining args) |
| **Flags** | `--spec <name>` — target specific spec (overrides branch resolution) |
| **Behavior** | Resolves active spec. If spec dir doesn't exist, creates it. Spawns `claude -p "/speckit.specify <description>"`. |
| **Error** | No description provided → usage error |

### `ralph plan`

| Field | Value |
|-------|-------|
| **Use** | `plan` |
| **Short** | Generate implementation plan from spec |
| **Args** | None |
| **Flags** | `--spec <name>` |
| **Behavior** | Resolves active spec. Requires `spec.md` to exist. Spawns `claude -p "/speckit.plan"`. |
| **Error** | No active spec → error with guidance. No `spec.md` → error suggesting `ralph specify` first. |

### `ralph clarify`

| Field | Value |
|-------|-------|
| **Use** | `clarify` |
| **Short** | Resolve ambiguities in feature specification |
| **Args** | None |
| **Flags** | `--spec <name>` |
| **Behavior** | Resolves active spec. Requires `spec.md` to exist. Spawns `claude -p "/speckit.clarify"`. |
| **Error** | No active spec → error. No `spec.md` → error suggesting `ralph specify` first. |

### `ralph tasks`

| Field | Value |
|-------|-------|
| **Use** | `tasks` |
| **Short** | Break implementation plan into task list |
| **Args** | None |
| **Flags** | `--spec <name>` |
| **Behavior** | Resolves active spec. Requires `plan.md` to exist. Spawns `claude -p "/speckit.tasks"`. |
| **Error** | No active spec → error. No `plan.md` → error suggesting `ralph plan` first. |

### `ralph run`

| Field | Value |
|-------|-------|
| **Use** | `run` |
| **Short** | Execute tasks from task breakdown |
| **Args** | None |
| **Flags** | `--spec <name>`, `--max <N>` (override max iterations if loop is used internally) |
| **Behavior** | Resolves active spec. Requires `tasks.md` to exist. Spawns `claude -p "/speckit.implement"`. |
| **Error** | No active spec → error. No `tasks.md` → error suggesting `ralph tasks` first. |

## Loop Commands (preserved behavior)

### `ralph loop build`

| Field | Value |
|-------|-------|
| **Use** | `build` (under `loop` parent) |
| **Short** | Run Claude in build mode (autonomous loop) |
| **Flags** | `--max <N>`, `--no-tui` |
| **Behavior** | Identical to old `ralph build` |

### `ralph loop run`

| Field | Value |
|-------|-------|
| **Use** | `run` (under `loop` parent) |
| **Short** | Run the autonomous build loop; use `--roam` for codebase-wide roaming |
| **Flags** | `--max <N>`, `--no-tui` |
| **Behavior** | Runs the same loop engine as build mode |

## Modified Commands

### `ralph spec list`

| Field | Value |
|-------|-------|
| **Use** | `list` (under `spec` parent) |
| **Short** | List all spec features with status |
| **Output change** | Shows one entry per directory-based feature (not per `.md` file). Status column shows artifact-presence status (`📋 specified`, `📐 planned`, `✅ tasked`). Legacy flat `.md` files still shown with CHRONICLE.md-derived status. |

### `ralph build`

| Field | Value |
|-------|-------|
| **Behavior** | UNCHANGED — runs Claude loop in build mode. Also available as `ralph loop build`. |

## Global Flags

| Flag | Scope | Description |
|------|-------|-------------|
| `--no-tui` | Loop commands only | Disable TUI, use plain text output |
| `--spec <name>` | Speckit commands only | Override active spec resolution |
| `--max <N>` | Loop commands + `ralph run` | Override max iterations |
