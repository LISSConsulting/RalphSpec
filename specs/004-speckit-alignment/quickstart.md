# Quickstart: Spec Kit Alignment

**Feature**: 004-speckit-alignment

## Prerequisites

- Go 1.24+
- Claude Code CLI installed and on PATH (`claude --version`)
- Speckit skills available in Claude Code environment

## Building

```sh
go build ./cmd/ralph/
```

## Speckit Workflow (new)

```sh
# 1. Create a new feature spec
ralph specify "Add user authentication with OAuth2"
#    → creates specs/005-user-auth/spec.md via /speckit.specify

# 2. Clarify ambiguities (optional)
ralph clarify
#    → runs /speckit.clarify on the active spec

# 3. Generate implementation plan
ralph plan
#    → creates specs/005-user-auth/plan.md via /speckit.plan

# 4. Break plan into tasks
ralph tasks
#    → creates specs/005-user-auth/tasks.md via /speckit.tasks

# 5. Execute tasks
ralph run
#    → runs /speckit.implement against tasks.md
```

## Active Spec Resolution

Ralph determines the active spec from the current git branch:

```sh
git checkout 005-user-auth     # branch matches specs/005-user-auth/
ralph plan                      # automatically targets specs/005-user-auth/
```

Override with `--spec`:

```sh
ralph plan --spec 004-speckit-alignment   # target a specific spec
```

## Autonomous Loop (existing behavior, moved)

```sh
ralph loop build                # autonomous loop build mode
ralph loop build                # old: ralph build
ralph loop run                  # old: ralph run
ralph build                     # unchanged (also available as ralph loop build)
```

## Listing Specs

```sh
ralph spec list
# Output:
#   📋  004-speckit-alignment    specs/004-speckit-alignment    specified
#   📐  003-tui-redesign         specs/003-tui-redesign         planned
#   ✅  002-v2-improvements      specs/002-v2-improvements      tasked
```

## Testing

```sh
go test ./...                   # all tests
go test ./internal/spec/...     # spec discovery + resolution tests
go test ./cmd/ralph/...         # command registration + integration tests
go vet ./...                    # must pass clean
```
