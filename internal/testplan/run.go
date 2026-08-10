package testplan

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

type StepResult struct {
	Step   Step   `json:"step"`
	Passed bool   `json:"passed"`
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

type Result struct {
	Passed bool         `json:"passed"`
	Steps  []StepResult `json:"steps"`
}

// Run executes every high-confidence step in deterministic plan order.
func Run(ctx context.Context, plan Plan) Result {
	result := Result{Passed: len(plan.Steps) > 0}
	for _, step := range plan.Steps {
		if step.Confidence != ConfidenceHigh {
			continue
		}
		stepResult := runStep(ctx, step)
		result.Steps = append(result.Steps, stepResult)
		if !stepResult.Passed {
			result.Passed = false
			break
		}
	}
	if len(result.Steps) == 0 {
		result.Passed = false
	}
	return result
}

func runStep(ctx context.Context, step Step) StepResult {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		shell := os.Getenv("COMSPEC")
		if shell == "" {
			shell = "cmd.exe"
		}
		cmd = exec.CommandContext(ctx, shell, "/D", "/S", "/C", step.Command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", step.Command)
	}
	cmd.Dir = step.Dir
	var combined bytes.Buffer
	cmd.Stdout = &combined
	cmd.Stderr = &combined
	err := cmd.Run()
	stepResult := StepResult{Step: step, Passed: err == nil, Output: combined.String()}
	if err != nil {
		stepResult.Error = fmt.Sprintf("%v", err)
	}
	return stepResult
}
