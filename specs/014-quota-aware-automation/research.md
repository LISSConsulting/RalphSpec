# Research: Quota-Aware Goal Automation

## Terminal outcome contract

**Decision**: Keep streamed progress events, but require every adapter to emit exactly one terminal event carrying a typed outcome. A terminal error wins over any earlier success-looking result. The loop returns a typed error for every non-success terminal outcome and clears its prior-success completion signal.

**Rationale**: Ralph currently logs `claude.EventError` and then returns `nil` from `iteration`, allowing the previous iteration's `success` subtype to satisfy the no-change completion rule. Regent only retries non-nil loop errors. A typed terminal outcome fixes the source instead of teaching every caller to scrape log text.

**Alternatives rejected**:

- Classifying JSONL log messages in Regent: too late and text-only.
- Treating every streamed error as terminal immediately: some harnesses emit recoverable warnings before a successful terminal result.
- Relying only on subprocess exit status: structured provider failures can be emitted before a nominal or adapter-normalized exit.

## Outcome classification

**Decision**: Normalize terminal results to `success`, `quota_exhausted`, `authentication_required`, `transient_provider_failure`, `context_exhausted`, `cancelled`, or `agent_failure`. Prefer structured subtype/status fields, then process cancellation/exit state, then a narrow case-insensitive message classifier shared by adapters.

**Rationale**: Claude Code and Codex expose different stream schemas. A shared classification function prevents divergent retry policies while retaining the original message for diagnostics.

## Quota capability

**Decision**: Use an optional `QuotaProvider` interface. A missing implementation returns capability `unknown`, never unlimited. The preflight gate evaluates the minimum remaining percentage across all reported windows and blocks at or below the configured reserve.

**Codex**: Implement a fresh probe with the documented `codex app-server` JSONL protocol: initialize, send `initialized`, then call `account/rateLimits/read`. Convert `primary` and `secondary` windows from `usedPercent`, `windowDurationMins`, and Unix `resetsAt`.

**Claude Code**: No supported cold-start external quota RPC exists. Claude status-line input can contain `rate_limits.five_hour` and `rate_limits.seven_day`, but only after Claude has produced provider data in a running session. Ralph therefore classifies terminal usage-limit responses and reports preflight quota as unknown instead of fabricating availability.


**Alternatives rejected**:

- Scraping `/usage` UI text: unstable and unavailable on a clean non-interactive start.
- Estimating remaining quota from local token counts: provider windows, cached tokens, plan rules, and shared usage make the estimate unsafe.
- Automatic model/credential fallback: violates explicit selection and can change cost or authority.

## Quota scheduling policy

**Decision**: Gate immediately before every new iteration and before each parallel worker launch. Configuration supplies `reserve_percent`, `exhausted_policy` (`fail_closed` or `wait`), `unknown_policy` (`allow` or `fail_closed`), and `max_wait_seconds`. Waiting is allowed only when every blocking window has a valid future reset within the bound. Quota waits do not enter Regent's crash-retry counter.

**Rationale**: Quota cannot safely throttle a request already in flight. Central launch admission prevents oversubscription better than per-worker checks taken concurrently.

## Test-plan discovery

**Decision**: Add deterministic discovery with explicit-command precedence and immutable run-start snapshots. Auto-execution includes only high-confidence steps.

Detection order:

1. Explicit `regent.test_command` — one authoritative root step.
2. Recognized repository aggregate recipes — root `justfile` recipes `release-check`, `check`, `test`, or `ci`, in that order.
3. Workspace manifests — package-manager workspace root, Cargo workspace, or `go.work`.
4. Independent project manifests — package script, declared Python test runner, `go.mod`, and Cargo package.

Supported high-confidence evidence:

- `package.json`: a non-placeholder `scripts.test`; package manager from `packageManager`, then lockfile. Use `<manager> test`.
- `pyproject.toml`: `pytest` only when declared in project/optional dependencies or pytest configuration is present. Use `python -m pytest`.
- `go.work`: `go test ./...` at workspace root.
- `go.mod`: `go test ./...` in the module directory.
- `Cargo.toml`: `cargo test --workspace` for a workspace, otherwise `cargo test`.
- `justfile`: `just <recipe>` for a recognized aggregate recipe. An aggregate root step suppresses child steps.

Ignored traversal directories include VCS metadata, dependency/vendor trees, build output, caches, and Ralph worktrees. Malformed metadata is reported as evidence, not guessed around.

## Ludicrous completion

**Decision**: Ludicrous mode changes persistence and completion evidence only. Unless the operator supplies a positive `--max`, iterations are unbounded. Each prompt receives a goal-persistence section. Completion requires: successful terminal outcome, clean worktree, every declared task checked in spec-bound runs, a passing required test-plan snapshot when one exists, and a confirming no-change iteration. Missing or unreadable task evidence blocks completion for a spec-bound run and is logged as inconclusive.

**Rationale**: This makes the mode useful in projects without Spec Kit while preventing an agent's self-reported `success` from bypassing machine-verifiable evidence.

## Sources

- Codex app-server protocol and `account/rateLimits/read`: https://github.com/openai/codex/blob/main/codex-rs/app-server/README.md
- Claude Code status-line `rate_limits` fields: https://code.claude.com/docs/en/statusline
- Claude Code `/usage` command: https://code.claude.com/docs/en/commands
