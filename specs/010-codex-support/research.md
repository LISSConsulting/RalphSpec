# Research: Codex Agent Support

## R-001: Agent Selection Contract

**Decision**: Use `[agent] type = "claude" | "codex"` in `ralph.toml` and a per-run `--agent` flag on supported loop commands. The command-line flag overrides the project default for one run only.

**Rationale**: This gives Ralph one canonical way to resolve the effective agent while keeping existing Claude-backed projects unchanged.

**Alternatives considered**:
- Environment-variable-only agent selection
- Separate Codex-specific commands
- Storing the agent under `[project]`

## R-002: Codex Runtime Strategy

**Decision**: Launch Codex with `codex exec --json --full-auto --cd <dir>` and optional `--model <model>`, then translate JSONL events into Ralph's shared event model.

**Rationale**: The installed Codex CLI exposes a non-interactive `exec` mode with structured JSON output, which matches Ralph's loop architecture.

**Alternatives considered**:
- Interactive `codex` mode
- Direct API integration
- Disabling sandbox/approvals with the dangerous bypass flag

## R-003: Interface Reuse vs Rename

**Decision**: Reuse the existing shared `internal/claude.Agent` interface for this release and add a new `internal/codex` adapter package.

**Rationale**: The loop already depends on the interface rather than the concrete Claude runner, so this is the smallest correct change.

**Alternatives considered**:
- Renaming the shared interface package immediately
- Duplicating loop execution for Codex

## R-004: Agent Metadata Persistence

**Decision**: Persist the effective agent in loop log entries, session summaries, iteration summaries, and Regent state.

**Rationale**: Historical session review must remain accurate after the project default changes.

**Alternatives considered**:
- Inferring agent identity from current config
- Showing agent only in live console output

## R-005: Unsupported Scope Handling

**Decision**: Reject Codex for worktree mode, orchestrator-managed parallel flows, and dashboard-started loops until those paths are explicitly implemented.

**Rationale**: Those paths still construct Claude directly today. Explicit rejection prevents ambiguous or misleading behavior.

**Alternatives considered**:
- Silent fallback to Claude
- Partial support in advanced flows

## R-006: Codex Configuration Scope

**Decision**: Add a minimal `[codex]` config section with only `model = ""` in the initial release.

**Rationale**: Ralph needs a small stable contract now; broader Codex tuning can be layered later if real demand appears.

**Alternatives considered**:
- Mirroring all Codex CLI options in config
- No Codex-specific config at all
