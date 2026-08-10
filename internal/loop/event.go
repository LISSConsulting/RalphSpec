package loop

import (
	"time"

	"github.com/LISSConsulting/RalphSpec/internal/quota"
)

// LogKind identifies the type of a loop log event.
type LogKind int

const (
	LogInfo          LogKind = iota // General informational message
	LogIterStart                    // Iteration starting
	LogToolUse                      // Claude tool use event
	LogText                         // Claude text/reasoning output between tool calls
	LogIterComplete                 // Iteration finished
	LogError                        // Error from Claude or loop
	LogGitPull                      // Git pull operation
	LogGitPush                      // Git push operation
	LogDone                         // Loop finished normally
	LogStopped                      // Loop stopped (context cancelled)
	LogRegent                       // Regent supervisor message
	LogSpecComplete                 // Spec boundary reached — success with no new commits (default mode)
	LogSweepComplete                // Roam complete — no spec boundary (--roam mode)
	LogSteer                        // Operator steering message consumed by the loop
)

// LogEntry is a structured event emitted by the loop during execution.
// When the Loop.Events channel is set, entries are sent there for TUI
// consumption. Otherwise, they fall back to the Loop.Log io.Writer.
type LogEntry struct {
	Kind      LogKind
	Timestamp time.Time
	Message   string

	// ToolUse fields
	ToolName  string
	ToolInput string

	// Cost/timing fields
	CostUSD   float64
	Duration  float64
	TotalCost float64
	Subtype   string // result exit subtype: "success", "error_max_turns", etc.

	// Iteration state
	Iteration int
	MaxIter   int

	// Git state
	Agent  string
	Branch string
	Commit string

	// Mode identifies plan/build execution; Ludicrous marks evidence-bounded
	// unbounded build mode for live and historical status surfaces.
	Mode      string
	Ludicrous bool

	// QuotaDecision records the admission decision associated with this event.
	QuotaDecision *quota.Decision
}
