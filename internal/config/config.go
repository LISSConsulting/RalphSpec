// Package config parses ralph.toml project configuration.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

// DefaultAccentColor is the default TUI accent color (indigo).
const DefaultAccentColor = "#7D56F4"

// hexColorRe matches a 6-digit hex color string like "#7D56F4".
var hexColorRe = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// Config is the top-level ralph.toml configuration.
type Config struct {
	Project       ProjectConfig       `toml:"project"`
	Agent         AgentConfig         `toml:"agent"`
	Harness       HarnessConfig       `toml:"harness"`
	Claude        ClaudeConfig        `toml:"claude"`
	Codex         CodexConfig         `toml:"codex"`
	Build         BuildConfig         `toml:"build"`
	Roam          RoamConfig          `toml:"roam"`
	Git           GitConfig           `toml:"git"`
	Regent        RegentConfig        `toml:"regent"`
	TUI           TUIConfig           `toml:"tui"`
	Notifications NotificationsConfig `toml:"notifications"`
	Worktree      WorktreeConfig      `toml:"worktree"`
}

const (
	AgentClaude = "claude"
	AgentCodex  = "codex"
)

// AgentConfig controls which agent Ralph should use by default.
type AgentConfig struct {
	Type       string `toml:"type"`
	StdinSteer bool   `toml:"stdin_steer"` // write TUI steer messages to the running agent's stdin mid-turn (experimental)
}

// HarnessConfig selects the executable invoked for each agent harness.
// Point these at shims (e.g. "claude-kimi", "codex-minimax") to run the
// stock CLIs against custom providers.
type HarnessConfig struct {
	Claude string `toml:"claude"`
	Codex  string `toml:"codex"`
}

// WorktreeConfig controls git worktree support via worktrunk.
type WorktreeConfig struct {
	Enabled      bool   `toml:"enabled"`
	MaxParallel  int    `toml:"max_parallel"`
	AutoMerge    bool   `toml:"auto_merge"`
	MergeTarget  string `toml:"merge_target"`
	PathTemplate string `toml:"path_template"` // deprecated: use worktree_dir
	WorktreeDir  string `toml:"worktree_dir"`  // base directory for worktrees; default ~/.ralph/worktrees
}

// ResolvedWorktreeDir returns the absolute path for worktree storage.
// Defaults to ~/.ralph/worktrees if WorktreeDir is empty.
func (w WorktreeConfig) ResolvedWorktreeDir() string {
	if w.WorktreeDir != "" {
		return w.WorktreeDir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".ralph", "worktrees")
}

// NotificationsConfig controls webhook/ntfy.sh notifications.
type NotificationsConfig struct {
	URL        string `toml:"url"`
	OnComplete bool   `toml:"on_complete"`
	OnError    bool   `toml:"on_error"`
	OnStop     bool   `toml:"on_stop"`
}

// TUIConfig controls the terminal UI appearance.
type TUIConfig struct {
	AccentColor  string `toml:"accent_color"`
	LogRetention int    `toml:"log_retention"` // number of session logs to keep; 0 = unlimited
}

// ProjectConfig identifies the project.
type ProjectConfig struct {
	Name string `toml:"name"`
}

// ClaudeConfig controls the Claude CLI invocation.
type ClaudeConfig struct {
	Model                 string `toml:"model"`
	MaxTurns              int    `toml:"max_turns"`
	DangerSkipPermissions bool   `toml:"danger_skip_permissions"`
}

// CodexConfig controls the Codex CLI invocation.
type CodexConfig struct {
	Model string `toml:"model"`
}

// BuildConfig controls the build loop.
type BuildConfig struct {
	PromptFile    string `toml:"prompt_file"`
	MaxIterations int    `toml:"max_iterations"`
}

// RoamConfig controls codebase-wide roaming mode.
type RoamConfig struct {
	Enabled       bool   `toml:"enabled"` // roam freely across the codebase (--roam flag overrides)
	PromptFile    string `toml:"prompt_file"`
	MaxIterations int    `toml:"max_iterations"`
	Focus         string `toml:"focus"` // constrain roam to a specific topic (--focus flag overrides)
}

// GitConfig controls git operations between iterations.
type GitConfig struct {
	AutoPullRebase bool `toml:"auto_pull_rebase"`
	AutoPush       bool `toml:"auto_push"`
}

// RegentConfig controls the Regent supervisor.
type RegentConfig struct {
	Enabled               bool   `toml:"enabled"`
	RollbackOnTestFailure bool   `toml:"rollback_on_test_failure"`
	TestCommand           string `toml:"test_command"`
	MaxRetries            int    `toml:"max_retries"`
	RetryBackoffSeconds   int    `toml:"retry_backoff_seconds"`
	HangTimeoutSeconds    int    `toml:"hang_timeout_seconds"`
}

// Validate checks the configuration for issues that would cause confusing
// runtime failures. It returns all found issues joined together.
func (c *Config) Validate() error {
	var errs []error

	if c.Agent.Type != "" && c.Agent.Type != AgentClaude && c.Agent.Type != AgentCodex {
		errs = append(errs, fmt.Errorf("agent.type must be one of %s,%s", AgentClaude, AgentCodex))
	}
	if c.Build.PromptFile == "" {
		errs = append(errs, fmt.Errorf("build.prompt_file must not be empty"))
	}
	if c.Roam.PromptFile == "" {
		errs = append(errs, fmt.Errorf("roam.prompt_file must not be empty"))
	}
	if c.Build.MaxIterations < 0 {
		errs = append(errs, fmt.Errorf("build.max_iterations must be >= 0 (0 = unlimited)"))
	}
	if c.Roam.MaxIterations < 0 {
		errs = append(errs, fmt.Errorf("roam.max_iterations must be >= 0 (0 = unlimited)"))
	}

	if c.Claude.MaxTurns < 0 {
		errs = append(errs, fmt.Errorf("claude.max_turns must be >= 0 (0 = unlimited)"))
	}

	if c.Regent.Enabled {
		if c.Regent.MaxRetries < 0 {
			errs = append(errs, fmt.Errorf("regent.max_retries must be >= 0"))
		}
		if c.Regent.RetryBackoffSeconds < 0 {
			errs = append(errs, fmt.Errorf("regent.retry_backoff_seconds must be >= 0"))
		}
		if c.Regent.HangTimeoutSeconds < 0 {
			errs = append(errs, fmt.Errorf("regent.hang_timeout_seconds must be >= 0 (0 = no hang detection)"))
		}
	}

	if c.Regent.Enabled && c.Regent.RollbackOnTestFailure && c.Regent.TestCommand == "" {
		errs = append(errs, fmt.Errorf("regent.test_command must be set when regent.rollback_on_test_failure is true"))
	}

	if c.TUI.AccentColor != "" && !hexColorRe.MatchString(c.TUI.AccentColor) {
		errs = append(errs, fmt.Errorf("tui.accent_color must be a hex color (e.g. \"#7D56F4\")"))
	}
	if c.TUI.LogRetention < 0 {
		errs = append(errs, fmt.Errorf("tui.log_retention must be >= 0 (0 = unlimited)"))
	}

	if c.Notifications.URL != "" {
		u, parseErr := url.ParseRequestURI(c.Notifications.URL)
		if parseErr != nil || (u.Scheme != "http" && u.Scheme != "https") {
			errs = append(errs, fmt.Errorf("notifications.url must be a valid http or https URL"))
		}
	}

	if c.Worktree.MaxParallel < 1 {
		errs = append(errs, fmt.Errorf("worktree.max_parallel must be >= 1"))
	}

	return errors.Join(errs...)
}

// Defaults returns a Config with sensible defaults matching the spec.
func Defaults() Config {
	return Config{
		Project: ProjectConfig{Name: ""},
		Agent:   AgentConfig{Type: AgentClaude},
		Harness: HarnessConfig{Claude: "claude", Codex: "codex"},
		Claude: ClaudeConfig{
			Model:                 "sonnet",
			DangerSkipPermissions: true,
		},
		Codex: CodexConfig{},
		Build: BuildConfig{
			PromptFile:    "BUILD.md",
			MaxIterations: 0,
		},
		Roam: RoamConfig{
			Enabled:       false,
			PromptFile:    "ROAM.md",
			MaxIterations: 0,
		},
		Git: GitConfig{
			AutoPullRebase: true,
			AutoPush:       true,
		},
		Regent: RegentConfig{
			Enabled:               true,
			RollbackOnTestFailure: false,
			TestCommand:           "",
			MaxRetries:            3,
			RetryBackoffSeconds:   30,
			HangTimeoutSeconds:    300,
		},
		TUI: TUIConfig{
			AccentColor:  DefaultAccentColor,
			LogRetention: 20,
		},
		Notifications: NotificationsConfig{
			URL:        "",
			OnComplete: true,
			OnError:    true,
			OnStop:     true,
		},
		Worktree: WorktreeConfig{
			Enabled:     false,
			MaxParallel: 5,
			AutoMerge:   false,
			MergeTarget: "",
		},
	}
}

// Load reads ralph.toml from the given path. If path is empty, it walks up
// from the current working directory looking for ralph.toml. Returns an error
// if the file contains unknown keys (likely typos).
func Load(path string) (*Config, error) {
	if path == "" {
		found, err := findConfig()
		if err != nil {
			return nil, err
		}
		path = found
	}

	cfg := Defaults()
	meta, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		return nil, fmt.Errorf("config: decode %s: %w", path, err)
	}

	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, len(undecoded))
		for i, k := range undecoded {
			keys[i] = k.String()
		}
		return nil, fmt.Errorf("config: unknown keys in %s: %s (possible typos?)", path, joinKeys(keys))
	}

	if cfg.Project.Name == "" {
		cfg.Project.Name = DetectProjectName(filepath.Dir(path))
	}

	return &cfg, nil
}

// joinKeys formats a slice of key names for display.
func joinKeys(keys []string) string {
	return strings.Join(keys, ", ")
}

// findConfig walks up from the current directory looking for ralph.toml.
func findConfig() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("config: get working directory: %w", err)
	}

	for {
		candidate := filepath.Join(dir, "ralph.toml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("config: ralph.toml not found (searched up from %s)", dir)
		}
		dir = parent
	}
}

const defaultConfigTemplate = `# ralph.toml — RalphSpec project configuration
# Place this file in the root of your project.

[project]
name = ""

[agent]
type = "claude"
stdin_steer = false  # experimental: steer messages go straight to the running agent's stdin mid-turn

[harness]
claude = "claude"  # executable for the claude harness, e.g. "claude-kimi"
codex = "codex"    # executable for the codex harness, e.g. "codex-minimax"

[claude]
model = "sonnet"
max_turns = 0  # 0 = unlimited agentic turns per iteration
danger_skip_permissions = true

[codex]
model = ""

[build]
prompt_file = "BUILD.md"
max_iterations = 0  # 0 = unlimited

[roam]
enabled = false     # roam freely across the codebase (--roam flag overrides)
prompt_file = "ROAM.md"
max_iterations = 0  # 0 = unlimited
focus = ""          # constrain roam to a specific topic (--focus flag overrides)

[git]
auto_pull_rebase = true
auto_push = true

[regent]
enabled = true
rollback_on_test_failure = false
test_command = ""
max_retries = 3
retry_backoff_seconds = 30
hang_timeout_seconds = 300

[tui]
accent_color = "#7D56F4"  # hex color for header/accent elements
log_retention = 20        # number of session logs to keep; 0 = unlimited

[notifications]
url = ""           # ntfy.sh topic URL or any HTTP webhook (empty = disabled)
on_complete = true # notify on each iteration complete
on_error = true    # notify on loop error
on_stop = true     # notify when loop finishes or is stopped

[worktree]
enabled = false        # enable git worktree support via worktrunk
max_parallel = 5       # maximum number of concurrent worktree agents
auto_merge = false     # automatically merge on spec-complete + tests pass
merge_target = ""      # branch to merge into (empty = branch worktree was created from)
path_template = ""     # deprecated: use worktree_dir
worktree_dir = ""      # base directory for worktrees (default: ~/.ralph/worktrees)
`

// InitFile writes a default ralph.toml template to the given directory.
func InitFile(dir string) (string, error) {
	path := filepath.Join(dir, "ralph.toml")
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("config: ralph.toml already exists at %s", path)
	}

	if err := writeDefaultConfigFile(path); err != nil {
		return "", err
	}
	return path, nil
}

func writeDefaultConfigFile(path string) error {
	if err := os.WriteFile(path, []byte(defaultConfigTemplate), 0644); err != nil {
		return fmt.Errorf("config: write %s: %w", path, err)
	}
	return nil
}
