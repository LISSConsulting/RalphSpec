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
