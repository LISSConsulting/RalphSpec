package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// ScaffoldProject creates the full ralph project structure in the given
// directory. It creates or updates ralph.toml, prompt files for plan and build
// modes, the specs/ directory, .gitignore, and CHRONICLE.md. Prompt and
// chronicle files that already exist are left untouched. Returns the list of
// changed paths.
func ScaffoldProject(dir string) ([]string, error) {
	var created []string

	// ralph.toml
	tomlPath := filepath.Join(dir, "ralph.toml")
	if _, err := os.Stat(tomlPath); os.IsNotExist(err) {
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

	// PLAN.md
	planPath := filepath.Join(dir, "PLAN.md")
	if _, err := os.Stat(planPath); os.IsNotExist(err) {
		if writeErr := os.WriteFile(planPath, []byte(planPromptTemplate), 0644); writeErr != nil {
			return created, fmt.Errorf("scaffold: write %s: %w", planPath, writeErr)
		}
		created = append(created, planPath)
	}

	// BUILD.md
	buildPath := filepath.Join(dir, "BUILD.md")
	if _, err := os.Stat(buildPath); os.IsNotExist(err) {
		if writeErr := os.WriteFile(buildPath, []byte(buildPromptTemplate), 0644); writeErr != nil {
			return created, fmt.Errorf("scaffold: write %s: %w", buildPath, writeErr)
		}
		created = append(created, buildPath)
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
	if _, err := os.Stat(chroniclePath); os.IsNotExist(err) {
		if writeErr := os.WriteFile(chroniclePath, []byte(implementationPlanTemplate), 0644); writeErr != nil {
			return created, fmt.Errorf("scaffold: write %s: %w", chroniclePath, writeErr)
		}
		created = append(created, chroniclePath)
	}

	return created, nil
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
	{name: "plan", entries: []configScaffoldEntry{
		{key: "prompt_file", line: `prompt_file = "PLAN.md"`},
		{key: "max_iterations", line: `max_iterations = 3`},
	}},
	{name: "build", entries: []configScaffoldEntry{
		{key: "prompt_file", line: `prompt_file = "BUILD.md"`},
		{key: "max_iterations", line: `max_iterations = 0  # 0 = unlimited`},
		{key: "roam", line: `roam = false        # roam freely across the codebase (--roam flag overrides)`},
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

const planPromptTemplate = `Read the specs in ` + "`specs/`" + ` and study the codebase.
Create or update ` + "`CHRONICLE.md`" + ` with:

- A summary of current state (what exists, test coverage)
- Remaining work organized by priority (highest-impact items first)
- Key learnings and architectural decisions

Do NOT write application code — this is a planning phase only.
`

const buildPromptTemplate = `Read the specs in ` + "`specs/`" + ` and the implementation plan.
Pick the highest-priority incomplete item from ` + "`CHRONICLE.md`" + `.

1. Study the codebase to understand what already exists.
2. Implement the feature fully — no placeholders, no stubs.
3. Run tests and ensure they pass.
4. Commit with a descriptive message.
5. Update ` + "`CHRONICLE.md`" + ` to reflect progress.
`

const implementationPlanTemplate = `> [Project]: spec-driven AI coding loop.
> Current state: **Initialization complete.** Specs pending implementation.

## Completed Work

| Phase | Features | Tags |
|-------|----------|------|

## Remaining Work

| Priority | Item | Location | Notes |
|----------|------|----------|-------|

## Key Learnings

-
`
