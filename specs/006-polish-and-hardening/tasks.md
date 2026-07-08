# Tasks 006 — Polish & Hardening

## Phase 1: ANTHROPIC_API_KEY Warning + --no-color Flag

- [x] T001: Add `--no-color` persistent flag to `rootCmd()` in `cmd/ralph/main.go`
- [x] T002: Add `PersistentPreRunE` to `rootCmd()` that checks `os.Getenv("ANTHROPIC_API_KEY")` and prints styled warning to stderr (yellow bold via lipgloss, plain if `--no-color`)
- [x] T003: Tests for API key warning — set/unset env var, assert warning present/absent on stderr; `--no-color` asserts no ANSI escapes

## Phase 2: Colorful --no-tui Output

- [x] T004: Create `cmd/ralph/format.go` with `lineFormatter` struct (`color bool`) and `format(entry loop.LogEntry) string` method applying lipgloss styles per LogKind (gray timestamp, red bold error, green bold complete, blue/green/yellow tool use, orange regent, etc.)
- [x] T005: Create `cmd/ralph/format_test.go` with table-driven tests: each LogKind x {color: true, false}, assert ANSI presence/absence
- [x] T006: Delete old `formatLogLine()` from `execute.go`; wire `lineFormatter` into all no-TUI drain goroutines in `wiring.go` (runWithRegent, runWithStateTracking) using `--no-color` flag value
- [x] T007: Migrate any existing `formatLogLine` tests to new formatter tests

## Phase 3: Extract Common Setup

- [x] T008: Create `loopSetup` struct and `setupLoop(noTUI, roam bool) (*loopSetup, error)` in `cmd/ralph/execute.go` extracting shared config load, validation, working dir, signal context, git runner, roam, loop init, spec resolution, store init
- [x] T009: Rewrite `executeLoop()` to call `setupLoop()` then diverge on run logic only
- [x] T010: Rewrite `executeRun()` to call `setupLoop()` then diverge on run logic only
- [x] T011: Verify all existing `execute_test.go` tests pass unchanged

## Phase 4: Test Coverage Easy Wins

- [x] T012: `internal/spec/resolve_test.go` — test `checkDir()` with regular file instead of directory, assert error returned
- [x] T013: `internal/regent/state_test.go` — test `SaveState()` with read-only temp dir, assert write error (skip on Windows if perms not enforceable)
- [x] T014: `internal/config/config_test.go` — test `findConfig()` from nested child dir, assert config found in parent
- [x] T015: `internal/regent/tester_test.go` — test `RunTests()` with nonexistent command, assert non-ExitError path
- [x] T016: `internal/store/jsonl_test.go` — test `Append()` on closed file, assert write error
- [x] T017: `internal/store/jsonl_test.go` — test `EnforceRetention()` with read-only dir, assert remove error (skip on Windows)
- [x] T018: `internal/store/jsonl_test.go` — test `NewJSONL()` with invalid path (e.g. dir as file), assert error

## Phase 5: Documentation

- [x] T019: Update README.md — add `--no-tui`, `--no-color`, `--max N`, `--roam` flag docs with usage examples
- [x] T020: Update README.md — add ANTHROPIC_API_KEY warning explanation

## Phase 6: Dependency Updates

- [x] T021: Run `go get -u ./...` + `go mod tidy`, verify `go test ./...` and `go vet ./...` pass

## Phase 7: CI Improvements

- [x] T022: Update `.github/workflows/ci.yml` test step to emit `coverage.out` and upload as artifact
- [x] T023: Create `.github/workflows/release.yml` — on tag `v*`, cross-compile and create GitHub Release with binaries
