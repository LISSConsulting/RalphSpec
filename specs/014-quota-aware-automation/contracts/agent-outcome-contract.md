# Agent Outcome and Quota Contract

## Adapter interface

Every harness adapter implements the existing asynchronous run boundary and emits normalized events:

```go
type Agent interface {
    Run(ctx context.Context, prompt string, opts RunOptions) (<-chan Event, error)
}
```

`Run` returns an error only when the process cannot be prepared or started. Once a channel is returned, the adapter closes it after emitting exactly one terminal event.

Optional quota capability:

```go
type QuotaProvider interface {
    Quota(ctx context.Context) (QuotaSnapshot, error)
}
```

Not implementing `QuotaProvider` means quota is unknown. A probe error is also unknown with a diagnostic; it is not unlimited.

## Event terminal rules

- Tool and text events are non-terminal.
- Recoverable warnings remain non-terminal text/error diagnostics and do not determine the invocation result.
- A terminal event includes `TerminalOutcome`.
- The loop drains the channel before selecting the final terminal outcome.
- If multiple terminal events are emitted due to a malformed adapter stream, the last non-success outcome wins; otherwise the last terminal event wins and a contract diagnostic is logged.
- Channel close without a terminal event becomes `agent_failure`.

## Error precedence

From strongest to weakest:

1. Context cancellation → `cancelled`.
2. Explicit structured provider terminal failure.
3. Non-zero subprocess exit plus captured stderr.
4. Structured success result.
5. Missing terminal event → `agent_failure`.

A success event cannot erase a terminal error already observed in the same invocation.

## Classification

Structured harness subtypes are mapped first. Narrow text fallback recognizes:

- quota/usage/credit/rate-limit exhaustion;
- authentication/login/token expiration;
- transient 429/5xx/overload/connection failures;
- context-window exhaustion;
- cancellation/interruption.

Unrecognized non-success is `agent_failure`. Classification preserves original text and never creates a reset time absent provider evidence.

## Harness commands

- Claude Code: existing stream-JSON print invocation.
- Codex: existing `codex exec --json` invocation.

All processes receive cancellation through `exec.CommandContext`, set their working directory from `RunOptions.Dir`, and include stderr in terminal diagnostics.

## Codex quota transaction

1. Start `codex app-server --listen stdio://` in the run directory.
2. Send `initialize` with Ralph client metadata.
3. Await matching success response.
4. Send `initialized` notification.
5. Send `account/rateLimits/read` request.
6. Await matching response while ignoring unrelated notifications.
7. Decode primary/secondary windows; close stdin and reap process.

Protocol errors or missing rate-limit data produce unknown quota with a diagnostic, not an allow decision.
