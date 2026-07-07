package codex

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

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
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("codex agent: start: %w", err)
	}
	parsed := ParseStream(stdout)
	ch := make(chan claude.Event, 64)
	go func() {
		defer close(ch)
		for ev := range parsed {
			ch <- ev
		}
		if err := cmd.Wait(); err != nil && ctx.Err() == nil {
			msg := fmt.Sprintf("codex exited: %v", err)
			if detail := strings.TrimSpace(stderrBuf.String()); detail != "" {
				msg = fmt.Sprintf("codex exited: %v: %s", err, detail)
			}
			ch <- claude.ErrorEvent(msg)
		}
	}()
	return ch, nil
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
