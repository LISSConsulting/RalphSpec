package loop

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/LISSConsulting/RalphSpec/internal/claude"
	"github.com/LISSConsulting/RalphSpec/internal/config"
)

// Mode selects which loop configuration to use.
type Mode string

const (
	ModePlan  Mode = "plan"
	ModeBuild Mode = "build"
)

// GitOps defines the git operations the loop needs.
// *git.Runner satisfies this interface.
type GitOps interface {
	CurrentBranch() (string, error)
	HasUncommittedChanges() (bool, error)
	HasRemoteBranch(branch string) bool
	Pull(branch string) error
	Push(branch string) error
	Stash() error
	StashPop() error
	LastCommit() (string, error)
	DiffFromRemote(branch string) (bool, error)
}

// Loop orchestrates the prompt -> claude -> parse -> git iteration cycle.
type Loop struct {
	Agent            claude.Agent
	AgentType        string
	Git              GitOps
	Config           *config.Config
	Log              io.Writer       // output destination; defaults to os.Stdout
	Events           chan<- LogEntry // optional: structured event sink for TUI
	Dir              string          // working directory for prompt file resolution
	PostIteration    func()          // optional: called after each iteration (e.g., test-gated rollback)
	StopAfter        <-chan struct{} // optional: closed to request graceful stop after current iteration
	NotificationHook func(LogEntry)  // optional: called on every emitted event for external notifications
	Roam             bool            // roam freely across the codebase (--roam flag)
	Spec             string          // active spec name for prompt augmentation (empty = no augmentation)
	SpecDir          string          // active spec directory for prompt augmentation
	Focus            string          // constrain roam to a specific topic (empty = no constraint)
}

// Run executes the loop in the given mode. It runs iterations until the
// configured max is reached, the context is cancelled, or an error occurs.
// If maxOverride > 0, it overrides the config's max_iterations.
func (l *Loop) Run(ctx context.Context, mode Mode, maxOverride int) error {
	// If stop was already requested (e.g., Ctrl+C during a prior phase of
	// smart run), exit immediately without starting a new run.
	if l.StopAfter != nil {
		select {
		case <-l.StopAfter:
			return nil
		default:
		}
	}

	promptFile, maxIter := l.modeConfig(mode)
	if maxOverride > 0 {
		maxIter = maxOverride
	}

	promptPath := filepath.Join(l.Dir, promptFile)
	promptBytes, err := os.ReadFile(promptPath)
	if err != nil {
		return fmt.Errorf("loop: read prompt %s: %w", promptFile, err)
	}

	// Augment prompt with spec context guardrails when applicable.
	prompt := augmentPrompt(string(promptBytes), l.Spec, l.SpecDir, l.Roam, l.Focus)

	branch, err := l.Git.CurrentBranch()
	if err != nil {
		return fmt.Errorf("loop: get branch: %w", err)
	}

	// Include current HEAD commit so TUI footer shows it from the start,
	// rather than showing "—" until the first push.
	commit, _ := l.Git.LastCommit()

	l.emit(LogEntry{
		Kind:    LogInfo,
		Message: fmt.Sprintf("Starting %s loop with %s on branch %s (max: %s)", mode, l.agentName(), branch, iterLabel(maxIter)),
		Agent:   l.agentName(),
		Branch:  branch,
		Commit:  commit,
		MaxIter: maxIter,
		Mode:    string(mode),
	})

	var totalCost float64
	var prevSubtype string
	for i := 1; maxIter == 0 || i <= maxIter; i++ {
		select {
		case <-ctx.Done():
			l.emit(LogEntry{
				Kind:    LogStopped,
				Message: fmt.Sprintf("Loop stopped: %v", ctx.Err()),
				Agent:   l.agentName(),
			})
			return ctx.Err()
		default:
		}

		cost, subtype, commitsProduced, iterErr := l.iteration(ctx, i, maxIter, prompt, branch)
		if iterErr != nil {
			return fmt.Errorf("loop: iteration %d: %w", i, iterErr)
		}
		totalCost += cost

		// Spec completion detection: two-signal check — previous iteration
		// reported "success" and this iteration produced no new commits.
		if prevSubtype == "success" && !commitsProduced {
			if l.Roam {
				l.emit(LogEntry{
					Kind:      LogSweepComplete,
					Message:   fmt.Sprintf("Roam complete (%d iterations, $%.2f)", i, totalCost),
					Agent:     l.agentName(),
					TotalCost: totalCost,
				})
			} else {
				l.emit(LogEntry{
					Kind:      LogSpecComplete,
					Message:   fmt.Sprintf("Spec complete (%d iterations, $%.2f)", i, totalCost),
					Agent:     l.agentName(),
					TotalCost: totalCost,
				})
			}
			return nil
		}
		prevSubtype = subtype

		// Run post-iteration hook (e.g., test-gated rollback from Regent)
		if l.PostIteration != nil {
			l.PostIteration()
		}

		l.emit(LogEntry{
			Kind:      LogInfo,
			Message:   fmt.Sprintf("Running total: $%.2f", totalCost),
			Agent:     l.agentName(),
			TotalCost: totalCost,
		})

		// Check for user-requested graceful stop (TUI 's' key).
		if l.StopAfter != nil {
			select {
			case <-l.StopAfter:
				l.emit(LogEntry{
					Kind:    LogStopped,
					Message: "Stop requested — exiting after this iteration",
					Agent:   l.agentName(),
				})
				return nil
			default:
			}
		}
	}

	l.emit(LogEntry{
		Kind:      LogDone,
		Message:   fmt.Sprintf("Loop complete — %s iterations done, total cost: $%.2f", iterLabel(maxIter), totalCost),
		Agent:     l.agentName(),
		TotalCost: totalCost,
		MaxIter:   maxIter,
	})
	return nil
}

func (l *Loop) iteration(ctx context.Context, n, maxIter int, prompt, branch string) (cost float64, subtype string, commitsProduced bool, err error) {
	l.emit(LogEntry{
		Kind:      LogIterStart,
		Message:   fmt.Sprintf("── iteration %d ──", n),
		Agent:     l.agentName(),
		Iteration: n,
		MaxIter:   maxIter,
		Branch:    branch,
	})

	// Stash uncommitted changes before pulling
	stashed, err := l.stashIfDirty()
	if err != nil {
		return 0, "", false, err
	}

	// Pull latest from remote (skip if no remote tracking branch yet)
	if l.Config.Git.AutoPullRebase && l.Git.HasRemoteBranch(branch) {
		l.emit(LogEntry{
			Kind:    LogGitPull,
			Message: fmt.Sprintf("Pulling %s", branch),
			Agent:   l.agentName(),
			Branch:  branch,
		})
		if pullErr := l.Git.Pull(branch); pullErr != nil {
			l.emit(LogEntry{
				Kind:    LogInfo,
				Message: fmt.Sprintf("Pull failed: %v (continuing)", pullErr),
				Agent:   l.agentName(),
			})
		}
	}

	// Restore stashed changes
	if stashed {
		if popErr := l.Git.StashPop(); popErr != nil {
			l.emit(LogEntry{
				Kind:    LogInfo,
				Message: fmt.Sprintf("Stash pop failed: %v", popErr),
				Agent:   l.agentName(),
			})
		}
	}

	// Capture HEAD before Claude runs to detect new commits afterward.
	headBefore, _ := l.Git.LastCommit()

	// Run the selected agent.
	l.emit(LogEntry{
		Kind:    LogInfo,
		Agent:   l.agentName(),
		Message: fmt.Sprintf("Running %s...", l.agentName()),
	})
	events, agentErr := l.Agent.Run(ctx, prompt, claude.RunOptions{
		Model:                 l.agentModel(),
		MaxTurns:              l.agentMaxTurns(),
		DangerSkipPermissions: l.agentDangerSkipPermissions(),
		Dir:                   l.Dir,
	})
	if agentErr != nil {
		return 0, "", false, fmt.Errorf("start %s: %w", l.agentName(), agentErr)
	}

	// Drain events
	for ev := range events {
		switch ev.Type {
		case claude.EventToolUse:
			l.emit(LogEntry{
				Kind:      LogToolUse,
				Message:   fmt.Sprintf("tool: %s  %s", ev.ToolName, summarizeInput(ev.ToolInput)),
				Agent:     l.agentName(),
				ToolName:  ev.ToolName,
				ToolInput: summarizeInput(ev.ToolInput),
			})
		case claude.EventText:
			if ev.Text != "" {
				l.emit(LogEntry{
					Kind:    LogText,
					Agent:   l.agentName(),
					Message: ev.Text,
				})
			}
		case claude.EventResult:
			cost = ev.CostUSD
			subtype = ev.Subtype
			msg := fmt.Sprintf("Iteration %d complete — $%.2f — %.1fs", n, ev.CostUSD, ev.Duration)
			if ev.Subtype != "" {
				msg += fmt.Sprintf(" — %s", ev.Subtype)
			}
			l.emit(LogEntry{
				Kind:      LogIterComplete,
				Message:   msg,
				Agent:     l.agentName(),
				Iteration: n,
				CostUSD:   ev.CostUSD,
				Duration:  ev.Duration,
				Subtype:   ev.Subtype,
			})
		case claude.EventError:
			l.emit(LogEntry{
				Kind:    LogError,
				Agent:   l.agentName(),
				Message: fmt.Sprintf("Error: %s", ev.Error),
			})
		}
	}

	// Push if there are new local commits
	if l.Config.Git.AutoPush {
		if pushErr := l.pushIfNeeded(branch); pushErr != nil {
			l.emit(LogEntry{
				Kind:    LogError,
				Agent:   l.agentName(),
				Message: fmt.Sprintf("Push error: %v", pushErr),
			})
		}
	}

	// Detect whether Claude produced new commits during this iteration.
	headAfter, _ := l.Git.LastCommit()
	commitsProduced = headBefore != headAfter

	return cost, subtype, commitsProduced, nil
}

func (l *Loop) stashIfDirty() (bool, error) {
	dirty, err := l.Git.HasUncommittedChanges()
	if err != nil {
		return false, fmt.Errorf("check changes: %w", err)
	}
	if dirty {
		l.emit(LogEntry{
			Kind:    LogInfo,
			Message: "Stashing uncommitted changes",
			Agent:   l.agentName(),
		})
		if stashErr := l.Git.Stash(); stashErr != nil {
			return false, fmt.Errorf("stash: %w", stashErr)
		}
		return true, nil
	}
	return false, nil
}

func (l *Loop) pushIfNeeded(branch string) error {
	hasChanges, err := l.Git.DiffFromRemote(branch)
	if err != nil {
		// Can't determine diff state (e.g., no remote tracking branch yet).
		// Push anyway — Push() handles -u fallback for new branches.
		l.emit(LogEntry{
			Kind:    LogInfo,
			Message: fmt.Sprintf("Diff check failed: %v (pushing anyway)", err),
			Agent:   l.agentName(),
		})
	}
	if err == nil && !hasChanges {
		return nil
	}
	l.emit(LogEntry{
		Kind:    LogGitPush,
		Message: fmt.Sprintf("Pushing %s", branch),
		Agent:   l.agentName(),
		Branch:  branch,
	})
	if pushErr := l.Git.Push(branch); pushErr != nil {
		return pushErr
	}
	commit, commitErr := l.Git.LastCommit()
	if commitErr != nil {
		commit = "(unknown)"
	}
	l.emit(LogEntry{
		Kind:    LogGitPush,
		Message: fmt.Sprintf("Pushed — last commit: %s", commit),
		Agent:   l.agentName(),
		Commit:  commit,
		Branch:  branch,
	})
	return nil
}

// emit sends a structured log entry. When Events is set, it sends to the
// channel for TUI consumption. Otherwise, it writes formatted text to Log.
// The channel send is non-blocking to prevent deadlock if the TUI exits
// while the loop is still draining events. NotificationHook, when set, is
// always called regardless of the TUI/log path.
func (l *Loop) emit(entry LogEntry) {
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}
	if l.NotificationHook != nil {
		l.NotificationHook(entry)
	}
	if l.Events != nil {
		select {
		case l.Events <- entry:
		default:
		}
		return
	}
	w := l.Log
	if w == nil {
		w = os.Stdout
	}
	ts := entry.Timestamp.Format("15:04:05")
	_, _ = fmt.Fprintf(w, "[%s]  %s\n", ts, entry.Message)
}

func (l *Loop) modeConfig(mode Mode) (promptFile string, maxIter int) {
	switch mode {
	case ModePlan:
		return l.Config.Plan.PromptFile, l.Config.Plan.MaxIterations
	default:
		return l.Config.Build.PromptFile, l.Config.Build.MaxIterations
	}
}

func iterLabel(max int) string {
	if max == 0 {
		return "unlimited"
	}
	return fmt.Sprintf("%d", max)
}

func (l *Loop) agentName() string {
	if l.AgentType != "" {
		return l.AgentType
	}
	return config.AgentClaude
}

func (l *Loop) agentModel() string {
	if l.agentName() == config.AgentCodex {
		return l.Config.Codex.Model
	}
	return l.Config.Claude.Model
}

func (l *Loop) agentMaxTurns() int {
	if l.agentName() == config.AgentCodex {
		return 0
	}
	return l.Config.Claude.MaxTurns
}

func (l *Loop) agentDangerSkipPermissions() bool {
	if l.agentName() == config.AgentCodex {
		return false
	}
	return l.Config.Claude.DangerSkipPermissions
}

// augmentPrompt appends a ## Spec Context section to the prompt when applicable.
// In roam mode, Claude is told to roam freely across the entire codebase.
// When a spec name is set (and roam is false), the section names the active spec
// and its directory to keep Claude focused on the spec boundary.
// When focus is non-empty, a focus directive is appended to constrain the topic.
// When neither applies, the prompt is returned unchanged.
func augmentPrompt(prompt, spec, specDir string, roam bool, focus string) string {
	if roam {
		result := prompt + "\n\n## Spec Context\n\nRoam mode is active. You are free to review and improve the entire codebase — refactor, fix, optimise, and tidy without being confined to any single spec."
		if focus != "" {
			result += fmt.Sprintf("\n\nFocus your work on: %s. Prioritize changes related to this area over other improvements.", focus)
		}
		return result
	}
	if spec != "" {
		result := prompt + fmt.Sprintf("\n\n## Spec Context\n\nActive spec: %s\nSpec directory: %s\n\nStay focused on this spec. When the work described in this spec is complete, stop making changes.", spec, specDir)
		if focus != "" {
			result += fmt.Sprintf("\n\nFocus your work on: %s. Prioritize changes related to this area over other improvements.", focus)
		}
		return result
	}
	if focus != "" {
		return prompt + fmt.Sprintf("\n\n## Spec Context\n\nFocus your work on: %s. Prioritize changes related to this area over other improvements.", focus)
	}
	return prompt
}

func summarizeInput(input map[string]any) string {
	// Check well-known field names in priority order.
	for _, key := range []string{
		"file_path", "command", "path", "url", "pattern", // core tools
		"description", "prompt", // Task / agent tools
		"query",         // WebSearch
		"notebook_path", // NotebookEdit
		"task_id",       // TaskOutput
	} {
		if v, ok := input[key]; ok {
			return fmt.Sprintf("%v", v)
		}
	}
	return ""
}
