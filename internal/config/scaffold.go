package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// ScaffoldOptions controls project scaffold behavior.
type ScaffoldOptions struct {
	// Force overwrites Ralph-owned scaffold files and removes legacy PLAN.md.
	Force bool
}

// ScaffoldProject creates the full ralph project structure in the given
// directory. It creates or updates ralph.toml, prompt files for build and roam
// modes, the specs/ directory, .gitignore, and CHRONICLE.md. Prompt and
// chronicle files that already exist are left untouched. Returns the list of
// changed paths.
func ScaffoldProject(dir string) ([]string, error) {
	return ScaffoldProjectWithOptions(dir, ScaffoldOptions{})
}

// ScaffoldProjectWithOptions creates or updates the full Ralph project
// structure. With Force, Ralph-owned files are overwritten from current
// templates and legacy PLAN.md is removed.
func ScaffoldProjectWithOptions(dir string, opts ScaffoldOptions) ([]string, error) {
	var created []string

	// ralph.toml
	tomlPath := filepath.Join(dir, "ralph.toml")
	if opts.Force {
		if err := writeDefaultConfigFile(tomlPath); err != nil {
			return created, err
		}
		created = append(created, tomlPath)
	} else if _, err := os.Stat(tomlPath); os.IsNotExist(err) {
		if _, initErr := InitFile(dir); initErr != nil {
			return created, initErr
		}
		created = append(created, tomlPath)
	} else if err != nil {
		return created, fmt.Errorf("scaffold: stat %s: %w", tomlPath, err)
	} else {
		updated, updateErr := updateExistingConfigFile(tomlPath)
		if updateErr != nil {
			return created, updateErr
		}
		if updated {
			created = append(created, tomlPath)
		}
	}

	// ROAM.md
	roamPath := filepath.Join(dir, "ROAM.md")
	if opts.Force || fileMissing(roamPath) {
		if writeErr := os.WriteFile(roamPath, []byte(roamPromptTemplate), 0644); writeErr != nil {
			return created, fmt.Errorf("scaffold: write %s: %w", roamPath, writeErr)
		}
		created = append(created, roamPath)
	} else if _, err := os.Stat(roamPath); err != nil {
		return created, fmt.Errorf("scaffold: stat %s: %w", roamPath, err)
	}

	// BUILD.md
	buildPath := filepath.Join(dir, "BUILD.md")
	if opts.Force || fileMissing(buildPath) {
		if writeErr := os.WriteFile(buildPath, []byte(buildPromptTemplate), 0644); writeErr != nil {
			return created, fmt.Errorf("scaffold: write %s: %w", buildPath, writeErr)
		}
		created = append(created, buildPath)
	} else if _, err := os.Stat(buildPath); err != nil {
		return created, fmt.Errorf("scaffold: stat %s: %w", buildPath, err)
	}

	// specs/ directory
	specsDir := filepath.Join(dir, "specs")
	if _, err := os.Stat(specsDir); os.IsNotExist(err) {
		if mkErr := os.MkdirAll(specsDir, 0755); mkErr != nil {
			return created, fmt.Errorf("scaffold: create %s: %w", specsDir, mkErr)
		}
		created = append(created, specsDir)
	}

	// .gitignore — ensure the regent state file is excluded from version control
	const gitignoreEntry = ".ralph/regent-state.json"
	gitignorePath := filepath.Join(dir, ".gitignore")
	existing, err := os.ReadFile(gitignorePath)
	switch {
	case os.IsNotExist(err):
		if writeErr := os.WriteFile(gitignorePath, []byte(gitignoreEntry+"\n"), 0644); writeErr != nil {
			return created, fmt.Errorf("scaffold: write %s: %w", gitignorePath, writeErr)
		}
		created = append(created, gitignorePath)
	case err != nil:
		return created, fmt.Errorf("scaffold: read %s: %w", gitignorePath, err)
	case !strings.Contains(string(existing), gitignoreEntry):
		content := string(existing)
		if len(content) > 0 && content[len(content)-1] != '\n' {
			content += "\n"
		}
		content += gitignoreEntry + "\n"
		if writeErr := os.WriteFile(gitignorePath, []byte(content), 0644); writeErr != nil {
			return created, fmt.Errorf("scaffold: write %s: %w", gitignorePath, writeErr)
		}
		created = append(created, gitignorePath)
	}

	// CHRONICLE.md
	chroniclePath := filepath.Join(dir, "CHRONICLE.md")
	if opts.Force || fileMissing(chroniclePath) {
		if writeErr := os.WriteFile(chroniclePath, []byte(implementationPlanTemplate), 0644); writeErr != nil {
			return created, fmt.Errorf("scaffold: write %s: %w", chroniclePath, writeErr)
		}
		created = append(created, chroniclePath)
	} else if _, err := os.Stat(chroniclePath); err != nil {
		return created, fmt.Errorf("scaffold: stat %s: %w", chroniclePath, err)
	}

	if opts.Force {
		planPath := filepath.Join(dir, "PLAN.md")
		if err := os.Remove(planPath); err == nil {
			created = append(created, planPath)
		} else if !os.IsNotExist(err) {
			return created, fmt.Errorf("scaffold: remove %s: %w", planPath, err)
		}
	}

	return created, nil
}

func fileMissing(path string) bool {
	_, err := os.Stat(path)
	return os.IsNotExist(err)
}

type configScaffoldSection struct {
	name    string
	entries []configScaffoldEntry
}

type configScaffoldEntry struct {
	key  string
	line string
}

var configScaffoldSections = []configScaffoldSection{
	{name: "project", entries: []configScaffoldEntry{
		{key: "name", line: `name = ""`},
	}},
	{name: "agent", entries: []configScaffoldEntry{
		{key: "type", line: `type = "claude"`},
	}},
	{name: "claude", entries: []configScaffoldEntry{
		{key: "model", line: `model = "sonnet"`},
		{key: "max_turns", line: `max_turns = 0  # 0 = unlimited agentic turns per iteration`},
		{key: "danger_skip_permissions", line: `danger_skip_permissions = true`},
	}},
	{name: "codex", entries: []configScaffoldEntry{
		{key: "model", line: `model = ""`},
	}},
	{name: "build", entries: []configScaffoldEntry{
		{key: "prompt_file", line: `prompt_file = "BUILD.md"`},
		{key: "max_iterations", line: `max_iterations = 0  # 0 = unlimited`},
	}},
	{name: "roam", entries: []configScaffoldEntry{
		{key: "enabled", line: `enabled = false     # roam freely across the codebase (--roam flag overrides)`},
		{key: "prompt_file", line: `prompt_file = "ROAM.md"`},
		{key: "max_iterations", line: `max_iterations = 0  # 0 = unlimited`},
		{key: "focus", line: `focus = ""          # constrain roam to a specific topic (--focus flag overrides)`},
	}},
	{name: "git", entries: []configScaffoldEntry{
		{key: "auto_pull_rebase", line: `auto_pull_rebase = true`},
		{key: "auto_push", line: `auto_push = true`},
	}},
	{name: "regent", entries: []configScaffoldEntry{
		{key: "enabled", line: `enabled = true`},
		{key: "rollback_on_test_failure", line: `rollback_on_test_failure = false`},
		{key: "test_command", line: `test_command = ""`},
		{key: "max_retries", line: `max_retries = 3`},
		{key: "retry_backoff_seconds", line: `retry_backoff_seconds = 30`},
		{key: "hang_timeout_seconds", line: `hang_timeout_seconds = 300`},
	}},
	{name: "tui", entries: []configScaffoldEntry{
		{key: "accent_color", line: `accent_color = "#7D56F4"  # hex color for header/accent elements`},
		{key: "log_retention", line: `log_retention = 20        # number of session logs to keep; 0 = unlimited`},
	}},
	{name: "notifications", entries: []configScaffoldEntry{
		{key: "url", line: `url = ""           # ntfy.sh topic URL or any HTTP webhook (empty = disabled)`},
		{key: "on_complete", line: `on_complete = true # notify on each iteration complete`},
		{key: "on_error", line: `on_error = true    # notify on loop error`},
		{key: "on_stop", line: `on_stop = true     # notify when loop finishes or is stopped`},
	}},
	{name: "worktree", entries: []configScaffoldEntry{
		{key: "enabled", line: `enabled = false        # enable git worktree support via worktrunk`},
		{key: "max_parallel", line: `max_parallel = 5       # maximum number of concurrent worktree agents`},
		{key: "auto_merge", line: `auto_merge = false     # automatically merge on spec-complete + tests pass`},
		{key: "merge_target", line: `merge_target = ""      # branch to merge into (empty = branch worktree was created from)`},
		{key: "path_template", line: `path_template = ""     # deprecated: use worktree_dir`},
		{key: "worktree_dir", line: `worktree_dir = ""      # base directory for worktrees (default: ~/.ralph/worktrees)`},
	}},
}

func updateExistingConfigFile(path string) (bool, error) {
	existing, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("config: read %s: %w", path, err)
	}

	var cfg Config
	meta, err := toml.Decode(string(existing), &cfg)
	if err != nil {
		return false, fmt.Errorf("config: decode %s: %w", path, err)
	}
	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, len(undecoded))
		for i, k := range undecoded {
			keys[i] = k.String()
		}
		return false, fmt.Errorf("config: unknown keys in %s: %s (possible typos?)", path, joinKeys(keys))
	}

	missing := map[string][]string{}
	for _, section := range configScaffoldSections {
		for _, entry := range section.entries {
			if !meta.IsDefined(section.name, entry.key) {
				missing[section.name] = append(missing[section.name], entry.line)
			}
		}
	}
	if len(missing) == 0 {
		return false, nil
	}

	updated := appendMissingConfigEntries(string(existing), missing)
	if err := os.WriteFile(path, []byte(updated), 0644); err != nil {
		return false, fmt.Errorf("config: write %s: %w", path, err)
	}
	return true, nil
}

type configSectionRange struct {
	start int
	end   int
}

func appendMissingConfigEntries(content string, missing map[string][]string) string {
	var lines []string
	if content != "" {
		lines = strings.Split(content, "\n")
	}
	if content != "" && strings.HasSuffix(content, "\n") {
		lines = lines[:len(lines)-1]
	}

	sectionRanges := findConfigSectionRanges(lines)
	insertions := map[int][]string{}
	var appendSections []configScaffoldSection
	for _, section := range configScaffoldSections {
		entries := missing[section.name]
		if len(entries) == 0 {
			continue
		}
		sectionRange, ok := sectionRanges[section.name]
		if !ok {
			appendSections = append(appendSections, configScaffoldSection{name: section.name, entries: linesToEntries(entries)})
			continue
		}

		insertAt := sectionRange.end
		for insertAt > sectionRange.start+1 && strings.TrimSpace(lines[insertAt-1]) == "" {
			insertAt--
		}
		insertions[insertAt] = append(insertions[insertAt], entries...)
	}

	updated := make([]string, 0, len(lines)+len(missing))
	for i, line := range lines {
		updated = append(updated, insertions[i]...)
		updated = append(updated, line)
	}
	updated = append(updated, insertions[len(lines)]...)

	if len(appendSections) > 0 {
		if len(updated) > 0 && strings.TrimSpace(updated[len(updated)-1]) != "" {
			updated = append(updated, "")
		}
		for _, section := range appendSections {
			updated = append(updated, "["+section.name+"]")
			for _, entry := range section.entries {
				updated = append(updated, entry.line)
			}
			updated = append(updated, "")
		}
		updated = updated[:len(updated)-1]
	}

	return strings.Join(updated, "\n") + "\n"
}

func linesToEntries(lines []string) []configScaffoldEntry {
	entries := make([]configScaffoldEntry, len(lines))
	for i, line := range lines {
		entries[i] = configScaffoldEntry{line: line}
	}
	return entries
}

func findConfigSectionRanges(lines []string) map[string]configSectionRange {
	ranges := map[string]configSectionRange{}
	current := ""
	for i, line := range lines {
		section, ok := parseTopLevelSection(line)
		if !ok {
			continue
		}
		if current != "" {
			r := ranges[current]
			r.end = i
			ranges[current] = r
		}
		current = section
		ranges[current] = configSectionRange{start: i, end: len(lines)}
	}
	return ranges
}

func parseTopLevelSection(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "[[") {
		return "", false
	}
	closeBracket := strings.Index(trimmed, "]")
	if closeBracket <= 1 {
		return "", false
	}
	remainder := strings.TrimSpace(trimmed[closeBracket+1:])
	if remainder != "" && !strings.HasPrefix(remainder, "#") {
		return "", false
	}
	section := strings.TrimSpace(trimmed[1:closeBracket])
	if section == "" || strings.Contains(section, ".") {
		return "", false
	}
	return section, true
}

const roamPromptTemplate = `You are a roaming build agent. Your state file is @CHRONICLE.md.

Roaming mode is for codebase-wide improvement when no single active spec should constrain the work.

## Context

Read these sources using parallel subagents before making changes:
- @CHRONICLE.md - unresolved blockers, open findings, current decisions, and active follow-ups only
- specs/ - application specifications; treat these as read-only source material
- The codebase - implementation, tests, documentation, workflows, and configuration

## Mission

Find and complete one high-leverage improvement per iteration. Good roam work includes:
- Verified spec drift or missing behavior found by comparing specs/ against implementation
- Bugs, flaky tests, failing tests, weak coverage, or untested edge cases
- Stale README/help text/docs, outdated examples, or misleading comments
- TODO/FIXME/HACK/XXX items with clear, contained fixes
- Dead code, unused helpers, duplication, or small maintainability wins
- CI, tooling, or configuration problems that are safe to correct

## Constraints

- Search before assuming. Confirm every issue from source, tests, docs, or command output.
- Do not modify specs/ unless the user explicitly asks.
- Avoid broad rewrites, aesthetic churn, placeholder code, and unrelated changes.
- Prefer the smallest complete fix that leaves the repository healthier.
- Keep @CHRONICLE.md compact. Record unresolved blockers, newly discovered follow-ups, and current decisions; avoid replaying completed history that already exists in git and JSONL logs.

## Workflow

1. Inspect @CHRONICLE.md and choose the highest-value roam item that is still valid.
2. If no valid item exists, perform a focused sweep across tests, docs, TODOs, dead code, and spec drift.
3. Implement one cohesive improvement completely.
4. Run the relevant tests or checks for the files changed.
5. Update @CHRONICLE.md only if there is an unresolved blocker, new follow-up, current decision, or short completion note worth carrying forward, then commit with a descriptive message.

## Completion Criteria

This iteration is complete when one verified improvement is shipped with tests/checks run, durable state recorded only if needed, and changes committed.
`

const buildPromptTemplate = `You are a build agent implementing the active specification.

## Inputs

- Active spec context is provided by Ralph when available. Stay inside that spec boundary.
- In the active specs/NNN-name/ directory, read spec.md, plan.md, and tasks.md.
- Treat specs as read-only. If a spec is wrong or ambiguous, record the issue instead of editing specs.
- Use @CHRONICLE.md only for current blockers, open findings, and notes that are not already captured in spec artifacts. Do not replay old completed-work history.

## Rules

- Search before assuming missing behavior.
- Implement one highest-priority incomplete task or blocker completely.
- No placeholders, stubs, broad rewrites, or compatibility shims without a concrete need.
- Stage specific files only; never use git add -A.
- Keep @AGENTS.md operational only. Put transient findings in @CHRONICLE.md.

## Workflow

1. Select the next incomplete task from the active spec's tasks.md. If none is actionable, use the highest-priority unresolved blocker in @CHRONICLE.md.
2. Confirm current implementation state with code search and tests before editing.
3. Make the smallest complete change that satisfies the task.
4. Run the relevant tests or checks for the changed area.
5. Update @CHRONICLE.md only with unresolved blockers, newly discovered follow-ups, or a short note that the selected item is complete.
6. Commit and push when tests pass.

## Empty Queue

If the active spec has no incomplete tasks and @CHRONICLE.md has no unresolved blockers, perform one focused improvement sweep:

- tests below useful coverage thresholds
- TODO/FIXME/HACK/XXX with contained fixes
- stale README/help/docs
- CI/tooling drift
- obvious dead code

Ship at most one cohesive improvement per iteration, then update @CHRONICLE.md with the result.

## Completion

Stop after one task or one focused improvement is fully implemented, verified, recorded if needed, committed, and pushed.
`

const implementationPlanTemplate = `> Project working memory for Ralph agents.
> Keep this file compact: unresolved state only, not a changelog. Completed history belongs in git, releases, and .ralph/logs/*.jsonl.

## Current Focus

- No active focus yet.

## Open Blockers

| Priority | Blocker | Evidence | Next Action |
|----------|---------|----------|-------------|

## Open Findings

| Priority | Finding | Evidence | Status |
|----------|---------|----------|--------|

## Decisions To Preserve

-
`
