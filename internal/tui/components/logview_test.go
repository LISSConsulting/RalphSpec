package components

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNewLogView(t *testing.T) {
	lv := NewLogView(80, 24)
	if !lv.Following() {
		t.Error("NewLogView: expected follow mode to be enabled by default")
	}
	if lv.width != 80 || lv.height != 24 {
		t.Errorf("dimensions: got %dx%d, want 80x24", lv.width, lv.height)
	}
}

func TestLogView_AppendLine(t *testing.T) {
	lv := NewLogView(80, 10)
	lv = lv.AppendLine("line 1")
	lv = lv.AppendLine("line 2")
	lv = lv.AppendLine("line 3")

	if len(lv.lines) != 3 {
		t.Errorf("expected 3 lines, got %d", len(lv.lines))
	}

	view := lv.View()
	for _, want := range []string{"line 1", "line 2", "line 3"} {
		if !strings.Contains(view, want) {
			t.Errorf("View() missing %q: %q", want, view)
		}
	}
}

func TestLogView_SetContent(t *testing.T) {
	lv := NewLogView(80, 10)
	lv = lv.AppendLine("old line")
	lv = lv.SetContent([]string{"new 1", "new 2"})

	if len(lv.lines) != 2 {
		t.Errorf("expected 2 lines after SetContent, got %d", len(lv.lines))
	}
	if lv.lines[0] != "new 1" || lv.lines[1] != "new 2" {
		t.Errorf("lines mismatch: %v", lv.lines)
	}
}

func TestLogView_SetContent_IndependentCopy(t *testing.T) {
	lv := NewLogView(80, 10)
	original := []string{"a", "b"}
	lv = lv.SetContent(original)
	original[0] = "mutated"
	if lv.lines[0] != "a" {
		t.Error("SetContent should copy the slice, not reference it")
	}
}

func TestLogView_ToggleFollow(t *testing.T) {
	lv := NewLogView(80, 10)
	if !lv.Following() {
		t.Error("initial follow should be true")
	}
	lv = lv.ToggleFollow()
	if lv.Following() {
		t.Error("after first toggle follow should be false")
	}
	lv = lv.ToggleFollow()
	if !lv.Following() {
		t.Error("after second toggle follow should be true")
	}
}

func TestLogView_GotoTopBottom(t *testing.T) {
	lv := NewLogView(80, 2)
	for i := 0; i < 20; i++ {
		lv = lv.AppendLine(fmt.Sprintf("line %02d", i))
	}

	lv = lv.GotoTop()
	if lv.Following() {
		t.Error("GotoTop should disable follow")
	}
	if lv.vp.YOffset != 0 {
		t.Errorf("GotoTop: YOffset = %d, want 0", lv.vp.YOffset)
	}

	lv = lv.GotoBottom()
	if !lv.Following() {
		t.Error("GotoBottom should enable follow")
	}
	if !lv.vp.AtBottom() {
		t.Error("GotoBottom should scroll to bottom")
	}
}

func TestLogView_SetSize(t *testing.T) {
	lv := NewLogView(80, 10)
	lv = lv.SetSize(100, 20)
	if lv.width != 100 || lv.height != 20 {
		t.Errorf("SetSize: got %dx%d, want 100x20", lv.width, lv.height)
	}
	if lv.vp.Width != 100 || lv.vp.Height != 20 {
		t.Errorf("viewport dimensions: got %dx%d, want 100x20", lv.vp.Width, lv.vp.Height)
	}
}

func TestLogView_View_Empty(t *testing.T) {
	lv := NewLogView(80, 10)
	// Should not panic; returns viewport content (may be empty or whitespace).
	_ = lv.View()
}

// scrollableLV returns a LogView with enough content to be scrollable.
// It then sets the viewport offset to 0 (top) so AtBottom() is false.
func scrollableLV(t *testing.T) LogView {
	t.Helper()
	lv := NewLogView(80, 2)
	for i := 0; i < 20; i++ {
		lv = lv.AppendLine(fmt.Sprintf("line %02d", i))
	}
	// After appending in follow mode the viewport is at the bottom.
	// Force it to the top so AtBottom() returns false.
	lv.vp.YOffset = 0
	if lv.vp.AtBottom() {
		t.Skip("viewport content does not exceed height — cannot test scroll path")
	}
	return lv
}

// TestLogView_Update_KeyMsg verifies that a KeyMsg when not at the bottom
// disables follow mode.
func TestLogView_Update_KeyMsg(t *testing.T) {
	lv := scrollableLV(t)
	if !lv.Following() {
		t.Fatal("precondition: follow should be on")
	}
	lv2, _ := lv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if lv2.Following() {
		t.Error("expected follow mode off after KeyMsg when not at bottom")
	}
}

// TestLogView_Update_MouseMsg verifies that a MouseMsg when not at the bottom
// disables follow mode.
func TestLogView_Update_MouseMsg(t *testing.T) {
	lv := scrollableLV(t)
	lv2, _ := lv.Update(tea.MouseMsg{Button: tea.MouseButtonWheelUp})
	if lv2.Following() {
		t.Error("expected follow mode off after MouseMsg when not at bottom")
	}
}

// TestLogView_Update_NonScrollMsg verifies that a non-Key/Mouse message does
// not disable follow mode even when the viewport is not at the bottom.
func TestLogView_Update_NonScrollMsg(t *testing.T) {
	lv := scrollableLV(t)
	lv2, _ := lv.Update(tea.WindowSizeMsg{Width: 80, Height: 2})
	if !lv2.Following() {
		t.Error("expected follow mode to remain on after non-key/mouse msg")
	}
}

// TestLogView_Update_FollowAlreadyOff verifies that follow remains off
// regardless of message type.
func TestLogView_Update_FollowAlreadyOff(t *testing.T) {
	lv := scrollableLV(t)
	lv = lv.ToggleFollow() // turn off follow
	if lv.Following() {
		t.Fatal("precondition: follow should be off")
	}
	lv2, _ := lv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if lv2.Following() {
		t.Error("expected follow mode to remain off")
	}
}

// TestLogView_Update_AtBottom verifies that follow mode is not disabled when
// the viewport is already at the bottom (content fits in the view height).
func TestLogView_Update_AtBottom(t *testing.T) {
	// Large height so all 3 lines fit — viewport is always at bottom.
	lv := NewLogView(80, 100)
	for i := 0; i < 3; i++ {
		lv = lv.AppendLine(fmt.Sprintf("line %d", i))
	}
	if !lv.vp.AtBottom() {
		t.Skip("expected viewport to be at bottom for this test case")
	}
	lv2, _ := lv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if !lv2.Following() {
		t.Error("expected follow mode to remain on when viewport is at bottom")
	}
}
