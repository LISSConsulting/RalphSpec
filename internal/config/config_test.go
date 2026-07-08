package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaults(t *testing.T) {
	cfg := Defaults()

	tests := []struct {
		name string
		got  any
		want any
	}{
		{"agent.type", cfg.Agent.Type, AgentClaude},
		{"claude.model", cfg.Claude.Model, "sonnet"},
		{"codex.model", cfg.Codex.Model, ""},
		{"claude.max_turns", cfg.Claude.MaxTurns, 0},
		{"claude.danger_skip_permissions", cfg.Claude.DangerSkipPermissions, true},
		{"build.prompt_file", cfg.Build.PromptFile, "BUILD.md"},
		{"build.max_iterations", cfg.Build.MaxIterations, 0},
		{"roam.enabled", cfg.Roam.Enabled, false},
		{"roam.prompt_file", cfg.Roam.PromptFile, "ROAM.md"},
		{"roam.max_iterations", cfg.Roam.MaxIterations, 0},
		{"git.auto_pull_rebase", cfg.Git.AutoPullRebase, true},
		{"git.auto_push", cfg.Git.AutoPush, true},
		{"regent.enabled", cfg.Regent.Enabled, true},
		{"regent.max_retries", cfg.Regent.MaxRetries, 3},
		{"regent.retry_backoff_seconds", cfg.Regent.RetryBackoffSeconds, 30},
		{"regent.hang_timeout_seconds", cfg.Regent.HangTimeoutSeconds, 300},
		{"regent.rollback_on_test_failure", cfg.Regent.RollbackOnTestFailure, false},
		{"regent.test_command", cfg.Regent.TestCommand, ""},
		{"tui.accent_color", cfg.TUI.AccentColor, DefaultAccentColor},
		{"tui.log_retention", cfg.TUI.LogRetention, 20},
		{"notifications.url", cfg.Notifications.URL, ""},
		{"notifications.on_complete", cfg.Notifications.OnComplete, true},
		{"notifications.on_error", cfg.Notifications.OnError, true},
		{"notifications.on_stop", cfg.Notifications.OnStop, true},
		{"worktree.enabled", cfg.Worktree.Enabled, false},
		{"worktree.max_parallel", cfg.Worktree.MaxParallel, 5},
		{"worktree.auto_merge", cfg.Worktree.AutoMerge, false},
		{"worktree.merge_target", cfg.Worktree.MergeTarget, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %v, want %v", tt.got, tt.want)
			}
		})
	}
}

func TestLoad(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		dir := t.TempDir()
		content := `
[project]
name = "TestProject"

[agent]
type = "codex"

[claude]
model = "opus"
max_turns = 25
danger_skip_permissions = false

[build]
prompt_file = "MY_BUILD.md"
max_iterations = 10

[roam]
enabled = true
prompt_file = "MY_ROAM.md"
max_iterations = 7
focus = "UI"

[git]
auto_pull_rebase = false
auto_push = false

[regent]
enabled = false
rollback_on_test_failure = true
test_command = "go test ./..."
max_retries = 5
retry_backoff_seconds = 60
hang_timeout_seconds = 600

[tui]
accent_color = "#FF0000"
log_retention = 10

[codex]
model = "gpt-5-codex"
`
		path := filepath.Join(dir, "ralph.toml")
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		cfg, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}

		tests := []struct {
			name string
			got  any
			want any
		}{
			{"project.name", cfg.Project.Name, "TestProject"},
			{"agent.type", cfg.Agent.Type, AgentCodex},
			{"claude.model", cfg.Claude.Model, "opus"},
			{"codex.model", cfg.Codex.Model, "gpt-5-codex"},
			{"claude.max_turns", cfg.Claude.MaxTurns, 25},
			{"claude.danger_skip_permissions", cfg.Claude.DangerSkipPermissions, false},
			{"build.prompt_file", cfg.Build.PromptFile, "MY_BUILD.md"},
			{"build.max_iterations", cfg.Build.MaxIterations, 10},
			{"roam.enabled", cfg.Roam.Enabled, true},
			{"roam.prompt_file", cfg.Roam.PromptFile, "MY_ROAM.md"},
			{"roam.max_iterations", cfg.Roam.MaxIterations, 7},
			{"roam.focus", cfg.Roam.Focus, "UI"},
			{"git.auto_pull_rebase", cfg.Git.AutoPullRebase, false},
			{"git.auto_push", cfg.Git.AutoPush, false},
			{"regent.enabled", cfg.Regent.Enabled, false},
			{"regent.rollback_on_test_failure", cfg.Regent.RollbackOnTestFailure, true},
			{"regent.test_command", cfg.Regent.TestCommand, "go test ./..."},
			{"regent.max_retries", cfg.Regent.MaxRetries, 5},
			{"regent.retry_backoff_seconds", cfg.Regent.RetryBackoffSeconds, 60},
			{"regent.hang_timeout_seconds", cfg.Regent.HangTimeoutSeconds, 600},
			{"tui.accent_color", cfg.TUI.AccentColor, "#FF0000"},
			{"tui.log_retention", cfg.TUI.LogRetention, 10},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if tt.got != tt.want {
					t.Errorf("got %v, want %v", tt.got, tt.want)
				}
			})
		}
	})

	t.Run("partial config uses defaults", func(t *testing.T) {
		dir := t.TempDir()
		content := `
[project]
name = "Partial"
`
		path := filepath.Join(dir, "ralph.toml")
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		cfg, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}

		if cfg.Project.Name != "Partial" {
			t.Errorf("project.name: got %q, want %q", cfg.Project.Name, "Partial")
		}
		if cfg.Claude.Model != "sonnet" {
			t.Errorf("claude.model: got %q, want %q (default)", cfg.Claude.Model, "sonnet")
		}
		if cfg.Regent.MaxRetries != 3 {
			t.Errorf("regent.max_retries: got %d, want %d (default)", cfg.Regent.MaxRetries, 3)
		}
	})

	t.Run("missing file returns error", func(t *testing.T) {
		_, err := Load("/nonexistent/ralph.toml")
		if err == nil {
			t.Error("expected error for missing file")
		}
	})

	t.Run("invalid toml returns error", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "ralph.toml")
		if err := os.WriteFile(path, []byte("not valid [[[ toml"), 0644); err != nil {
			t.Fatal(err)
		}

		_, err := Load(path)
		if err == nil {
			t.Error("expected error for invalid TOML")
		}
	})
}

func TestLoadAutoDiscovery(t *testing.T) {
	t.Run("finds ralph.toml in parent directory", func(t *testing.T) {
		root := t.TempDir()
		child := filepath.Join(root, "sub", "dir")
		if err := os.MkdirAll(child, 0755); err != nil {
			t.Fatal(err)
		}

		// Write ralph.toml in root
		content := `[project]
name = "FoundIt"
`
		if err := os.WriteFile(filepath.Join(root, "ralph.toml"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		// Change to child directory to test walk-up
		origDir, _ := os.Getwd()
		t.Cleanup(func() { _ = os.Chdir(origDir) })
		if err := os.Chdir(child); err != nil {
			t.Fatal(err)
		}

		cfg, err := Load("")
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Project.Name != "FoundIt" {
			t.Errorf("project.name: got %q, want %q", cfg.Project.Name, "FoundIt")
		}
	})

	t.Run("returns error when ralph.toml not found anywhere", func(t *testing.T) {
		dir := t.TempDir()
		origDir, _ := os.Getwd()
		t.Cleanup(func() { _ = os.Chdir(origDir) })
		if err := os.Chdir(dir); err != nil {
			t.Fatal(err)
		}

		_, err := Load("")
		if err == nil {
			t.Error("expected error when ralph.toml not found")
		}
	})
}

func TestInitFile(t *testing.T) {
	t.Run("creates ralph.toml", func(t *testing.T) {
		dir := t.TempDir()
		path, err := InitFile(dir)
		if err != nil {
			t.Fatal(err)
		}

		if filepath.Base(path) != "ralph.toml" {
			t.Errorf("expected ralph.toml, got %s", filepath.Base(path))
		}

		// Verify it's valid TOML by loading it
		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("generated file is not valid: %v", err)
		}
		if cfg.Claude.Model != "sonnet" {
			t.Errorf("default model: got %q, want %q", cfg.Claude.Model, "sonnet")
		}
		if cfg.TUI.AccentColor != DefaultAccentColor {
			t.Errorf("default accent color: got %q, want %q", cfg.TUI.AccentColor, DefaultAccentColor)
		}
		if cfg.Claude.MaxTurns != 0 {
			t.Errorf("default max_turns: got %d, want 0", cfg.Claude.MaxTurns)
		}
	})

	t.Run("refuses to overwrite existing", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "ralph.toml")
		if err := os.WriteFile(path, []byte("existing"), 0644); err != nil {
			t.Fatal(err)
		}

		_, err := InitFile(dir)
		if err == nil {
			t.Error("expected error when ralph.toml already exists")
		}
	})

	t.Run("write error returns error", func(t *testing.T) {
		// Pass a non-existent subdirectory — os.WriteFile fails because the
		// parent directory does not exist.
		dir := filepath.Join(t.TempDir(), "nonexistent")
		_, err := InitFile(dir)
		if err == nil {
			t.Error("expected error when directory does not exist")
		}
	})
}

func TestLoadUnknownKeys(t *testing.T) {
	tests := []struct {
		name    string
		toml    string
		wantErr string
	}{
		{
			name: "single unknown key",
			toml: `[project]
name = "Test"
unknown_key = true
`,
			wantErr: "unknown keys",
		},
		{
			name: "unknown key in section",
			toml: `[claude]
model = "sonnet"
typo_field = "oops"
`,
			wantErr: "typo_field",
		},
		{
			name: "unknown section",
			toml: `[notasection]
foo = "bar"
`,
			wantErr: "notasection",
		},
		{
			name: "multiple unknown keys",
			toml: `[project]
name = "Test"
[roam]
promptfile = "missing_underscore.md"
maxiterations = 5
`,
			wantErr: "possible typos?",
		},
		{
			name: "all valid keys accepted",
			toml: `[project]
name = "Test"
[claude]
model = "opus"
max_turns = 10
danger_skip_permissions = false
[build]
prompt_file = "b.md"
max_iterations = 0
[roam]
enabled = false
prompt_file = "r.md"
max_iterations = 0
focus = ""
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
accent_color = "#FF0000"
log_retention = 20
[worktree]
enabled = false
max_parallel = 5
auto_merge = false
merge_target = ""
path_template = ""
worktree_dir = ""
`,
		},
		{
			name: "empty file accepted",
			toml: "",
		},
		{
			name: "comments only accepted",
			toml: `# This is a comment
# Another comment
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "ralph.toml")
			if err := os.WriteFile(path, []byte(tt.toml), 0644); err != nil {
				t.Fatal(err)
			}

			_, err := Load(path)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Errorf("expected error containing %q, got nil", tt.wantErr)
				return
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr string
	}{
		{
			name:   "defaults are valid",
			modify: func(c *Config) {},
		},
		{
			name:    "empty build.prompt_file",
			modify:  func(c *Config) { c.Build.PromptFile = "" },
			wantErr: "build.prompt_file must not be empty",
		},
		{
			name:    "empty roam.prompt_file",
			modify:  func(c *Config) { c.Roam.PromptFile = "" },
			wantErr: "roam.prompt_file must not be empty",
		},
		{
			name:    "negative build.max_iterations",
			modify:  func(c *Config) { c.Build.MaxIterations = -1 },
			wantErr: "build.max_iterations must be >= 0",
		},
		{
			name:    "negative roam.max_iterations",
			modify:  func(c *Config) { c.Roam.MaxIterations = -1 },
			wantErr: "roam.max_iterations must be >= 0",
		},
		{
			name:    "negative claude.max_turns",
			modify:  func(c *Config) { c.Claude.MaxTurns = -1 },
			wantErr: "claude.max_turns must be >= 0",
		},
		{
			name:   "zero claude.max_turns is valid (unlimited)",
			modify: func(c *Config) { c.Claude.MaxTurns = 0 },
		},
		{
			name:   "positive claude.max_turns is valid",
			modify: func(c *Config) { c.Claude.MaxTurns = 50 },
		},
		{
			name: "negative regent.max_retries when enabled",
			modify: func(c *Config) {
				c.Regent.Enabled = true
				c.Regent.MaxRetries = -1
			},
			wantErr: "regent.max_retries must be >= 0",
		},
		{
			name: "negative regent.retry_backoff_seconds when enabled",
			modify: func(c *Config) {
				c.Regent.Enabled = true
				c.Regent.RetryBackoffSeconds = -1
			},
			wantErr: "regent.retry_backoff_seconds must be >= 0",
		},
		{
			name: "negative regent.hang_timeout_seconds when enabled",
			modify: func(c *Config) {
				c.Regent.Enabled = true
				c.Regent.HangTimeoutSeconds = -1
			},
			wantErr: "regent.hang_timeout_seconds must be >= 0",
		},
		{
			name: "regent numeric checks skipped when disabled",
			modify: func(c *Config) {
				c.Regent.Enabled = false
				c.Regent.MaxRetries = -1
				c.Regent.RetryBackoffSeconds = -1
				c.Regent.HangTimeoutSeconds = -1
			},
		},
		{
			name: "rollback_on_test_failure without test_command skipped when disabled",
			modify: func(c *Config) {
				c.Regent.Enabled = false
				c.Regent.RollbackOnTestFailure = true
				c.Regent.TestCommand = ""
			},
		},
		{
			name: "rollback_on_test_failure without test_command",
			modify: func(c *Config) {
				c.Regent.RollbackOnTestFailure = true
				c.Regent.TestCommand = ""
			},
			wantErr: "regent.test_command must be set",
		},
		{
			name: "rollback_on_test_failure with test_command",
			modify: func(c *Config) {
				c.Regent.RollbackOnTestFailure = true
				c.Regent.TestCommand = "go test ./..."
			},
		},
		{
			name: "zero max_iterations is valid (unlimited)",
			modify: func(c *Config) {
				c.Build.MaxIterations = 0
				c.Roam.MaxIterations = 0
			},
		},
		{
			name: "zero hang_timeout is valid (no hang detection)",
			modify: func(c *Config) {
				c.Regent.Enabled = true
				c.Regent.HangTimeoutSeconds = 0
			},
		},
		{
			name:    "negative tui.log_retention",
			modify:  func(c *Config) { c.TUI.LogRetention = -1 },
			wantErr: "tui.log_retention must be >= 0",
		},
		{
			name:   "zero tui.log_retention is valid (unlimited)",
			modify: func(c *Config) { c.TUI.LogRetention = 0 },
		},
		{
			name:   "positive tui.log_retention is valid",
			modify: func(c *Config) { c.TUI.LogRetention = 10 },
		},
		{
			name:    "invalid tui.accent_color",
			modify:  func(c *Config) { c.TUI.AccentColor = "not-a-color" },
			wantErr: "tui.accent_color must be a hex color",
		},
		{
			name:    "tui.accent_color missing hash",
			modify:  func(c *Config) { c.TUI.AccentColor = "FF0000" },
			wantErr: "tui.accent_color must be a hex color",
		},
		{
			name:    "tui.accent_color too short",
			modify:  func(c *Config) { c.TUI.AccentColor = "#FFF" },
			wantErr: "tui.accent_color must be a hex color",
		},
		{
			name:   "valid tui.accent_color",
			modify: func(c *Config) { c.TUI.AccentColor = "#FF0000" },
		},
		{
			name:   "empty tui.accent_color uses default (valid)",
			modify: func(c *Config) { c.TUI.AccentColor = "" },
		},
		{
			name:   "empty notifications.url is valid (disabled)",
			modify: func(c *Config) { c.Notifications.URL = "" },
		},
		{
			name:   "valid https notifications.url",
			modify: func(c *Config) { c.Notifications.URL = "https://ntfy.sh/my-topic" },
		},
		{
			name:   "valid http notifications.url",
			modify: func(c *Config) { c.Notifications.URL = "http://localhost:8080/webhook" },
		},
		{
			name:    "invalid notifications.url not a url",
			modify:  func(c *Config) { c.Notifications.URL = "not-a-url" },
			wantErr: "notifications.url must be a valid http or https URL",
		},
		{
			name:    "invalid notifications.url ftp scheme",
			modify:  func(c *Config) { c.Notifications.URL = "ftp://example.com/webhook" },
			wantErr: "notifications.url must be a valid http or https URL",
		},
		{
			name:    "worktree.max_parallel zero is invalid",
			modify:  func(c *Config) { c.Worktree.MaxParallel = 0 },
			wantErr: "worktree.max_parallel must be >= 1",
		},
		{
			name:    "worktree.max_parallel negative is invalid",
			modify:  func(c *Config) { c.Worktree.MaxParallel = -1 },
			wantErr: "worktree.max_parallel must be >= 1",
		},
		{
			name:   "worktree.max_parallel one is valid",
			modify: func(c *Config) { c.Worktree.MaxParallel = 1 },
		},
		{
			name:   "worktree.merge_target empty is valid",
			modify: func(c *Config) { c.Worktree.MergeTarget = "" },
		},
		{
			name:   "worktree.merge_target set is valid",
			modify: func(c *Config) { c.Worktree.MergeTarget = "main" },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Defaults()
			tt.modify(&cfg)
			err := cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Errorf("expected error containing %q, got nil", tt.wantErr)
				return
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidateMultipleErrors(t *testing.T) {
	cfg := Defaults()
	cfg.Build.PromptFile = ""
	cfg.Roam.PromptFile = ""
	cfg.Roam.MaxIterations = -1

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	msg := err.Error()
	expected := []string{
		"build.prompt_file must not be empty",
		"roam.prompt_file must not be empty",
		"roam.max_iterations must be >= 0",
	}
	for _, want := range expected {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not contain %q", msg, want)
		}
	}
}

func TestRoamConfig(t *testing.T) {
	t.Run("enabled = true parses from TOML", func(t *testing.T) {
		dir := t.TempDir()
		content := "[build]\nprompt_file = \"BUILD.md\"\n[roam]\nenabled = true\nprompt_file = \"ROAM.md\"\n"
		if err := os.WriteFile(filepath.Join(dir, "ralph.toml"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load(filepath.Join(dir, "ralph.toml"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !cfg.Roam.Enabled {
			t.Error("expected Roam.Enabled = true after parsing enabled = true")
		}
	})

	t.Run("unknown key near roam is rejected", func(t *testing.T) {
		dir := t.TempDir()
		content := "[roam]\nprompt_file = \"ROAM.md\"\nenabled = true\nroam_typo = true\n"
		if err := os.WriteFile(filepath.Join(dir, "ralph.toml"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		_, err := Load(filepath.Join(dir, "ralph.toml"))
		if err == nil {
			t.Fatal("expected error for unknown key 'roam_typo'")
		}
		if !strings.Contains(err.Error(), "unknown keys") {
			t.Errorf("error should mention unknown keys, got: %v", err)
		}
	})
}

func TestResolvedWorktreeDir(t *testing.T) {
	t.Run("explicit WorktreeDir is returned as-is", func(t *testing.T) {
		w := WorktreeConfig{WorktreeDir: "/custom/path"}
		got := w.ResolvedWorktreeDir()
		if got != "/custom/path" {
			t.Errorf("got %q, want %q", got, "/custom/path")
		}
	})

	t.Run("empty WorktreeDir defaults to ~/.ralph/worktrees", func(t *testing.T) {
		w := WorktreeConfig{}
		got := w.ResolvedWorktreeDir()
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skip("cannot determine home dir")
		}
		want := filepath.Join(home, ".ralph", "worktrees")
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}
