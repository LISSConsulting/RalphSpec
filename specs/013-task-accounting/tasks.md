# Tasks: Task Accounting

**Spec**: [spec.md](./spec.md)

- [X] T001 [US2] Add `countTasks` helper + table tests in `internal/loop`
- [X] T002 [US2] Inject per-iteration `## Task Accounting` section + change-detected `LogInfo` in `internal/loop/loop.go`
- [X] T003 [US1] Update `buildPromptTemplate` and `roamPromptTemplate` in `internal/config/scaffold.go`
- [X] T004 Backfill `agent.stdin_steer` + `[harness]` in `configScaffoldSections`
- [X] T005 Sync repo's own `BUILD.md` with the new template
- [X] T006 Tests for T002–T004; run `gofmt`, `go vet ./...`, grouped `go test`
