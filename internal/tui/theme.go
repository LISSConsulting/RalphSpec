package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/LISSConsulting/RalphSpec/internal/loop"
)

const (
	toolIconColumnWidth = 2
	toolNameColumnWidth = len("PowerShell")
)

// FormatToolUse renders the plain, aligned tool action used by terminal logs.
func FormatToolUse(toolName, toolInput string) string {
	displayName := displayToolName(toolName)
	if len(displayName) > 14 {
		displayName = displayName[:13] + "…"
	}
	if len(displayName) < toolNameColumnWidth {
		displayName += strings.Repeat(" ", toolNameColumnWidth-len(displayName))
	}
	return fmt.Sprintf("%s %s", displayName, singleLine(toolInput))
}

func displayToolName(toolName string) string {
	if toolName == "Command Prompt" {
		return "Cmd"
	}
	return toolName
}

func formatToolIcon(icon string) string {
	width := lipgloss.Width(icon)
	if width >= toolIconColumnWidth {
		return icon
	}
	return icon + strings.Repeat(" ", toolIconColumnWidth-width)
}

// Theme holds accent-color-derived styles for the multi-panel TUI.
// Non-accent styles (toolIcon, toolStyle, color vars) are package-level
// and shared with the existing single-panel TUI via styles.go.
type Theme struct {
	accentStyle     lipgloss.Style         // for header background / focused elements
	gitStyle        lipgloss.Style         // for git operation messages
	borderFocused   lipgloss.Style         // focused panel border
	borderUnfocused lipgloss.Style         // unfocused panel border
	focusedColor    lipgloss.TerminalColor // raw accent color for border chars
	unfocusedColor  lipgloss.TerminalColor // raw gray color for border chars
}

// NewTheme creates a Theme from a hex accent color string (e.g. "#7D56F4").
// If accentColor is empty, the default accent color is used.
func NewTheme(accentColor string) Theme {
	color := defaultAccentColor
	if accentColor != "" {
		color = accentColor
	}
	c := lipgloss.Color(color)
	return Theme{
		accentStyle: lipgloss.NewStyle().
			Background(c).
			Foreground(lipgloss.Color("#FFFFFF")).
			Bold(true),
		gitStyle: lipgloss.NewStyle().
			Foreground(c),
		borderFocused: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(c),
		borderUnfocused: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorGray),
		focusedColor:   c,
		unfocusedColor: colorGray,
	}
}

// AccentHeaderStyle returns the style for the header bar.
func (t Theme) AccentHeaderStyle() lipgloss.Style {
	return t.accentStyle
}

// AccentBorderStyle returns a focused-panel border style using the accent color.
func (t Theme) AccentBorderStyle() lipgloss.Style {
	return t.borderFocused
}

// DimBorderStyle returns an unfocused-panel border style using gray.
func (t Theme) DimBorderStyle() lipgloss.Style {
	return t.borderUnfocused
}

// PanelBorderStyle returns the appropriate border style for a panel based on
// whether it currently holds keyboard focus.
func (t Theme) PanelBorderStyle(focused bool) lipgloss.Style {
	if focused {
		return t.borderFocused
	}
	return t.borderUnfocused
}

// RenderLogLine renders a loop.LogEntry as a single terminal line.
// Carries forward all LogKind rendering from view.go:renderLine(), but
// accepts width and theme as explicit parameters instead of model fields.
func (t Theme) RenderLogLine(entry loop.LogEntry, width int) string {
	ts := timestampStyle.Render(fmt.Sprintf("[%s]", entry.Timestamp.Format("15:04:05")))

	switch entry.Kind {
	case loop.LogToolUse:
		icon := formatToolIcon(toolIcon(entry.ToolName))
		style := toolStyle(entry.ToolName)
		toolUse := FormatToolUse(entry.ToolName, entry.ToolInput)
		name, input, _ := strings.Cut(toolUse, " ")
		name = style.Render(name)
		maxInput := width - lipgloss.Width(ts) - 2 - lipgloss.Width(icon) - 1 - lipgloss.Width(name) - 1
		if maxInput < 20 {
			maxInput = 20
		}
		if len(input) > maxInput {
			input = input[:maxInput-1] + "…"
		}
		return fmt.Sprintf("%s  %s %s %s", ts, icon, name, input)

	case loop.LogText:
		text := singleLine(entry.Message)
		maxText := width - 17
		if maxText < 20 {
			maxText = 20
		}
		if len([]rune(text)) > maxText {
			runes := []rune(text)
			text = string(runes[:maxText-1]) + "…"
		}
		return fmt.Sprintf("%s  %s", ts, reasoningStyle.Render("💭 "+text))

	case loop.LogIterStart:
		return fmt.Sprintf("%s  ── iteration %d ──", ts, entry.Iteration)

	case loop.LogIterComplete:
		iterMsg := fmt.Sprintf("✅ iteration %d complete  —  $%.2f  —  %.1fs", entry.Iteration, entry.CostUSD, entry.Duration)
		if entry.Subtype != "" {
			iterMsg += fmt.Sprintf("  —  %s", singleLine(entry.Subtype))
		}
		return fmt.Sprintf("%s  %s", ts, resultStyle.Render(iterMsg))

	case loop.LogError:
		return fmt.Sprintf("%s  %s", ts, errorStyle.Render("❌ "+singleLine(entry.Message)))

	case loop.LogGitPull:
		return fmt.Sprintf("%s  %s", ts, t.gitStyle.Render("⬆ "+singleLine(entry.Message)))

	case loop.LogGitPush:
		return fmt.Sprintf("%s  %s", ts, t.gitStyle.Render("⬇ "+singleLine(entry.Message)))

	case loop.LogDone, loop.LogSpecComplete, loop.LogSweepComplete:
		return fmt.Sprintf("%s  %s", ts, resultStyle.Render("✅ "+singleLine(entry.Message)))

	case loop.LogStopped:
		return fmt.Sprintf("%s  %s", ts, errorStyle.Render("⏹ "+singleLine(entry.Message)))

	case loop.LogRegent:
		return fmt.Sprintf("%s  %s", ts, regentStyle.Render("🛡️  Regent: "+singleLine(entry.Message)))

	case loop.LogSteer:
		return fmt.Sprintf("%s  %s", ts, t.gitStyle.Render("🧭 Steer: "+singleLine(entry.Message)))

	default:
		return fmt.Sprintf("%s  %s", ts, infoStyle.Render(singleLine(entry.Message)))
	}
}

// RenderLogLine is also exported as a package-level function for convenience.
// It delegates to theme.RenderLogLine.
func RenderLogLine(entry loop.LogEntry, width int, theme Theme) string {
	return theme.RenderLogLine(entry, width)
}

// RenderPanelBox renders content inside a bordered panel with the title
// embedded in the top border line (lazygit style).  w and h are the inner
// content dimensions (from innerDims).
func (t Theme) RenderPanelBox(content string, number int, title string, focused bool, w, h int) string {
	border := lipgloss.RoundedBorder()
	bc := t.unfocusedColor
	if focused {
		bc = t.focusedColor
	}
	bStyle := lipgloss.NewStyle().Foreground(bc)

	// Build title label with color.
	label := fmt.Sprintf("[%d] %s", number, title)
	var styledLabel string
	if focused {
		styledLabel = t.gitStyle.Bold(true).Render(label)
	} else {
		styledLabel = timestampStyle.Render(label)
	}

	// Top line: ╭─[N] Title──────╮
	// Inner width w means the total top line is w + 2 chars (corners).
	labelVis := lipgloss.Width(label)
	dashes := w - labelVis - 1 // 1 leading dash before label
	if dashes < 0 {
		dashes = 0
	}
	topLine := bStyle.Render(border.TopLeft+border.Top) +
		styledLabel +
		bStyle.Render(strings.Repeat(border.Top, dashes)+border.TopRight)

	// Render body with border on 3 sides (no top).
	body := t.PanelBorderStyle(focused).
		BorderTop(false).
		Width(w).Height(h).
		Render(content)

	return topLine + "\n" + body
}
