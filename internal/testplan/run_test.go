package testplan

import (
	"context"
	"runtime"
	"testing"
)

func TestRunPlan(t *testing.T) {
	pass := "exit 0"
	fail := "exit 7"
	if runtime.GOOS != "windows" {
		pass = "true"
		fail = "false"
	}
	root := t.TempDir()

	t.Run("all required steps pass", func(t *testing.T) {
		result := Run(context.Background(), Plan{Steps: []Step{
			{Command: pass, Dir: root, Confidence: ConfidenceHigh},
			{Command: pass, Dir: root, Confidence: ConfidenceHigh},
		}})
		if !result.Passed || len(result.Steps) != 2 {
			t.Fatalf("result = %#v", result)
		}
	})

	t.Run("failure stops plan", func(t *testing.T) {
		result := Run(context.Background(), Plan{Steps: []Step{
			{Command: fail, Dir: root, Confidence: ConfidenceHigh},
			{Command: pass, Dir: root, Confidence: ConfidenceHigh},
		}})
		if result.Passed || len(result.Steps) != 1 {
			t.Fatalf("result = %#v", result)
		}
	})

	t.Run("empty or guessed plan does not pass", func(t *testing.T) {
		if Run(context.Background(), Plan{}).Passed {
			t.Fatal("empty plan must be inconclusive")
		}
		if Run(context.Background(), Plan{Steps: []Step{{Command: pass, Dir: root, Confidence: ConfidenceMedium}}}).Passed {
			t.Fatal("medium-confidence step must not auto-run or pass")
		}
	})
}
