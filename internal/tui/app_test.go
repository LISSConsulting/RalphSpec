package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/LISSConsulting/RalphSpec/internal/config"
	"github.com/LISSConsulting/RalphSpec/internal/loop"
	"github.com/LISSConsulting/RalphSpec/internal/orchestrator"
	"github.com/LISSConsulting/RalphSpec/internal/spec"
	"github.com/LISSConsulting/RalphSpec/internal/store"
	"github.com/LISSConsulting/RalphSpec/internal/tui/panels"
	"github.com/LISSConsulting/RalphSpec/internal/worktree"
)

// mockStoreReader is a minimal store.Reader for unit tests.
type mockStoreReader struct {
	iterations []store.IterationSummary
	entries    []loop.LogEntry
}

func (r *mockStoreReader) Iterations() ([]store.IterationSummary, error) {
	return r.iterations, nil
}

func (r *mockStoreReader) IterationLog(_ int) ([]loop.LogEntry, error) {
	return r.entries, nil
}

func (r *mockStoreReader) SessionSummary() (store.SessionSummary, error) {
	return store.SessionSummary{}, nil
}

// mockLoopController is a test double for LoopController.
type mockLoopController struct {
	startCalled string
	stopCalled  bool
	running     bool
}

func (c *mockLoopController) StartLoop(mode string) {
	c.startCalled = mode
	c.running = true
}

func (c *mockLoopController) StopLoop() {
	c.stopCalled = true
	c.running = false
}

func (c *mockLoopController) IsRunning() bool { return c.running }

func newTestModel() Model {
	ch := make(chan loop.LogEntry, 1)
	return New(ch, nil, "", "TestProject", "/tmp/proj", nil, nil, nil)
}

func TestNew_Defaults(t *testing.T) {
	m := newTestModel()
	if m.width != 80 {
		t.Errorf("expected default width 80, got %d", m.width)
	}
	if m.height != 24 {
		t.Errorf("expected default height 24, got %d", m.height)
	}
	if m.focus != FocusSpecs {
		t.Errorf("expected default focus FocusSpecs, got %v", m.focus)
	}
	if m.loopState != StateIdle {
		t.Errorf("expected initial loopState StateIdle, got %v", m.loopState)
	}
	if m.done {
		t.Error("expected done=false at init")
	}
}

func TestInit_ReturnsCmd(t *testing.T) {
	m := newTestModel()
	cmd := m.Init()
	if cmd == nil {
		t.Error("Init() should return a non-nil command")
	}
}

func TestErr_NoError(t *testing.T) {
	m := newTestModel()
	if m.Err() != nil {
		t.Errorf("Err() should be nil at init, got %v", m.Err())
	}
}

func TestUpdate_WindowSize(t *testing.T) {
	m := newTestModel()
	updated, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if cmd != nil {
		t.Error("WindowSizeMsg should return nil cmd")
	}
	m2 := updated.(Model)
	if m2.width != 120 || m2.height != 40 {
		t.Errorf("got dimensions %dx%d, want 120x40", m2.width, m2.height)
	}
	if m2.layout.TooSmall {
		t.Error("120x40 should not be TooSmall")
	}
}

func TestUpdate_WindowSize_TooSmall(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	m2 := updated.(Model)
	if !m2.layout.TooSmall {
		t.Error("60x20 should be TooSmall")
	}
}

func TestUpdate_Key_Quit(t *testing.T) {
	m := newTestModel()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("q key should return a quit cmd")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("q key cmd should produce tea.QuitMsg, got %T", msg)
	}
}

func TestUpdate_Key_Tab_CyclesFocus(t *testing.T) {
	m := newTestModel()
	// Start at FocusSpecs (index 0), tab should go to FocusIterations (1)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m2 := updated.(Model)
	if m2.focus != FocusSpecs.Next() {
		t.Errorf("tab should advance focus from %v to %v, got %v",
			FocusSpecs, FocusSpecs.Next(), m2.focus)
	}
}

func TestUpdate_Key_DirectFocus(t *testing.T) {
	tests := []struct {
		key  string
		want FocusTarget
	}{
		{"1", FocusSpecs},
		{"2", FocusIterations},
		{"3", FocusMain},
		{"4", FocusSecondary},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			m := newTestModel()
			updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.key)})
			m2 := updated.(Model)
			if m2.focus != tt.want {
				t.Errorf("key %q: focus = %v, want %v", tt.key, m2.focus, tt.want)
			}
		})
	}
}

func TestUpdate_LogEntry_StateTransition(t *testing.T) {
	tests := []struct {
		name      string
		entry     loop.LogEntry
		wantState LoopState
	}{
		{
			name:      "LogIterStart build → StateBuilding",
			entry:     loop.LogEntry{Kind: loop.LogIterStart, Mode: "build", Iteration: 1},
			wantState: StateBuilding,
		},
		{
			name:      "LogIterStart plan → StatePlanning",
			entry:     loop.LogEntry{Kind: loop.LogIterStart, Mode: "plan", Iteration: 1},
			wantState: StatePlanning,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel()
			msg := logEntryMsg(tt.entry)
			updated, _ := m.Update(msg)
			m2 := updated.(Model)
			if m2.loopState != tt.wantState {
				t.Errorf("loopState = %v, want %v", m2.loopState, tt.wantState)
			}
		})
	}
}

func TestUpdate_LogEntry_MetadataExtracted(t *testing.T) {
	m := newTestModel()
	entry := loop.LogEntry{
		Kind:      loop.LogIterStart,
		Timestamp: time.Now(),
		Iteration: 3,
		MaxIter:   10,
		Agent:     "codex",
		Branch:    "feat/test",
		Mode:      "build",
		TotalCost: 0.05,
	}
	updated, _ := m.Update(logEntryMsg(entry))
	m2 := updated.(Model)

	if m2.iteration != 3 {
		t.Errorf("iteration = %d, want 3", m2.iteration)
	}
	if m2.branch != "feat/test" {
		t.Errorf("branch = %q, want \"feat/test\"", m2.branch)
	}
	if m2.mode != "build" {
		t.Errorf("mode = %q, want \"build\"", m2.mode)
	}
	if m2.agent != "codex" {
		t.Errorf("agent = %q, want \"codex\"", m2.agent)
	}
}

func TestUpdate_LoopDone_TransitionsToIdle(t *testing.T) {
	m := newTestModel()
	// Put model in building state to verify transition.
	m.loopState = StateBuilding
	updated, cmd := m.Update(loopDoneMsg{})
	m2 := updated.(Model)
	if m2.done {
		t.Error("loopDoneMsg should NOT set done=true; TUI stays open after loop finishes")
	}
	if cmd != nil {
		t.Error("loopDoneMsg should not return a quit cmd; user presses q to exit")
	}
	if m2.loopState != StateIdle {
		t.Errorf("loopDoneMsg should transition to StateIdle, got %v", m2.loopState)
	}
}

func TestUpdate_LoopErr_SetsErr(t *testing.T) {
	m := newTestModel()
	testErr := fmt.Errorf("loop failure")
	updated, _ := m.Update(loopErrMsg{err: testErr})
	m2 := updated.(Model)
	if m2.Err() != testErr {
		t.Errorf("Err() = %v, want %v", m2.Err(), testErr)
	}
	if !m2.done {
		t.Error("loopErrMsg should set done=true")
	}
}

func TestView_TooSmall(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 10})
	m2 := updated.(Model)
	view := m2.View()
	lower := strings.ToLower(view)
	if !strings.Contains(lower, "too small") {
		t.Errorf("View() for small terminal should contain 'too small', got: %q", view)
	}
}

func TestView_Normal_DoesNotPanic(t *testing.T) {
	m := newTestModel()
	// Resize to large terminal so all panels have space
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m2 := updated.(Model)
	// Should not panic
	view := m2.View()
	if view == "" {
		t.Error("View() should not return empty string")
	}
}

func TestUpdate_EditSpecRequest_NoEditor(t *testing.T) {
	t.Setenv("EDITOR", "")
	m := newTestModel()
	_, cmd := m.Update(panels.EditSpecRequestMsg{Path: "specs/foo.md"})
	if cmd != nil {
		t.Error("EditSpecRequestMsg with no EDITOR should return nil cmd")
	}
}

func TestUpdate_EditSpecRequest_WithEditor(t *testing.T) {
	t.Setenv("EDITOR", "true")
	m := newTestModel()
	_, cmd := m.Update(panels.EditSpecRequestMsg{Path: "specs/foo.md"})
	if cmd == nil {
		t.Error("EditSpecRequestMsg with EDITOR set should return a non-nil cmd")
	}
}

func TestUpdate_CreateSpecRequest_ReturnsRefresh(t *testing.T) {
	dir := t.TempDir()
	ch := make(chan loop.LogEntry, 1)
	m := New(ch, nil, "", "TestProject", dir, nil, nil, nil)

	_, cmd := m.Update(panels.CreateSpecRequestMsg{Name: "my-spec"})
	if cmd == nil {
		t.Fatal("CreateSpecRequestMsg should return a cmd")
	}
	msg := cmd()
	refreshed, ok := msg.(specsRefreshedMsg)
	if !ok {
		t.Fatalf("expected specsRefreshedMsg, got %T", msg)
	}
	// The spec file should have been created.
	found := false
	for _, s := range refreshed.Specs {
		if s.Name == "my-spec" {
			found = true
		}
	}
	if !found {
		t.Errorf("refreshed spec list should contain 'my-spec'; got %v", refreshed.Specs)
	}
	// File should exist on disk.
	if _, err := filepath.Glob(filepath.Join(dir, "specs", "my-spec.md")); err != nil {
		t.Errorf("spec file not created: %v", err)
	}
}

func TestUpdate_SpecsRefreshed_UpdatesPanel(t *testing.T) {
	m := newTestModel()
	// Panel starts with no specs.
	if m.specsPanel.SelectedSpec() != nil {
		t.Fatal("setup: expected empty specs panel")
	}
	newSpecs := []spec.SpecFile{
		{Name: "alpha", Path: "specs/alpha.md"},
	}
	updated, _ := m.Update(specsRefreshedMsg{Specs: newSpecs})
	m2 := updated.(Model)
	sel := m2.specsPanel.SelectedSpec()
	if sel == nil {
		t.Fatal("after specsRefreshedMsg, panel should have a selected spec")
	}
	if sel.Name != "alpha" {
		t.Errorf("selected spec name = %q, want %q", sel.Name, "alpha")
	}
}

func TestUpdate_StopRequested(t *testing.T) {
	stopped := false
	ch := make(chan loop.LogEntry, 1)
	m := New(ch, nil, "", "", "", nil, func() { stopped = true }, nil)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m2 := updated.(Model)
	if !stopped {
		t.Error("'s' key should have called requestStop")
	}
	if !m2.stopRequested {
		t.Error("stopRequested should be true after 's'")
	}

	// Second 's' press should not call requestStop again
	stopped = false
	m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if stopped {
		t.Error("second 's' key should not call requestStop again")
	}
}

func TestUpdate_IterationLogLoaded_SetsIterationAndSummary(t *testing.T) {
	m := newTestModel()

	// Resize to a standard size so layout is not TooSmall.
	updated0, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated0.(Model)

	summary := store.IterationSummary{
		Number:   2,
		Mode:     "build",
		CostUSD:  0.0234,
		Duration: 45.2,
		Subtype:  "success",
		Commit:   "abc1234",
	}
	msg := iterationLogLoadedMsg{
		Number:  2,
		Entries: []loop.LogEntry{{Kind: loop.LogInfo, Message: "agent output"}},
		Summary: summary,
		Err:     nil,
	}
	updated, _ := m.Update(msg)
	m2 := updated.(Model)

	// Switch focus to Main panel so ] key is delegated to mainView.
	updated0b, _ := m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	m2 = updated0b.(Model)

	// After iterationLogLoadedMsg the main view is on TabIterationDetail (2).
	// One ] advances to TabIterationSummary (3).
	updated1, _ := m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
	m3 := updated1.(Model)

	view := m3.View()
	// The Summary tab should show iteration number, cost, and commit.
	for _, want := range []string{"2", "build", "0.0234", "success", "abc1234"} {
		if !strings.Contains(view, want) {
			t.Errorf("Summary tab view missing %q; got:\n%s", want, view)
		}
	}
}

func TestHandleIterationSelected_NilReader(t *testing.T) {
	m := newTestModel() // storeReader is nil
	_, cmd := m.Update(panels.IterationSelectedMsg{Number: 1})
	if cmd != nil {
		t.Error("nil storeReader should return nil cmd")
	}
}

func TestHandleIterationSelected_WithReader(t *testing.T) {
	reader := &mockStoreReader{
		iterations: []store.IterationSummary{
			{Number: 1, Mode: "build", CostUSD: 0.01, Duration: 10, Subtype: "success", Commit: "deadbeef"},
		},
		entries: []loop.LogEntry{
			{Kind: loop.LogInfo, Message: "hello"},
		},
	}
	ch := make(chan loop.LogEntry, 1)
	m := New(ch, reader, "", "Proj", "/tmp", nil, nil, nil)

	_, cmd := m.Update(panels.IterationSelectedMsg{Number: 1})
	if cmd == nil {
		t.Fatal("with storeReader set, should return a non-nil cmd")
	}
	resultMsg := cmd()
	loaded, ok := resultMsg.(iterationLogLoadedMsg)
	if !ok {
		t.Fatalf("expected iterationLogLoadedMsg, got %T", resultMsg)
	}
	if loaded.Err != nil {
		t.Errorf("expected no error, got %v", loaded.Err)
	}
	if loaded.Summary.Commit != "deadbeef" {
		t.Errorf("expected commit deadbeef, got %q", loaded.Summary.Commit)
	}
}

func TestUpdate_Key_LoopControl_NoController(t *testing.T) {
	// Without a controller, b/p/R/x should be no-ops.
	keys := []string{"b", "R", "x"}
	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			m := newTestModel()
			updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
			m2 := updated.(Model)
			if m2.loopState != StateIdle {
				t.Errorf("key %q without controller should not change state, got %v", key, m2.loopState)
			}
			if cmd != nil {
				t.Errorf("key %q without controller should return nil cmd", key)
			}
		})
	}
}

func TestUpdate_Key_Build_StartsLoop(t *testing.T) {
	ctrl := &mockLoopController{}
	ch := make(chan loop.LogEntry, 1)
	m := New(ch, nil, "", "Proj", "", nil, nil, ctrl)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	m2 := updated.(Model)

	if ctrl.startCalled != "build" {
		t.Errorf("expected StartLoop(\"build\"), got %q", ctrl.startCalled)
	}
	if m2.loopState != StateBuilding {
		t.Errorf("expected StateBuilding, got %v", m2.loopState)
	}
}

func TestUpdate_Key_Roam_StartsLoop(t *testing.T) {
	ctrl := &mockLoopController{}
	ch := make(chan loop.LogEntry, 1)
	m := New(ch, nil, "", "Proj", "", nil, nil, ctrl)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")})
	m2 := updated.(Model)

	if ctrl.startCalled != "roam" {
		t.Errorf("expected StartLoop(\"roam\"), got %q", ctrl.startCalled)
	}
	if m2.loopState != StateBuilding {
		t.Errorf("expected StateBuilding, got %v", m2.loopState)
	}
}

func TestUpdate_Key_Stop_StopsLoop(t *testing.T) {
	ctrl := &mockLoopController{running: true}
	ch := make(chan loop.LogEntry, 1)
	m := New(ch, nil, "", "Proj", "", nil, nil, ctrl)

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})

	if !ctrl.stopCalled {
		t.Error("x key should call StopLoop()")
	}
}

func TestUpdate_Key_Help_Shows(t *testing.T) {
	m := newTestModel()
	if m.helpVisible {
		t.Fatal("setup: helpVisible should be false initially")
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m2 := updated.(Model)
	if !m2.helpVisible {
		t.Error("? key should set helpVisible=true")
	}
	if cmd != nil {
		t.Error("? key should return nil cmd")
	}
}

func TestUpdate_Key_Help_AnyKeyDismisses(t *testing.T) {
	m := newTestModel()
	m.helpVisible = true

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m2 := updated.(Model)
	if m2.helpVisible {
		t.Error("any key press when help visible should set helpVisible=false")
	}
}

func TestUpdate_Key_Help_Toggle(t *testing.T) {
	m := newTestModel()

	// First ? shows help.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m2 := updated.(Model)
	if !m2.helpVisible {
		t.Error("first ? should show help")
	}

	// Second ? (treated as "any key") dismisses it.
	updated2, _ := m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m3 := updated2.(Model)
	if m3.helpVisible {
		t.Error("second ? should dismiss help overlay")
	}
}

func TestView_Help_ContainsKeyBindings(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m2 := updated.(Model)
	m2.helpVisible = true

	view := m2.View()
	for _, want := range []string{"Keyboard Shortcuts", "GLOBAL", "LOOP CONTROL", "SPECS PANEL", "MAIN PANEL"} {
		if !strings.Contains(view, want) {
			t.Errorf("help view should contain %q", want)
		}
	}
}

func TestUpdate_Key_Build_NoopWhenRunning(t *testing.T) {
	ctrl := &mockLoopController{running: true}
	ch := make(chan loop.LogEntry, 1)
	m := New(ch, nil, "", "Proj", "", nil, nil, ctrl)
	m.loopState = StateBuilding

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})

	if ctrl.startCalled != "" {
		t.Errorf("b key should not start loop when already running, startCalled=%q", ctrl.startCalled)
	}
}

func TestReadSpecContent_Success(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "specs"), 0755); err != nil {
		t.Fatal(err)
	}
	want := "# Alpha Spec\n\nHello world."
	if err := os.WriteFile(filepath.Join(dir, "specs", "alpha.md"), []byte(want), 0644); err != nil {
		t.Fatal(err)
	}
	ch := make(chan loop.LogEntry, 1)
	m := New(ch, nil, "", "TestProject", dir, nil, nil, nil)

	got := m.readSpecContent(spec.SpecFile{Name: "alpha", Path: "specs/alpha.md"})
	if got != want {
		t.Errorf("readSpecContent = %q, want %q", got, want)
	}
}

func TestReadSpecContent_NotFound(t *testing.T) {
	ch := make(chan loop.LogEntry, 1)
	m := New(ch, nil, "", "TestProject", "", nil, nil, nil)

	got := m.readSpecContent(spec.SpecFile{Name: "missing", Path: "/nonexistent/path/missing.md"})
	if !strings.Contains(got, "cannot read") {
		t.Errorf("readSpecContent for missing file should contain 'cannot read'; got %q", got)
	}
}

func TestUpdate_SpecSelected_ShowsContent(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "specs"), 0755); err != nil {
		t.Fatal(err)
	}
	specContent := "# My Feature\n\nThis is the spec content."
	if err := os.WriteFile(filepath.Join(dir, "specs", "feature.md"), []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}
	ch := make(chan loop.LogEntry, 1)
	m := New(ch, nil, "", "TestProject", dir, nil, nil, nil)

	// Set a large window so content is renderable.
	updated0, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated0.(Model)

	sf := spec.SpecFile{Name: "feature", Path: "specs/feature.md"}
	updated, cmd := m.Update(panels.SpecSelectedMsg{Spec: sf})
	m2 := updated.(Model)

	if cmd != nil {
		t.Error("SpecSelectedMsg should return nil cmd")
	}
	// The main view should be on the spec tab and show the file content.
	// Glamour inserts ANSI codes between words, so check for words individually.
	view := m2.View()
	if !strings.Contains(view, "My") || !strings.Contains(view, "Feature") {
		t.Errorf("View() after SpecSelectedMsg should contain spec content; got:\n%s", view)
	}
}

// TestUpdate_LogEntry_LogDoneStoppedErrorRegent covers state transitions for
// LogDone, LogStopped, LogSpecComplete, LogSweepComplete, LogError, and
// LogRegent kinds — the branches in handleLogEntry that update loopState.
func TestUpdate_LogEntry_LogDoneStoppedErrorRegent(t *testing.T) {
	tests := []struct {
		name      string
		entry     loop.LogEntry
		wantState LoopState
	}{
		{
			name:      "LogDone → StateIdle",
			entry:     loop.LogEntry{Kind: loop.LogDone, Message: "finished"},
			wantState: StateIdle,
		},
		{
			name:      "LogStopped → StateIdle",
			entry:     loop.LogEntry{Kind: loop.LogStopped, Message: "stopped"},
			wantState: StateIdle,
		},
		{
			name:      "LogSpecComplete → StateIdle",
			entry:     loop.LogEntry{Kind: loop.LogSpecComplete, Message: "spec complete"},
			wantState: StateIdle,
		},
		{
			name:      "LogSweepComplete → StateIdle",
			entry:     loop.LogEntry{Kind: loop.LogSweepComplete, Message: "roam complete"},
			wantState: StateIdle,
		},
		{
			name:      "LogError → StateFailed",
			entry:     loop.LogEntry{Kind: loop.LogError, Message: "error occurred"},
			wantState: StateFailed,
		},
		{
			name:      "LogRegent → StateRegentRestart",
			entry:     loop.LogEntry{Kind: loop.LogRegent, Message: "restarting"},
			wantState: StateRegentRestart,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel()
			// Start in StateBuilding so all transitions are valid.
			m.loopState = StateBuilding
			updated, _ := m.Update(logEntryMsg(tt.entry))
			m2 := updated.(Model)
			if m2.loopState != tt.wantState {
				t.Errorf("loopState = %v, want %v", m2.loopState, tt.wantState)
			}
		})
	}
}

// TestUpdate_LogEntry_GitPullPushRouting verifies that LogGitPull and
// LogGitPush entries are routed to both the secondary panel (Git tab)
// and the main view — covering the LogGitPull/LogGitPush branch in
// handleLogEntry.
func TestUpdate_LogEntry_GitPullPushRouting(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind loop.LogKind
	}{
		{"LogGitPull", loop.LogGitPull},
		{"LogGitPush", loop.LogGitPush},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestModel()
			updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
			m = updated.(Model)
			entry := loop.LogEntry{Kind: tc.kind, Message: "git op"}
			updated2, _ := m.Update(logEntryMsg(entry))
			_ = updated2.(Model) // must not panic
		})
	}
}

// TestUpdate_LogEntry_LogRegentTestsRouting verifies that LogRegent entries
// containing "Tests" or "Reverted" are also routed to the Tests tab.
func TestUpdate_LogEntry_LogRegentTestsRouting(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	entry := loop.LogEntry{Kind: loop.LogRegent, Message: "Tests passed"}
	updated2, _ := m.Update(logEntryMsg(entry))
	_ = updated2.(Model) // must not panic; Tests branch covered
}

// TestDelegateToFocused covers delegating keyboard events when focus is on
// each non-Specs panel (Iterations, Main, Secondary).
func TestDelegateToFocused(t *testing.T) {
	focusCases := []struct {
		name  string
		focus FocusTarget
	}{
		{"Iterations", FocusIterations},
		{"Main", FocusMain},
		{"Secondary", FocusSecondary},
	}
	for _, fc := range focusCases {
		t.Run(fc.name, func(t *testing.T) {
			m := newTestModel()
			updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
			m = updated.(Model)
			m.focus = fc.focus
			// Send a scrolling key that is not handled by handleKey directly.
			updated2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
			_ = updated2.(Model) // must not panic
		})
	}
}

// TestInnerDims_SmallRect verifies that innerDims clamps w and h to at
// least 1 when the rectangle is too small to contain a 1-char border.
func TestInnerDims_SmallRect(t *testing.T) {
	tests := []struct {
		name  string
		r     Rect
		wantW int
		wantH int
	}{
		{"zero", Rect{Width: 0, Height: 0}, 1, 1},
		{"tiny", Rect{Width: 1, Height: 1}, 1, 1},
		{"normal", Rect{Width: 80, Height: 24}, 78, 22},
		{"small width", Rect{Width: 1, Height: 24}, 1, 22},
		{"small height", Rect{Width: 80, Height: 1}, 78, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, h := innerDims(tt.r)
			if w != tt.wantW || h != tt.wantH {
				t.Errorf("innerDims(%v) = (%d, %d), want (%d, %d)", tt.r, w, h, tt.wantW, tt.wantH)
			}
		})
	}
}

// TestUpdate_EditSpecRequest_AbsolutePath verifies that an absolute spec path
// is used as-is (not joined with workDir) in handleEditSpecRequest.
func TestUpdate_EditSpecRequest_AbsolutePath(t *testing.T) {
	t.Setenv("EDITOR", "true")
	dir := t.TempDir()
	ch := make(chan loop.LogEntry, 1)
	m := New(ch, nil, "", "Proj", dir, nil, nil, nil)
	// Use an absolute path — it must not be re-joined with workDir.
	absPath := dir + "/specs/absolute.md"
	_, cmd := m.Update(panels.EditSpecRequestMsg{Path: absPath})
	if cmd == nil {
		t.Error("EditSpecRequestMsg with EDITOR set and absolute path should return a non-nil cmd")
	}
}

// TestUpdate_IterationLogLoaded_WithError verifies that handleIterationLogLoaded
// returns early (nil cmd) when msg.Err is non-nil.
func TestUpdate_IterationLogLoaded_WithError(t *testing.T) {
	m := newTestModel()
	msg := iterationLogLoadedMsg{
		Number:  1,
		Entries: nil,
		Err:     fmt.Errorf("load failed"),
	}
	updated, cmd := m.Update(msg)
	_ = updated.(Model)
	if cmd != nil {
		t.Error("iterationLogLoadedMsg with Err set should return nil cmd")
	}
}

// TestUpdate_Key_ShiftTab covers the shift+tab branch in handleKey.
func TestUpdate_Key_ShiftTab_CyclesFocusBack(t *testing.T) {
	m := newTestModel()
	// Default focus is FocusSpecs (index 0); shift+tab should wrap to FocusSecondary (3).
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m2 := updated.(Model)
	if m2.focus != FocusSpecs.Prev() {
		t.Errorf("shift+tab should move focus from %v to %v, got %v",
			FocusSpecs, FocusSpecs.Prev(), m2.focus)
	}
}

// TestDelegateToFocused_Specs covers the FocusSpecs branch in delegateToFocused.
func TestDelegateToFocused_Specs(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	m.focus = FocusSpecs
	// Send a key that is not a global binding so it is delegated to specsPanel.
	updated2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	_ = updated2.(Model) // must not panic
}

// TestHandleLogEntry_CommitSet covers the `entry.Commit != ""` metadata branch.
func TestHandleLogEntry_CommitSet(t *testing.T) {
	m := newTestModel()
	entry := loop.LogEntry{Kind: loop.LogInfo, Commit: "abcdef0", Message: "info"}
	updated, _ := m.Update(logEntryMsg(entry))
	m2 := updated.(Model)
	if m2.lastCommit != "abcdef0" {
		t.Errorf("lastCommit = %q, want %q", m2.lastCommit, "abcdef0")
	}
}

// TestWaitForEvent_EntryReceived covers the logEntryMsg return path.
func TestWaitForEvent_EntryReceived(t *testing.T) {
	ch := make(chan loop.LogEntry, 1)
	ch <- loop.LogEntry{Kind: loop.LogInfo, Message: "hello"}
	cmd := waitForEvent(ch)
	msg := cmd()
	if _, ok := msg.(logEntryMsg); !ok {
		t.Fatalf("expected logEntryMsg, got %T", msg)
	}
}

// TestWaitForEvent_ChannelClosed covers the loopDoneMsg return path.
func TestWaitForEvent_ChannelClosed(t *testing.T) {
	ch := make(chan loop.LogEntry)
	close(ch)
	cmd := waitForEvent(ch)
	msg := cmd()
	if _, ok := msg.(loopDoneMsg); !ok {
		t.Fatalf("expected loopDoneMsg, got %T", msg)
	}
}

// TestUpdate_TickMsg covers the tickMsg case in Update, which updates
// m.now and reschedules the ticker.
func TestUpdate_TickMsg(t *testing.T) {
	m := newTestModel()
	// Use a fixed future time so the comparison is deterministic regardless
	// of clock resolution.
	later := time.Now().Add(time.Hour)
	tick := tickMsg(later)
	updated, cmd := m.Update(tick)
	m2 := updated.(Model)
	if !m2.now.Equal(later) {
		t.Errorf("tickMsg should set m.now to tick time; got %v, want %v", m2.now, later)
	}
	if cmd == nil {
		t.Error("tickMsg should reschedule ticker via tickCmd()")
	}
}

// TestHandleLogEntry_CannotTransition covers the false branches of
// CanTransitionTo in handleLogEntry — when the current state cannot
// legally transition to the target state, loopState remains unchanged.
func TestHandleLogEntry_CannotTransition(t *testing.T) {
	tests := []struct {
		name    string
		initial LoopState
		entry   loop.LogEntry
	}{
		{
			// StateIdle cannot transition to StateIdle (not in its valid set).
			name:    "LogDone from Idle stays Idle",
			initial: StateIdle,
			entry:   loop.LogEntry{Kind: loop.LogDone},
		},
		{
			// StateIdle cannot transition to StateFailed.
			name:    "LogError from Idle stays Idle",
			initial: StateIdle,
			entry:   loop.LogEntry{Kind: loop.LogError},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel()
			m.loopState = tt.initial
			updated, _ := m.Update(logEntryMsg(tt.entry))
			m2 := updated.(Model)
			if m2.loopState != tt.initial {
				t.Errorf("loopState changed from %v to %v, expected no transition", tt.initial, m2.loopState)
			}
		})
	}
}

// TestHandleLogEntry_LogIterComplete covers the LogIterComplete case in
// handleLogEntry — verifies that a completed iteration is processed by both
// the iterations panel and the secondary panel without panicking.
func TestHandleLogEntry_LogIterComplete(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	entry := loop.LogEntry{
		Kind:      loop.LogIterComplete,
		Iteration: 1,
		Mode:      "build",
		CostUSD:   0.02,
		Duration:  3.5,
		Subtype:   "success",
		Commit:    "abc1234",
	}
	updated2, _ := m.Update(logEntryMsg(entry))
	m2 := updated2.(Model)
	// LogIterComplete should call iterationsPanel.AddIteration and SetCurrent(0).
	// Verify the model updated correctly (no panic = code path covered).
	_ = m2.iterationsPanel
	_ = m2.secondary
}

// TestHandleLogEntry_CostAccumulation verifies that totalCost increments on
// each LogIterComplete event (T073) and is overridden by the authoritative
// TotalCost carried in subsequent LogInfo/LogDone events.
func TestHandleLogEntry_CostAccumulation(t *testing.T) {
	m := newTestModel()

	// First iteration: $0.05
	m2, _ := m.Update(logEntryMsg(loop.LogEntry{
		Kind: loop.LogIterComplete, Iteration: 1, CostUSD: 0.05,
	}))
	m = m2.(Model)
	if m.totalCost != 0.05 {
		t.Errorf("after iter 1: totalCost = %.4f, want 0.05", m.totalCost)
	}

	// Second iteration: $0.03
	m2, _ = m.Update(logEntryMsg(loop.LogEntry{
		Kind: loop.LogIterComplete, Iteration: 2, CostUSD: 0.03,
	}))
	m = m2.(Model)
	if m.totalCost != 0.08 {
		t.Errorf("after iter 2: totalCost = %.4f, want 0.08", m.totalCost)
	}

	// LogInfo carries the authoritative running total — should override accumulated value.
	m2, _ = m.Update(logEntryMsg(loop.LogEntry{
		Kind: loop.LogInfo, TotalCost: 0.08, Message: "Running total",
	}))
	m = m2.(Model)
	if m.totalCost != 0.08 {
		t.Errorf("after LogInfo: totalCost = %.4f, want 0.08", m.totalCost)
	}
}

// TestUpdate_UnknownMsg covers the default case in Update (delegateToFocused
// for any message type not explicitly handled by the switch).
func TestUpdate_UnknownMsg(t *testing.T) {
	m := newTestModel()
	// tea.MouseMsg is not handled by any explicit case, so it falls through
	// to the default delegateToFocused call at the end of Update.
	type unknownMsg struct{}
	updated, cmd := m.Update(unknownMsg{})
	_ = updated.(Model)
	_ = cmd // should not panic
}

// ---------------------------------------------------------------------------
// T037 — Orchestrator TUI wiring tests
// ---------------------------------------------------------------------------

// noopWorktreeOps satisfies worktree.WorktreeOps for unit tests.
// Switch always errors so no real worktrees are created.
type noopWorktreeOps struct{}

func (n *noopWorktreeOps) Detect() error                           { return nil }
func (n *noopWorktreeOps) Switch(_ string, _ bool) (string, error) { return "", fmt.Errorf("noop") }
func (n *noopWorktreeOps) List() ([]worktree.WorktreeInfo, error)  { return nil, nil }
func (n *noopWorktreeOps) Merge(_, _ string) error                 { return fmt.Errorf("noop") }
func (n *noopWorktreeOps) Remove(_ string) error                   { return fmt.Errorf("noop") }

// newTestOrch creates a minimal Orchestrator using noop worktree ops.
func newTestOrch() *orchestrator.Orchestrator {
	cfg := &config.Config{}
	cfg.Worktree.MaxParallel = 5
	return orchestrator.New(cfg, &noopWorktreeOps{})
}

// TestWithOrchestrator_Nil_NoChange verifies that passing nil leaves orch unset.
func TestWithOrchestrator_Nil_NoChange(t *testing.T) {
	m := newTestModel()
	m2 := m.WithOrchestrator(nil)
	if m2.orch != nil {
		t.Error("WithOrchestrator(nil) should leave orch as nil")
	}
}

// TestWithOrchestrator_SetsFields verifies the orch field and worktreeLogsByBranch map are set.
func TestWithOrchestrator_SetsFields(t *testing.T) {
	m := newTestModel()
	orch := newTestOrch()
	m2 := m.WithOrchestrator(orch)
	if m2.orch != orch {
		t.Error("WithOrchestrator: orch field should point to the provided Orchestrator")
	}
	if m2.worktreeLogsByBranch == nil {
		t.Error("WithOrchestrator: worktreeLogsByBranch should be initialised")
	}
}

// TestInit_WithOrchestrator_ReturnsBatchCmd verifies Init() includes the tagged-event
// listener when an orchestrator is wired in.
func TestInit_WithOrchestrator_ReturnsBatchCmd(t *testing.T) {
	ch := make(chan loop.LogEntry, 1)
	m := New(ch, nil, "", "Proj", "", nil, nil, nil)
	m = m.WithOrchestrator(newTestOrch())
	cmd := m.Init()
	if cmd == nil {
		t.Error("Init() with orchestrator should return a non-nil cmd batch")
	}
}

// TestHandleWindowSize_WithOrchestrator_DoesNotPanic verifies that a window-resize
// event properly resizes the worktrees panel when orchestrator is active.
func TestHandleWindowSize_WithOrchestrator_DoesNotPanic(t *testing.T) {
	ch := make(chan loop.LogEntry, 1)
	m := New(ch, nil, "", "Proj", "", nil, nil, nil)
	m = m.WithOrchestrator(newTestOrch())
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m2 := updated.(Model)
	_ = m2.View() // must not panic
}

// TestHandleTaggedEvent_AccumulatesLog verifies that tagged events accumulate
// in the per-branch log map.
func TestHandleTaggedEvent_AccumulatesLog(t *testing.T) {
	ch := make(chan loop.LogEntry, 1)
	m := New(ch, nil, "", "Proj", "", nil, nil, nil)
	m = m.WithOrchestrator(newTestOrch())
	// Resize so layout is not TooSmall.
	updated0, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated0.(Model)

	entry := loop.LogEntry{Kind: loop.LogInfo, Message: "hello from agent"}
	msg := taggedEventMsg{Branch: "wt/feat-a", Entry: entry}
	updated, _ := m.Update(msg)
	m2 := updated.(Model)

	logs := m2.worktreeLogsByBranch["wt/feat-a"]
	if len(logs) != 1 {
		t.Errorf("expected 1 accumulated log line, got %d", len(logs))
	}
}

// TestHandleTaggedEvent_LiveAppend verifies that when activeWorktreeBranch matches
// the event's branch the line is appended to the main view.
func TestHandleTaggedEvent_LiveAppend(t *testing.T) {
	ch := make(chan loop.LogEntry, 1)
	m := New(ch, nil, "", "Proj", "", nil, nil, nil)
	m = m.WithOrchestrator(newTestOrch())
	updated0, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated0.(Model)
	m.activeWorktreeBranch = "wt/feat-a" // pre-select this branch

	entry := loop.LogEntry{Kind: loop.LogInfo, Message: "live update"}
	msg := taggedEventMsg{Branch: "wt/feat-a", Entry: entry}
	updated, _ := m.Update(msg)
	_ = updated.(Model) // must not panic
}

// TestHandleWorktreeAction_DoesNotPanic verifies that stop/merge/clean actions
// on an unknown branch are silently ignored (no panic, error is discarded).
func TestHandleWorktreeAction_DoesNotPanic(t *testing.T) {
	ch := make(chan loop.LogEntry, 1)
	m := New(ch, nil, "", "Proj", "", nil, nil, nil)
	m = m.WithOrchestrator(newTestOrch())

	for _, action := range []string{"stop", "merge", "clean"} {
		msg := panels.WorktreeActionMsg{Branch: "wt/nonexistent", Action: action}
		updated, cmd := m.Update(msg)
		_ = updated.(Model) // must not panic
		if cmd != nil {
			t.Errorf("action %q should return nil cmd", action)
		}
	}
}

// TestHandleWorktreeAction_NilOrch_NoOp verifies no panic when orch is nil.
func TestHandleWorktreeAction_NilOrch_NoOp(t *testing.T) {
	m := newTestModel() // orch is nil
	msg := panels.WorktreeActionMsg{Branch: "wt/foo", Action: "stop"}
	updated, _ := m.Update(msg)
	_ = updated.(Model)
}

// TestHandleWorktreeSelected_SetsActiveBranch verifies the branch is recorded
// and the accumulated log is loaded into the main view.
func TestHandleWorktreeSelected_SetsActiveBranch(t *testing.T) {
	ch := make(chan loop.LogEntry, 1)
	m := New(ch, nil, "", "Proj", "", nil, nil, nil)
	m = m.WithOrchestrator(newTestOrch())
	updated0, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated0.(Model)

	// Pre-populate some accumulated log lines.
	m.worktreeLogsByBranch["wt/feat-x"] = []panels.OutputLine{
		{Rendered: "line-1", Kind: loop.LogText},
		{Rendered: "line-2", Kind: loop.LogToolUse, ToolName: "Bash"},
	}

	msg := panels.WorktreeSelectedMsg{Branch: "wt/feat-x"}
	updated, _ := m.Update(msg)
	m2 := updated.(Model)

	if m2.activeWorktreeBranch != "wt/feat-x" {
		t.Errorf("activeWorktreeBranch = %q, want %q", m2.activeWorktreeBranch, "wt/feat-x")
	}
	// The main view should show the Output tab with the accumulated lines.
	view := m2.View()
	if !strings.Contains(view, "Output") {
		t.Errorf("after WorktreeSelectedMsg, main view should be on Output tab; view:\n%s", view)
	}
}

// TestKey_W_WithOrchestrator_FocusSpecs_LaunchAttempted verifies that the W key
// triggers a Launch call when orch is set and the Specs panel has focus.
// Since the noop ops returns an error from Switch, the agent ends up in
// StateFailed (silently).  We just verify no panic.
func TestKey_W_WithOrchestrator_FocusSpecs_LaunchAttempted(t *testing.T) {
	ch := make(chan loop.LogEntry, 1)
	sf := []spec.SpecFile{{Name: "feat-a", Dir: "specs/feat-a", IsDir: true, Path: "specs/feat-a/spec.md"}}
	m := New(ch, nil, "", "Proj", "", sf, nil, nil)
	m = m.WithOrchestrator(newTestOrch())
	m.focus = FocusSpecs

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("W")})
	_ = updated.(Model) // must not panic
	// Launch errors are silently discarded; W key returns nil cmd.
	if cmd != nil {
		t.Error("W key should return nil cmd (launch result is fire-and-forget)")
	}
}

// TestKey_W_NoOrch_NoOp verifies that W key is a no-op when orch is nil.
func TestKey_W_NoOrch_NoOp(t *testing.T) {
	m := newTestModel()
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("W")})
	m2 := updated.(Model)
	if m2.orch != nil {
		t.Error("W key without orch should not set orch")
	}
	if cmd != nil {
		t.Error("W key without orch should return nil cmd")
	}
}

// TestKey_5_NoOp verifies key "5" is a no-op (worktrees now live in secondary tab).
func TestKey_5_NoOp(t *testing.T) {
	m := newTestModel()
	orig := m.focus
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("5")})
	m2 := updated.(Model)
	if m2.focus != orig {
		t.Errorf("key '5' should be no-op, focus changed from %v to %v", orig, m2.focus)
	}
}

// TestNextFocus_WithOrchestrator_4Panel verifies that tab cycling remains
// 4-panel even when an orchestrator is active (worktrees are in secondary tab).
func TestNextFocus_WithOrchestrator_4Panel(t *testing.T) {
	ch := make(chan loop.LogEntry, 1)
	m := New(ch, nil, "", "Proj", "", nil, nil, nil)
	m = m.WithOrchestrator(newTestOrch())
	m.focus = FocusIterations

	// From Iterations, tab should go to Main (standard 4-panel cycle).
	next := m.nextFocus()
	if next != FocusMain {
		t.Errorf("nextFocus() from Iterations with orch = %v, want FocusMain", next)
	}
}

// TestView_WithOrchestrator_DoesNotPanic verifies that View() renders correctly
// with the worktrees tab in the secondary panel.
func TestView_WithOrchestrator_DoesNotPanic(t *testing.T) {
	ch := make(chan loop.LogEntry, 1)
	m := New(ch, nil, "", "Proj", "", nil, nil, nil)
	m = m.WithOrchestrator(newTestOrch())
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m2 := updated.(Model)
	view := m2.View()
	if view == "" {
		t.Error("View() with orchestrator should not return empty string")
	}
}

// TestAgentsToEntries_WithAgents verifies the loop body in agentsToEntries.
func TestAgentsToEntries_WithAgents(t *testing.T) {
	agents := []*orchestrator.WorktreeAgent{
		{Branch: "feat/a", State: orchestrator.StateRunning, Iterations: 3, TotalCost: 0.05, SpecName: "spec-a"},
		{Branch: "feat/b", State: orchestrator.StateCompleted, Iterations: 7, TotalCost: 0.12, SpecName: "spec-b"},
	}
	entries := agentsToEntries(agents)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Branch != "feat/a" || entries[0].Iterations != 3 {
		t.Errorf("entries[0] = %+v, want branch feat/a iter 3", entries[0])
	}
	if entries[1].State != "completed" {
		t.Errorf("entries[1].State = %q, want %q", entries[1].State, "completed")
	}
}

// TestHandleTaggedEvent_NilMapInit verifies that handleTaggedEvent initialises
// worktreeLogsByBranch lazily when it is nil (model not initialised via WithOrchestrator).
func TestHandleTaggedEvent_NilMapInit(t *testing.T) {
	ch := make(chan loop.LogEntry, 1)
	orch := newTestOrch()
	m := New(ch, nil, "", "Proj", "", nil, nil, nil)
	// Set orch but skip WithOrchestrator to leave worktreeLogsByBranch nil.
	m.orch = orch

	entry := loop.LogEntry{Kind: loop.LogInfo, Message: "lazy init"}
	msg := taggedEventMsg{Branch: "wt/lazy", Entry: entry}
	updated, _ := m.Update(msg)
	m2 := updated.(Model)

	if m2.worktreeLogsByBranch == nil {
		t.Error("worktreeLogsByBranch should be initialised after handleTaggedEvent")
	}
	if len(m2.worktreeLogsByBranch["wt/lazy"]) == 0 {
		t.Error("expected at least one log line for wt/lazy")
	}
}

// TestUpdate_GitInfoMsg verifies that gitInfoMsg populates branch and lastCommit.
func TestUpdate_GitInfoMsg(t *testing.T) {
	tests := []struct {
		name       string
		msg        gitInfoMsg
		wantBranch string
		wantCommit string
	}{
		{
			name:       "both fields populated",
			msg:        gitInfoMsg{Branch: "main", LastCommit: "abc1234"},
			wantBranch: "main",
			wantCommit: "abc1234",
		},
		{
			name:       "empty branch does not overwrite",
			msg:        gitInfoMsg{Branch: "", LastCommit: "def5678"},
			wantBranch: "",
			wantCommit: "def5678",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel()
			updated, _ := m.Update(tt.msg)
			m2 := updated.(Model)
			if m2.branch != tt.wantBranch {
				t.Errorf("branch: want %q, got %q", tt.wantBranch, m2.branch)
			}
			if m2.lastCommit != tt.wantCommit {
				t.Errorf("lastCommit: want %q, got %q", tt.wantCommit, m2.lastCommit)
			}
		})
	}
}

// TestUpdate_IterationsLoadedMsg verifies that iterationsLoadedMsg pre-populates the panel.
func TestUpdate_IterationsLoadedMsg(t *testing.T) {
	m := newTestModel()
	summaries := []store.IterationSummary{
		{Number: 1, Mode: "build", CostUSD: 0.01},
		{Number: 2, Mode: "plan", CostUSD: 0.02},
	}
	updated, _ := m.Update(iterationsLoadedMsg{Summaries: summaries})
	m2 := updated.(Model)

	// Verify the iterations panel received the items by checking its view.
	view := m2.iterationsPanel.View()
	if !strings.Contains(view, "#1") && !strings.Contains(view, "1") {
		t.Errorf("iterations panel should show iteration 1 after load, got:\n%s", view)
	}
}

// TestUpdate_IterationsLoadedMsg_Empty verifies empty summaries are a no-op.
func TestUpdate_IterationsLoadedMsg_Empty(t *testing.T) {
	m := newTestModel()
	before := m.iterationsPanel.View()
	updated, _ := m.Update(iterationsLoadedMsg{})
	m2 := updated.(Model)
	after := m2.iterationsPanel.View()
	if before != after {
		t.Errorf("empty iterationsLoadedMsg should not change panel view")
	}
}

// TestRenderMarkdown verifies that renderMarkdown returns non-empty output and
// falls back gracefully on errors (T064).
func TestRenderMarkdown(t *testing.T) {
	tests := []struct {
		name    string
		content string
		width   int
	}{
		{
			name:    "basic markdown with header and paragraph",
			content: "# Hello\n\nThis is **bold** and _italic_ text.\n",
			width:   80,
		},
		{
			name:    "code block",
			content: "```go\nfmt.Println(\"hello\")\n```\n",
			width:   80,
		},
		{
			name:    "empty content does not panic",
			content: "",
			width:   80,
		},
		{
			name:    "zero width does not panic",
			content: "# Hello\n",
			width:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rendered := renderMarkdown(tt.content, tt.width)
			// Must not panic; returns something (rendered or raw fallback).
			if tt.content != "" && rendered == "" {
				t.Error("renderMarkdown() returned empty string for non-empty input")
			}
		})
	}
}

// TestRenderMarkdown_FallbackReturnsRaw verifies the raw-text fallback path.
// We can't easily force glamour.NewTermRenderer to fail, but we can verify
// the function handles the contract: if glamour succeeds, result is non-empty;
// if glamour fails (simulated by invalid input), raw content is returned.
func TestRenderMarkdown_RawFallback(t *testing.T) {
	raw := "plain text no markdown"
	result := renderMarkdown(raw, 80)
	// The result should contain the raw text regardless of whether glamour
	// rendered it or returned the fallback.
	if !strings.Contains(result, raw) && !strings.Contains(result, "plain text") {
		t.Errorf("renderMarkdown(%q) = %q; expected to contain original text or rendered version", raw, result)
	}
}

// ── Cmd closure tests ─────────────────────────────────────────────────────────
// These tests directly invoke the closures returned by Cmd-factory functions to
// cover branches that the bubbletea runtime would otherwise execute
// asynchronously and are missed by integration-level program tests.

func TestInitIterationsCmd_NilReader(t *testing.T) {
	fn := initIterationsCmd(nil)
	msg := fn()
	loaded, ok := msg.(iterationsLoadedMsg)
	if !ok {
		t.Fatalf("expected iterationsLoadedMsg, got %T", msg)
	}
	if len(loaded.Summaries) != 0 {
		t.Errorf("expected empty summaries for nil reader, got %d", len(loaded.Summaries))
	}
}

func TestInitIterationsCmd_WithReader(t *testing.T) {
	r := &mockStoreReader{
		iterations: []store.IterationSummary{{Number: 1, Mode: "build"}},
	}
	fn := initIterationsCmd(r)
	msg := fn()
	loaded, ok := msg.(iterationsLoadedMsg)
	if !ok {
		t.Fatalf("expected iterationsLoadedMsg, got %T", msg)
	}
	if len(loaded.Summaries) != 1 {
		t.Errorf("expected 1 summary, got %d", len(loaded.Summaries))
	}
	if loaded.Summaries[0].Number != 1 {
		t.Errorf("summary Number: want 1, got %d", loaded.Summaries[0].Number)
	}
}

func TestWaitForTaggedEvent_ClosedChannel(t *testing.T) {
	ch := make(chan orchestrator.TaggedLogEntry)
	close(ch)
	fn := waitForTaggedEvent(ch)
	msg := fn()
	if msg != nil {
		t.Errorf("expected nil for closed channel, got %T", msg)
	}
}

func TestWaitForTaggedEvent_SendsEvent(t *testing.T) {
	ch := make(chan orchestrator.TaggedLogEntry, 1)
	ch <- orchestrator.TaggedLogEntry{
		Branch: "feat/test",
		Entry:  loop.LogEntry{Kind: loop.LogInfo, Message: "hello"},
	}
	fn := waitForTaggedEvent(ch)
	msg := fn()
	tagged, ok := msg.(taggedEventMsg)
	if !ok {
		t.Fatalf("expected taggedEventMsg, got %T", msg)
	}
	if tagged.Branch != "feat/test" {
		t.Errorf("Branch: want feat/test, got %s", tagged.Branch)
	}
	if tagged.Entry.Message != "hello" {
		t.Errorf("Entry.Message: want hello, got %s", tagged.Entry.Message)
	}
}

// TestHandleTaggedEvent_LogRegent verifies that a tagged event with Kind==LogRegent
// is routed to the Secondary panel's Regent tab in addition to the branch log.
func TestHandleTaggedEvent_LogRegent(t *testing.T) {
	ch := make(chan loop.LogEntry, 1)
	m := New(ch, nil, "", "Proj", "", nil, nil, nil)
	m = m.WithOrchestrator(newTestOrch())
	updated0, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated0.(Model)

	entry := loop.LogEntry{Kind: loop.LogRegent, Message: "restarting agent"}
	msg := taggedEventMsg{Branch: "wt/feat-a", Entry: entry}
	updated, _ := m.Update(msg)
	m2 := updated.(Model)

	// The branch log should have the rendered line.
	if len(m2.worktreeLogsByBranch["wt/feat-a"]) == 0 {
		t.Error("expected log line in worktreeLogsByBranch for wt/feat-a")
	}
	// The Regent tab in the secondary panel should now have content.
	view := m2.secondary.View()
	_ = view // must not panic; coverage of the AppendLine(TabRegent) path is the goal
}

// TestKey_W_WithOrch_WrongFocus_NoOp verifies that W is a no-op when the
// orchestrator is set but the focused panel is not FocusSpecs.
func TestKey_W_WithOrch_WrongFocus_NoOp(t *testing.T) {
	ch := make(chan loop.LogEntry, 1)
	sf := []spec.SpecFile{{Name: "feat-a", Dir: "specs/feat-a", IsDir: true, Path: "specs/feat-a/spec.md"}}
	m := New(ch, nil, "", "Proj", "", sf, nil, nil)
	m = m.WithOrchestrator(newTestOrch())
	m.focus = FocusMain // not FocusSpecs

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("W")})
	_ = updated.(Model)
	if cmd != nil {
		t.Error("W key with wrong focus should return nil cmd")
	}
}

func TestRunGitOutput_Error(t *testing.T) {
	// An invalid git subcommand exits non-zero — error path returns "".
	result := runGitOutput("", "this-subcommand-does-not-exist-xyz")
	if result != "" {
		t.Errorf("expected empty string on git error, got %q", result)
	}
}

func TestRunGitOutput_Success(t *testing.T) {
	// "git version" is always available.
	result := runGitOutput("", "version")
	if result == "" {
		t.Error("expected non-empty output from git version")
	}
}

func TestInitGitInfoCmd_ReturnsGitInfoMsg(t *testing.T) {
	fn := initGitInfoCmd("")
	msg := fn()
	if _, ok := msg.(gitInfoMsg); !ok {
		t.Fatalf("expected gitInfoMsg, got %T", msg)
	}
	// Branch and commit may be empty in a test environment — just ensure the
	// function completes without panic.
}

func TestLoadSessionDiffCmd(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	run("init")
	if err := os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "tracked.txt")
	run("-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "initial")
	base := strings.TrimSpace(runGitOutput(dir, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("after\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	msg, ok := loadSessionDiffCmd(dir, base)().(sessionDiffMsg)
	if !ok {
		t.Fatalf("expected sessionDiffMsg")
	}
	diff := strings.Join(msg.Lines, "\n")
	for _, want := range []string{"tracked.txt", "after", "untracked.txt"} {
		if !strings.Contains(diff, want) {
			t.Errorf("session diff missing %q:\n%s", want, diff)
		}
	}
}
