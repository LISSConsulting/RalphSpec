package tui

import "testing"

func TestIsGlobalKey(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"tab", true},
		{"shift+tab", true},
		{"1", true},
		{"2", true},
		{"3", true},
		{"4", true},
		{"q", true},
		{"ctrl+c", true},
		{"s", true},
		{"i", true},
		{"I", true},
		{"?", true},
		// Not global
		{"j", false},
		{"k", false},
		{"enter", false},
		{"f", false},
		{"[", false},
		{"]", false},
		{"e", false},
		{"n", false},
		{"", false},
		{"ctrl+d", false},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got := IsGlobalKey(tt.key)
			if got != tt.want {
				t.Errorf("IsGlobalKey(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

func TestPanelKeys(t *testing.T) {
	tests := []struct {
		focus FocusTarget
		must  []string // keys that must be present
	}{
		{FocusSpecs, []string{"j", "k", "enter", "e", "n"}},
		{FocusIterations, []string{"j", "k", "enter"}},
		{FocusMain, []string{"f", "[", "]", "{", "}", "g", "G", "ctrl+u", "ctrl+d", "j", "k"}},
		{FocusSecondary, []string{"[", "]", "j", "k"}},
	}

	for _, tt := range tests {
		t.Run(tt.focus.String(), func(t *testing.T) {
			keys := PanelKeys(tt.focus)
			keySet := make(map[string]bool, len(keys))
			for _, k := range keys {
				keySet[k] = true
			}
			for _, want := range tt.must {
				if !keySet[want] {
					t.Errorf("PanelKeys(%v) missing key %q; got %v", tt.focus, want, keys)
				}
			}
		})
	}
}

func TestPanelKeys_NonOverlapWithGlobal(t *testing.T) {
	// Panel keys should not overlap with global keys for the same focus target,
	// except for single-letter keys that may be reused (tab is handled globally).
	for _, focus := range []FocusTarget{FocusSpecs, FocusIterations, FocusMain, FocusSecondary} {
		for _, pk := range PanelKeys(focus) {
			// "s" and "q" are global but also checked here as panel keys should
			// only conflict knowingly. Verify tab and ctrl+c are NOT in panel keys.
			if pk == "tab" || pk == "ctrl+c" {
				t.Errorf("focus %v panel key %q conflicts with critical global key", focus, pk)
			}
		}
	}
}
