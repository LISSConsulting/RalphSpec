package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"github.com/LISSConsulting/RalphSpec/internal/loop"
	"github.com/LISSConsulting/RalphSpec/internal/orchestrator"
	"github.com/LISSConsulting/RalphSpec/internal/spec"
	"github.com/LISSConsulting/RalphSpec/internal/store"
	"github.com/LISSConsulting/RalphSpec/internal/tui/panels"
)

// Model is the root bubbletea model for the multi-panel Ralph TUI.
type Model struct {
	// Event source
	events      <-chan loop.LogEntry
	storeReader store.Reader

	// Sub-panels
	specsPanel      panels.SpecsPanel
	iterationsPanel panels.IterationsPanel
	mainView        panels.MainView
	secondary       panels.SecondaryPanel

	// Layout and focus
	layout Layout
	focus  FocusTarget
	theme  Theme
	width  int
	height int

	// Loop state
	loopState  LoopState
	iteration  int
	maxIter    int
	agent      string
	mode       string
	branch     string
	totalCost  float64
	lastCommit string

	// Time
	startedAt time.Time
	now       time.Time

	// Identity
	projectName string
	workDir     string

	// Graceful stop
	requestStop   func()
	stopRequested bool

	// Help overlay
	helpVisible bool

	// Loop control (nil when launched from ralph build/plan/run)
	controller LoopController

	// Worktree mode (nil when [worktree] is disabled)
	orch                 *orchestrator.Orchestrator
	worktreeLogsByBranch map[string][]string // branch → accumulated rendered log lines
	activeWorktreeBranch string              // branch currently shown in Main panel

	// Error/done
	err  error
	done bool
}

// New creates the multi-panel TUI Model.
// storeReader may be nil if no session log is available.
// specFiles is the initial list of specs for the sidebar; nil is allowed.
// requestStop, if non-nil, is called once when the user presses 's'.
func New(events <-chan loop.LogEntry, storeReader store.Reader, accentColor, projectName, workDir string, specFiles []spec.SpecFile, requestStop func(), controller LoopController) Model {
	now := time.Now()
	th := NewTheme(accentColor)
	layout := Calculate(80, 24)

	specsW, specsH := innerDims(layout.Specs)
	itersW, itersH := innerDims(layout.Iterations)
	mainW, mainH := innerDims(layout.Main)
	secW, secH := innerDims(layout.Secondary)

	return Model{
		events:          events,
		storeReader:     storeReader,
		specsPanel:      panels.NewSpecsPanel(specFiles, workDir, specsW, specsH),
		iterationsPanel: panels.NewIterationsPanel(itersW, itersH),
		mainView:        panels.NewMainView(mainW, mainH),
		secondary:       panels.NewSecondaryPanel(secW, secH),
		layout:          layout,
		focus:           FocusSpecs,
		theme:           th,
		width:           80,
		height:          24,
		loopState:       StateIdle,
		startedAt:       now,
		now:             now,
		projectName:     projectName,
		workDir:         workDir,
		requestStop:     requestStop,
		controller:      controller,
	}
}

// WithOrchestrator enables worktree mode for this TUI model.
// When set, a Worktrees tab appears in the Secondary panel and the W/x/M/D
// keybinds become active.  Pass nil to disable (same as not calling this method).
func (m Model) WithOrchestrator(orch *orchestrator.Orchestrator) Model {
	if orch == nil {
		return m
	}
	m.orch = orch
	m.worktreeLogsByBranch = make(map[string][]string)
	m.secondary = m.secondary.EnableWorktrees(agentsToEntries(orch.ActiveAgents()))
	return m
}

// Err returns any error recorded from the loop.
func (m Model) Err() error { return m.err }

// Init returns the initial commands: event listener + clock ticker + optional
// worktree tagged-event listener + startup data loading.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{waitForEvent(m.events), tickCmd(), initGitInfoCmd(), initIterationsCmd(m.storeReader)}
	if m.orch != nil {
		cmds = append(cmds, waitForTaggedEvent(m.orch.MergedEvents))
	}
	return tea.Batch(cmds...)
}

// initGitInfoCmd reads the current git branch and last commit asynchronously.
func initGitInfoCmd() tea.Cmd {
	return func() tea.Msg {
		branch := runGitOutput("branch", "--show-current")
		commit := runGitOutput("log", "-1", "--format=%h")
		return gitInfoMsg{Branch: strings.TrimSpace(branch), LastCommit: strings.TrimSpace(commit)}
	}
}

// runGitOutput runs a git subcommand and returns its stdout, or "" on error.
func runGitOutput(args ...string) string {
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return ""
	}
	return string(out)
}

// initIterationsCmd pre-loads past iteration summaries from the store.
func initIterationsCmd(sr store.Reader) tea.Cmd {
	return func() tea.Msg {
		if sr == nil {
			return iterationsLoadedMsg{}
		}
		summaries, _ := sr.Iterations()
		return iterationsLoadedMsg{Summaries: summaries}
	}
}

// tickCmd schedules the next one-second clock tick.
func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// waitForEvent blocks on the event channel and returns the next message.
func waitForEvent(ch <-chan loop.LogEntry) tea.Cmd {
	return func() tea.Msg {
		entry, ok := <-ch
		if !ok {
			return loopDoneMsg{}
		}
		return logEntryMsg(entry)
	}
}

// waitForTaggedEvent blocks on the orchestrator fan-in channel and returns the
// next tagged event.  Returns nil when the channel is closed (no-op for Update).
func waitForTaggedEvent(ch <-chan orchestrator.TaggedLogEntry) tea.Cmd {
	return func() tea.Msg {
		tagged, ok := <-ch
		if !ok {
			return nil
		}
		entry := tagged.Entry
		if entry.Agent == "" {
			entry.Agent = tagged.Agent
		}
		return taggedEventMsg{Branch: tagged.Branch, Entry: entry}
	}
}

// Update handles all incoming bubbletea messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.handleWindowSize(msg)
	case tea.KeyMsg:
		return m.handleKey(msg)
	case logEntryMsg:
		return m.handleLogEntry(msg)
	case taggedEventMsg:
		return m.handleTaggedEvent(msg)
	case tickMsg:
		m.now = time.Time(msg)
		return m, tickCmd()
	case loopDoneMsg:
		// Channel closed — loop finished. Transition to idle but keep TUI open.
		// In dashboard mode the channel is never closed; for ralph build/plan/run
		// this fires once when the single loop finishes. User presses q to exit.
		if m.loopState.CanTransitionTo(StateIdle) {
			m.loopState = StateIdle
		}
		return m, nil
	case loopErrMsg:
		m.err = msg.err
		m.done = true
		return m, tea.Quit
	case panels.SpecSelectedMsg:
		return m.handleSpecSelected(msg)
	case panels.EditSpecRequestMsg:
		return m.handleEditSpecRequest(msg)
	case panels.CreateSpecRequestMsg:
		return m.handleCreateSpecRequest(msg)
	case specsRefreshedMsg:
		return m.handleSpecsRefreshed(msg)
	case gitInfoMsg:
		if msg.Branch != "" {
			m.branch = msg.Branch
		}
		if msg.LastCommit != "" {
			m.lastCommit = msg.LastCommit
		}
		return m, nil
	case iterationsLoadedMsg:
		for _, s := range msg.Summaries {
			m.iterationsPanel = m.iterationsPanel.AddIteration(s)
		}
		return m, nil
	case panels.IterationSelectedMsg:
		return m.handleIterationSelected(msg)
	case iterationLogLoadedMsg:
		return m.handleIterationLogLoaded(msg)
	case panels.WorktreeActionMsg:
		return m.handleWorktreeAction(msg)
	case panels.WorktreeSelectedMsg:
		return m.handleWorktreeSelected(msg)
	}
	return m.delegateToFocused(msg)
}

func (m Model) handleWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height
	m.layout = Calculate(msg.Width, msg.Height)
	if !m.layout.TooSmall {
		specsW, specsH := innerDims(m.layout.Specs)
		mainW, mainH := innerDims(m.layout.Main)
		secW, secH := innerDims(m.layout.Secondary)
		m.specsPanel = m.specsPanel.SetSize(specsW, specsH)
		m.mainView = m.mainView.SetSize(mainW, mainH)
		m.secondary = m.secondary.SetSize(secW, secH)

		itersW, itersH := innerDims(m.layout.Iterations)
		m.iterationsPanel = m.iterationsPanel.SetSize(itersW, itersH)
	}
	return m, nil
}

// nextFocus returns the next panel in the 4-panel forward tab order.
func (m Model) nextFocus() FocusTarget { return m.focus.Next() }

// prevFocus returns the previous panel in the 4-panel reverse tab order.
func (m Model) prevFocus() FocusTarget { return m.focus.Prev() }

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Help overlay absorbs the key press to dismiss it.
	if m.helpVisible {
		m.helpVisible = false
		return m, nil
	}
	switch msg.String() {
	case "?":
		m.helpVisible = true
		return m, nil
	case "q", "ctrl+c":
		return m, tea.Quit
	case "s":
		if m.requestStop != nil && !m.stopRequested {
			m.stopRequested = true
			m.requestStop()
		}
		return m, nil
	case "b":
		if m.controller != nil && !m.controller.IsRunning() {
			if m.loopState.CanTransitionTo(StateBuilding) {
				m.loopState = StateBuilding
			}
			m.controller.StartLoop("build")
		}
		return m, nil
	case "p":
		if m.controller != nil && !m.controller.IsRunning() {
			if m.loopState.CanTransitionTo(StatePlanning) {
				m.loopState = StatePlanning
			}
			m.controller.StartLoop("plan")
		}
		return m, nil
	case "R":
		if m.controller != nil && !m.controller.IsRunning() {
			if m.loopState.CanTransitionTo(StateBuilding) {
				m.loopState = StateBuilding
			}
			m.controller.StartLoop("smart")
		}
		return m, nil
	case "x":
		if m.controller != nil {
			m.controller.StopLoop()
		}
		return m, nil
	case "W":
		// Launch a worktree agent for the currently selected spec.
		// Use the spec name as the branch — it matches the existing feature branch
		// (e.g. "023-cadence-msp"). Do NOT prefix with "wt/" — that would create
		// a new branch from the current HEAD (trunk) instead of using the spec's branch.
		if m.orch == nil {
			m.mainView = m.mainView.AppendLine(m.theme.RenderLogLine(loop.LogEntry{
				Kind:    loop.LogError,
				Message: "worktree mode unavailable — wt (worktrunk) not detected on PATH",
			}, m.layout.Main.Width))
			return m, nil
		}
		if m.focus != FocusSpecs {
			return m, nil
		}
		if sel := m.specsPanel.SelectedSpec(); sel != nil {
			if err := m.orch.Launch(context.Background(), sel.Name, sel.Name, sel.Dir, loop.ModeBuild, 0); err != nil {
				m.mainView = m.mainView.AppendLine(m.theme.RenderLogLine(loop.LogEntry{
					Kind:    loop.LogError,
					Message: fmt.Sprintf("worktree launch failed: %v", err),
				}, m.layout.Main.Width))
			} else {
				m.mainView = m.mainView.AppendLine(m.theme.RenderLogLine(loop.LogEntry{
					Kind:    loop.LogInfo,
					Message: fmt.Sprintf("worktree agent launched for %s", sel.Name),
				}, m.layout.Main.Width))
				m.secondary = m.secondary.SetWorktreeEntries(agentsToEntries(m.orch.ActiveAgents()))
			}
		}
		return m, nil
	case "tab":
		m.focus = m.nextFocus()
		return m, nil
	case "shift+tab":
		m.focus = m.prevFocus()
		return m, nil
	case "1":
		m.focus = FocusSpecs
		return m, nil
	case "2":
		m.focus = FocusIterations
		return m, nil
	case "3":
		m.focus = FocusMain
		return m, nil
	case "4":
		m.focus = FocusSecondary
		return m, nil
	case "5":
		// Reserved — no-op (worktrees live in Secondary panel's Worktrees tab).
		return m, nil
	}
	return m.delegateToFocused(msg)
}

func (m Model) delegateToFocused(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.focus {
	case FocusSpecs:
		m.specsPanel, cmd = m.specsPanel.Update(msg)
	case FocusIterations:
		m.iterationsPanel, cmd = m.iterationsPanel.Update(msg)
	case FocusMain:
		m.mainView, cmd = m.mainView.Update(msg)
	case FocusSecondary:
		m.secondary, cmd = m.secondary.Update(msg)
	}
	return m, cmd
}

func (m Model) handleLogEntry(msg logEntryMsg) (tea.Model, tea.Cmd) {
	entry := loop.LogEntry(msg)

	// Update loop metadata from entry
	if entry.Branch != "" {
		m.branch = entry.Branch
	}
	if entry.Mode != "" {
		m.mode = entry.Mode
	}
	if entry.Agent != "" {
		m.agent = entry.Agent
	}
	if entry.MaxIter > 0 {
		m.maxIter = entry.MaxIter
	}
	if entry.Iteration > 0 {
		m.iteration = entry.Iteration
	}
	if entry.TotalCost > 0 {
		m.totalCost = entry.TotalCost
	}
	if entry.Commit != "" {
		m.lastCommit = entry.Commit
	}

	// Derive LoopState transitions from log kind
	switch entry.Kind {
	case loop.LogIterStart:
		next := StateBuilding
		if entry.Mode == "plan" {
			next = StatePlanning
		}
		if m.loopState.CanTransitionTo(next) {
			m.loopState = next
		}
		m.iterationsPanel = m.iterationsPanel.SetCurrent(entry.Iteration)

	case loop.LogIterComplete:
		// Accumulate cost immediately so the header total updates before the
		// subsequent LogInfo event. LogInfo/LogDone carry the authoritative
		// TotalCost from the loop and will override this via the check above.
		m.totalCost += entry.CostUSD
		summary := store.IterationSummary{
			Number:   entry.Iteration,
			Agent:    entry.Agent,
			Mode:     entry.Mode,
			CostUSD:  entry.CostUSD,
			Duration: entry.Duration,
			Subtype:  entry.Subtype,
			Commit:   entry.Commit,
		}
		m.iterationsPanel = m.iterationsPanel.AddIteration(summary).SetCurrent(0)
		m.secondary = m.secondary.AddIteration(summary)

	case loop.LogDone, loop.LogStopped, loop.LogSpecComplete, loop.LogSweepComplete:
		if m.loopState.CanTransitionTo(StateIdle) {
			m.loopState = StateIdle
		}

	case loop.LogError:
		if m.loopState.CanTransitionTo(StateFailed) {
			m.loopState = StateFailed
		}

	case loop.LogRegent:
		if m.loopState.CanTransitionTo(StateRegentRestart) {
			m.loopState = StateRegentRestart
		}
	}

	// Render once at current width; route by kind
	rendered := m.theme.RenderLogLine(entry, m.layout.Main.Width)
	switch entry.Kind {
	case loop.LogRegent:
		m.secondary = m.secondary.AppendLine(rendered, panels.TabRegent)
		if strings.Contains(rendered, "Tests") || strings.Contains(rendered, "Reverted") {
			m.secondary = m.secondary.AppendLine(rendered, panels.TabTests)
		}
	case loop.LogGitPull, loop.LogGitPush:
		m.secondary = m.secondary.AppendLine(rendered, panels.TabGit)
		m.mainView = m.mainView.AppendLine(rendered)
	default:
		m.mainView = m.mainView.AppendLine(rendered)
	}

	return m, waitForEvent(m.events)
}

// handleTaggedEvent processes a log entry from a worktree agent.
// It accumulates rendered lines per branch and refreshes the WorktreesPanel.
// If the event's branch is the currently active worktree, the line is also
// appended to the Main panel in real time.
func (m Model) handleTaggedEvent(msg taggedEventMsg) (tea.Model, tea.Cmd) {
	rendered := m.theme.RenderLogLine(msg.Entry, m.layout.Main.Width)

	// Accumulate per-branch log.
	if m.worktreeLogsByBranch == nil {
		m.worktreeLogsByBranch = make(map[string][]string)
	}
	m.worktreeLogsByBranch[msg.Branch] = append(m.worktreeLogsByBranch[msg.Branch], rendered)

	// Live-append to Main panel when viewing this branch's log.
	if m.activeWorktreeBranch == msg.Branch {
		m.mainView = m.mainView.AppendLine(rendered)
	}

	// Route Regent events to the Secondary panel so the Regent tab is populated.
	if msg.Entry.Kind == loop.LogRegent {
		m.secondary = m.secondary.AppendLine(rendered, panels.TabRegent)
	}

	// Update worktrees tab with fresh agent state snapshot.
	if m.orch != nil {
		m.secondary = m.secondary.SetWorktreeEntries(agentsToEntries(m.orch.ActiveAgents()))
	}

	return m, waitForTaggedEvent(m.orch.MergedEvents)
}

// handleWorktreeAction dispatches stop/merge/clean on the orchestrator.
func (m Model) handleWorktreeAction(msg panels.WorktreeActionMsg) (tea.Model, tea.Cmd) {
	if m.orch == nil {
		return m, nil
	}
	switch msg.Action {
	case "stop":
		_ = m.orch.Stop(msg.Branch)
	case "merge":
		_ = m.orch.Merge(msg.Branch)
	case "clean":
		_ = m.orch.Clean(msg.Branch)
	}
	// Refresh worktrees tab after state change.
	m.secondary = m.secondary.SetWorktreeEntries(agentsToEntries(m.orch.ActiveAgents()))
	return m, nil
}

// handleWorktreeSelected switches the Main panel to show the selected agent's log.
func (m Model) handleWorktreeSelected(msg panels.WorktreeSelectedMsg) (tea.Model, tea.Cmd) {
	m.activeWorktreeBranch = msg.Branch
	logs := m.worktreeLogsByBranch[msg.Branch]
	m.mainView = m.mainView.ShowWorktreeLog(logs)
	return m, nil
}

func (m Model) handleSpecSelected(msg panels.SpecSelectedMsg) (tea.Model, tea.Cmd) {
	content := m.readSpecContent(msg.Spec)
	w, _ := innerDims(m.layout.Main)
	m.mainView = m.mainView.ShowSpec(renderMarkdown(content, w))
	return m, nil
}

// renderMarkdown renders markdown content with glamour for display in the Main panel.
// Falls back to raw text if glamour fails (e.g. unsupported environment).
func renderMarkdown(content string, width int) string {
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return content
	}
	rendered, err := r.Render(content)
	if err != nil {
		return content
	}
	return rendered
}

func (m Model) handleEditSpecRequest(msg panels.EditSpecRequestMsg) (tea.Model, tea.Cmd) {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		return m, nil
	}
	path := msg.Path
	if !filepath.IsAbs(path) && m.workDir != "" {
		path = filepath.Join(m.workDir, path)
	}
	parts := strings.Fields(editor)
	cmdArgs := make([]string, len(parts)-1, len(parts))
	copy(cmdArgs, parts[1:])
	cmdArgs = append(cmdArgs, path)
	cmd := exec.Command(parts[0], cmdArgs...) //nolint:gosec
	workDir := m.workDir
	return m, tea.ExecProcess(cmd, func(_ error) tea.Msg {
		specs, _ := spec.List(workDir)
		return specsRefreshedMsg{Specs: specs}
	})
}

func (m Model) handleCreateSpecRequest(msg panels.CreateSpecRequestMsg) (tea.Model, tea.Cmd) {
	workDir := m.workDir
	name := msg.Name
	return m, func() tea.Msg {
		// Create a spec kit feature directory; spec.md is created by 'ralph specify'.
		_ = os.MkdirAll(filepath.Join(workDir, "specs", name), 0o755)
		specs, _ := spec.List(workDir)
		return specsRefreshedMsg{Specs: specs}
	}
}

func (m Model) handleSpecsRefreshed(msg specsRefreshedMsg) (tea.Model, tea.Cmd) {
	specsW, specsH := innerDims(m.layout.Specs)
	m.specsPanel = panels.NewSpecsPanel(msg.Specs, m.workDir, specsW, specsH)
	return m, nil
}

func (m Model) readSpecContent(sf spec.SpecFile) string {
	path := sf.Path
	if !filepath.IsAbs(path) && m.workDir != "" {
		path = filepath.Join(m.workDir, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("(cannot read %s: %v)", sf.Path, err)
	}
	return string(data)
}

func (m Model) handleIterationSelected(msg panels.IterationSelectedMsg) (tea.Model, tea.Cmd) {
	if m.storeReader == nil {
		return m, nil
	}
	n := msg.Number
	return m, func() tea.Msg {
		entries, err := m.storeReader.IterationLog(n)
		var summary store.IterationSummary
		if summaries, sErr := m.storeReader.Iterations(); sErr == nil {
			for _, s := range summaries {
				if s.Number == n {
					summary = s
					break
				}
			}
		}
		return iterationLogLoadedMsg{Number: n, Entries: entries, Summary: summary, Err: err}
	}
}

func (m Model) handleIterationLogLoaded(msg iterationLogLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		return m, nil
	}
	rendered := make([]string, len(msg.Entries))
	for i, e := range msg.Entries {
		rendered[i] = m.theme.RenderLogLine(e, m.layout.Main.Width)
	}
	m.mainView = m.mainView.ShowIterationLog(rendered)
	detail := renderIterationSummary(msg.Summary)
	m.mainView = m.mainView.SetIterationSummary(detail)
	m.secondary = m.secondary.ShowDetail(detail)
	return m, nil
}

// renderIterationSummary formats an IterationSummary as key-value lines for the Summary tab.
func renderIterationSummary(s store.IterationSummary) []string {
	lines := []string{
		fmt.Sprintf("%-12s %d", "Iteration:", s.Number),
		fmt.Sprintf("%-12s %s", "Mode:", s.Mode),
		fmt.Sprintf("%-12s $%.4f", "Cost:", s.CostUSD),
		fmt.Sprintf("%-12s %.1fs", "Duration:", s.Duration),
	}
	if s.Subtype != "" {
		lines = append(lines, fmt.Sprintf("%-12s %s", "Exit:", s.Subtype))
	}
	if s.Commit != "" {
		lines = append(lines, fmt.Sprintf("%-12s %s", "Commit:", s.Commit))
	}
	return lines
}

// renderHelp renders a centered keybinding reference overlay.
func (m Model) renderHelp() string {
	lines := []string{
		"Keyboard Shortcuts",
		"",
		"  GLOBAL",
		"    ?           Show / hide this help",
		"    q / ctrl+c  Quit",
		"    s           Stop loop after current iteration",
		"    tab         Cycle panel focus forward",
		"    shift+tab   Cycle panel focus backward",
		"    1-4         Jump to Specs / Iterations / Main / Secondary",
		"",
		"  LOOP CONTROL  (dashboard mode only)",
		"    b           Start build loop",
		"    p           Start plan loop",
		"    R           Smart run (auto plan+build)",
		"    x           Stop loop immediately",
		"",
		"  SPECS PANEL",
		"    j / k       Navigate specs",
		"    enter       View spec",
		"    e           Edit spec in $EDITOR",
		"    n           Create new spec",
		"    W           Launch worktree agent for selected spec",
		"",
		"  ITERATIONS PANEL",
		"    j / k       Navigate iterations",
		"    enter       Open iteration log",
		"",
		"  MAIN PANEL",
		"    f           Toggle follow (auto-scroll)",
		"    [ / ]       Cycle tabs",
		"    ctrl+u/d    Page up / down",
		"    j / k       Scroll line up / down",
		"",
		"  SECONDARY PANEL",
		"    [ / ]       Cycle tabs (Regent/Git/Tests/Cost/Worktrees)",
		"    j / k       Scroll / navigate worktree agents",
		"    enter       View worktree agent log (Worktrees tab)",
		"    x / M / D   Stop / merge / clean agent (Worktrees tab)",
		"",
		"  Press any key to close",
	}
	box := m.theme.AccentBorderStyle().Padding(1, 3).Render(strings.Join(lines, "\n"))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

// View renders the full multi-panel TUI.
func (m Model) View() string {
	if m.layout.TooSmall {
		msg := fmt.Sprintf("Terminal too small (%dx%d).\nPlease resize to at least 80x24.", m.width, m.height)
		return lipgloss.NewStyle().
			Width(m.width).
			Align(lipgloss.Center).
			Render(msg)
	}

	if m.helpVisible {
		return m.renderHelp()
	}

	header := panels.RenderHeader(panels.HeaderProps{
		ProjectName: m.projectName,
		WorkDir:     m.workDir,
		Branch:      m.branch,
		Agent:       m.agent,
		Mode:        m.mode,
		Iteration:   m.iteration,
		MaxIter:     m.maxIter,
		TotalCost:   m.totalCost,
		StateSymbol: m.loopState.Symbol(),
		StateLabel:  m.loopState.Label(),
		Elapsed:     m.now.Sub(m.startedAt),
		Clock:       m.now,
	}, m.layout.Header.Width, m.theme.AccentHeaderStyle())

	footer := panels.RenderFooter(panels.FooterProps{
		Focus:         m.focus.String(),
		LastCommit:    m.lastCommit,
		StopRequested: m.stopRequested,
		StateLabel:    m.loopState.Label(),
	}, m.layout.Footer.Width)

	// Left sidebar: specs (top) + iterations (bottom)
	specsW, specsH := innerDims(m.layout.Specs)
	itersW, itersH := innerDims(m.layout.Iterations)
	mainW, mainH := innerDims(m.layout.Main)
	secW, secH := innerDims(m.layout.Secondary)

	sidebar := lipgloss.JoinVertical(lipgloss.Left,
		m.theme.RenderPanelBox(m.specsPanel.View(), 1, "Specs", m.focus == FocusSpecs, specsW, specsH),
		m.theme.RenderPanelBox(m.iterationsPanel.View(), 2, "Iterations", m.focus == FocusIterations, itersW, itersH),
	)

	rightCol := lipgloss.JoinVertical(lipgloss.Left,
		m.theme.RenderPanelBox(m.mainView.View(), 3, "Output", m.focus == FocusMain, mainW, mainH),
		m.theme.RenderPanelBox(m.secondary.View(), 4, "Secondary", m.focus == FocusSecondary, secW, secH),
	)

	body := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, rightCol)
	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

// innerDims returns the content dimensions for a panel rect accounting for
// the 1-character border on each side (2 total per dimension).
func innerDims(r Rect) (w, h int) {
	w = r.Width - 2
	if w < 1 {
		w = 1
	}
	h = r.Height - 2
	if h < 1 {
		h = 1
	}
	return
}

// agentsToEntries converts a slice of WorktreeAgent pointers to WorktreeEntry
// view-models understood by WorktreesPanel.
func agentsToEntries(agents []*orchestrator.WorktreeAgent) []panels.WorktreeEntry {
	entries := make([]panels.WorktreeEntry, len(agents))
	for i, a := range agents {
		entries[i] = panels.WorktreeEntry{
			Branch:     a.Branch,
			Agent:      a.Agent,
			State:      a.State.String(),
			Iterations: a.Iterations,
			TotalCost:  a.TotalCost,
			SpecName:   a.SpecName,
		}
	}
	return entries
}
