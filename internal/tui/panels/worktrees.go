package panels

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// WorktreeEntry is a view-model for a single worktree agent displayed in the panel.
// It is populated from orchestrator.WorktreeAgent fields by the TUI layer.
type WorktreeEntry struct {
	Branch     string
	Agent      string
	State      string // "creating", "running", "completed", "failed", "stopped", etc.
	Iterations int
	TotalCost  float64
	SpecName   string
}

// WorktreeActionMsg is emitted when the user requests stop/merge/clean on the selected worktree.
type WorktreeActionMsg struct {
	Branch string
	Action string // "stop", "merge", "clean"
}

// WorktreeSelectedMsg is emitted when the user presses enter on a worktree to view its log.
type WorktreeSelectedMsg struct {
	Branch string
}

// worktreeItem wraps a WorktreeEntry for use as a list.Item.
type worktreeItem struct {
	entry WorktreeEntry
}

func (w worktreeItem) Title() string {
	return fmt.Sprintf("%s %s", worktreeStateIcon(w.entry.State), w.entry.Branch)
}

func (w worktreeItem) Description() string {
	agent := w.entry.Agent
	if agent == "" {
		agent = "agent"
	}
	return fmt.Sprintf("%s  iter:%-3d  $%.4f  %s", agent, w.entry.Iterations, w.entry.TotalCost, w.entry.SpecName)
}

func (w worktreeItem) FilterValue() string { return w.entry.Branch }

// worktreeStateIcon returns a compact status icon for the given agent state string.
func worktreeStateIcon(state string) string {
	switch state {
	case "creating":
		return "⏳"
	case "running":
		return "🔨"
	case "completed":
		return "✅"
	case "failed":
		return "❌"
	case "stopped":
		return "⏹"
	case "merging":
		return "🔀"
	case "merged":
		return "✅"
	case "merge_failed":
		return "❌"
	case "removed":
		return "🗑"
	default:
		return "?"
	}
}

// worktreeDelegate renders compact single-line worktree entries.
type worktreeDelegate struct{}

func (d worktreeDelegate) Height() int                             { return 1 }
func (d worktreeDelegate) Spacing() int                            { return 0 }
func (d worktreeDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d worktreeDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	wi, ok := item.(worktreeItem)
	if !ok {
		return
	}
	s := wi.Title()
	if index == m.Index() {
		s = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7D56F4")).Render("> " + s)
	} else {
		s = "  " + s
	}
	_, _ = fmt.Fprint(w, s)
}

// WorktreesPanel displays a navigable list of worktree agents.
// Keys: j/k navigate, enter selects (shows log in main panel),
// x stops, M merges, D cleans the selected agent.
type WorktreesPanel struct {
	list    list.Model
	entries []WorktreeEntry
	width   int
	height  int
}

// NewWorktreesPanel creates a WorktreesPanel with the given entries.
func NewWorktreesPanel(entries []WorktreeEntry, w, h int) WorktreesPanel {
	items := make([]list.Item, len(entries))
	for i, e := range entries {
		items[i] = worktreeItem{entry: e}
	}
	l := list.New(items, worktreeDelegate{}, w, h)
	l.SetShowTitle(false)
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	return WorktreesPanel{list: l, entries: entries, width: w, height: h}
}

// SetEntries replaces the panel's list with new entries.
func (p WorktreesPanel) SetEntries(entries []WorktreeEntry) WorktreesPanel {
	items := make([]list.Item, len(entries))
	for i, e := range entries {
		items[i] = worktreeItem{entry: e}
	}
	p.entries = entries
	p.list.SetItems(items)
	return p
}

// SelectedBranch returns the branch name of the currently highlighted entry, or "".
func (p WorktreesPanel) SelectedBranch() string {
	if wi, ok := p.list.SelectedItem().(worktreeItem); ok {
		return wi.entry.Branch
	}
	return ""
}

// SetSize resizes the panel.
func (p WorktreesPanel) SetSize(w, h int) WorktreesPanel {
	p.width = w
	p.height = h
	p.list.SetSize(w, h)
	return p
}

// Update handles keyboard navigation and action keybinds.
func (p WorktreesPanel) Update(msg tea.Msg) (WorktreesPanel, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "j", "down":
			p.list, _ = p.list.Update(tea.KeyMsg{Type: tea.KeyDown})
			return p, nil
		case "k", "up":
			p.list, _ = p.list.Update(tea.KeyMsg{Type: tea.KeyUp})
			return p, nil
		case "enter":
			branch := p.SelectedBranch()
			if branch != "" {
				b := branch
				return p, func() tea.Msg { return WorktreeSelectedMsg{Branch: b} }
			}
		case "x":
			branch := p.SelectedBranch()
			if branch != "" {
				b := branch
				return p, func() tea.Msg { return WorktreeActionMsg{Branch: b, Action: "stop"} }
			}
		case "M":
			branch := p.SelectedBranch()
			if branch != "" {
				b := branch
				return p, func() tea.Msg { return WorktreeActionMsg{Branch: b, Action: "merge"} }
			}
		case "D":
			branch := p.SelectedBranch()
			if branch != "" {
				b := branch
				return p, func() tea.Msg { return WorktreeActionMsg{Branch: b, Action: "clean"} }
			}
		}
	}
	var cmd tea.Cmd
	p.list, cmd = p.list.Update(msg)
	return p, cmd
}

// View renders the worktrees panel.
func (p WorktreesPanel) View() string {
	if len(p.entries) == 0 {
		hint := strings.Join([]string{
			"No worktrees",
			"",
			"Press W on a spec",
			"to launch an agent",
		}, "\n")
		return lipgloss.NewStyle().
			Width(p.width).Height(p.height).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(lipgloss.Color("#888888")).
			Render(hint)
	}
	return p.list.View()
}
