package main

import (
	"fmt"

	"github.com/LISSConsulting/RalphSpec/internal/loop"
	"github.com/LISSConsulting/RalphSpec/internal/tui"
)

// lineFormatter renders loop.LogEntry values as terminal lines.
// When color is true, lipgloss styles matching the TUI palette are applied.
// When color is false, plain ASCII text is produced (suitable for piped output
// or --no-color mode).
type lineFormatter struct {
	color bool
}

// format renders entry as a single terminal line.
// In color mode it delegates to tui.RenderLogLine (width 200 avoids truncation
// in drain-goroutine context where the terminal width is unknown).
// In plain mode it produces a simple "[HH:MM:SS]  message" string, with a
// shield prefix for Regent entries.
func (f lineFormatter) format(entry loop.LogEntry) string {
	if f.color {
		return tui.RenderLogLine(entry, 200, tui.NewTheme(""))
	}
	ts := entry.Timestamp.Format("15:04:05")
	agentPrefix := ""
	if entry.Agent != "" {
		agentPrefix = fmt.Sprintf("[%s] ", entry.Agent)
	}
	if entry.Kind == loop.LogRegent {
		return fmt.Sprintf("[%s]  🛡️  Regent: %s%s", ts, agentPrefix, entry.Message)
	}
	if entry.Kind == loop.LogToolUse && entry.ToolName != "" {
		return fmt.Sprintf("[%s]  %s%s", ts, agentPrefix, tui.FormatToolUse(entry.ToolName, entry.ToolInput))
	}
	return fmt.Sprintf("[%s]  %s%s", ts, agentPrefix, entry.Message)
}
