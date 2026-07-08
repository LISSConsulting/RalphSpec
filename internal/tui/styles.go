// Package tui provides a bubbletea + lipgloss terminal UI for the Ralph loop.
package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// defaultAccentColor is the default accent color (indigo).
const defaultAccentColor = "#7D56F4"

// Color palette matching the spec.
var (
	colorWhite  = lipgloss.Color("#FAFAFA")
	colorGray   = lipgloss.Color("#888888")
	colorBlue   = lipgloss.Color("#5B9BD5")
	colorGreen  = lipgloss.Color("#6BCB77")
	colorYellow = lipgloss.Color("#FFD93D")
	colorRed    = lipgloss.Color("#FF6B6B")
	colorOrange = lipgloss.Color("#FFA54F")
)

// Styles used across the TUI. Accent-dependent styles (header, git) live
// on the Model and are computed from the configured accent color at creation.
var (
	timestampStyle = lipgloss.NewStyle().
			Foreground(colorGray)

	readStyle = lipgloss.NewStyle().
			Foreground(colorBlue)

	writeStyle = lipgloss.NewStyle().
			Foreground(colorGreen)

	searchStyle = lipgloss.NewStyle().
			Foreground(colorOrange)

	bashStyle = lipgloss.NewStyle().
			Foreground(colorYellow)

	errorStyle = lipgloss.NewStyle().
			Foreground(colorRed).
			Bold(true)

	resultStyle = lipgloss.NewStyle().
			Foreground(colorGreen).
			Bold(true)

	regentStyle = lipgloss.NewStyle().
			Foreground(colorOrange)

	infoStyle = lipgloss.NewStyle().
			Foreground(colorWhite)

	reasoningStyle = lipgloss.NewStyle().
			Foreground(colorGray)
)

// singleLine replaces newline sequences with a space so every log entry
// renders as exactly one terminal line.
func singleLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

// toolIcon returns the emoji icon for a given tool name.
func toolIcon(toolName string) string {
	switch toolName {
	case "Read", "read_file":
		return "📄"
	case "Glob":
		return "🗂"
	case "Grep":
		return "🔎"
	case "Write", "write_file", "Edit", "NotebookEdit":
		return "✎"
	case "PowerShell":
		return "❯"
	case "Bash":
		return "$"
	case "Command Prompt", "Cmd":
		return ">"
	case "WebFetch", "WebSearch":
		return "🌐"
	case "Task":
		return "🔀"
	default:
		return "⚡"
	}
}

// toolStyle returns the lipgloss style for a given tool name.
func toolStyle(toolName string) lipgloss.Style {
	switch toolName {
	case "Read", "read_file":
		return readStyle
	case "Glob", "Grep":
		return searchStyle
	case "Write", "write_file", "Edit", "NotebookEdit":
		return writeStyle
	case "Bash", "PowerShell", "Command Prompt", "Cmd":
		return bashStyle
	default:
		return infoStyle
	}
}
