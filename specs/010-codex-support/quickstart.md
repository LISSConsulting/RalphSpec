# Quickstart: Codex Agent Support

## 1. Ensure Codex CLI is available

```powershell
codex --help
codex login
```

## 2. Select Codex for the project

Add or update `ralph.toml`:

```toml
[agent]
type = "codex"

[codex]
model = ""
```

## 3. Run a supported Codex flow

Examples:

```powershell
ralph loop plan --agent codex --no-tui
ralph build --agent codex --no-tui
ralph loop run --agent codex --no-tui
```

Expected behavior:
- Ralph resolves `codex` as the effective agent
- Startup validates Codex availability before the first iteration
- Plain live output includes a `[codex]` prefix on loop events
- Stored session metadata identifies Codex as the active agent

## 4. Verify persisted metadata

- Inspect `.ralph/regent-state.json` for the recorded agent
- Inspect the latest JSONL session log in `.ralph/logs/`
- Confirm `ralph status` or TUI status surfaces the selected agent

Example status excerpt:

```text
Ralph Status
  Agent:               codex
  Branch:              test-branch
  Mode:                build
```

## 5. Verify unsupported scope is rejected cleanly

```powershell
ralph build --worktree --agent codex
```

Expected behavior:
- Ralph exits before starting work
- The error explains that Codex worktree support is not part of the initial release
- The error suggests using `--agent claude` or omitting `--worktree`

Example error:

```text
codex agent unsupported for worktree mode; use --agent claude or omit --worktree
```
