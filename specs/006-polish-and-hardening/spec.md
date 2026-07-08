# Spec 006 — Polish & Hardening

## Summary

Quality sweep covering colorful --no-tui output, ANTHROPIC_API_KEY safety warning, test coverage easy wins, code deduplication, documentation gaps, dependency updates, and CI improvements.

---

## Features

### F1: Colorful --no-tui Output

**Problem:** `--no-tui` mode outputs plain uncolored text via `formatLogLine()` (`execute.go:324`). The TUI has a rich color palette (styles.go) but none of it reaches the plain-text path.

**Solution:**
- Add a `--no-color` persistent flag to rootCmd (default: false).
- When `--no-tui` is active and `--no-color` is not set, use lipgloss inline styles in `formatLogLine()`:
  - Timestamp: gray
  - Regent messages: orange with shield
  - Error entries (`LogError`): red bold
  - Iteration complete (`LogIterComplete`): green bold
  - Tool use (`LogToolUse`): blue/green/yellow per tool category (reuse `toolStyle` logic)
  - Done/Stopped (`LogDone`, `LogStopped`): green/gray
  - Info: white
- `formatLogLine` gains a `color bool` parameter (or use a small `lineFormatter` struct).
- Lipgloss auto-detects terminal capability; on dumb terminals or piped output it degrades gracefully — no manual detection needed.

**Acceptance:**
- `ralph build --no-tui` shows colored output matching the TUI palette.
- `ralph build --no-tui --no-color` shows plain text (current behavior).
- Piping to a file produces no ANSI escapes (lipgloss handles this).

---

### F2: ANTHROPIC_API_KEY Warning

**Problem:** If `ANTHROPIC_API_KEY` is set in the environment, the Claude CLI may use it for direct API calls instead of the user's Claude Pro/Max subscription, leading to unexpected charges.

**Solution:**
- At startup in `rootCmd().PersistentPreRunE`, check `os.Getenv("ANTHROPIC_API_KEY")`.
- If non-empty, print a prominent warning to stderr:
  ```
  WARNING: ANTHROPIC_API_KEY is set. Claude may use direct API billing
  instead of your subscription. Unset it to avoid unexpected charges.
  ```
- Use lipgloss yellow+bold styling (respecting `--no-color`).
- Do NOT block execution — warning only.

**Acceptance:**
- Warning appears on stderr when env var is set.
- Warning is styled (yellow bold) unless `--no-color` or piped.
- No warning when env var is unset or empty.

---

### F3: Test Coverage Easy Wins

Targeted test additions for functions with addressable coverage gaps. NOT coverage for coverage's sake — each targets a real code path.

| Function | File | Current | Target | Approach |
|---|---|---|---|---|
| `checkDir()` non-dir | spec/resolve.go:92 | 88% | 100% | Create file at spec path, assert error |
| `RunTests()` edge | regent/tester.go:29 | 94% | 99% | Non-ExitError failure scenario |
| `findConfig()` parent | config/config.go:225 | 91% | 99% | Nested dir with config in parent |
| `SaveState()` write err | regent/state.go:57 | 67% | 95% | Read-only temp dir |
| `NewJSONL()` seek err | store/jsonl.go:40 | 77% | 95% | Unreadable/corrupt file scenario |
| `Append()` write err | store/jsonl.go:68 | 90% | 99% | Close file before append |
| `EnforceRetention()` err | store/jsonl.go:150 | 89% | 99% | Read-only dir for remove failure |

**Out of scope:** TTY-dependent functions (runWithRegentTUI, runDashboard, tickCmd), signal tests on Windows.

---

### F4: Extract Common Setup in execute.go

**Problem:** `executeLoop()` and `executeRun()` share setup: config load, validation, working dir, signal context, git runner, roam computation, loop struct init, spec resolution, store init.

**Solution:** Extract into a `setupLoop(noTUI, roam bool) (*loopSetup, error)` helper returning a struct with all initialized components. Both functions call it then diverge only on their run logic.

**Acceptance:**
- Zero duplicated setup code between the two functions.
- `executeDashboard()` also uses a subset (store init) from the same helper.
- All existing tests pass unchanged.

---

### F5: Documentation Gaps

**README.md updates:**
- Document `--no-tui` flag with usage example for CI/headless.
- Document `--max N` flag: `ralph build --max 5`.
- Document `--roam` flag with explanation of what roam means.
- Document `--no-color` flag (new from F1).
- Add `ANTHROPIC_API_KEY` warning explanation.

---

### F6: Dependency Updates

Run `go get -u ./...` to pull minor/patch updates on transitive deps. No major version bumps. Verify `go test ./...` and `go vet ./...` pass after update.

Known available updates:
- `charmbracelet/colorprofile` 0.4.1 -> 0.4.2
- `mattn/go-runewidth` 0.0.19 -> 0.0.20
- `spf13/pflag` 1.0.9 -> 1.0.10
- Various clipperhouse minor bumps

---

### F7: CI Improvements

- Add coverage artifact: `go test -coverprofile=coverage.out ./...` + upload as artifact.
- Add release job: on tag push (`v*`), create GitHub Release with cross-compiled binaries using `gh release create`.

---

## Non-Goals

- Refactoring TUI internals (already at 99%+ coverage).
- Adding new loop modes or spec workflow changes.
- Godoc additions on internal types (low value for a CLI tool).
- Flag registration dedup (cosmetic, low ROI for 3 commands).

## Priority Order

F2 (API key warning) > F1 (color output) > F3 (test coverage) > F4 (dedup) > F5 (docs) > F6 (deps) > F7 (CI)
