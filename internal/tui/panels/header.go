// Package panels provides the panel components for the Ralph multi-panel TUI.
package panels

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// HeaderProps holds all data needed to render the header bar.
// String fields for state/focus avoid importing the parent tui package (circular dep prevention).
type HeaderProps struct {
	ProjectName string
	WorkDir     string
	Branch      string
	Agent       string
	Mode        string
	Iteration   int
	MaxIter     int
	TotalCost   float64
	StateSymbol string // e.g. "●", "✓", "✗", "⟳"
	StateLabel  string // e.g. "BUILDING", "IDLE", "FAILED"
	Elapsed     time.Duration
	Clock       time.Time
}

// AbbreviatePath returns a display-friendly path, replacing the home directory
// with "~" and converting backslashes to forward slashes.
func AbbreviatePath(path string) string {
	if path == "" {
		return ""
	}
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(path, home) {
		path = "~" + path[len(home):]
	}
	return strings.ReplaceAll(path, "\\", "/")
}

// FormatElapsed renders a duration as a compact string: "5s", "2m30s", "1h15m".
func FormatElapsed(d time.Duration) string {
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%dm", h, m)
	}
	if m > 0 {
		return fmt.Sprintf("%dm%ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// truncateToWidth truncates a plain-text string to fit within maxWidth visual
// cells, appending "…" if truncated.  Uses lipgloss.Width for correct handling
// of wide characters (emoji, CJK, etc.).
func truncateToWidth(s string, maxWidth int) string {
	if maxWidth <= 0 || lipgloss.Width(s) <= maxWidth {
		return s
	}
	runes := []rune(s)
	for len(runes) > 0 && lipgloss.Width(string(runes)) > maxWidth-1 {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "…"
}

// RenderHeader renders the header bar for the multi-panel TUI.
// accentStyle is applied to the full header bar width.
func RenderHeader(props HeaderProps, width int, accentStyle lipgloss.Style) string {
	iter := fmt.Sprintf("%d", props.Iteration)
	maxLabel := "∞"
	if props.MaxIter > 0 {
		maxLabel = fmt.Sprintf("%d", props.MaxIter)
	}

	branch := props.Branch
	if branch == "" {
		branch = "—"
	}

	mode := props.Mode
	if mode == "" {
		mode = "—"
	}

	name := "RalphSpec"
	if props.ProjectName != "" {
		name = props.ProjectName
	}

	parts := []string{"👑 " + name}
	if props.WorkDir != "" {
		parts = append(parts, "dir: "+AbbreviatePath(props.WorkDir))
	}
	if props.Agent != "" {
		parts = append(parts, "agent: "+props.Agent)
	}

	stateLabel := props.StateLabel
	if props.StateSymbol != "" && props.StateLabel != "" {
		stateLabel = props.StateSymbol + " " + props.StateLabel
	}

	parts = append(parts,
		fmt.Sprintf("mode: %s", mode),
		fmt.Sprintf("branch: %s", branch),
		fmt.Sprintf("iter: %s/%s", iter, maxLabel),
		fmt.Sprintf("cost: $%.2f", props.TotalCost),
	)
	if stateLabel != "" {
		parts = append(parts, stateLabel)
	}
	if props.Elapsed > 0 {
		parts = append(parts, fmt.Sprintf("elapsed: %s", FormatElapsed(props.Elapsed)))
	}
	if !props.Clock.IsZero() {
		parts = append(parts, props.Clock.Format("15:04"))
	}

	content := strings.Join(parts, "  │  ")
	content = truncateToWidth(content, width)
	return accentStyle.Width(width).MaxWidth(width).Render(content)
}
