package tui

import (
	"time"

	"github.com/LISSConsulting/RalphSpec/internal/loop"
	"github.com/LISSConsulting/RalphSpec/internal/spec"
	"github.com/LISSConsulting/RalphSpec/internal/store"
)

// logEntryMsg wraps a LogEntry for broadcasting to all panels.
type logEntryMsg loop.LogEntry

// loopDoneMsg signals the event channel closed.
type loopDoneMsg struct{}

// loopErrMsg carries an error from the event loop.
type loopErrMsg struct{ err error }

// tickMsg is sent every second for the clock.
type tickMsg time.Time

// iterationLogLoadedMsg carries loaded iteration log data.
type iterationLogLoadedMsg struct {
	Number  int
	Entries []loop.LogEntry
	Summary store.IterationSummary
	Err     error
}

// specsRefreshedMsg carries refreshed spec list after creation/edit.
type specsRefreshedMsg struct{ Specs []spec.SpecFile }

// gitInfoMsg carries git branch and last commit read on startup.
type gitInfoMsg struct {
	Branch     string
	LastCommit string
}

// sessionDiffMsg carries a rendered git diff from the TUI session's starting revision.
type sessionDiffMsg struct {
	Lines []string
}

// iterationsLoadedMsg carries iteration summaries pre-loaded from the store on startup.
type iterationsLoadedMsg struct {
	Summaries []store.IterationSummary
}

// sessionsLoadedMsg carries the list of past session logs for the picker.
type sessionsLoadedMsg struct {
	Sessions []store.SessionSummary
}

// sessionLoadedMsg carries a past session's reader and iteration summaries
// after the user picks it from the session picker.
type sessionLoadedMsg struct {
	Reader    store.Reader
	Summaries []store.IterationSummary
	SessionID string
	Err       error
}

// taggedEventMsg wraps a log entry from the orchestrator fan-in channel together
// with the source worktree branch name.  Defined here without importing
// orchestrator so that msg.go stays import-free of business-logic packages.
type taggedEventMsg struct {
	Branch string
	Entry  loop.LogEntry
}
