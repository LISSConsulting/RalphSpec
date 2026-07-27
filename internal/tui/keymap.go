package tui

// GlobalKeyBindings lists the keys that are always handled by the root model
// before dispatching to focused panels.
var GlobalKeyBindings = []string{"tab", "shift+tab", "1", "2", "3", "4", "q", "ctrl+c", "s", "?", "b", "p", "R", "x"}

// panelKeys maps each FocusTarget to the keys that panel handles internally.
var panelKeys = map[FocusTarget][]string{
	FocusSpecs:      {"j", "k", "enter", "e", "n"},
	FocusIterations: {"j", "k", "enter"},
	FocusMain:       {"f", "[", "]", "{", "}", "ctrl+u", "ctrl+d", "j", "k"},
	FocusSecondary:  {"[", "]", "j", "k"},
}

// IsGlobalKey reports whether key is a global keybinding (handled before panel dispatch).
func IsGlobalKey(key string) bool {
	for _, k := range GlobalKeyBindings {
		if k == key {
			return true
		}
	}
	return false
}

// PanelKeys returns the list of keys handled by the given focused panel.
func PanelKeys(focus FocusTarget) []string {
	return panelKeys[focus]
}
