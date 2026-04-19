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
