# Tasks: Stdin Steering for Running Agents

**Spec**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md)

## Phase 1 — Core loop

- [X] T001 [US2] Add `LogSteer` LogKind in `internal/loop/event.go`
- [X] T002 [US1] Add `Loop.Steer` channel + `drainSteers()` + prompt nudge + `LogSteer` emission in `internal/loop/loop.go`
- [X] T003 [US3] Add `RunOptions.Steer` in `internal/claude/claude.go`
- [X] T004 [US3] Pass `RunOptions.Steer` when `[agent] stdin_steer` is true in `internal/loop/loop.go`

## Phase 2 — Agent adapters

- [X] T005 [US3] Claude SDK pipe mode + stdin writer in `internal/loop/runner.go`
- [X] T006 [US3] Codex stdin pipe writer in `internal/codex/agent.go`
- [X] T007 [P] Add `AgentConfig.StdinSteer` (`stdin_steer`) + scaffold template in `internal/config/config.go`

## Phase 3 — TUI + wiring

- [X] T008 [US2] Render `LogSteer` with compass icon in `internal/tui/theme.go`
- [X] T009 [US1] Steer input mode (`i`, Enter, Esc) + footer swap in `internal/tui/app.go`
- [X] T010 [US1] Create steer channel + `requestSteer` closures in `cmd/ralph/wiring.go`
- [X] T011 [P] Update help overlay + keymap in `internal/tui/app.go`, `internal/tui/keymap.go`

## Phase 4 — Tests + gates

- [X] T012 [US1] Loop nudge + `LogSteer` tests in `internal/loop/loop_test.go`
- [X] T013 [US3] Claude args/stdin writer tests in `internal/loop/runner_test.go`
- [X] T014 [US3] Codex steer drain test in `internal/codex/agent_test.go`
- [X] T015 [P] Config parse/default tests in `internal/config/config_test.go`
- [X] T016 [US1] TUI steer input tests in `internal/tui/app_test.go`
- [X] T017 Run `gofmt`, `go vet ./...`, grouped `go test ./internal/...`
