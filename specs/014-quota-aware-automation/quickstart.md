# Quickstart: Quota-Aware Goal Automation

## Configure a harness

```toml
[agent]
type = "codex" # claude or codex

[harness]
claude = "claude"
codex = "codex"
```

A run-specific selection overrides configuration:

```powershell
ralph run --agent codex --no-tui --max 1
```

Unavailable harnesses fail before an iteration starts; Ralph does not fall back.

## Configure quota admission

```toml
[quota]
enabled = true
reserve_percent = 10
exhausted_policy = "fail_closed" # or wait
unknown_policy = "allow"         # or fail_closed
max_wait_seconds = 3600
```

Codex supplies a fresh documented quota snapshot. Claude Code may be unknown at preflight when its operator-managed snapshot is unavailable; `unknown_policy` determines admission. Terminal quota failures always stop completion and are classified even when preflight is unknown.

## Preview and run detected tests

```powershell
ralph tests detect
ralph tests detect --json
ralph tests run
```

Enable the discovered high-confidence plan as the run test gate:

```toml
[regent]
enabled = true
auto_discover_tests = true
rollback_on_test_failure = true
test_command = ""
```

An explicit `test_command` remains authoritative. In this repository, root `just release-check` is selected as the aggregate gate and suppresses child Cargo/package steps.

## Enable ludicrous mode

```powershell
ralph run --ludicrous
```

Or configure it:

```toml
[build]
ludicrous = true
```

With no positive `--max`, the loop is unbounded. The mode does not relax permissions, scope, quota policy, test gates, or cancellation. Completion requires the available objective evidence: terminal success, clean worktree, completed declared tasks, passing snapshotted tests, and a confirming no-change iteration.

## Acceptance checks

```powershell
go test ./internal/loop ./internal/regent ./internal/codex ./internal/testplan ./internal/config ./cmd/ralph
go test ./...
go vet ./...
```

Smoke checks use fake harness executables in tests for deterministic streamed events and errors. Real-provider smoke checks are operator-controlled because they consume quota.
