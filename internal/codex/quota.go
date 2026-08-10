package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"time"

	"github.com/LISSConsulting/RalphSpec/internal/quota"
)

type rpcResponse struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type rateLimitsResult struct {
	RateLimits struct {
		Primary              *rateLimitWindow `json:"primary"`
		Secondary            *rateLimitWindow `json:"secondary"`
		RateLimitReachedType *string          `json:"rateLimitReachedType"`
		SpendControlReached  *bool            `json:"spendControlReached"`
		IndividualLimit      *struct {
			RemainingPercent float64 `json:"remainingPercent"`
			ResetsAt         int64   `json:"resetsAt"`
		} `json:"individualLimit"`
	} `json:"rateLimits"`
	SpendControlReached *bool `json:"spendControlReached"`
}

type rateLimitWindow struct {
	UsedPercent       float64 `json:"usedPercent"`
	WindowDurationMin int64   `json:"windowDurationMins"`
	ResetsAt          int64   `json:"resetsAt"`
}

// Quota reads the current ChatGPT Codex rate limits through the documented
// app-server account/rateLimits/read method.
func (a *Agent) Quota(ctx context.Context) (quota.Snapshot, error) {
	exe := a.Executable
	if exe == "" {
		exe = "codex"
	}
	probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(probeCtx, exe, "app-server", "--listen", "stdio://")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return quota.Snapshot{}, fmt.Errorf("codex quota stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return quota.Snapshot{}, fmt.Errorf("codex quota stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return quota.Snapshot{}, fmt.Errorf("codex quota start: %w", err)
	}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Wait()
	}()

	encoder := json.NewEncoder(stdin)
	decoder := bufio.NewScanner(stdout)
	decoder.Buffer(make([]byte, 64*1024), 4*1024*1024)
	if err := encoder.Encode(map[string]any{
		"method": "initialize",
		"id":     1,
		"params": map[string]any{
			"clientInfo": map[string]string{
				"name": "ralphspec", "title": "RalphSpec", "version": "1",
			},
		},
	}); err != nil {
		return quota.Snapshot{}, fmt.Errorf("codex quota initialize: %w", err)
	}
	if _, err := readRPCResponse(decoder, 1); err != nil {
		return quota.Snapshot{}, fmt.Errorf("codex quota initialize: %w", err)
	}
	if err := encoder.Encode(map[string]any{"method": "initialized"}); err != nil {
		return quota.Snapshot{}, fmt.Errorf("codex quota initialized: %w", err)
	}
	if err := encoder.Encode(map[string]any{"method": "account/rateLimits/read", "id": 2}); err != nil {
		return quota.Snapshot{}, fmt.Errorf("codex quota request: %w", err)
	}
	response, err := readRPCResponse(decoder, 2)
	if err != nil {
		return quota.Snapshot{}, fmt.Errorf("codex quota response: %w", err)
	}
	var result rateLimitsResult
	if err := json.Unmarshal(response.Result, &result); err != nil {
		return quota.Snapshot{}, fmt.Errorf("codex quota decode: %w", err)
	}

	snapshot := quota.Snapshot{
		Agent:               "codex",
		Provider:            "openai",
		ObservedAt:          time.Now(),
		SpendControlReached: result.SpendControlReached,
	}
	if snapshot.SpendControlReached == nil {
		snapshot.SpendControlReached = result.RateLimits.SpendControlReached
	}
	if result.RateLimits.RateLimitReachedType != nil {
		snapshot.LimitReached = *result.RateLimits.RateLimitReachedType
	}
	appendWindow := func(name string, window *rateLimitWindow) {
		if window == nil {
			return
		}
		entry := quota.Window{
			Name:        name,
			UsedPercent: window.UsedPercent,
			Duration:    time.Duration(window.WindowDurationMin) * time.Minute,
		}
		if window.ResetsAt > 0 {
			entry.ResetAt = time.Unix(window.ResetsAt, 0)
		}
		snapshot.Windows = append(snapshot.Windows, entry)
	}
	appendWindow("primary", result.RateLimits.Primary)
	appendWindow("secondary", result.RateLimits.Secondary)
	if individual := result.RateLimits.IndividualLimit; individual != nil {
		window := quota.Window{
			Name:        "individual_spend_control",
			UsedPercent: 100 - individual.RemainingPercent,
		}
		if individual.ResetsAt > 0 {
			window.ResetAt = time.Unix(individual.ResetsAt, 0)
		}
		snapshot.Windows = append(snapshot.Windows, window)
	}
	return snapshot, nil
}

func readRPCResponse(scanner *bufio.Scanner, id int) (rpcResponse, error) {
	for scanner.Scan() {
		var response rpcResponse
		if err := json.Unmarshal(scanner.Bytes(), &response); err != nil || response.ID != id {
			continue
		}
		if response.Error != nil {
			return rpcResponse{}, fmt.Errorf("rpc %d: %s", response.Error.Code, response.Error.Message)
		}
		return response, nil
	}
	if err := scanner.Err(); err != nil {
		return rpcResponse{}, err
	}
	return rpcResponse{}, io.ErrUnexpectedEOF
}
