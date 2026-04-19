// Package regent provides the Regent supervisor that watches the Ralph loop,
// detects crashes/hangs, rolls back bad commits, and restarts on failure.
package regent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// State tracks the Regent's operational state, persisted to .ralph/regent-state.json.
type State struct {
	RalphPID        int       `json:"ralph_pid"`
	Iteration       int       `json:"iteration"`
	ConsecutiveErrs int       `json:"consecutive_errors"`
	LastOutputAt    time.Time `json:"last_output_at"`
	Agent           string    `json:"agent"`
	LastCommit      string    `json:"last_commit"`
	TotalCostUSD    float64   `json:"total_cost_usd"`
	Branch          string    `json:"branch"`
	Mode            string    `json:"mode"`
	StartedAt       time.Time `json:"started_at"`
	FinishedAt      time.Time `json:"finished_at"`
	Passed          bool      `json:"passed"`
}

// stateFileName is the path within the .ralph directory.
const stateFileName = "regent-state.json"

// stateDirName is the directory that holds the state file.
const stateDirName = ".ralph"

// LoadState reads the Regent state from .ralph/regent-state.json in dir.
// Returns a zero State (not an error) if the file does not exist.
func LoadState(dir string) (State, error) {
	path := filepath.Join(dir, stateDirName, stateFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return State{}, nil
		}
		return State{}, fmt.Errorf("regent: read state: %w", err)
	}

	var s State
	if jsonErr := json.Unmarshal(data, &s); jsonErr != nil {
		return State{}, fmt.Errorf("regent: parse state: %w", jsonErr)
	}
	return s, nil
}

// SaveState writes the Regent state to .ralph/regent-state.json in dir.
// Creates the .ralph directory if it does not exist.
// Uses a write-then-rename pattern so concurrent callers never observe a
// partially-written file.
func SaveState(dir string, s State) error {
	stateDir := filepath.Join(dir, stateDirName)
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return fmt.Errorf("regent: create state dir: %w", err)
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("regent: marshal state: %w", err)
	}

	tmp, err := os.CreateTemp(stateDir, ".regent-state-*.tmp")
	if err != nil {
		return fmt.Errorf("regent: create temp state: %w", err)
	}
	if _, writeErr := tmp.Write(data); writeErr != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("regent: write state: %w", writeErr)
	}
	if closeErr := tmp.Close(); closeErr != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("regent: close state: %w", closeErr)
	}
	path := filepath.Join(stateDir, stateFileName)
	if renameErr := os.Rename(tmp.Name(), path); renameErr != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("regent: finalize state: %w", renameErr)
	}
	return nil
}
