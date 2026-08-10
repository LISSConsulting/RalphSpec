package claude

import (
	"fmt"
	"io"
	"strings"
	"testing"
)

func TestParseStream(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		events []struct {
			typ      EventType
			toolName string
			text     string
			costUSD  float64
			errMsg   string
			subtype  string
		}
	}{
		{
			name:  "tool_use event",
			input: `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"write_file","input":{"path":"main.go"}}]}}`,
			events: []struct {
				typ      EventType
				toolName string
				text     string
				costUSD  float64
				errMsg   string
				subtype  string
			}{
				{typ: EventToolUse, toolName: "write_file"},
			},
		},
		{
			name:  "text event",
			input: `{"type":"assistant","message":{"content":[{"type":"text","text":"Hello world"}]}}`,
			events: []struct {
				typ      EventType
				toolName string
				text     string
				costUSD  float64
				errMsg   string
				subtype  string
			}{
				{typ: EventText, text: "Hello world"},
			},
		},
		{
			name:  "result event",
			input: `{"type":"result","cost_usd":0.14,"duration_ms":4200}`,
			events: []struct {
				typ      EventType
				toolName string
				text     string
				costUSD  float64
				errMsg   string
				subtype  string
			}{
				{typ: EventResult, costUSD: 0.14},
			},
		},
		{
			name:  "error event",
			input: `{"type":"system","subtype":"error","error":"something broke"}`,
			events: []struct {
				typ      EventType
				toolName string
				text     string
				costUSD  float64
				errMsg   string
				subtype  string
			}{
				{typ: EventError, errMsg: "something broke"},
			},
		},
		{
			name:  "multiple content blocks",
			input: `{"type":"assistant","message":{"content":[{"type":"text","text":"analyzing"},{"type":"tool_use","name":"bash","input":{"command":"go test"}}]}}`,
			events: []struct {
				typ      EventType
				toolName string
				text     string
				costUSD  float64
				errMsg   string
				subtype  string
			}{
				{typ: EventText, text: "analyzing"},
				{typ: EventToolUse, toolName: "bash"},
			},
		},
		{
			name:  "empty text block ignored",
			input: `{"type":"assistant","message":{"content":[{"type":"text","text":""}]}}`,
			events: []struct {
				typ      EventType
				toolName string
				text     string
				costUSD  float64
				errMsg   string
				subtype  string
			}{},
		},
		{
			name:  "invalid json ignored",
			input: `not valid json`,
			events: []struct {
				typ      EventType
				toolName string
				text     string
				costUSD  float64
				errMsg   string
				subtype  string
			}{},
		},
		{
			name:  "empty lines ignored",
			input: "\n\n",
			events: []struct {
				typ      EventType
				toolName string
				text     string
				costUSD  float64
				errMsg   string
				subtype  string
			}{},
		},
		{
			name:  "system non-error ignored",
			input: `{"type":"system","subtype":"init"}`,
			events: []struct {
				typ      EventType
				toolName string
				text     string
				costUSD  float64
				errMsg   string
				subtype  string
			}{},
		},
		{
			name:  "result event with is_error emits one terminal error",
			input: `{"type":"result","cost_usd":0.08,"duration_ms":3000,"is_error":true,"result":"API rate limit exceeded"}`,
			events: []struct {
				typ      EventType
				toolName string
				text     string
				costUSD  float64
				errMsg   string
				subtype  string
			}{
				{typ: EventError, errMsg: "API rate limit exceeded", costUSD: 0.08},
			},
		},
		{
			name:  "result event with is_error and empty result uses fallback message",
			input: `{"type":"result","cost_usd":0.01,"duration_ms":500,"is_error":true}`,
			events: []struct {
				typ      EventType
				toolName string
				text     string
				costUSD  float64
				errMsg   string
				subtype  string
			}{
				{typ: EventError, errMsg: "claude run failed", costUSD: 0.01},
			},
		},
		{
			name:  "result event with is_error false is normal result with subtype",
			input: `{"type":"result","cost_usd":0.14,"duration_ms":4200,"is_error":false,"subtype":"success"}`,
			events: []struct {
				typ      EventType
				toolName string
				text     string
				costUSD  float64
				errMsg   string
				subtype  string
			}{
				{typ: EventResult, costUSD: 0.14, subtype: "success"},
			},
		},
		{
			name:  "result event with error_max_turns subtype",
			input: `{"type":"result","cost_usd":0.30,"duration_ms":5000,"is_error":true,"subtype":"error_max_turns","result":"Hit maximum turns"}`,
			events: []struct {
				typ      EventType
				toolName string
				text     string
				costUSD  float64
				errMsg   string
				subtype  string
			}{
				{typ: EventError, errMsg: "Hit maximum turns", costUSD: 0.30, subtype: "error_max_turns"},
			},
		},
		{
			name: "multi-line stream",
			input: `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"read_file","input":{"path":"go.mod"}}]}}
{"type":"assistant","message":{"content":[{"type":"tool_use","name":"write_file","input":{"path":"main.go"}}]}}
{"type":"result","cost_usd":0.05,"duration_ms":2000}`,
			events: []struct {
				typ      EventType
				toolName string
				text     string
				costUSD  float64
				errMsg   string
				subtype  string
			}{
				{typ: EventToolUse, toolName: "read_file"},
				{typ: EventToolUse, toolName: "write_file"},
				{typ: EventResult, costUSD: 0.05},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := ParseStream(strings.NewReader(tt.input))

			var got []Event
			for ev := range ch {
				got = append(got, ev)
			}

			if len(got) != len(tt.events) {
				t.Fatalf("got %d events, want %d", len(got), len(tt.events))
			}

			for i, want := range tt.events {
				ev := got[i]
				if ev.Type != want.typ {
					t.Errorf("event[%d].Type = %q, want %q", i, ev.Type, want.typ)
				}
				if want.toolName != "" && ev.ToolName != want.toolName {
					t.Errorf("event[%d].ToolName = %q, want %q", i, ev.ToolName, want.toolName)
				}
				if want.text != "" && ev.Text != want.text {
					t.Errorf("event[%d].Text = %q, want %q", i, ev.Text, want.text)
				}
				if want.costUSD != 0 && ev.CostUSD != want.costUSD {
					t.Errorf("event[%d].CostUSD = %f, want %f", i, ev.CostUSD, want.costUSD)
				}
				if want.errMsg != "" && ev.Error != want.errMsg {
					t.Errorf("event[%d].Error = %q, want %q", i, ev.Error, want.errMsg)
				}
				if want.subtype != "" && ev.Subtype != want.subtype {
					t.Errorf("event[%d].Subtype = %q, want %q", i, ev.Subtype, want.subtype)
				}
			}
		})
	}
}

// errAfterReader returns valid data first, then an error on the next read.
type errAfterReader struct {
	data io.Reader
	err  error
	done bool
}

func (r *errAfterReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, r.err
	}
	n, err := r.data.Read(p)
	if err == io.EOF {
		r.done = true
		if n > 0 {
			return n, nil
		}
		return 0, r.err
	}
	return n, err
}

// TestParseStream_AssistantNoMessage covers the msg.Message == nil branch in
// parseAssistantMessage — an "assistant" line without a "message" field
// results in no events being emitted.
func TestParseStream_AssistantNoMessage(t *testing.T) {
	// Send an assistant event with no "message" field so msg.Message == nil.
	input := `{"type":"assistant"}`
	ch := ParseStream(strings.NewReader(input))
	var got []Event
	for ev := range ch {
		got = append(got, ev)
	}
	if len(got) != 0 {
		t.Errorf("assistant with no message field should emit 0 events, got %d", len(got))
	}
}

// TestParseStream_HugeLine ensures the parser handles stream-JSON lines that
// exceed bufio.Scanner's default cap. This used to fail with "bufio.Scanner:
// token too long" when Playwright screenshot tool results (base64 PNG) arrived
// as a single line of several MB.
func TestParseStream_HugeLine(t *testing.T) {
	// 4 MB of filler text inside a text block — comfortably past bufio.Scanner's
	// 1 MB cap that the parser previously used.
	const size = 4 * 1024 * 1024
	filler := strings.Repeat("A", size)
	input := fmt.Sprintf(`{"type":"assistant","message":{"content":[{"type":"text","text":%q}]}}`+"\n", filler)

	ch := ParseStream(strings.NewReader(input))
	var got []Event
	for ev := range ch {
		got = append(got, ev)
	}

	if len(got) != 1 {
		t.Fatalf("got %d events, want 1", len(got))
	}
	if got[0].Type != EventText {
		t.Fatalf("event[0].Type = %q, want %q", got[0].Type, EventText)
	}
	if len(got[0].Text) != size {
		t.Errorf("event[0].Text length = %d, want %d", len(got[0].Text), size)
	}
}

func TestParseStream_ScannerError(t *testing.T) {
	validLine := `{"type":"result","cost_usd":0.05,"duration_ms":1000}` + "\n"
	r := &errAfterReader{
		data: strings.NewReader(validLine),
		err:  fmt.Errorf("connection reset"),
	}

	ch := ParseStream(r)

	var got []Event
	for ev := range ch {
		got = append(got, ev)
	}

	if len(got) != 2 {
		t.Fatalf("got %d events, want 2 (result + error)", len(got))
	}

	if got[0].Type != EventResult {
		t.Errorf("event[0].Type = %q, want %q", got[0].Type, EventResult)
	}
	if got[1].Type != EventError {
		t.Errorf("event[1].Type = %q, want %q", got[1].Type, EventError)
	}
	if !strings.Contains(got[1].Error, "stream read error") {
		t.Errorf("event[1].Error = %q, want it to contain %q", got[1].Error, "stream read error")
	}
}
