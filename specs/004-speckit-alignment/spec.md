# Feature Specification: Spec Kit Alignment

**Feature Branch**: `004-speckit-alignment`
**Created**: 2026-03-01
**Status**: Draft
**Input**: User description: "Align Ralph with spec kit framework — Ralph must understand the spec kit directory layout, map CLI commands to speckit skills, and update prompt files accordingly."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Spec Kit Directory Discovery (Priority: P1)

As a developer using Ralph, I want Ralph to understand the spec kit directory layout so that spec status, navigation, and artifact display reflect the full spec kit structure — not just flat `.md` files.

The spec kit canonical layout is:

```
specs/NNN-feature-name/
├── spec.md
├── plan.md
├── tasks.md
├── data-model.md       (optional)
├── quickstart.md       (optional)
├── research.md         (optional)
├── checklists/
│   └── requirements.md
└── contracts/
    └── *.md            (optional)
```

Ralph's spec discovery currently treats every `.md` file inside a spec subdirectory as a separate spec. Instead, each `specs/NNN-name/` directory should be treated as **one spec feature**, with `spec.md` being the primary artifact and the other files being supporting artifacts of that same feature.

**Why this priority**: Without correct directory understanding, every downstream command (specify, plan, clarify, tasks, run) operates on the wrong data model. This is the foundation.

**Independent Test**: Can be fully tested by creating a spec kit directory structure and verifying that `ralph spec list` shows one entry per feature directory (not one per `.md` file), and that the TUI specs panel groups artifacts under a single feature.

**Acceptance Scenarios**:

1. **Given** a `specs/004-speckit-alignment/` directory with `spec.md`, `plan.md`, `tasks.md`, and `checklists/requirements.md`, **When** I run `ralph spec list`, **Then** I see one spec entry named `004-speckit-alignment` with status derived from artifact presence (`spec.md` only = "specified", `+plan.md` = "planned", `+tasks.md` = "tasked") — not four separate entries.
2. **Given** a spec feature directory, **When** I view it in the TUI specs panel, **Then** the feature is listed once and selecting it shows `spec.md` content, with the ability to navigate to other artifacts (plan.md, tasks.md, etc.).
3. **Given** a `specs/` directory containing both legacy flat `.md` files and spec kit directories, **When** I run `ralph spec list`, **Then** both formats are listed — flat files as individual specs, directories as feature specs.

---

### User Story 2 - Speckit Command Mapping (Priority: P1)

As a developer, I want Ralph CLI commands to map directly to spec kit phases so that running `ralph specify`, `ralph plan`, `ralph clarify`, `ralph tasks`, and `ralph run` invokes the corresponding speckit skill via Claude Code.

The command mapping is:

| Ralph Command   | Speckit Skill        | Purpose                           |
| --------------- | -------------------- | --------------------------------- |
| `ralph specify` | `/speckit.specify`   | Create/update feature spec        |
| `ralph plan`    | `/speckit.plan`      | Generate implementation plan      |
| `ralph clarify` | `/speckit.clarify`   | Resolve ambiguities in spec       |
| `ralph tasks`   | `/speckit.tasks`     | Break plan into task list         |
| `ralph run`     | `/speckit.implement` | Execute tasks from task breakdown |

**Why this priority**: Equal to P1 with directory discovery — these commands are the primary user interface for the spec kit workflow. Without them, there is no spec kit integration.

**Independent Test**: Can be tested by running each command and verifying that Claude Code is invoked with the correct speckit slash command and that the resulting artifacts land in the correct spec directory.

**Acceptance Scenarios**:

1. **Given** I am on branch `004-speckit-alignment`, **When** I run `ralph specify "Add encryption service"`, **Then** Ralph invokes Claude Code with the `/speckit.specify` skill and the description argument, and a `spec.md` is created/updated in the active spec directory.
2. **Given** a spec directory with `spec.md`, **When** I run `ralph plan`, **Then** Ralph invokes Claude Code with `/speckit.plan` targeting the active spec, and a `plan.md` is created in the same directory.
3. **Given** a spec directory with `spec.md` and `plan.md`, **When** I run `ralph tasks`, **Then** Ralph invokes Claude Code with `/speckit.tasks` and a `tasks.md` is created.
4. **Given** a spec directory with `tasks.md`, **When** I run `ralph run`, **Then** Ralph invokes Claude Code with `/speckit.implement` and tasks are executed.
5. **Given** I run `ralph clarify`, **When** there are `[NEEDS CLARIFICATION]` markers in `spec.md`, **Then** Ralph invokes Claude Code with `/speckit.clarify` and the spec is updated with answers.

---

### User Story 3 - Active Spec Resolution (Priority: P2)

As a developer, I want Ralph to automatically determine which spec feature is active based on the current git branch so that commands operate on the correct spec directory without requiring explicit paths.

**Why this priority**: Quality-of-life feature that makes the command mapping (P1) ergonomic. Without it, every command would need an explicit spec path argument.

**Independent Test**: Can be tested by checking out branch `004-speckit-alignment` and verifying that Ralph resolves the active spec to `specs/004-speckit-alignment/`.

**Acceptance Scenarios**:

1. **Given** I am on branch `004-speckit-alignment`, **When** I run any speckit command (e.g., `ralph plan`), **Then** Ralph resolves the active spec directory to `specs/004-speckit-alignment/` by matching the branch name to a spec directory name.
2. **Given** I am on branch `main` (no matching spec directory), **When** I run `ralph plan`, **Then** Ralph shows an error explaining no active spec was found and suggests using `--spec` to target one explicitly.
3. **Given** I am on branch `004-speckit-alignment` but `specs/004-speckit-alignment/` does not exist, **When** I run `ralph specify "description"`, **Then** Ralph creates the directory and proceeds with spec creation.

---

### User Story 4 - Repurpose Existing plan/run Commands (Priority: P2)

As a developer, I want the existing Claude loop behavior preserved under a `loop` parent command so that the top-level `plan` and `run` commands can map to speckit while the autonomous loop remains accessible.

| Current Command | New Name            | Old Behavior                                  |
| --------------- | ------------------- | --------------------------------------------- |
| `ralph build`   | `ralph loop build`  | Run Claude loop in build mode (BUILD.md)      |
| `ralph run`     | `ralph loop run`    | Run the autonomous build loop                 |

**Why this priority**: Prevents breaking changes for users relying on the current loop behavior, while freeing the top-level command namespace for speckit.

**Independent Test**: Can be tested by verifying `ralph loop build` invokes the autonomous loop with BUILD.md, and `ralph plan` invokes `/speckit.plan`.

**Acceptance Scenarios**:

1. **Given** existing `ralph build` behavior, **When** I run `ralph loop build`, **Then** the Claude loop runs in build mode exactly as the old `ralph build` did.
2. **Given** existing `ralph run` behavior, **When** I run `ralph loop run`, **Then** the autonomous build loop remains available under `loop`.
3. **Given** I run `ralph plan` (without `loop` prefix), **Then** it invokes `/speckit.plan`.

---

### User Story 5 - ROAM.md and BUILD.md Update (Priority: P3)

As a developer, I want ROAM.md and BUILD.md to be updated so that when Ralph's autonomous loop runs (`ralph build --roam` or `ralph loop build`), its prompts understand and reference the spec kit directory structure instead of assuming flat spec files.

**Why this priority**: The autonomous loop (now under `ralph loop`) still needs correct prompts. Lower priority because the primary workflow shifts to speckit commands.

**Independent Test**: Can be tested by running `ralph build --roam` and verifying the agent correctly discovers and reads artifacts from spec kit directories.

**Acceptance Scenarios**:

1. **Given** ROAM.md is updated, **When** the roaming agent runs, **Then** it reads `spec.md`, `plan.md`, and `tasks.md` from each `specs/NNN-name/` directory — not just any `.md` file in `specs/`.
2. **Given** BUILD.md is updated, **When** the build agent runs, **Then** it picks tasks from `tasks.md` within the active spec directory and references the corresponding `spec.md` and `plan.md` for context.

---

### Edge Cases

- What happens when a spec directory has no `spec.md`? Ralph should treat it as an incomplete/empty spec and show a warning.
- How does Ralph handle multiple spec directories matching partial branch names? It requires an exact match between branch name and directory name.
- What happens when Claude Code doesn't have the speckit skills installed? Ralph should detect invocation failure and show a clear error message explaining the dependency.
- What happens when the user runs `ralph specify` without a description argument? Ralph should show usage help explaining a description is required. `ralph spec new` no longer exists — `specify` is the only creation path.
- What happens when `ralph run` is invoked but `tasks.md` doesn't exist yet? Ralph should show an error suggesting the user run `ralph tasks` first.
- What happens when the user runs `ralph build` (without `loop`)? It remains unchanged — `build` was not remapped and still runs the Claude loop directly.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST discover spec features as directories under `specs/`, treating each `specs/NNN-name/` directory as a single feature — not individual `.md` files. Status MUST be derived from artifact presence: `spec.md` only = "specified", `+plan.md` = "planned", `+tasks.md` = "tasked".
- **FR-002**: System MUST support the canonical spec kit file layout: `spec.md`, `plan.md`, `tasks.md`, `data-model.md`, `quickstart.md`, `research.md`, plus `checklists/` and `contracts/` subdirectories.
- **FR-003**: System MUST register `specify`, `plan`, `clarify`, and `tasks` as top-level commands that invoke the corresponding speckit skills via Claude Code.
- **FR-004**: System MUST repurpose the `run` command to invoke `/speckit.implement` via Claude Code.
- **FR-005**: System MUST expose Claude loop commands under a `loop` parent command (`ralph loop build`, `ralph loop run`). Top-level `ralph build` MUST also remain as a direct alias since it has no speckit equivalent.
- **FR-006**: System MUST resolve the active spec directory from the current git branch name by matching against `specs/` directory names.
- **FR-007**: System MUST invoke Claude Code by spawning `claude` with the appropriate speckit slash command and passing relevant context (spec directory path, description arguments).
- **FR-008**: System MUST update ROAM.md to reference the spec kit directory structure when roaming across specs vs. codebase.
- **FR-009**: System MUST update BUILD.md to read tasks from `tasks.md` within spec directories and reference `spec.md` and `plan.md` for context.
- **FR-010**: System MUST remove `ralph spec new` entirely — `ralph specify` is the sole way to create specs. `ralph spec list` MUST be updated to work with the spec kit directory model.
- **FR-011**: System MUST display a clear error when a speckit command is run but no active spec is found (no branch match and no explicit path).
- **FR-012**: System MUST support the `--spec` flag on speckit commands to explicitly target a spec directory when branch-based resolution is not desired.

### Key Entities

- **Spec Feature**: A directory under `specs/` representing a complete feature with its spec kit artifacts. Identified by the directory name (e.g., `004-speckit-alignment`). Contains a primary artifact (`spec.md`) and optional supporting artifacts. Status is derived from artifact presence: "specified" → "planned" → "tasked" based on which files exist.
- **Spec Kit Artifact**: An individual file within a spec feature directory (`spec.md`, `plan.md`, `tasks.md`, `data-model.md`, etc.). Each artifact type has a defined role in the spec kit workflow.
- **Spec Kit Phase**: A step in the spec-driven workflow (specify → clarify → plan → tasks → implement). Each phase maps to a Ralph command and a speckit skill.
- **Active Spec**: The spec feature that Ralph operates on, resolved from the current git branch name or an explicit `--spec` flag.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: All five speckit commands (`specify`, `plan`, `clarify`, `tasks`, `run`) successfully invoke Claude Code and produce the expected artifacts in the correct spec directory.
- **SC-002**: `ralph spec list` shows one entry per feature directory (not one per `.md` file) for all existing spec directories.
- **SC-003**: Existing Claude loop behavior remains fully functional under `ralph loop build` and `ralph loop run` with zero regressions.
- **SC-004**: Active spec resolution correctly identifies the spec directory in 100% of cases where the branch name exactly matches a directory under `specs/`.
- **SC-005**: All existing tests continue to pass after the refactor.
- **SC-006**: Users can complete the full spec-driven workflow (specify → clarify → plan → tasks → run) using only Ralph CLI commands without needing to invoke Claude Code slash commands directly.

## Clarifications

### Session 2026-03-01

- Q: How is spec feature status determined in the new model? → A: Artifact-presence detection — `spec.md` only = "specified", `+plan.md` = "planned", `+tasks.md` = "tasked", all present = "ready".
- Q: Should `ralph spec new` coexist with `ralph specify`? → A: Remove `ralph spec new` completely. Greenfield project — no legacy cruft. `ralph specify` is the sole way to create specs.

## Assumptions

- Claude Code is installed and available on PATH as `claude`.
- The speckit skills (`/speckit.specify`, `/speckit.plan`, `/speckit.clarify`, `/speckit.tasks`, `/speckit.implement`) are available in the user's Claude Code environment.
- The spec kit directory naming convention follows the pattern `NNN-feature-name` matching the git branch name.
- CHRONICLE.md may still be used for loop-mode status tracking but is not the primary workflow state for speckit commands.
- The `ralph init` command scaffold may need updating to reflect spec kit structure, but that is out of scope for this spec.
- The `ralph build` command (without `loop`) remains unchanged — it is not remapped to a speckit skill.
