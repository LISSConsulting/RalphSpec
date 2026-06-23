// Package orchestrator manages the lifecycle of multiple WorktreeAgents.
// Each agent is a Loop instance running inside a dedicated git worktree.
package orchestrator

import (
	"github.com/LISSConsulting/RalphSpec/internal/loop"
)

// AgentState represents the lifecycle state of a worktree agent.
type AgentState int

const (
	StateCreating    AgentState = iota // worktree is being created
	StateRunning                       // loop is actively running
	StateCompleted                     // loop finished successfully
	StateFailed                        // loop exited with error
	StateStopped                       // stop was requested and honoured
	StateMerging                       // wt merge is in progress
	StateMerged                        // worktree was merged and removed
	StateMergeFailed                   // wt merge returned an error
	StateRemoved                       // worktree was removed without merging
)

func (s AgentState) String() string {
	switch s {
	case StateCreating:
		return "creating"
	case StateRunning:
		return "running"
	case StateCompleted:
		return "completed"
	case StateFailed:
		return "failed"
	case StateStopped:
		return "stopped"
	case StateMerging:
		return "merging"
	case StateMerged:
		return "merged"
	case StateMergeFailed:
		return "merge_failed"
	case StateRemoved:
		return "removed"
	default:
		return "unknown"
	}
}

// WorktreeAgent tracks one agent running inside a git worktree.
type WorktreeAgent struct {
	Branch       string
	WorktreePath string
	SpecName     string
	SpecDir      string
	Agent        string
	State        AgentState
	Iterations   int
	TotalCost    float64
	Events       chan loop.LogEntry // receives loop events; closed when loop exits
	StopCh       chan struct{}      // close to request graceful stop
	Error        error              // non-nil when State == StateFailed
}

// TaggedLogEntry wraps a loop.LogEntry with the source branch name so the
// TUI and other consumers can distinguish events from multiple parallel agents.
type TaggedLogEntry struct {
	Branch string
	Agent  string
	Entry  loop.LogEntry
}
