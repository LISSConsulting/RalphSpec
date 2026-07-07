package codex

import (
	"strings"
	"testing"

	"github.com/LISSConsulting/RalphSpec/internal/claude"
)

func TestParseStream(t *testing.T) {
	input := strings.Join([]string{
		`{"type":"message","text":"thinking"}`,
		`{"type":"tool_call","tool_name":"read_file","tool_input":{"path":"main.go"}}`,
		`{"type":"result","cost_usd":0.12,"duration_ms":1500,"subtype":"success"}`,
	}, "\n")

	var events []claude.Event
	for ev := range ParseStream(strings.NewReader(input)) {
		events = append(events, ev)
	}

	if len(events) != 3 {
		t.Fatalf("got %d events, want 3", len(events))
	}
	if events[0].Type != claude.EventText || events[0].Text != "thinking" {
		t.Fatalf("unexpected first event: %#v", events[0])
	}
	if events[1].Type != claude.EventToolUse || events[1].ToolName != "read_file" {
		t.Fatalf("unexpected tool event: %#v", events[1])
	}
	if events[2].Type != claude.EventResult || events[2].Subtype != "success" {
		t.Fatalf("unexpected result event: %#v", events[2])
	}
}

func TestParseStreamModernCodexEvents(t *testing.T) {
	input := strings.Join([]string{
		`{"type":"session_configured","session_id":"abc","model":"gpt-5.5"}`,
		`{"type":"turn_started"}`,
		`{"type":"item_completed","item":{"type":"reasoning","text":"Inspecting the repo"}}`,
		`{"type":"item_completed","item":{"type":"function_call","name":"shell","arguments":"{\"command\":\"go test ./...\"}"}}`,
		`{"type":"item_completed","item":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Finished"}]}}`,
		`{"type":"turn_completed","usage":{"input_tokens":1,"output_tokens":2}}`,
	}, "\n")

	var events []claude.Event
	for ev := range ParseStream(strings.NewReader(input)) {
		events = append(events, ev)
	}

	if len(events) != 6 {
		t.Fatalf("got %d events, want 6: %#v", len(events), events)
	}
	if events[0].Type != claude.EventText || events[0].Text != "codex: session configured" {
		t.Fatalf("unexpected session event: %#v", events[0])
	}
	if events[1].Type != claude.EventText || events[1].Text != "codex: turn started" {
		t.Fatalf("unexpected turn event: %#v", events[1])
	}
	if events[2].Type != claude.EventText || events[2].Text != "Inspecting the repo" {
		t.Fatalf("unexpected reasoning event: %#v", events[2])
	}
	if events[3].Type != claude.EventToolUse || events[3].ToolName != "shell" {
		t.Fatalf("unexpected tool event: %#v", events[3])
	}
	if events[3].ToolInput["command"] != "go test ./..." {
		t.Fatalf("unexpected tool input: %#v", events[3].ToolInput)
	}
	if events[4].Type != claude.EventText || events[4].Text != "Finished" {
		t.Fatalf("unexpected message event: %#v", events[4])
	}
	if events[5].Type != claude.EventResult || events[5].Subtype != "success" {
		t.Fatalf("unexpected result event: %#v", events[5])
	}
}

func TestParseStreamCurrentCodexItemEvents(t *testing.T) {
	input := strings.Join([]string{
		`{"type":"item.started","item":{"id":"item_1","type":"command_execution","command":"pwsh -Command pwd","aggregated_output":"","exit_code":null,"status":"in_progress"}}`,
		`{"type":"item.completed","item":{"id":"item_1","type":"command_execution","command":"pwsh -Command pwd","aggregated_output":"C:\\repo","exit_code":0,"status":"completed"}}`,
		`{"type":"item.started","item":{"id":"item_2","type":"file_change","changes":[{"path":"C:\\repo\\hello.txt","kind":"add"}],"status":"in_progress"}}`,
		`{"type":"item.completed","item":{"id":"item_2","type":"file_change","changes":[{"path":"C:\\repo\\hello.txt","kind":"add"}],"status":"completed"}}`,
		`{"type":"item.completed","item":{"id":"item_3","type":"agent_message","text":"done"}}`,
		`{"type":"turn.completed","usage":{"input_tokens":1,"output_tokens":2}}`,
	}, "\n")

	var events []claude.Event
	for ev := range ParseStream(strings.NewReader(input)) {
		events = append(events, ev)
	}

	if len(events) != 4 {
		t.Fatalf("got %d events, want 4: %#v", len(events), events)
	}
	if events[0].Type != claude.EventToolUse || events[0].ToolName != "PowerShell" {
		t.Fatalf("unexpected command event: %#v", events[0])
	}
	if events[0].ToolInput["command"] != "pwd" {
		t.Fatalf("unexpected command input: %#v", events[0].ToolInput)
	}
	if events[1].Type != claude.EventToolUse || events[1].ToolName != "Write" {
		t.Fatalf("unexpected file-change event: %#v", events[1])
	}
	if events[1].ToolInput["path"] != `add C:\repo\hello.txt` {
		t.Fatalf("unexpected file-change input: %#v", events[1].ToolInput)
	}
	if events[2].Type != claude.EventText || events[2].Text != "done" {
		t.Fatalf("unexpected text event: %#v", events[2])
	}
	if events[3].Type != claude.EventResult || events[3].Subtype != "success" {
		t.Fatalf("unexpected result event: %#v", events[3])
	}
}

func TestDisplayCommandStripsPowerShellLauncher(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "pwsh shorthand",
			in:   `pwsh -Command pwd`,
			want: `pwd`,
		},
		{
			name: "quoted powershell path",
			in:   `"C:\Program Files\PowerShell\7\pwsh.exe" -Command "Get-Content -Raw -LiteralPath .\hello.txt"`,
			want: `Get-Content -Raw -LiteralPath .\hello.txt`,
		},
		{
			name: "non powershell command",
			in:   `node -e "console.log('hi')"`,
			want: `node -e "console.log('hi')"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := displayCommand(tt.in); got != tt.want {
				t.Fatalf("displayCommand() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCommandToolNameInfersLauncher(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "pwsh shorthand",
			in:   `pwsh -Command pwd`,
			want: "PowerShell",
		},
		{
			name: "quoted powershell path",
			in:   `"C:\Program Files\PowerShell\7\pwsh.exe" -Command "Get-ChildItem"`,
			want: "PowerShell",
		},
		{
			name: "bash launcher",
			in:   `bash -lc "go test ./..."`,
			want: "Bash",
		},
		{
			name: "absolute sh launcher",
			in:   `/bin/sh -c "go test ./..."`,
			want: "Bash",
		},
		{
			name: "cmd launcher",
			in:   `cmd.exe /C dir`,
			want: "Command Prompt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := commandToolName(tt.in); got != tt.want {
				t.Fatalf("commandToolName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseStreamCodexMCPToolCall(t *testing.T) {
	input := `{"type":"item.started","item":{"id":"item_1","type":"mcp_tool_call","server":"filesystem","tool":"read_file","arguments":"{\"path\":\"main.go\"}","status":"in_progress"}}`

	var events []claude.Event
	for ev := range ParseStream(strings.NewReader(input)) {
		events = append(events, ev)
	}

	if len(events) != 1 {
		t.Fatalf("got %d events, want 1: %#v", len(events), events)
	}
	if events[0].Type != claude.EventToolUse || events[0].ToolName != "filesystem:read_file" {
		t.Fatalf("unexpected MCP event: %#v", events[0])
	}
	if events[0].ToolInput["path"] != "main.go" {
		t.Fatalf("unexpected MCP input: %#v", events[0].ToolInput)
	}
}
