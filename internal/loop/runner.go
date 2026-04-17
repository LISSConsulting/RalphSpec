// Package loop implements the core iteration cycle: prompt -> claude -> parse -> git.
package loop

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/LISSConsulting/RalphSpec/internal/claude"
)

// ClaudeAgent implements claude.Agent by spawning the Claude CLI as a subprocess.
// It feeds the prompt via the -p flag and parses stream-JSON output.
type ClaudeAgent struct {
	// Executable is the path to the Claude CLI binary. Defaults to "claude".
	Executable string
}

// NewClaudeAgent creates a ClaudeAgent that uses the default "claude" binary.
func NewClaudeAgent() *ClaudeAgent {
	return &ClaudeAgent{Executable: "claude"}
}

// Run spawns the Claude CLI with the given prompt and streams parsed events back
// on the returned channel. The channel is closed when the process exits.
func (a *ClaudeAgent) Run(ctx context.Context, prompt string, opts claude.RunOptions) (<-chan claude.Event, error) {
	args := a.buildArgs(prompt, opts)

	exe := a.Executable
	if exe == "" {
		exe = "claude"
	}

	cmd := exec.CommandContext(ctx, exe, args...)
	if opts.Dir != "" {
		cmd.Dir = opts.Dir
	}
	isolateProcess(cmd)
	// Override the default ctx-cancel behaviour (Kill on the top-level process
	// only) so Claude's own tool subprocesses are torn down with it. Without
	// this, the Node process dies but its children are reparented and keep
	// running — which is how Ralph ended up leaving Claude instances alive
	// after the TUI exited.
	cmd.Cancel = func() error { return killProcessTree(cmd.Process) }
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("claude agent: stdout pipe: %w", err)
	}

	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("claude agent: start: %w", err)
	}

	parsed := claude.ParseStream(stdout)

	ch := make(chan claude.Event, 64)
	go func() {
		defer close(ch)
		for ev := range parsed {
			ch <- ev
		}
		if err := cmd.Wait(); err != nil {
			// Context cancellation produces a non-zero exit — that's expected
			if ctx.Err() == nil {
				msg := fmt.Sprintf("claude exited: %v", err)
				if detail := strings.TrimSpace(stderrBuf.String()); detail != "" {
					msg = fmt.Sprintf("claude exited: %v: %s", err, detail)
				}
				ch <- claude.ErrorEvent(msg)
			}
		}
	}()

	return ch, nil
}

// buildArgs constructs the CLI arguments for a Claude invocation.
func (a *ClaudeAgent) buildArgs(prompt string, opts claude.RunOptions) []string {
	args := []string{
		"-p", prompt,
		"--output-format", "stream-json",
		"--verbose",
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.MaxTurns > 0 {
		args = append(args, "--max-turns", fmt.Sprintf("%d", opts.MaxTurns))
	}
	if opts.DangerSkipPermissions {
		args = append(args, "--dangerously-skip-permissions")
	}
	return args
}
