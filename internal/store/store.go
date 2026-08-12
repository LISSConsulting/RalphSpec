// Package store persists loop events to a JSONL session log and provides
// indexed read-back of past iterations. One store instance is created per
// ralph invocation in cmd/ralph/wiring.go and reused across Regent restarts
// (which occur in the same OS process, keeping session identity stable).
package store

import (
	"time"

	"github.com/LISSConsulting/RalphSpec/internal/loop"
)

// Writer persists loop events to durable storage.
type Writer interface {
	Append(entry loop.LogEntry) error
	Close() error
}

// Reader retrieves past iteration data from storage.
type Reader interface {
	Iterations() ([]IterationSummary, error)
	IterationLog(n int) ([]loop.LogEntry, error)
	SessionSummary() (SessionSummary, error)
}

// Store combines Writer and Reader into a single session-scoped handle.
// Created once per process in wiring.go; the same instance is reused
// across Regent restarts (which stay within the same OS process).
type Store interface {
	Writer
	Reader
}

// IterationSummary summarises one completed loop iteration.
type IterationSummary struct {
	Number   int
	Agent    string
	Provider string
	Mode     string
	CostUSD  float64
	Duration float64
	Subtype  string // "success", "error_max_turns", etc.
	Commit   string
	StartAt  time.Time
	EndAt    time.Time
}

// SessionSummary summarises the current session.
type SessionSummary struct {
	SessionID  string
	Agent      string
	Provider   string
	StartedAt  time.Time
	TotalCost  float64
	Iterations int
	LastCommit string
	Branch     string
}
