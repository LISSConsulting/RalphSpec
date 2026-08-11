package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/LISSConsulting/RalphSpec/internal/quota"
)

type statusLineSnapshot struct {
	RateLimits struct {
		FiveHour *statusLineWindow `json:"five_hour"`
		SevenDay *statusLineWindow `json:"seven_day"`
	} `json:"rate_limits"`
}

type statusLineWindow struct {
	UsedPercentage float64         `json:"used_percentage"`
	ResetsAt       json.RawMessage `json:"resets_at"`
}

// ReadQuotaSnapshot ingests the documented Claude Code status-line JSON shape
// from a file maintained by the operator's status-line script.
func ReadQuotaSnapshot(path string, maxAge time.Duration) (quota.Snapshot, error) {
	if strings.TrimSpace(path) == "" {
		return quota.Snapshot{}, fmt.Errorf("claude quota snapshot file is not configured")
	}
	info, err := os.Stat(path)
	if err != nil {
		return quota.Snapshot{}, fmt.Errorf("stat Claude quota snapshot: %w", err)
	}
	if maxAge > 0 && time.Since(info.ModTime()) > maxAge {
		return quota.Snapshot{}, fmt.Errorf("claude quota snapshot is stale (modified %s)", info.ModTime().Format(time.RFC3339))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return quota.Snapshot{}, fmt.Errorf("read Claude quota snapshot: %w", err)
	}
	var input statusLineSnapshot
	if err := json.Unmarshal(data, &input); err != nil {
		return quota.Snapshot{}, fmt.Errorf("decode Claude quota snapshot: %w", err)
	}

	snapshot := quota.Snapshot{Agent: "claude", Provider: "anthropic", ObservedAt: info.ModTime()}
	appendWindow := func(name string, value *statusLineWindow, duration time.Duration) error {
		if value == nil {
			return nil
		}
		resetAt, err := parseResetAt(value.ResetsAt)
		if err != nil {
			return fmt.Errorf("%s reset: %w", name, err)
		}
		snapshot.Windows = append(snapshot.Windows, quota.Window{
			Name: name, UsedPercent: value.UsedPercentage, ResetAt: resetAt, Duration: duration,
		})
		return nil
	}
	if err := appendWindow("five_hour", input.RateLimits.FiveHour, 5*time.Hour); err != nil {
		return quota.Snapshot{}, err
	}
	if err := appendWindow("seven_day", input.RateLimits.SevenDay, 7*24*time.Hour); err != nil {
		return quota.Snapshot{}, err
	}
	if len(snapshot.Windows) == 0 {
		return quota.Snapshot{}, fmt.Errorf("claude quota snapshot contains no rate_limits windows")
	}
	return snapshot, nil
}

func parseResetAt(raw json.RawMessage) (time.Time, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return time.Time{}, nil
	}
	var unix int64
	if err := json.Unmarshal(raw, &unix); err == nil {
		return time.Unix(unix, 0), nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return time.Time{}, fmt.Errorf("invalid reset timestamp")
	}
	if seconds, err := strconv.ParseInt(text, 10, 64); err == nil {
		return time.Unix(seconds, 0), nil
	}
	parsed, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid reset timestamp %q", text)
	}
	return parsed, nil
}
