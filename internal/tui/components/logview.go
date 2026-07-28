package components

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// LogView is a scrollable log panel that wraps bubbles/viewport.
// In follow mode (default), new lines cause the view to auto-scroll to the bottom.
// Pressing 'f' toggles follow mode on/off.
type LogView struct {
	vp     viewport.Model
	lines  []string // rendered (pre-styled) lines
	follow bool
	width  int
	height int
}

// NewLogView creates a LogView with the given dimensions, initially in follow mode.
func NewLogView(w, h int) LogView {
	vp := viewport.New(w, h)
	return LogView{
		vp:     vp,
		follow: true,
		width:  w,
		height: h,
	}
}

// AppendLine appends a pre-rendered (styled) line to the log.
// If follow mode is enabled, the viewport scrolls to the bottom.
func (v LogView) AppendLine(rendered string) LogView {
	v.lines = append(v.lines, rendered)
	v.vp.SetContent(strings.Join(v.lines, "\n"))
	if v.follow {
		v.vp.GotoBottom()
	}
	return v
}

// SetContent replaces all log lines with the given slice.
// Scrolls to the bottom if follow mode is enabled.
func (v LogView) SetContent(lines []string) LogView {
	v.lines = make([]string, len(lines))
	copy(v.lines, lines)
	v.vp.SetContent(strings.Join(v.lines, "\n"))
	if v.follow {
		v.vp.GotoBottom()
	}
	return v
}

// ToggleFollow switches follow mode on or off.
// When turned on, scrolls immediately to the bottom.
func (v LogView) ToggleFollow() LogView {
	v.follow = !v.follow
	if v.follow {
		v.vp.GotoBottom()
	}
	return v
}

// GotoTop scrolls to the first line and disables follow mode.
func (v LogView) GotoTop() LogView {
	v.follow = false
	v.vp.GotoTop()
	return v
}

// GotoBottom scrolls to the last line and enables follow mode.
func (v LogView) GotoBottom() LogView {
	v.follow = true
	v.vp.GotoBottom()
	return v
}

// SetSize resizes the log view to the given dimensions.
func (v LogView) SetSize(w, h int) LogView {
	v.width = w
	v.height = h
	v.vp.Width = w
	v.vp.Height = h
	if v.follow {
		v.vp.GotoBottom()
	}
	return v
}

// Following reports whether follow mode is currently active.
func (v LogView) Following() bool {
	return v.follow
}

// Update handles bubbletea messages (scroll keys, mouse events).
func (v LogView) Update(msg tea.Msg) (LogView, tea.Cmd) {
	var cmd tea.Cmd
	v.vp, cmd = v.vp.Update(msg)
	// If user scrolled away from bottom, exit follow mode.
	if v.follow && !v.vp.AtBottom() {
		// Only disable follow on explicit scroll messages, not on resize.
		switch msg.(type) {
		case tea.KeyMsg, tea.MouseMsg:
			v.follow = false
		}
	}
	return v, cmd
}

// View renders the log view content.
func (v LogView) View() string {
	return v.vp.View()
}
