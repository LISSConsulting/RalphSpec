// Package loop implements the core iteration cycle: prompt -> claude -> parse -> git.
package loop

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/LISSConsulting/RalphSpec/internal/claude"
	"github.com/LISSConsulting/RalphSpec/internal/quota"
)

// ClaudeAgent implements claude.Agent by spawning the Claude CLI as a subprocess.
// It feeds the prompt via the -p flag and parses stream-JSON output.
type ClaudeAgent struct {
	// Executable is the path to the Claude CLI binary. Defaults to "claude".
	QuotaSnapshotFile   string
	QuotaSnapshotMaxAge time.Duration
	Executable          string
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

	// Live steering: keep a stdin pipe open so operator messages can be
	// delivered mid-turn as stream-JSON user messages (SDK pipe mode).
	var stdin io.WriteCloser
	if opts.Steer != nil {
		stdin, err = cmd.StdinPipe()
		if err != nil {
			return nil, fmt.Errorf("claude agent: stdin pipe: %w", err)
		}
	}

	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("claude agent: start: %w", err)
	}

	stdinDone := make(chan struct{})
	if stdin != nil {
		go steerStdin(stdin, prompt, opts.Steer, stdinDone, ctx)
	}

	parsed := claude.ParseStream(stdout)

	ch := make(chan claude.Event, 64)
	go func() {
		defer close(ch)
		terminalSeen := false
		for ev := range parsed {
			if ev.Outcome != nil {
				terminalSeen = true
			}
			ch <- ev
		}
		waitErr := cmd.Wait()
		switch {
		case ctx.Err() != nil:
			outcome := claude.TerminalOutcome{Kind: claude.OutcomeCancelled, Message: ctx.Err().Error()}
			ch <- claude.Event{Type: claude.EventError, Timestamp: time.Now(), Error: outcome.Message, Outcome: &outcome}
		case waitErr != nil:
			msg := fmt.Sprintf("claude exited: %v", waitErr)
			if detail := strings.TrimSpace(stderrBuf.String()); detail != "" {
				msg = fmt.Sprintf("claude exited: %v: %s", waitErr, detail)
			}
			ch <- claude.ErrorEvent(msg)
		case !terminalSeen:
			ch <- claude.ErrorEvent("claude exited without a terminal result")
		}
		close(stdinDone)
	}()

	return ch, nil
}

// Quota returns the latest Claude status-line snapshot configured for Ralph.
func (a *ClaudeAgent) Quota(context.Context) (quota.Snapshot, error) {
	return claude.ReadQuotaSnapshot(a.QuotaSnapshotFile, a.QuotaSnapshotMaxAge)
}

// steerStdin writes the iteration prompt as the first stream-JSON user
// message, then forwards each operator steering message as it arrives. It
// stops on context cancellation, channel close, process exit (stdinDone), or
// the first write error — a dead pipe must never fail the iteration.
func steerStdin(stdin io.WriteCloser, prompt string, steer <-chan string, done <-chan struct{}, ctx context.Context) {
	defer func() { _ = stdin.Close() }()
	if !writeUserMessage(stdin, prompt) {
		return
	}
	for {
		select {
		case <-done:
			return
		case <-ctx.Done():
			return
		case msg, ok := <-steer:
			if !ok {
				return
			}
			if !writeUserMessage(stdin, msg) {
				return
			}
		}
	}
}

// writeUserMessage encodes one stream-JSON user message line. Returns false
// when the pipe is broken.
func writeUserMessage(w io.Writer, text string) bool {
	payload := map[string]any{
		"type": "user",
		"message": map[string]any{
			"role":    "user",
			"content": text,
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return false
	}
	_, err = w.Write(append(data, '\n'))
	return err == nil
}

// buildArgs constructs the CLI arguments for a Claude invocation.
// When live steering is enabled the prompt travels via stdin as a stream-JSON
// user message, so -p is bare and --input-format stream-json is added (SDK
// pipe mode). Otherwise the invocation is byte-identical to legacy behavior.
func (a *ClaudeAgent) buildArgs(prompt string, opts claude.RunOptions) []string {
	var args []string
	if opts.Steer != nil {
		args = []string{
			"-p",
			"--output-format", "stream-json",
			"--input-format", "stream-json",
			"--verbose",
		}
	} else {
		args = []string{
			"-p", prompt,
			"--output-format", "stream-json",
			"--verbose",
		}
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
