# Test Plan Discovery Contract

## CLI

```text
ralph tests detect [--json]
ralph tests run [--json]
```

`detect` never executes commands. `run` executes the same deterministic plan that `detect` would print. Both honor `regent.test_command` before metadata discovery.

Exit behavior:

- detection with a plan: `0`;
- inconclusive detection: non-zero with diagnostics;
- run with all required steps passing: `0`;
- any failed/unstartable step or inconclusive plan: non-zero.

## Configuration

```toml
[regent]
test_command = ""          # explicit command remains authoritative
auto_discover_tests = false # opt in to run-start discovery
```

When `auto_discover_tests` is false and `test_command` is empty, existing no-test behavior remains unchanged. CLI preview still performs discovery because it is an explicit operator request.

## Text output

Each step displays:

```text
HIGH  <working-directory>  <command>
      source: <manifest-or-recipe evidence>
```

Diagnostics follow the plan. Aggregate steps are labeled and covered child manifests are not listed.

## JSON output

```json
{
  "explicit": false,
  "detected_at": "2026-08-10T12:00:00Z",
  "steps": [
    {
      "command": "pnpm test",
      "dir": "portal",
      "source": "portal/package.json scripts.test + packageManager",
      "confidence": "high",
      "aggregate": false
    }
  ],
  "diagnostics": []
}
```

Paths are relative to the discovery root in serialized output where possible.

## Run-start snapshot

When automatic discovery is enabled, the command layer computes one plan before starting agent work and attaches it to the loop/Regent test gate. Later manifest edits do not change the commands in that run.

## Precedence and deduplication

1. Explicit `regent.test_command`.
2. Root recognized aggregate `justfile` recipe.
3. Recognized workspace aggregate manifest.
4. Independent nested manifests ordered by normalized path.

An aggregate step suppresses only children it demonstrably covers. Exact `(directory, command)` duplicates are removed.
