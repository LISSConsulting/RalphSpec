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
	Text       string         `json:"text"`
	Delta      string         `json:"delta"`
	ToolName   string         `json:"tool_name"`
	ToolInput  map[string]any `json:"tool_input"`
	Input      map[string]any `json:"input"`
	Result     string         `json:"result"`
	Error      string         `json:"error"`
	IsError    bool           `json:"is_error"`
	CostUSD    float64        `json:"cost_usd"`
	DurationMS float64        `json:"duration_ms"`
	DurationS  float64        `json:"duration_seconds"`
	Subtype    string         `json:"subtype"`
	Status     string         `json:"status"`
}

func parseLine(line []byte) []claude.Event {
	var msg streamMessage
	if err := json.Unmarshal(line, &msg); err != nil {
		return nil
	}

	if text := messageText(msg); text != "" {
		return []claude.Event{claude.TextEvent(text)}
	}

	toolName, toolInput := messageTool(msg)
	if toolName != "" {
		return []claude.Event{claude.ToolUseEvent(toolName, toolInput)}
	}

	if msg.IsError || msg.Error != "" || msg.Type == "error" {
		errText := msg.Error
		if errText == "" {
			errText = msg.Result
		}
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
	for _, s := range []string{msg.Text, msg.Delta, textFromContent(msg.Content)} {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
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
	return name, input
}

func isResultMessage(msg streamMessage) bool {
	return msg.Type == "result" || msg.Type == "response.completed" || msg.Status == "completed" || msg.Subtype == "success"
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
