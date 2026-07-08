# Implementation Plan: Spec Kit Alignment

**Branch**: `004-speckit-alignment` | **Date**: 2026-03-01 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `specs/004-speckit-alignment/spec.md`

## Summary

Align Ralph's CLI commands, spec discovery, and prompt files with the spec kit framework. The core change: Ralph shifts from treating each `.md` file as a separate spec to treating each `specs/NNN-name/` directory as a single feature, and new top-level commands (`specify`, `plan`, `clarify`, `tasks`, `run`) delegate to Claude Code's speckit skills by spawning `claude` in interactive mode. Existing loop behavior moves under `ralph loop`.

## Technical Context

**Language/Version**: Go 1.24
**Primary Dependencies**: cobra, BurntSushi/toml, bubbletea, lipgloss, bubbles
**Storage**: Filesystem (spec directories, TOML config, JSONL logs)
**Testing**: `go test ./...` with table-driven subtests, `go vet ./...`
**Target Platform**: darwin/arm64, darwin/amd64, linux/amd64, windows/amd64
**Project Type**: CLI tool
**Performance Goals**: N/A — CLI startup latency; speckit operations dominated by Claude execution time
**Constraints**: Minimal binary size, no new dependencies beyond stdlib for speckit features
**Scale/Scope**: Single-user local CLI, ~15 packages in `internal/`

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Spec-Driven | PASS | This feature originates from `specs/004-speckit-alignment/spec.md` |
| II. Supervised Autonomy | PASS | Speckit commands are user-initiated, interactive sessions. Loop commands under `ralph loop` retain Regent supervision |
| III. Test-Gated Commits | PASS | All changes will have table-driven tests. Existing tests must continue to pass (SC-005) |
| IV. Idiomatic Go | PASS | No new dependencies. Follows existing patterns: exported interfaces, unexported implementations, explicit error returns |
| V. Observable Loops | PASS | Loop mode under `ralph loop` retains full observability. Speckit commands are interactive — output is directly visible to the user |

No violations. No complexity tracking needed.

## Project Structure

### Documentation (this feature)

```text
specs/004-speckit-alignment/
├── spec.md              # Feature specification
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── checklists/
│   └── requirements.md  # Spec quality checklist
├── contracts/
│   └── cli-commands.md  # CLI command contract
└── tasks.md             # Phase 2 output (/speckit.tasks)
```

### Source Code (repository root)

```text
internal/
├── spec/
│   ├── spec.go          # MODIFY: directory-based discovery, artifact-presence status
│   ├── resolve.go       # NEW: active spec resolution (branch → directory)
│   └── *_test.go        # MODIFY + NEW: tests for new behavior
├── claude/
│   └── claude.go        # UNCHANGED (Agent interface stays the same)
├── loop/
│   └── runner.go        # UNCHANGED (ClaudeAgent for loop mode stays the same)
├── tui/
│   └── panels/
│       └── specs.go     # MODIFY: display artifact-presence status
└── ...                  # Other packages unchanged

cmd/ralph/
├── main.go              # MODIFY: register new command tree
├── commands.go          # MODIFY: remove specNewCmd, restructure commands
├── speckit_cmds.go      # NEW: specify, plan, clarify, tasks commands (speckit wrappers)
├── execute.go           # MODIFY: add executeSpeckit(), keep loop execution
├── wiring.go            # MINOR: adapt to new spec.List() return shape
└── ...                  # Other files unchanged

ROAM.md                  # MODIFY: update for roam mode directory awareness
BUILD.md                 # MODIFY: update for spec kit directory awareness
```

**Structure Decision**: Extend existing `internal/spec/` package rather than creating a new `internal/speckit/` package. Spec resolution is a natural extension of spec discovery. Speckit CLI invocation is thin enough for `cmd/ralph/` directly.

## Phase 0: Research

### Research Question 1: Claude Code Slash Command Invocation

**Decision**: Speckit commands spawn `claude` in **interactive terminal mode** — not stream-JSON mode.

**Rationale**: Speckit skills (`/speckit.clarify`, `/speckit.specify`) are inherently interactive — they ask questions, wait for user input, and produce files. The current `ClaudeAgent.Run()` method uses `--output-format stream-json` and doesn't connect stdin, which prevents interactive flows. Speckit commands don't need event parsing, iteration loops, TUI integration, or Regent supervision. They need a user-facing terminal session.

**Implementation**: Use `os/exec` directly (not the `Agent` interface) to spawn:
```
claude -p "/speckit.specify <args>" --verbose
```
With stdin, stdout, and stderr inherited from the parent process (`cmd.Stdin = os.Stdin`, etc.). This gives the user a native Claude Code experience.

**Alternatives considered**:
- Extending `Agent` interface with an `Interactive()` method — rejected as over-engineering; speckit invocation is a command concern, not a domain concern
- Passing slash commands via `--output-format stream-json` — rejected; interactive skills require stdin/stdout passthrough
- Embedding speckit logic in Go — rejected; speckit skills are maintained as Claude Code skills, not Go code

### Research Question 2: Active Spec Resolution Strategy

**Decision**: Match current git branch name exactly against `specs/` directory names. Use `git.Runner.CurrentBranch()` which already exists.

**Rationale**: Branch names follow `NNN-feature-name` pattern, and spec directories use the identical name. Exact match is unambiguous and fast (single `os.Stat` call after branch detection).

**Resolution order**:
1. `--spec <name>` flag (explicit override) → `specs/<name>/`
2. Current branch name → `specs/<branch-name>/`
3. Error: "no active spec found"

**Alternatives considered**:
- Fuzzy/prefix matching — rejected; creates ambiguity when multiple features share prefixes
- Config-based spec path — rejected; the convention is strong enough, no config needed

### Research Question 3: Status Model Transition

**Decision**: Replace `StatusDone/StatusInProgress/StatusNotStarted` with artifact-presence-based statuses for directory specs. Keep legacy statuses for flat `.md` files.

**New status progression**:
- `StatusSpecified` — `spec.md` exists (no `plan.md`)
- `StatusPlanned` — `plan.md` exists (no `tasks.md`)
- `StatusTasked` — `tasks.md` exists (ready for implementation)

**Detection**: Check file existence with `os.Stat()` — no CHRONICLE.md dependency for directory-based specs.

**Backward compatibility**: Flat `.md` files in `specs/` retain CHRONICLE.md-based detection (legacy).

## Phase 1: Design

### Data Model

See [data-model.md](data-model.md) for full entity definitions.

### Contracts

See [contracts/cli-commands.md](contracts/cli-commands.md) for the CLI command contract.

### Quickstart

See [quickstart.md](quickstart.md) for the developer getting-started guide.
