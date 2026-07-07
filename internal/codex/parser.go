package codex

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/LISSConsulting/RalphSpec/internal/claude"
)

// ParseStream reads Codex JSONL output and emits shared agent events.
func ParseStream(r io.Reader) <-chan claude.Event {
	ch := make(chan claude.Event, 64)
	go func() {
		defer close(ch)
		br := bufio.NewReaderSize(r, 64*1024)
		for {
			line, err := br.ReadBytes('\n')
			line = bytes.TrimRight(line, "\r\n")
			if len(line) > 0 {
				for _, ev := range parseLine(line) {
					ch <- ev
				}
			}
			if err != nil {
				if !errors.Is(err, io.EOF) {
					ch <- claude.ErrorEvent(fmt.Sprintf("stream read error: %v", err))
				}
				return
			}
		}
	}()
	return ch
}

type streamMessage struct {
	Type       string         `json:"type"`
	Role       string         `json:"role"`
	Name       string         `json:"name"`
	Content    any            `json:"content"`
	Item       *streamItem    `json:"item"`
	Text       string         `json:"text"`
	Delta      string         `json:"delta"`
	ToolName   string         `json:"tool_name"`
	ToolInput  map[string]any `json:"tool_input"`
	Input      map[string]any `json:"input"`
	Arguments  any            `json:"arguments"`
	Result     string         `json:"result"`
	Message    string         `json:"message"`
	Error      string         `json:"error"`
	IsError    bool           `json:"is_error"`
	CostUSD    float64        `json:"cost_usd"`
	DurationMS float64        `json:"duration_ms"`
	DurationS  float64        `json:"duration_seconds"`
	Subtype    string         `json:"subtype"`
	Status     string         `json:"status"`
}

type streamItem struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	Role      string         `json:"role"`
	Name      string         `json:"name"`
	Tool      string         `json:"tool"`
	Server    string         `json:"server"`
	Content   any            `json:"content"`
	Text      string         `json:"text"`
	Delta     string         `json:"delta"`
	ToolName  string         `json:"tool_name"`
	ToolInput map[string]any `json:"tool_input"`
	Input     map[string]any `json:"input"`
	Arguments any            `json:"arguments"`
	Command   string         `json:"command"`
	Changes   []fileChange   `json:"changes"`
	Output    string         `json:"output"`
	Status    string         `json:"status"`
}

type fileChange struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
}

func parseLine(line []byte) []claude.Event {
	var msg streamMessage
	if err := json.Unmarshal(line, &msg); err != nil {
		return nil
	}

	if text := messageText(msg); text != "" {
		return []claude.Event{claude.TextEvent(text)}
	}

	if events := itemEvents(msg); len(events) > 0 {
		return events
	}

	toolName, toolInput := messageTool(msg)
	if toolName != "" {
		return []claude.Event{claude.ToolUseEvent(toolName, toolInput)}
	}

	if msg.IsError || msg.Error != "" || msg.Type == "error" {
		errText := firstNonBlank(msg.Error, msg.Message, msg.Result)
		if errText == "" {
			errText = "codex run failed"
		}
		return []claude.Event{claude.ErrorEvent(errText)}
	}

	if isResultMessage(msg) {
		duration := msg.DurationS
		if duration == 0 && msg.DurationMS > 0 {
			duration = msg.DurationMS / 1000
		}
		subtype := msg.Subtype
		if subtype == "" {
			subtype = msg.Status
		}
		if subtype == "" {
			subtype = "success"
		}
		return []claude.Event{claude.ResultEvent(msg.CostUSD, duration, subtype)}
	}

	return nil
}

func messageText(msg streamMessage) string {
	return firstNonBlank(msg.Text, msg.Delta, textFromContent(msg.Content))
}

func itemEvents(msg streamMessage) []claude.Event {
	if msg.Item == nil {
		switch msg.Type {
		case "session_configured", "thread.started", "turn_started", "turn.started":
			return []claude.Event{claude.TextEvent(displayType(msg.Type))}
		}
		return nil
	}

	item := msg.Item
	if name, input := codexActionTool(msg.Type, item); name != "" {
		return []claude.Event{claude.ToolUseEvent(name, input)}
	}

	name, input := itemTool(item)
	if name != "" && isToolEvent(msg.Type, item.Type) {
		return []claude.Event{claude.ToolUseEvent(name, input)}
	}

	if text := itemText(item); text != "" {
		return []claude.Event{claude.TextEvent(text)}
	}

	if strings.Contains(msg.Type, "started") {
		label := item.Type
		if label == "" {
			label = msg.Type
		}
		return []claude.Event{claude.TextEvent(displayType(label))}
	}

	return nil
}

func messageTool(msg streamMessage) (string, map[string]any) {
	name := msg.ToolName
	if name == "" {
		name = msg.Name
	}
	if name == "" {
		return "", nil
	}
	input := msg.ToolInput
	if len(input) == 0 {
		input = msg.Input
	}
	if len(input) == 0 {
		input = argumentsMap(msg.Arguments)
	}
	return name, input
}

func isResultMessage(msg streamMessage) bool {
	return msg.Type == "result" || msg.Type == "response.completed" || msg.Type == "turn_completed" || msg.Type == "turn.completed" || msg.Status == "completed" || msg.Subtype == "success"
}

func textFromContent(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		var parts []string
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				if text, _ := m["text"].(string); strings.TrimSpace(text) != "" {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}

func itemText(item *streamItem) string {
	if item == nil || isToolLike(item.Type) {
		return ""
	}
	return firstNonBlank(item.Text, item.Delta, textFromContent(item.Content))
}

func itemTool(item *streamItem) (string, map[string]any) {
	name := firstNonBlank(item.ToolName, item.Name, item.Tool)
	if name == "" {
		return "", nil
	}
	input := item.ToolInput
	if len(input) == 0 {
		input = item.Input
	}
	if len(input) == 0 {
		input = argumentsMap(item.Arguments)
	}
	return name, input
}

func codexActionTool(eventType string, item *streamItem) (string, map[string]any) {
	if item == nil || !isActionStart(eventType, item.Status) {
		return "", nil
	}

	switch item.Type {
	case "command_execution":
		if strings.TrimSpace(item.Command) == "" {
			return "", nil
		}
		return "PowerShell", map[string]any{"command": displayCommand(item.Command)}
	case "file_change":
		path := summarizeFileChanges(item.Changes)
		if path == "" {
			return "", nil
		}
		return fileChangeToolName(item.Changes), map[string]any{"path": path}
	case "mcp_tool_call":
		name := firstNonBlank(item.ToolName, item.Name, item.Tool)
		if name == "" {
			name = "MCP"
		}
		if item.Server != "" && name != "MCP" {
			name = item.Server + ":" + name
		}
		input := item.ToolInput
		if len(input) == 0 {
			input = item.Input
		}
		if len(input) == 0 {
			input = argumentsMap(item.Arguments)
		}
		return name, input
	default:
		return "", nil
	}
}

func displayCommand(command string) string {
	command = strings.TrimSpace(command)
	lower := strings.ToLower(command)
	idx := strings.Index(lower, "-command")
	if idx < 0 {
		return command
	}

	launcher := strings.TrimSpace(command[:idx])
	launcherLower := strings.ToLower(launcher)
	if !strings.Contains(launcherLower, "pwsh") && !strings.Contains(launcherLower, "powershell") {
		return command
	}

	inner := strings.TrimSpace(command[idx+len("-command"):])
	if len(inner) >= 2 {
		first, last := inner[0], inner[len(inner)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			inner = inner[1 : len(inner)-1]
		}
	}
	if inner == "" {
		return command
	}
	return inner
}

func isActionStart(eventType, status string) bool {
	return strings.Contains(eventType, "started") || status == "in_progress"
}

func summarizeFileChanges(changes []fileChange) string {
	if len(changes) == 0 {
		return ""
	}
	parts := make([]string, 0, len(changes))
	for _, change := range changes {
		if change.Path == "" {
			continue
		}
		if change.Kind != "" {
			parts = append(parts, fmt.Sprintf("%s %s", change.Kind, change.Path))
		} else {
			parts = append(parts, change.Path)
		}
	}
	return strings.Join(parts, ", ")
}

func fileChangeToolName(changes []fileChange) string {
	if len(changes) == 1 {
		switch changes[0].Kind {
		case "add":
			return "Write"
		case "delete":
			return "Delete"
		}
	}
	return "Edit"
}

func isToolEvent(eventType, itemType string) bool {
	return isToolLike(itemType) && !strings.Contains(itemType, "output") && (strings.Contains(eventType, "completed") || strings.Contains(eventType, "started"))
}

func isToolLike(typ string) bool {
	return typ == "function_call" || strings.Contains(typ, "tool_call")
}

func argumentsMap(v any) map[string]any {
	switch arg := v.(type) {
	case nil:
		return nil
	case map[string]any:
		return arg
	case string:
		arg = strings.TrimSpace(arg)
		if arg == "" {
			return nil
		}
		var parsed map[string]any
		if err := json.Unmarshal([]byte(arg), &parsed); err == nil {
			return parsed
		}
		return map[string]any{"arguments": arg}
	default:
		return map[string]any{"arguments": fmt.Sprintf("%v", arg)}
	}
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func displayType(typ string) string {
	typ = strings.TrimSpace(strings.ReplaceAll(typ, ".", "_"))
	if typ == "" {
		return "codex event"
	}
	return "codex: " + strings.ReplaceAll(typ, "_", " ")
}
