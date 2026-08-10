package claude

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// ParseStream reads stream-JSON lines from r and sends parsed Events on the
// returned channel. The channel is closed when r reaches EOF or an error.
// This parses Claude CLI output from --output-format=stream-json --verbose.
//
// Uses bufio.Reader.ReadBytes rather than bufio.Scanner because a single
// stream-JSON line can be arbitrarily large — e.g. a Playwright screenshot
// tool result embedding a base64-encoded PNG — and bufio.Scanner has a hard
// line-length cap that aborts the parser with "token too long" when exceeded.
func ParseStream(r io.Reader) <-chan Event {
	ch := make(chan Event, 64)
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
					ch <- ErrorEvent(fmt.Sprintf("stream read error: %v", err))
				}
				return
			}
		}
	}()
	return ch
}

// streamMessage is the top-level JSON object in Claude's stream-json output.
type streamMessage struct {
	Type    string          `json:"type"`
	Subtype string          `json:"subtype"`
	Message *messageContent `json:"message"`
	// Result fields (type=result)
	CostUSD  float64 `json:"cost_usd"`
	Duration float64 `json:"duration_ms"`
	IsError  bool    `json:"is_error"`
	Result   string  `json:"result"`
	// Error fields (type=system, subtype=error)
	Error string `json:"error"`
}

type messageContent struct {
	Content []contentBlock `json:"content"`
}

type contentBlock struct {
	Type  string         `json:"type"`
	Text  string         `json:"text"`
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

// parseLine parses a single line of stream-JSON output into zero or more Events.
func parseLine(line []byte) []Event {
	var msg streamMessage
	if err := json.Unmarshal(line, &msg); err != nil {
		return nil
	}

	switch msg.Type {
	case "assistant":
		return parseAssistantMessage(msg)
	case "result":
		if msg.IsError {
			errText := msg.Result
			if errText == "" {
				errText = msg.Subtype
			}
			if errText == "" {
				errText = "claude run failed"
			}
			ev := ErrorEvent(errText)
			ev.CostUSD = msg.CostUSD
			ev.Duration = msg.Duration / 1000
			ev.Subtype = msg.Subtype
			return []Event{ev}
		}
		return []Event{ResultEvent(msg.CostUSD, msg.Duration/1000, msg.Subtype)}
	case "system":
		if msg.Subtype == "error" {
			return []Event{ErrorEvent(msg.Error)}
		}
	}
	return nil
}

// parseAssistantMessage extracts tool_use and text events from an assistant message.
func parseAssistantMessage(msg streamMessage) []Event {
	if msg.Message == nil {
		return nil
	}

	var events []Event
	for _, block := range msg.Message.Content {
		switch block.Type {
		case "tool_use":
			events = append(events, ToolUseEvent(block.Name, block.Input))
		case "text":
			text := block.Text
			if text != "" {
				events = append(events, TextEvent(text))
			}
		}
	}
	return events
}
