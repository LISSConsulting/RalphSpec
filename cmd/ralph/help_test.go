package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func executeHelpCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()
	t.Setenv("ANTHROPIC_API_KEY", "")
	root := rootCmd()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetErr(&output)
	root.SetArgs(args)
	err := root.Execute()
	return output.String(), err
}

func TestBuildHelpDocumentsLudicrousContract(t *testing.T) {
	output, err := executeHelpCommand(t, "build", "--help")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"Two consecutive structured agent-success outcomes",
		"No commit during the confirming iteration",
		"clean worktree",
		"ralph tests detect",
		"cannot be combined with --roam",
		"ralph build --ludicrous --max 25",
	} {
		if !strings.Contains(output, required) {
			t.Errorf("build help missing %q\n%s", required, output)
		}
	}
}

func TestLoopRunExposesLudicrousMode(t *testing.T) {
	output, err := executeHelpCommand(t, "loop", "run", "--help")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "--ludicrous") || !strings.Contains(output, "unbounded unless --max is positive") {
		t.Fatalf("loop run help does not expose ludicrous behavior:\n%s", output)
	}
}

func TestLudicrousHelpTopic(t *testing.T) {
	output, err := executeHelpCommand(t, "help", "ludicrous")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "Verified completion requires:") || !strings.Contains(output, "Persistent configuration:") {
		t.Fatalf("ludicrous help topic is incomplete:\n%s", output)
	}
}

func TestJSONHelpIsMachineReadable(t *testing.T) {
	output, err := executeHelpCommand(t, "help", "build", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var document commandHelpDocument
	if err := json.Unmarshal([]byte(output), &document); err != nil {
		t.Fatalf("decode JSON help: %v\n%s", err, output)
	}
	if document.SchemaVersion != "1" || document.Path != "ralph build" {
		t.Fatalf("document = %#v", document)
	}
	var found bool
	for _, flag := range document.Flags {
		if flag.Name == "ludicrous" {
			found = true
			if flag.Type != "bool" || flag.Default != false {
				t.Fatalf("ludicrous flag = %#v", flag)
			}
		}
	}
	if !found {
		t.Fatal("JSON help omitted ludicrous flag")
	}
}

func TestRootJSONHelpContainsCommandTree(t *testing.T) {
	output, err := executeHelpCommand(t, "help", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var document commandHelpDocument
	if err := json.Unmarshal([]byte(output), &document); err != nil {
		t.Fatal(err)
	}
	commands := make(map[string]bool)
	for _, command := range document.Commands {
		commands[command.Name] = true
	}
	for _, required := range []string{"build", "config", "ludicrous", "tests"} {
		if !commands[required] {
			t.Errorf("root JSON help omitted %q", required)
		}
	}
}

func TestConfigSchemaJSON(t *testing.T) {
	output, err := executeHelpCommand(t, "config", "schema", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal([]byte(output), &schema); err != nil {
		t.Fatalf("decode config schema: %v\n%s", err, output)
	}
	if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" || schema["additionalProperties"] != false {
		t.Fatalf("invalid schema root: %#v", schema)
	}
	properties := schema["properties"].(map[string]any)
	build := properties["build"].(map[string]any)["properties"].(map[string]any)
	ludicrous := build["ludicrous"].(map[string]any)
	if ludicrous["type"] != "boolean" || ludicrous["default"] != false || !strings.Contains(ludicrous["description"].(string), "evidence-gated") {
		t.Fatalf("build.ludicrous schema = %#v", ludicrous)
	}
	agent := properties["agent"].(map[string]any)["properties"].(map[string]any)
	agentType := agent["type"].(map[string]any)
	values := agentType["enum"].([]any)
	if len(values) != 3 || values[0] != "" || values[1] != "claude" || values[2] != "codex" {
		t.Fatalf("agent.type enum = %#v", values)
	}
	claudeProperties := properties["claude"].(map[string]any)["properties"].(map[string]any)
	fallbacks := claudeProperties["fallback_providers"].(map[string]any)
	if fallbacks["type"] != "array" || fallbacks["uniqueItems"] != true {
		t.Fatalf("claude.fallback_providers schema = %#v", fallbacks)
	}
}
