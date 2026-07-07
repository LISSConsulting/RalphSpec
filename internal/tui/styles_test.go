package tui

import "testing"

func TestToolIcon(t *testing.T) {
	tests := []struct {
		tool string
		want string
	}{
		{"Read", "📖"},
		{"read_file", "📖"},
		{"Glob", "📖"},
		{"Grep", "📖"},
		{"Write", "✏️"},
		{"write_file", "✏️"},
		{"Edit", "✏️"},
		{"NotebookEdit", "✏️"},
		{"Bash", "🔧"},
		{"PowerShell", "🔧"},
		{"WebFetch", "🌐"},
		{"WebSearch", "🌐"},
		{"Task", "🔀"},
		// default case
		{"Agent", "⚡"},
		{"", "⚡"},
	}
	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			if got := toolIcon(tt.tool); got != tt.want {
				t.Errorf("toolIcon(%q) = %q, want %q", tt.tool, got, tt.want)
			}
		})
	}
}

func TestToolStyle(t *testing.T) {
	// Verify all branches are reachable and return a usable style without panicking.
	tools := []string{
		"Read", "read_file", "Glob", "Grep",
		"Write", "write_file", "Edit", "NotebookEdit",
		"Bash", "PowerShell",
		"WebFetch", // default branch
		"Unknown",  // default branch
		"",         // default branch
	}
	for _, tool := range tools {
		t.Run(tool, func(t *testing.T) {
			style := toolStyle(tool)
			_ = style.Render("x") // must not panic
		})
	}
}
