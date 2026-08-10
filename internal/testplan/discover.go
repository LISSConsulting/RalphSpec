// Package testplan discovers and executes deterministic repository test gates.
package testplan

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

type Confidence string

const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
	ConfidenceLow    Confidence = "low"
)

type Step struct {
	Command    string     `json:"command"`
	Dir        string     `json:"dir"`
	Source     string     `json:"source"`
	Confidence Confidence `json:"confidence"`
	Aggregate  bool       `json:"aggregate"`
}

type Plan struct {
	Explicit    bool      `json:"explicit"`
	DetectedAt  time.Time `json:"detected_at"`
	Steps       []Step    `json:"steps"`
	Diagnostics []string  `json:"diagnostics,omitempty"`
}

type packageJSON struct {
	Scripts        map[string]string `json:"scripts"`
	PackageManager string            `json:"packageManager"`
	Workspaces     json.RawMessage   `json:"workspaces"`
}

type cargoManifest struct {
	Workspace *map[string]any `toml:"workspace"`
	Package   *struct {
		Name string `toml:"name"`
	} `toml:"package"`
}

var ignoredDirectories = map[string]bool{
	".git": true, ".ralph": true, ".codegraph": true, ".idea": true, ".vscode": true,
	"node_modules": true, "vendor": true, "dist": true, "build": true, "target": true,
	"coverage": true, ".next": true, ".venv": true, "venv": true, "__pycache__": true,
	".pytest_cache": true, ".tox": true, ".nox": true,
}

// Discover returns a deterministic run-start test plan. An explicit command is
// authoritative and bypasses metadata traversal.
func Discover(root, explicitCommand string) (Plan, error) {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return Plan{}, fmt.Errorf("test plan root: %w", err)
	}
	plan := Plan{DetectedAt: time.Now()}
	if command := strings.TrimSpace(explicitCommand); command != "" {
		plan.Explicit = true
		plan.Steps = []Step{{Command: command, Dir: absoluteRoot, Source: "regent.test_command", Confidence: ConfidenceHigh, Aggregate: true}}
		return plan, nil
	}

	var paths []string
	err = filepath.WalkDir(absoluteRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			plan.Diagnostics = append(plan.Diagnostics, fmt.Sprintf("%s: %v", relative(absoluteRoot, path), walkErr))
			return nil
		}
		if entry.IsDir() {
			if path != absoluteRoot && ignoredDirectories[strings.ToLower(entry.Name())] {
				return filepath.SkipDir
			}
			return nil
		}
		switch strings.ToLower(entry.Name()) {
		case "justfile", "package.json", "pyproject.toml", "go.mod", "go.work", "cargo.toml":
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return Plan{}, fmt.Errorf("discover test metadata: %w", err)
	}
	sort.Strings(paths)

	var aggregates []Step
	for _, path := range paths {
		if filepath.Dir(path) != absoluteRoot || strings.ToLower(filepath.Base(path)) != "justfile" {
			continue
		}
		step, ok, diagnostic := aggregateStep(absoluteRoot, path)
		if diagnostic != "" {
			plan.Diagnostics = append(plan.Diagnostics, diagnostic)
		}
		if ok {
			plan.Steps = []Step{step}
			return plan, nil
		}
	}

	for _, path := range paths {
		step, ok, diagnostic := aggregateStep(absoluteRoot, path)
		if diagnostic != "" {
			plan.Diagnostics = append(plan.Diagnostics, diagnostic)
		}
		if ok {
			aggregates = append(aggregates, step)
		}
	}
	sort.SliceStable(aggregates, func(i, j int) bool {
		return pathDepth(aggregates[i].Dir) < pathDepth(aggregates[j].Dir)
	})
	var selectedAggregates []Step
	for _, candidate := range aggregates {
		if !coveredBy(candidate.Dir, selectedAggregates) {
			selectedAggregates = append(selectedAggregates, candidate)
		}
	}
	plan.Steps = append(plan.Steps, selectedAggregates...)

	for _, path := range paths {
		dir := filepath.Dir(path)
		if coveredBy(dir, selectedAggregates) {
			continue
		}
		step, ok, diagnostic := projectStep(path)
		if diagnostic != "" {
			plan.Diagnostics = append(plan.Diagnostics, diagnostic)
		}
		if ok {
			plan.Steps = append(plan.Steps, step)
		}
	}
	plan.Steps = deduplicate(plan.Steps)
	if len(plan.Steps) == 0 {
		plan.Diagnostics = append(plan.Diagnostics, "no high-confidence test command found")
	}
	return plan, nil
}

func aggregateStep(root, path string) (Step, bool, string) {
	dir := filepath.Dir(path)
	switch strings.ToLower(filepath.Base(path)) {
	case "justfile":
		if dir != root {
			return Step{}, false, ""
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return Step{}, false, fmt.Sprintf("%s: %v", path, err)
		}
		recipes := parseJustRecipes(string(data))
		for _, name := range []string{"release-check", "check", "test", "ci"} {
			if recipes[name] {
				return Step{Command: "just " + name, Dir: dir, Source: path + " recipe " + name, Confidence: ConfidenceHigh, Aggregate: true}, true, ""
			}
		}
	case "package.json":
		manifest, err := readPackageJSON(path)
		if err != nil {
			return Step{}, false, fmt.Sprintf("%s: %v", path, err)
		}
		if len(manifest.Workspaces) > 0 && string(manifest.Workspaces) != "null" {
			if command, ok := packageTestCommand(dir, manifest); ok {
				return Step{Command: command, Dir: dir, Source: path + " workspace scripts.test", Confidence: ConfidenceHigh, Aggregate: true}, true, ""
			}
		}
	case "go.work":
		return Step{Command: "go test ./...", Dir: dir, Source: path, Confidence: ConfidenceHigh, Aggregate: true}, true, ""
	case "cargo.toml":
		manifest, err := readCargo(path)
		if err != nil {
			return Step{}, false, fmt.Sprintf("%s: %v", path, err)
		}
		if manifest.Workspace != nil {
			return Step{Command: "cargo test --workspace", Dir: dir, Source: path + " [workspace]", Confidence: ConfidenceHigh, Aggregate: true}, true, ""
		}
	}
	return Step{}, false, ""
}

func projectStep(path string) (Step, bool, string) {
	dir := filepath.Dir(path)
	switch strings.ToLower(filepath.Base(path)) {
	case "package.json":
		manifest, err := readPackageJSON(path)
		if err != nil {
			return Step{}, false, fmt.Sprintf("%s: %v", path, err)
		}
		if command, ok := packageTestCommand(dir, manifest); ok {
			return Step{Command: command, Dir: dir, Source: path + " scripts.test", Confidence: ConfidenceHigh}, true, ""
		}
	case "pyproject.toml":
		data, err := os.ReadFile(path)
		if err != nil {
			return Step{}, false, fmt.Sprintf("%s: %v", path, err)
		}
		var parsed map[string]any
		if err := toml.Unmarshal(data, &parsed); err != nil {
			return Step{}, false, fmt.Sprintf("%s: malformed TOML: %v", path, err)
		}
		if declaresDependency(parsed, "pytest") || hasNestedTable(parsed, "tool", "pytest") {
			return Step{Command: "python -m pytest", Dir: dir, Source: path + " pytest declaration", Confidence: ConfidenceHigh}, true, ""
		}
	case "go.mod":
		return Step{Command: "go test ./...", Dir: dir, Source: path, Confidence: ConfidenceHigh}, true, ""
	case "cargo.toml":
		manifest, err := readCargo(path)
		if err != nil {
			return Step{}, false, fmt.Sprintf("%s: %v", path, err)
		}
		if manifest.Package != nil && manifest.Package.Name != "" {
			return Step{Command: "cargo test", Dir: dir, Source: path + " [package]", Confidence: ConfidenceHigh}, true, ""
		}
	}
	return Step{}, false, ""
}

func readPackageJSON(path string) (packageJSON, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return packageJSON{}, err
	}
	var manifest packageJSON
	if err := json.Unmarshal(data, &manifest); err != nil {
		return packageJSON{}, fmt.Errorf("malformed JSON: %w", err)
	}
	return manifest, nil
}

func readCargo(path string) (cargoManifest, error) {
	var manifest cargoManifest
	if _, err := toml.DecodeFile(path, &manifest); err != nil {
		return cargoManifest{}, fmt.Errorf("malformed TOML: %w", err)
	}
	return manifest, nil
}

func packageTestCommand(dir string, manifest packageJSON) (string, bool) {
	script := strings.TrimSpace(manifest.Scripts["test"])
	if script == "" || strings.Contains(strings.ToLower(script), "no test specified") {
		return "", false
	}
	manager := strings.Split(manifest.PackageManager, "@")[0]
	if manager == "" {
		for _, candidate := range []struct{ file, manager string }{
			{"pnpm-lock.yaml", "pnpm"}, {"yarn.lock", "yarn"}, {"bun.lock", "bun"}, {"bun.lockb", "bun"}, {"package-lock.json", "npm"},
		} {
			if _, err := os.Stat(filepath.Join(dir, candidate.file)); err == nil {
				manager = candidate.manager
				break
			}
		}
	}
	if manager == "" {
		manager = "npm"
	}
	switch manager {
	case "npm", "pnpm", "yarn", "bun":
		return manager + " test", true
	default:
		return "", false
	}
}

func parseJustRecipes(content string) map[string]bool {
	result := make(map[string]bool)
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		if index := strings.IndexByte(trimmed, ':'); index > 0 {
			name := strings.Fields(trimmed[:index])[0]
			result[name] = true
		}
	}
	return result
}

func declaresDependency(values map[string]any, dependency string) bool {
	for key, value := range values {
		lowerKey := strings.ToLower(key)
		if strings.Contains(lowerKey, "dependencies") || lowerKey == "dependency-groups" {
			if dependencyValueContains(value, dependency) {
				return true
			}
		}
		if nested, ok := value.(map[string]any); ok && declaresDependency(nested, dependency) {
			return true
		}
	}
	return false
}

func dependencyValueContains(value any, dependency string) bool {
	switch typed := value.(type) {
	case string:
		name := strings.ToLower(strings.TrimSpace(typed))
		if index := strings.IndexAny(name, " <>=!~["); index >= 0 {
			name = name[:index]
		}
		return name == dependency
	case []map[string]any:
		for _, item := range typed {
			if dependencyValueContains(item, dependency) {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if dependencyValueContains(item, dependency) {
				return true
			}
		}
	case map[string]any:
		for key, item := range typed {
			if strings.EqualFold(key, dependency) || dependencyValueContains(item, dependency) {
				return true
			}
		}
	}
	return false
}

func hasNestedTable(values map[string]any, keys ...string) bool {
	var current any = values
	for _, key := range keys {
		table, ok := current.(map[string]any)
		if !ok {
			return false
		}
		current, ok = table[key]
		if !ok {
			return false
		}
	}
	return true
}

func coveredBy(dir string, aggregates []Step) bool {
	for _, aggregate := range aggregates {
		rel, err := filepath.Rel(aggregate.Dir, dir)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func deduplicate(steps []Step) []Step {
	seen := make(map[string]bool)
	result := make([]Step, 0, len(steps))
	for _, step := range steps {
		key := filepath.Clean(step.Dir) + "\x00" + step.Command
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, step)
	}
	return result
}

func pathDepth(path string) int {
	return strings.Count(filepath.Clean(path), string(filepath.Separator))
}

func relative(root, path string) string {
	if rel, err := filepath.Rel(root, path); err == nil {
		return rel
	}
	return path
}
