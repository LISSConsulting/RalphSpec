package panels

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/LISSConsulting/RalphSpec/internal/loop"
)

func TestNewMainView(t *testing.T) {
	mv := NewMainView(80, 20)
	if mv.activeTab != TabOutput {
		t.Errorf("activeTab: got %v, want TabOutput", mv.activeTab)
	}
}

func TestMainView_AppendLine(t *testing.T) {
	mv := NewMainView(80, 20)
	mv = mv.AppendLine("line 1")
	mv = mv.AppendLine("line 2")

	view := mv.View()
	for _, want := range []string{"line 1", "line 2"} {
		if !strings.Contains(view, want) {
			t.Errorf("View() missing %q; got %q", want, view)
		}
	}
}

func TestMainView_ShowSpec(t *testing.T) {
	mv := NewMainView(80, 20)
	mv = mv.ShowSpec("# Spec Title\n\nSome content.")
	if mv.activeTab != TabSpecContent {
		t.Errorf("ShowSpec should switch to TabSpecContent, got %v", mv.activeTab)
	}
	view := mv.View()
	if !strings.Contains(view, "Spec Title") {
		t.Errorf("View() after ShowSpec missing content; got %q", view)
	}
}

func TestMainView_ShowIterationLog(t *testing.T) {
	mv := NewMainView(80, 20)
	mv = mv.ShowIterationLog([]string{"[12:00:00]  tool call", "[12:00:01]  result"})
	if mv.activeTab != TabIterationDetail {
		t.Errorf("ShowIterationLog should switch to TabIterationDetail, got %v", mv.activeTab)
	}
	view := mv.View()
	if !strings.Contains(view, "tool call") {
		t.Errorf("View() after ShowIterationLog missing content; got %q", view)
	}
}

func TestMainView_SwitchToOutput(t *testing.T) {
	mv := NewMainView(80, 20)
	mv = mv.ShowSpec("content")
	mv = mv.SwitchToOutput()
	if mv.activeTab != TabOutput {
		t.Errorf("SwitchToOutput should return to TabOutput, got %v", mv.activeTab)
	}
}

func TestMainView_TabSwitching(t *testing.T) {
	mv := NewMainView(80, 20)
	// ] should cycle forward
	mv2, _ := mv.Update(keyMsg("]"))
	if mv2.activeTab == mv.activeTab {
		t.Error("'] key should change active tab")
	}
}

func TestMainView_SetSize(t *testing.T) {
	mv := NewMainView(80, 20)
	mv = mv.SetSize(100, 30)
	if mv.width != 100 || mv.height != 30 {
		t.Errorf("SetSize: got %dx%d, want 100x30", mv.width, mv.height)
	}
}

func TestMainView_View_ContainsTabBar(t *testing.T) {
	mv := NewMainView(80, 20)
	view := mv.View()
	if !strings.Contains(view, "Output") {
		t.Errorf("View() should contain tab label 'Output'; got %q", view)
	}
}

func TestMainView_SummaryTab(t *testing.T) {
	summaryLines := []string{
		"Iteration:   3",
		"Mode:        build",
		"Cost:        $0.0234",
		"Duration:    45.2s",
		"Exit:        success",
		"Commit:      abc1234",
	}

	mv := NewMainView(80, 20)
	mv = mv.SetIterationSummary(summaryLines)

	// Tab is still on Output after SetIterationSummary (no auto-switch).
	if mv.activeTab != TabOutput {
		t.Errorf("SetIterationSummary should not change activeTab, got %v", mv.activeTab)
	}

	// Cycle ] three times: Output → Spec → Iteration → Summary.
	mv, _ = mv.Update(keyMsg("]"))
	mv, _ = mv.Update(keyMsg("]"))
	mv, _ = mv.Update(keyMsg("]"))
	if mv.activeTab != TabIterationSummary {
		t.Errorf("after 3x ], expected TabIterationSummary, got %v", mv.activeTab)
	}

	view := mv.View()
	for _, want := range summaryLines {
		if !strings.Contains(view, want) {
			t.Errorf("View() missing %q; got:\n%s", want, view)
		}
	}
}

func TestNewMainView_ZeroHeight(t *testing.T) {
	// contentH < 1 clamp branch: should not panic.
	mv := NewMainView(80, 0)
	_ = mv.View()
}

func TestMainView_SetSize_ZeroHeight(t *testing.T) {
	// contentH < 1 clamp branch in SetSize.
	mv := NewMainView(80, 20)
	mv = mv.SetSize(100, 0)
	_ = mv.View()
}

func TestMainView_Update_PrevTab(t *testing.T) {
	mv := NewMainView(80, 20)
	mv, _ = mv.Update(keyMsg("]")) // advance to Spec tab
	prev, _ := mv.Update(keyMsg("["))
	if prev.activeTab == mv.activeTab {
		t.Error("'[' key should navigate to the previous tab")
	}
}

func TestMainView_Update_FKey(t *testing.T) {
	// 'f' on output tab toggles follow (non-summary tab branch).
	mv := NewMainView(80, 20)
	mv2, _ := mv.Update(keyMsg("f"))
	_ = mv2.View()

	// 'f' on summary tab is a no-op for follow (summary tab branch).
	mv = NewMainView(80, 20)
	mv, _ = mv.Update(keyMsg("]"))
	mv, _ = mv.Update(keyMsg("]"))
	mv, _ = mv.Update(keyMsg("]")) // TabIterationSummary
	if mv.activeTab != TabIterationSummary {
		t.Fatalf("setup: expected TabIterationSummary, got %v", mv.activeTab)
	}
	mv2, _ = mv.Update(keyMsg("f"))
	_ = mv2.View()
}

func TestMainView_Update_NonKeyMsg(t *testing.T) {
	wm := tea.WindowSizeMsg{Width: 100, Height: 30}

	// Non-key message on output tab (default outer switch — else branch).
	mv := NewMainView(80, 20)
	mv2, _ := mv.Update(wm)
	_ = mv2.View()

	// Non-key message on summary tab (default outer switch — if branch).
	mv = NewMainView(80, 20)
	mv, _ = mv.Update(keyMsg("]"))
	mv, _ = mv.Update(keyMsg("]"))
	mv, _ = mv.Update(keyMsg("]")) // TabIterationSummary
	mv2, _ = mv.Update(wm)
	_ = mv2.View()
}

func TestSplitLines_TrailingNewline(t *testing.T) {
	// A string ending with '\n' should not produce a spurious empty last element.
	got := splitLines("line1\nline2\n")
	if len(got) != 2 {
		t.Errorf("splitLines with trailing newline: expected 2 lines, got %d: %v", len(got), got)
	}
	if got[0] != "line1" || got[1] != "line2" {
		t.Errorf("splitLines: unexpected content: %v", got)
	}
}

func TestSplitLines_Empty(t *testing.T) {
	if got := splitLines(""); got != nil {
		t.Errorf("splitLines(\"\") = %v, want nil", got)
	}
}

func TestMainView_Update_DefaultKey_NonSummaryTab(t *testing.T) {
	// A key not matching ], [, or f on a non-summary tab is forwarded to the
	// logview (the else branch of the default key handler).
	mv := NewMainView(80, 20) // starts on TabOutput (non-summary)
	mv2, _ := mv.Update(keyMsg("g"))
	_ = mv2.View()
}

func TestMainView_SummaryTab_ContainsLabel(t *testing.T) {
	mv := NewMainView(80, 20)
	view := mv.View()
	if !strings.Contains(view, "Summary") {
		t.Errorf("View() should contain tab label 'Summary'; got %q", view)
	}
}

func TestMainView_SummaryTab_ScrollDelegation(t *testing.T) {
	// Verify scroll keys are delegated to summaryLogview when on summary tab.
	mv := NewMainView(80, 20)
	mv = mv.SetIterationSummary([]string{"line1", "line2"})
	// Advance to TabIterationSummary.
	mv, _ = mv.Update(keyMsg("]"))
	mv, _ = mv.Update(keyMsg("]"))
	mv, _ = mv.Update(keyMsg("]"))
	// Sending a scroll key should not panic.
	mv, _ = mv.Update(keyMsg("j"))
	_ = mv.View()
}

func TestMainView_ShowWorktreeLog_SetsContent(t *testing.T) {
	mv := NewMainView(80, 20)
	lines := []OutputLine{
		{Rendered: "line A", Kind: loop.LogText},
		{Rendered: "line B", Kind: loop.LogToolUse, ToolName: "Bash"},
		{Rendered: "line C", Kind: loop.LogToolUse, ToolName: "Edit"},
	}
	mv2 := mv.ShowWorktreeLog(lines)
	view := mv2.View()
	// After ShowWorktreeLog the output tab is active; view must not be empty.
	if view == "" {
		t.Error("View() after ShowWorktreeLog should not be empty")
	}
}

func TestMainView_OutputFilters(t *testing.T) {
	mv := NewMainView(80, 20)
	for _, line := range []OutputLine{
		{Rendered: "general line", Kind: loop.LogInfo},
		{Rendered: "thinking line", Kind: loop.LogText},
		{Rendered: "command line", Kind: loop.LogToolUse, ToolName: "PowerShell"},
		{Rendered: "edit line", Kind: loop.LogToolUse, ToolName: "apply_patch"},
		{Rendered: "read line", Kind: loop.LogToolUse, ToolName: "Read"},
	} {
		mv = mv.AppendOutput(line)
	}

	tests := []struct {
		filter OutputFilter
		want   string
		absent []string
	}{
		{FilterThinking, "thinking line", []string{"general line", "command line", "edit line", "read line"}},
		{FilterCommands, "command line", []string{"general line", "thinking line", "edit line", "read line"}},
		{FilterEdits, "edit line", []string{"general line", "thinking line", "command line", "read line"}},
	}
	for _, tt := range tests {
		mv.outputFilter = tt.filter
		mv.filterbar = mv.filterbar.SetActive(int(tt.filter))
		view := mv.View()
		if !strings.Contains(view, tt.want) {
			t.Errorf("filter %v missing %q; got %q", tt.filter, tt.want, view)
		}
		for _, absent := range tt.absent {
			if strings.Contains(view, absent) {
				t.Errorf("filter %v unexpectedly contains %q; got %q", tt.filter, absent, view)
			}
		}
	}
}

func TestMainView_DiffFilter(t *testing.T) {
	mv := NewMainView(80, 20).SetSessionDiff([]string{"diff --git a/main.go b/main.go", "+new line"})
	for i := 0; i < int(FilterDiff); i++ {
		mv, _ = mv.Update(keyMsg("}"))
	}
	if !mv.ShowingSessionDiff() {
		t.Fatal("expected Diff filter to be active")
	}
	view := mv.View()
	if !strings.Contains(view, "diff --git") || !strings.Contains(view, "+new line") {
		t.Errorf("Diff filter missing session diff; got %q", view)
	}
}

func TestMainView_OutputFilterPrevWraps(t *testing.T) {
	mv, _ := NewMainView(80, 20).Update(keyMsg("{"))
	if !mv.ShowingSessionDiff() {
		t.Fatalf("expected { to wrap from All to Diff, got filter %v", mv.outputFilter)
	}
}

// TestMainView_AppendLine_DoesNotAffectSpecLog verifies that appending output
// lines never displaces or contaminates the spec tab's independent buffer.
// This is the key invariant of the per-tab LogView refactor (T039).
func TestMainView_AppendLine_DoesNotAffectSpecLog(t *testing.T) {
	mv := NewMainView(80, 20)

	// Load spec content — tab switches to TabSpecContent.
	mv = mv.ShowSpec("# My Spec\n\nSome spec details here.")

	// Stream several output lines (simulating ongoing loop activity).
	mv = mv.AppendLine("output line 1")
	mv = mv.AppendLine("output line 2")
	mv = mv.AppendLine("output line 3")

	// Spec tab must still show spec content, not the output lines.
	specView := mv.View()
	if !strings.Contains(specView, "My Spec") {
		t.Errorf("Spec tab should still contain spec content after AppendLine; got: %q", specView)
	}
	for _, bad := range []string{"output line 1", "output line 2", "output line 3"} {
		if strings.Contains(specView, bad) {
			t.Errorf("Spec tab must not contain output line %q; got: %q", bad, specView)
		}
	}

	// Switch to output tab — output lines must be visible, spec text must not be.
	mv = mv.SwitchToOutput()
	outView := mv.View()
	for _, want := range []string{"output line 1", "output line 2", "output line 3"} {
		if !strings.Contains(outView, want) {
			t.Errorf("Output tab missing %q after AppendLine; got: %q", want, outView)
		}
	}
	if strings.Contains(outView, "My Spec") {
		t.Errorf("Output tab must not contain spec content; got: %q", outView)
	}
}
