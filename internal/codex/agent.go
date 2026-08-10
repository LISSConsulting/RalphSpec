package codex

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/LISSConsulting/RalphSpec/internal/claude"
)

type Agent struct {
	Executable string
}

func NewAgent() *Agent {
	return &Agent{Executable: "codex"}
}

func (a *Agent) Run(ctx context.Context, prompt string, opts claude.RunOptions) (<-chan claude.Event, error) {
	args := a.buildArgs(prompt, opts)
	exe := a.Executable
	if exe == "" {
		exe = "codex"
	}
	cmd := exec.CommandContext(ctx, exe, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("codex agent: stdout pipe: %w", err)
	}

	// Live steering: codex exec has no documented stdin protocol, so steer
	// messages are written as plain-text lines on a best-effort basis. Write
	// errors (e.g. process exit) are swallowed and never fail the iteration.
	var stdin io.WriteCloser
	if opts.Steer != nil {
		stdin, err = cmd.StdinPipe()
		if err != nil {
			return nil, fmt.Errorf("codex agent: stdin pipe: %w", err)
		}
	}

	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("codex agent: start: %w", err)
	}

	stdinDone := make(chan struct{})
	if stdin != nil {
		go drainSteerToStdin(stdin, opts.Steer, stdinDone, ctx)
	}

	parsed := ParseStream(stdout)
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
			msg := fmt.Sprintf("codex exited: %v", waitErr)
			if detail := strings.TrimSpace(stderrBuf.String()); detail != "" {
				msg = fmt.Sprintf("codex exited: %v: %s", waitErr, detail)
			}
			ch <- claude.ErrorEvent(msg)
		case !terminalSeen:
			ch <- claude.ErrorEvent("codex exited without a terminal result")
		}
		close(stdinDone)
	}()
	return ch, nil
}

// drainSteerToStdin forwards operator steering messages as plain-text lines.
// Stops on context cancellation, channel close, process exit, or the first
// write error.
func drainSteerToStdin(stdin io.WriteCloser, steer <-chan string, done <-chan struct{}, ctx context.Context) {
	defer func() { _ = stdin.Close() }()
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
			if _, err := io.WriteString(stdin, msg+"\n"); err != nil {
				return
			}
		}
	}
}

func (a *Agent) buildArgs(prompt string, opts claude.RunOptions) []string {
	args := []string{"exec", "--json", "--dangerously-bypass-approvals-and-sandbox"}
	if opts.Dir != "" {
		args = append(args, "--cd", opts.Dir)
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	args = append(args, prompt)
	return args
}

func CheckAvailable(executable string) error {
	if executable == "" {
		executable = "codex"
	}
	if _, err := exec.LookPath(executable); err != nil {
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("codex executable not found on PATH")
		}
		return fmt.Errorf("codex executable lookup failed: %w", err)
	}
	return nil
}
