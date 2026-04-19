package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScaffoldProject(t *testing.T) {
	t.Run("creates all files in empty directory", func(t *testing.T) {
		dir := t.TempDir()

		created, err := ScaffoldProject(dir)
		if err != nil {
			t.Fatal(err)
		}

		expected := []string{
			filepath.Join(dir, "ralph.toml"),
			filepath.Join(dir, "PLAN.md"),
			filepath.Join(dir, "BUILD.md"),
			filepath.Join(dir, "specs"),
			filepath.Join(dir, ".gitignore"),
			filepath.Join(dir, "CHRONICLE.md"),
		}

		if len(created) != len(expected) {
			t.Fatalf("created %d files, want %d: %v", len(created), len(expected), created)
		}
		for i, want := range expected {
			if created[i] != want {
				t.Errorf("created[%d] = %q, want %q", i, created[i], want)
			}
		}

		// Verify files exist and are non-empty
		for _, path := range expected[:3] {
			info, err := os.Stat(path)
			if err != nil {
				t.Errorf("expected file %s to exist: %v", path, err)
				continue
			}
			if info.Size() == 0 {
				t.Errorf("expected file %s to be non-empty", path)
			}
		}

		// Verify specs/ is a directory
		info, err := os.Stat(filepath.Join(dir, "specs"))
		if err != nil {
			t.Fatalf("specs dir: %v", err)
		}
		if !info.IsDir() {
			t.Error("specs should be a directory")
		}

		// Verify .gitignore contains the regent state entry
		content, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
		if err != nil {
			t.Fatalf(".gitignore: %v", err)
		}
		if !strings.Contains(string(content), ".ralph/regent-state.json") {
			t.Error(".gitignore should contain .ralph/regent-state.json")
		}
	})

	t.Run("skips existing files", func(t *testing.T) {
		dir := t.TempDir()

		// Pre-create ralph.toml and BUILD.md
		if err := os.WriteFile(filepath.Join(dir, "ralph.toml"), []byte("existing"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "BUILD.md"), []byte("custom build prompt"), 0644); err != nil {
			t.Fatal(err)
		}

		created, err := ScaffoldProject(dir)
		if err != nil {
			t.Fatal(err)
		}

		// Should only create the missing files (PLAN.md, specs/, .gitignore, CHRONICLE.md)
		expected := []string{
			filepath.Join(dir, "PLAN.md"),
			filepath.Join(dir, "specs"),
			filepath.Join(dir, ".gitignore"),
			filepath.Join(dir, "CHRONICLE.md"),
		}
		if len(created) != len(expected) {
			t.Fatalf("created %d files, want %d: %v", len(created), len(expected), created)
		}
		for i, want := range expected {
			if created[i] != want {
				t.Errorf("created[%d] = %q, want %q", i, created[i], want)
			}
		}

		// Verify pre-existing file was not overwritten
		content, err := os.ReadFile(filepath.Join(dir, "BUILD.md"))
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != "custom build prompt" {
			t.Error("BUILD.md was overwritten")
		}
	})

	t.Run("all files exist returns empty list", func(t *testing.T) {
		dir := t.TempDir()

		// Create all files including .gitignore with the required entry
		if err := os.WriteFile(filepath.Join(dir, "ralph.toml"), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "PLAN.md"), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "BUILD.md"), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(dir, "specs"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(".ralph/regent-state.json\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "CHRONICLE.md"), []byte("existing plan"), 0644); err != nil {
			t.Fatal(err)
		}

		created, err := ScaffoldProject(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(created) != 0 {
			t.Errorf("expected empty list, got %v", created)
		}
	})

	t.Run("appends entry to existing gitignore without entry", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := ScaffoldProject(dir); err != nil {
			t.Fatal(err)
		}
		// Remove the entry from .gitignore to simulate an existing file without it
		if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("node_modules/\n"), 0644); err != nil {
			t.Fatal(err)
		}

		created, err := ScaffoldProject(dir)
		if err != nil {
			t.Fatal(err)
		}
		// Only .gitignore should be in created (all other files exist)
		if len(created) != 1 || created[0] != filepath.Join(dir, ".gitignore") {
			t.Errorf("expected only .gitignore in created, got %v", created)
		}
		content, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(content), "node_modules/") {
			t.Error("existing content should be preserved")
		}
		if !strings.Contains(string(content), ".ralph/regent-state.json") {
			t.Error("entry should be appended")
		}
	})

	t.Run("appends entry to gitignore that has no trailing newline", func(t *testing.T) {
		dir := t.TempDir()
		// Pre-create all files so only .gitignore append logic runs.
		for name, content := range map[string]string{
			"ralph.toml":   "x",
			"PLAN.md":      "x",
			"BUILD.md":     "x",
			"CHRONICLE.md": "x",
		} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.MkdirAll(filepath.Join(dir, "specs"), 0755); err != nil {
			t.Fatal(err)
		}
		// Write .gitignore WITHOUT a trailing newline to trigger the newline-insertion branch.
		if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("node_modules/"), 0644); err != nil {
			t.Fatal(err)
		}

		created, err := ScaffoldProject(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(created) != 1 || created[0] != filepath.Join(dir, ".gitignore") {
			t.Errorf("expected only .gitignore in created, got %v", created)
		}
		content, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
		if err != nil {
			t.Fatal(err)
		}
		// The existing content and the new entry should each be on their own lines.
		if !strings.Contains(string(content), "node_modules/\n") {
			t.Errorf("existing content should end with newline; got %q", string(content))
		}
		if !strings.Contains(string(content), ".ralph/regent-state.json") {
			t.Error("entry should be appended")
		}
	})

	t.Run("skips gitignore when entry already present", func(t *testing.T) {
		dir := t.TempDir()
		// Pre-create all files including .gitignore with the entry already present
		if err := os.WriteFile(filepath.Join(dir, "ralph.toml"), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "PLAN.md"), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "BUILD.md"), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(dir, "specs"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("# existing\n.ralph/regent-state.json\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "CHRONICLE.md"), []byte("existing plan"), 0644); err != nil {
			t.Fatal(err)
		}

		created, err := ScaffoldProject(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(created) != 0 {
			t.Errorf("expected empty list when entry already present, got %v", created)
		}
	})

	t.Run("gitignore is a directory returns error", func(t *testing.T) {
		dir := t.TempDir()
		// Pre-create all files that scaffold writes before .gitignore
		for name, content := range map[string]string{
			"ralph.toml": "x",
			"PLAN.md":    "x",
			"BUILD.md":   "x",
		} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.MkdirAll(filepath.Join(dir, "specs"), 0755); err != nil {
			t.Fatal(err)
		}
		// Create .gitignore as a directory — os.ReadFile returns a non-IsNotExist error
		if err := os.MkdirAll(filepath.Join(dir, ".gitignore"), 0755); err != nil {
			t.Fatal(err)
		}

		_, err := ScaffoldProject(dir)
		if err == nil {
			t.Fatal("expected error when .gitignore is a directory")
		}
		if !strings.Contains(err.Error(), ".gitignore") {
			t.Errorf("error should mention .gitignore, got: %v", err)
		}
	})

	t.Run("plan prompt template contains key instructions", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := ScaffoldProject(dir); err != nil {
			t.Fatal(err)
		}

		content, err := os.ReadFile(filepath.Join(dir, "PLAN.md"))
		if err != nil {
			t.Fatal(err)
		}

		for _, want := range []string{"specs/", "CHRONICLE.md", "planning phase"} {
			if !strings.Contains(string(content), want) {
				t.Errorf("plan prompt should contain %q", want)
			}
		}
	})

	t.Run("build prompt template contains key instructions", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := ScaffoldProject(dir); err != nil {
			t.Fatal(err)
		}

		content, err := os.ReadFile(filepath.Join(dir, "BUILD.md"))
		if err != nil {
			t.Fatal(err)
		}

		for _, want := range []string{"specs/", "CHRONICLE.md", "Implement"} {
			if !strings.Contains(string(content), want) {
				t.Errorf("build prompt should contain %q", want)
			}
		}
	})

	t.Run("implementation plan template contains required sections", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := ScaffoldProject(dir); err != nil {
			t.Fatal(err)
		}

		content, err := os.ReadFile(filepath.Join(dir, "CHRONICLE.md"))
		if err != nil {
			t.Fatal(err)
		}

		for _, want := range []string{"## Completed Work", "## Remaining Work", "## Key Learnings"} {
			if !strings.Contains(string(content), want) {
				t.Errorf("CHRONICLE.md should contain %q", want)
			}
		}
	})

	t.Run("existing CHRONICLE.md is not overwritten", func(t *testing.T) {
		dir := t.TempDir()
		const existingContent = "# My existing chronicle\n\nDo not overwrite me.\n"
		if err := os.WriteFile(filepath.Join(dir, "CHRONICLE.md"), []byte(existingContent), 0644); err != nil {
			t.Fatal(err)
		}

		created, err := ScaffoldProject(dir)
		if err != nil {
			t.Fatal(err)
		}

		// CHRONICLE.md should NOT be in created list
		chroniclePath := filepath.Join(dir, "CHRONICLE.md")
		for _, p := range created {
			if p == chroniclePath {
				t.Error("CHRONICLE.md should not appear in created when it already exists")
			}
		}

		// Content should be unchanged
		content, err := os.ReadFile(chroniclePath)
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != existingContent {
			t.Error("existing CHRONICLE.md was overwritten")
		}
	})

	t.Run("ralph.toml created by scaffold is loadable", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := ScaffoldProject(dir); err != nil {
			t.Fatal(err)
		}

		cfg, err := Load(filepath.Join(dir, "ralph.toml"))
		if err != nil {
			t.Fatalf("scaffold ralph.toml is not valid: %v", err)
		}
		if cfg.Claude.Model != "sonnet" {
			t.Errorf("default model: got %q, want %q", cfg.Claude.Model, "sonnet")
		}
		if cfg.Agent.Type != AgentClaude {
			t.Errorf("default agent.type: got %q, want %q", cfg.Agent.Type, AgentClaude)
		}
		if cfg.Codex.Model != "" {
			t.Errorf("default codex.model: got %q, want empty", cfg.Codex.Model)
		}
	})
}
