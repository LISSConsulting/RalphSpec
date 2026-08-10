// Package claude provides shared agent events and the Claude CLI adapter.
package claude

import (
	"errors"
	"strings"
	"time"
)

// EventType identifies a normalized agent event.
type EventType string

const (
	EventToolUse EventType = "tool_use"
	EventText    EventType = "text"
	EventResult  EventType = "result"
	EventError   EventType = "error"
)

// OutcomeKind identifies the terminal result of one agent invocation.
type OutcomeKind string

const (
	OutcomeSuccess                OutcomeKind = "success"
	OutcomeQuotaExhausted         OutcomeKind = "quota_exhausted"
	OutcomeAuthenticationRequired OutcomeKind = "authentication_required"
	OutcomeTransientProvider      OutcomeKind = "transient_provider_failure"
	OutcomeContextExhausted       OutcomeKind = "context_exhausted"
	OutcomeCancelled              OutcomeKind = "cancelled"
	OutcomeAgentFailure           OutcomeKind = "agent_failure"
)

// TerminalOutcome is the final normalized result of one agent invocation.
type TerminalOutcome struct {
	Kind        OutcomeKind
	Message     string
	Provider    string
	QuotaWindow string
	RetryAt     time.Time
}

// Successful reports whether this outcome permits iteration completion.
func (o TerminalOutcome) Successful() bool {
	return o.Kind == OutcomeSuccess
}

// OutcomeError carries a terminal outcome through the loop and Regent.
type OutcomeError struct {
	Outcome TerminalOutcome
}

func (e *OutcomeError) Error() string {
	if e.Outcome.Message != "" {
		return e.Outcome.Message
	}
	return string(e.Outcome.Kind)
}

// AsOutcome extracts a normalized terminal outcome from an error.
func AsOutcome(err error) (TerminalOutcome, bool) {
	var outcomeErr *OutcomeError
	if !errors.As(err, &outcomeErr) {
		return TerminalOutcome{}, false
	}
	return outcomeErr.Outcome, true
}

// ClassifyOutcome maps structured subtype/error text to a terminal category.
// Adapters should prefer structured provider status and use this shared
// classifier only as their final compatibility fallback.
func ClassifyOutcome(message string) TerminalOutcome {
	trimmed := strings.TrimSpace(message)
	lower := strings.ToLower(trimmed)
	switch {
	case trimmed == "" || lower == "success" || lower == "completed":
		return TerminalOutcome{Kind: OutcomeSuccess}
	case containsAny(lower,
		"usage limit", "rate limit", "rate-limit", "quota", "credit limit",
		"credits exhausted", "insufficient credits", "free tier limit"):
		return TerminalOutcome{Kind: OutcomeQuotaExhausted, Message: trimmed}
	case containsAny(lower,
		"failed to authenticate", "authentication", "unauthorized", "not logged in",
		"login required", "oauth session expired", "token expired", "invalid api key"):
		return TerminalOutcome{Kind: OutcomeAuthenticationRequired, Message: trimmed}
	case containsAny(lower,
		"context window", "context length", "maximum context", "too many tokens",
		"error_max_turns", "maximum turns", "max turns"):
		return TerminalOutcome{Kind: OutcomeContextExhausted, Message: trimmed}
	case containsAny(lower, "cancelled", "canceled", "interrupted", "context canceled"):
		return TerminalOutcome{Kind: OutcomeCancelled, Message: trimmed}
	case containsAny(lower,
		"overloaded", "temporarily unavailable", "timeout", "timed out",
		"connection reset", "connection refused", "server error", " 500", " 502",
		" 503", " 504", "http 429", "status 429"):
		return TerminalOutcome{Kind: OutcomeTransientProvider, Message: trimmed}
	default:
		return TerminalOutcome{Kind: OutcomeAgentFailure, Message: trimmed}
	}
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

// Event is a normalized event from an agent CLI.
type Event struct {
	Type      EventType
	Timestamp time.Time

	ToolName  string
	ToolInput map[string]any
	Text      string

	CostUSD  float64
	Duration float64
	Subtype  string
	Error    string

	// Outcome is non-nil only for terminal events.
	Outcome *TerminalOutcome
}

// ToolUseEvent creates a tool-use event.
func ToolUseEvent(name string, input map[string]any) Event {
	return Event{
		Type:      EventToolUse,
		Timestamp: time.Now(),
		ToolName:  name,
		ToolInput: input,
	}
}

// TextEvent creates a text event.
func TextEvent(text string) Event {
	return Event{Type: EventText, Timestamp: time.Now(), Text: text}
}

// ResultEvent creates a terminal result event.
func ResultEvent(costUSD, duration float64, subtype string) Event {
	outcome := ClassifyOutcome(subtype)
	return Event{
		Type:      EventResult,
		Timestamp: time.Now(),
		CostUSD:   costUSD,
		Duration:  duration,
		Subtype:   subtype,
		Outcome:   &outcome,
	}
}

// ErrorEvent creates a terminal error event.
func ErrorEvent(msg string) Event {
	outcome := ClassifyOutcome(msg)
	return Event{
		Type:      EventError,
		Timestamp: time.Now(),
		Error:     msg,
		Outcome:   &outcome,
	}
}

// WarningEvent creates a non-terminal diagnostic event.
func WarningEvent(msg string) Event {
	return Event{Type: EventError, Timestamp: time.Now(), Error: msg}
}
