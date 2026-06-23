# Quickstart: Complete Codex Support

## 1. Verify Codex Is Available

```powershell
codex --help
codex login
```

Expected result: Codex is installed and authenticated before Ralph starts a Codex-backed run.

## 2. Select Codex for One Run

```powershell
ralph loop plan --agent codex --no-tui
ralph build --agent codex --no-tui
ralph loop run --agent codex --no-tui
```

Expected result:

- Ralph resolves Codex as the effective agent for that invocation only.
- Startup validates Codex before work begins.
- Output identifies Codex as the active agent.
- Existing project defaults remain unchanged.

## 3. Select Codex as the Project Default

Update `ralph.toml`:

```toml
[agent]
type = "codex"

[codex]
model = ""
```

Then run:

```powershell
ralph build --no-tui
```

Expected result: Ralph uses Codex because no per-run override was provided.

## 4. Verify Dashboard Support

```powershell
ralph
```

Expected result:

- Dashboard-started plan/build/smart-run actions honor the effective agent.
- Active run, log, and status surfaces identify Codex.
- Codex setup failures appear as actionable dashboard errors.

## 5. Verify Worktree or Parallel Support

```powershell
ralph build --worktree --agent codex
```

Expected result:

- Ralph starts an isolated Codex-backed worktree session when the workflow requires one.
- The session is shown separately from other active sessions.
- Logs, errors, and final state remain attributable to that Codex session.
- If `wt` or Codex is unavailable, Ralph fails before work starts with a setup-specific error rather than falling back to Claude.

## 6. Verify Mixed-Agent Safety

Run one workflow with Codex selected and another with Claude selected.

Expected result:

- Each run uses its own selected agent.
- Ralph displays the agent for each run or role.
- One failed Codex-backed run does not obscure or fail an unrelated Claude-backed run.

## 7. Verify Historical Metadata

Review normal status and run history surfaces after a Codex-backed run completes.

Expected result:

- The producing agent is visible for live and completed runs.
- Historical records continue to show Codex even if the project default later changes.

## 8. Regression Checks

```powershell
go test ./...
go vet ./...
```

Expected result: All tests and vet checks pass for Codex and non-Codex paths.
