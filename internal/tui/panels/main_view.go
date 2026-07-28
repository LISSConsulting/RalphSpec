package panels

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/LISSConsulting/RalphSpec/internal/loop"
	"github.com/LISSConsulting/RalphSpec/internal/tui/components"
)

// MainTab identifies the active content tab in the main view.
type MainTab int

const (
	TabOutput           MainTab = iota // Live loop output log
	TabSpecContent                     // Spec file content viewer (US2)
	TabIterationDetail                 // Past iteration log drill-down (US3)
	TabIterationSummary                // Iteration metadata summary (US3)
)

// OutputFilter selects a structured subset of the live session output.
type OutputFilter int

const (
	FilterAll OutputFilter = iota
	FilterThinking
	FilterCommands
	FilterEdits
	FilterDiff
)

// OutputLine couples a rendered line with the structured event fields needed
// to place it in the session's filtered output buffers.
type OutputLine struct {
	Rendered string
	Kind     loop.LogKind
	ToolName string
}

// MainView is the main (right-top) panel showing loop output and spec/iteration content.
// Each tab owns its own LogView so content is never displaced by output from another tab.
type MainView struct {
	tabbar       components.TabBar
	filterbar    components.TabBar
	outputLog    components.LogView // All live streaming output
	thinkingLog  components.LogView // Model text/reasoning only
	commandLog   components.LogView // Shell command tool calls only
	editLog      components.LogView // File mutation tool calls only
	diffLog      components.LogView // Git diff from the session's starting revision
	specLog      components.LogView // Tab 1: spec file content
	iterationLog components.LogView // Tab 2: past iteration log
	summaryLog   components.LogView // Tab 3: iteration metadata summary
	width        int
	height       int
	activeTab    MainTab
	outputFilter OutputFilter
	pendingG     bool // true after a lone 'g' keypress (awaiting second 'g')
}

var mainTabLabels = []string{"Output", "Spec", "Iteration", "Summary"}
var outputFilterLabels = []string{"All", "Thinking", "Commands", "Edits", "Diff"}

// NewMainView creates a MainView with the output tab active.
func NewMainView(w, h int) MainView {
	contentH := h - 1 // subtract main tab bar row
	if contentH < 1 {
		contentH = 1
	}
	outputH := contentH - 1 // subtract output filter row
	if outputH < 1 {
		outputH = 1
	}
	return MainView{
		tabbar:       components.NewTabBar(mainTabLabels).SetWidth(w),
		filterbar:    components.NewTabBar(outputFilterLabels).SetWidth(w),
		outputLog:    components.NewLogView(w, outputH),
		thinkingLog:  components.NewLogView(w, outputH),
		commandLog:   components.NewLogView(w, outputH),
		editLog:      components.NewLogView(w, outputH),
		diffLog:      components.NewLogView(w, outputH),
		specLog:      components.NewLogView(w, contentH),
		iterationLog: components.NewLogView(w, contentH),
		summaryLog:   components.NewLogView(w, contentH),
		width:        w,
		height:       h,
	}
}

// AppendLine appends a pre-rendered (styled) line to the output log only.
// It never touches specLog, iterationLog, or summaryLog so those tabs
// retain their content independently of live streaming output.
func (v MainView) AppendLine(rendered string) MainView {
	v.outputLog = v.outputLog.AppendLine(rendered)
	return v
}

// AppendOutput appends a structured event to the complete output and to its
// matching filtered view, when applicable.
func (v MainView) AppendOutput(line OutputLine) MainView {
	v.outputLog = v.outputLog.AppendLine(line.Rendered)
	switch {
	case line.Kind == loop.LogText:
		v.thinkingLog = v.thinkingLog.AppendLine(line.Rendered)
	case line.Kind == loop.LogToolUse && isCommandTool(line.ToolName):
		v.commandLog = v.commandLog.AppendLine(line.Rendered)
	case line.Kind == loop.LogToolUse && isEditTool(line.ToolName):
		v.editLog = v.editLog.AppendLine(line.Rendered)
	}
	return v
}

// SetSessionDiff replaces the current session diff view.
func (v MainView) SetSessionDiff(lines []string) MainView {
	v.diffLog = v.diffLog.SetContent(lines)
	return v
}

// ShowSpec loads spec content into the spec viewer and switches to TabSpecContent.
func (v MainView) ShowSpec(content string) MainView {
	v.specLog = v.specLog.SetContent(splitLines(content))
	v.activeTab = TabSpecContent
	v.tabbar = components.NewTabBar(mainTabLabels).SetWidth(v.width)
	for i := 0; i < int(TabSpecContent); i++ {
		v.tabbar = v.tabbar.Next()
	}
	return v
}

// ShowIterationLog loads a past iteration's log entries and switches to TabIterationDetail.
// entries are pre-rendered strings (app.go renders via theme.RenderLogLine before passing).
func (v MainView) ShowIterationLog(rendered []string) MainView {
	v.iterationLog = v.iterationLog.SetContent(rendered)
	v.activeTab = TabIterationDetail
	v.tabbar = components.NewTabBar(mainTabLabels).SetWidth(v.width)
	for i := 0; i < int(TabIterationDetail); i++ {
		v.tabbar = v.tabbar.Next()
	}
	return v
}

// SetIterationSummary loads summary key-value lines into the summary viewport.
// The tab is not switched; the user navigates to Summary with ].
func (v MainView) SetIterationSummary(lines []string) MainView {
	v.summaryLog = v.summaryLog.SetContent(lines)
	return v
}

// SwitchToOutput returns to the live output tab.
func (v MainView) SwitchToOutput() MainView {
	v.activeTab = TabOutput
	v.tabbar = v.tabbar.SetActive(int(TabOutput))
	return v
}

// ShowWorktreeLog loads a worktree agent's accumulated log entries into the
// Output tab so the user can review a specific agent's activity.
// Each line carries its original event category so filtered views also work.
func (v MainView) ShowWorktreeLog(lines []OutputLine) MainView {
	v.outputLog = v.outputLog.SetContent(nil)
	v.thinkingLog = v.thinkingLog.SetContent(nil)
	v.commandLog = v.commandLog.SetContent(nil)
	v.editLog = v.editLog.SetContent(nil)
	for _, line := range lines {
		v = v.AppendOutput(line)
	}
	v.activeTab = TabOutput
	v.outputFilter = FilterAll
	v.tabbar = v.tabbar.SetActive(int(TabOutput))
	v.filterbar = v.filterbar.SetActive(int(FilterAll))
	return v
}

// SetSize resizes the main view.
func (v MainView) SetSize(w, h int) MainView {
	v.width = w
	v.height = h
	contentH := h - 1
	if contentH < 1 {
		contentH = 1
	}
	outputH := contentH - 1
	if outputH < 1 {
		outputH = 1
	}
	v.tabbar = v.tabbar.SetWidth(w)
	v.filterbar = v.filterbar.SetWidth(w)
	v.outputLog = v.outputLog.SetSize(w, outputH)
	v.thinkingLog = v.thinkingLog.SetSize(w, outputH)
	v.commandLog = v.commandLog.SetSize(w, outputH)
	v.editLog = v.editLog.SetSize(w, outputH)
	v.diffLog = v.diffLog.SetSize(w, outputH)
	v.specLog = v.specLog.SetSize(w, contentH)
	v.iterationLog = v.iterationLog.SetSize(w, contentH)
	v.summaryLog = v.summaryLog.SetSize(w, contentH)
	return v
}

// activeLogView returns a pointer to the LogView for the current tab so that
// Update and View can dispatch to the right buffer without a switch per call-site.
func (v *MainView) activeLogView() *components.LogView {
	switch v.activeTab {
	case TabSpecContent:
		return &v.specLog
	case TabIterationDetail:
		return &v.iterationLog
	case TabIterationSummary:
		return &v.summaryLog
	default:
		return v.activeOutputLogView()
	}
}

func (v *MainView) activeOutputLogView() *components.LogView {
	switch v.outputFilter {
	case FilterThinking:
		return &v.thinkingLog
	case FilterCommands:
		return &v.commandLog
	case FilterEdits:
		return &v.editLog
	case FilterDiff:
		return &v.diffLog
	default:
		return &v.outputLog
	}
}

// ShowingSessionDiff reports whether the live diff filter is visible.
func (v MainView) ShowingSessionDiff() bool {
	return v.activeTab == TabOutput && v.outputFilter == FilterDiff
}

// Update handles key messages for the main panel.
func (v MainView) Update(msg tea.Msg) (MainView, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		key := msg.String()
		if key != "g" {
			v.pendingG = false
		}
		switch key {
		case "]":
			v.tabbar = v.tabbar.Next()
			v.activeTab = MainTab(v.tabbar.Active())
		case "[":
			v.tabbar = v.tabbar.Prev()
			v.activeTab = MainTab(v.tabbar.Active())
		case "}":
			if v.activeTab == TabOutput {
				v.filterbar = v.filterbar.Next()
				v.outputFilter = OutputFilter(v.filterbar.Active())
			}
		case "{":
			if v.activeTab == TabOutput {
				v.filterbar = v.filterbar.Prev()
				v.outputFilter = OutputFilter(v.filterbar.Active())
			}
		case "g":
			if v.pendingG {
				lv := v.activeLogView()
				*lv = lv.GotoTop()
				v.pendingG = false
			} else {
				v.pendingG = true
			}
		case "G":
			lv := v.activeLogView()
			*lv = lv.GotoBottom()
		case "f":
			if v.activeTab != TabIterationSummary {
				lv := v.activeLogView()
				*lv = lv.ToggleFollow()
			}
		default:
			lv := v.activeLogView()
			*lv, cmd = lv.Update(msg)
		}
	default:
		lv := v.activeLogView()
		*lv, cmd = lv.Update(msg)
	}
	return v, cmd
}

// View renders the main panel: tab bar + content area.
func (v MainView) View() string {
	tabRow := v.tabbar.View()
	lv := v.activeLogView()
	if v.activeTab == TabOutput {
		return lipgloss.JoinVertical(lipgloss.Left, tabRow, v.filterbar.View(), lv.View())
	}
	return lipgloss.JoinVertical(lipgloss.Left, tabRow, lv.View())
}

func isCommandTool(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, command := range []string{"bash", "powershell", "command prompt", "cmd", "shell", "shell_command", "exec_command", "command_execution"} {
		if name == command || strings.HasSuffix(name, ":"+command) {
			return true
		}
	}
	return false
}

func isEditTool(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	return strings.Contains(name, "edit") ||
		strings.Contains(name, "write") ||
		strings.Contains(name, "patch") ||
		name == "delete" || strings.HasSuffix(name, ":delete")
}

// splitLines splits a string into lines for SetContent.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
