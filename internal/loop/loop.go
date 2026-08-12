package loop

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/LISSConsulting/RalphSpec/internal/claude"
	"github.com/LISSConsulting/RalphSpec/internal/config"
	"github.com/LISSConsulting/RalphSpec/internal/quota"
	"github.com/LISSConsulting/RalphSpec/internal/testplan"
)

// Mode selects which loop configuration to use.
type Mode string

const (
	ModeBuild Mode = "build"
)

// ErrOperatorStopped reports a requested graceful stop without treating it as
// successful goal completion.
var ErrOperatorStopped = errors.New("operator stop requested")

// GitOps defines the git operations the loop needs.
// *git.Runner satisfies this interface.
type GitOps interface {
	CurrentBranch() (string, error)
	HasUncommittedChanges() (bool, error)
	HasRemoteBranch(branch string) bool
	Pull(branch string) error
	Push(branch string) error
	Stash() (bool, error)
	StashPop() error
	LastCommit() (string, error)
	DiffFromRemote(branch string) (bool, error)
}

type quotaFallbackAgent interface {
	FailoverQuota() (from, to string, ok bool)
}

type providerAgent interface {
	CurrentProvider() string
}

// Loop orchestrates the prompt -> claude -> parse -> git iteration cycle.
type Loop struct {
	Agent                claude.Agent
	AgentType            string
	Git                  GitOps
	Config               *config.Config
	QuotaGate            *quota.Gate
	InitialQuotaRelease  func()
	InitialQuotaDecision *quota.Decision
	TestPlan             *testplan.Plan
	Log                  io.Writer                  // output destination; defaults to os.Stdout
	Events               chan<- LogEntry            // optional: structured event sink for TUI
	Dir                  string                     // working directory for prompt file resolution
	PostIteration        func(context.Context) bool // optional: verifies the iteration; false blocks completion
	StopAfter            <-chan struct{}            // optional: closed to request graceful stop after current iteration
	Steer                <-chan string              // optional: operator steering messages; drained before each iteration
	NotificationHook     func(LogEntry)             // optional: called on every emitted event for external notifications
	Roam                 bool                       // roam freely across the codebase (--roam flag)
	Spec                 string                     // active spec name for prompt augmentation (empty = no augmentation)
	SpecDir              string                     // active spec directory for prompt augmentation
	Focus                string                     // constrain roam to a specific topic (empty = no constraint)
	now                  func() time.Time
}

// Run executes the loop in the given mode. It runs iterations until the
// configured max is reached, the context is cancelled, or an error occurs.
// If maxOverride > 0, it overrides the config's max_iterations.
func (l *Loop) Run(ctx context.Context, mode Mode, maxOverride int) error {
	initialQuotaRelease := l.InitialQuotaRelease
	initialQuotaDecision := cloneQuotaDecision(l.InitialQuotaDecision)
	initialQuotaHeld := initialQuotaRelease != nil
	l.InitialQuotaRelease = nil
	l.InitialQuotaDecision = nil
	if initialQuotaRelease != nil {
		defer initialQuotaRelease()
	}
	// If stop was already requested (e.g., Ctrl+C during a prior phase), exit
	// immediately without starting a new run.
	if l.StopAfter != nil {
		select {
		case <-l.StopAfter:
			return ErrOperatorStopped
		default:
		}
	}

	promptFile, maxIter := l.modeConfig(mode)
	ludicrous := mode == ModeBuild && l.Config != nil && l.Config.Build.Ludicrous
	if ludicrous && l.Roam {
		return fmt.Errorf("ludicrous mode cannot be combined with roam mode")
	}
	if maxOverride > 0 {
		maxIter = maxOverride
	}
	if ludicrous && maxOverride == 0 {
		maxIter = 0
	}

	promptPath := filepath.Join(l.Dir, promptFile)
	promptBytes, err := os.ReadFile(promptPath)
	if err != nil {
		return fmt.Errorf("loop: read prompt %s: %w", promptFile, err)
	}

	// Augment prompt with spec context guardrails when applicable.
	prompt := augmentPrompt(string(promptBytes), l.Spec, l.SpecDir, l.Roam, l.Focus)
	if ludicrous {
		prompt += "\n\n## Goal-Persistence Contract\nPersist toward the requested objective until completion is supported by concrete inspection and required test evidence. Do not report success merely because an iteration made no changes. Never bypass safety constraints, quota admission, permissions, or an operator stop."
	}

	branch, err := l.Git.CurrentBranch()
	if err != nil {
		return fmt.Errorf("loop: get branch: %w", err)
	}

	// Include current HEAD commit so TUI footer shows it from the start,
	// rather than showing "—" until the first push.
	commit, _ := l.Git.LastCommit()

	runDescription := string(mode)
	if ludicrous {
		runDescription += " (ludicrous)"
	}
	l.emit(LogEntry{
		Kind:      LogInfo,
		Message:   fmt.Sprintf("Starting %s loop with %s on branch %s (max: %s)", runDescription, l.agentName(), branch, iterLabel(maxIter)),
		Agent:     l.agentName(),
		Branch:    branch,
		Commit:    commit,
		MaxIter:   maxIter,
		Mode:      string(mode),
		Ludicrous: ludicrous,
	})
	if l.TestPlan != nil {
		commands := make([]string, 0, len(l.TestPlan.Steps))
		for _, step := range l.TestPlan.Steps {
			commands = append(commands, step.Command)
		}
		l.emit(LogEntry{
			Kind:      LogInfo,
			Message:   fmt.Sprintf("Test plan selected (%d): %s", len(commands), strings.Join(commands, "; ")),
			Agent:     l.agentName(),
			Branch:    branch,
			Mode:      string(mode),
			Ludicrous: ludicrous,
		})
	}

	var totalCost float64
	var prevSubtype string
	lastChecked, lastTotal := -1, -1
	initialQuotaLogged := false
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

		iterPrompt := prompt
		tasksComplete := true
		if l.SpecDir != "" && !l.Roam {
			checked, total, taskErr := countTasks(l.tasksPath())
			if taskErr != nil {
				if ludicrous {
					tasksComplete = false
					l.emit(LogEntry{Kind: LogError, Agent: l.agentName(), Message: fmt.Sprintf("Task completion evidence unavailable: %v", taskErr)})
				}
			} else {
				if total > 0 {
					iterPrompt = appendTaskAccounting(iterPrompt, l.SpecDir, checked, total)
					if checked != lastChecked || total != lastTotal {
						l.emit(LogEntry{
							Kind:    LogInfo,
							Message: fmt.Sprintf("tasks: %d/%d complete", checked, total),
							Agent:   l.agentName(),
						})
						lastChecked, lastTotal = checked, total
					}
				}
				if ludicrous {
					tasksComplete = checked == total
				}
			}
		}
		if steers := l.drainSteers(); len(steers) > 0 {
			iterPrompt = appendSteerSection(iterPrompt, steers)
		}
		releaseAgentSlot := func() {}
		if initialQuotaHeld {
			if !initialQuotaLogged && initialQuotaDecision != nil {
				decision := cloneQuotaDecision(initialQuotaDecision)
				l.emit(LogEntry{
					Kind:          LogInfo,
					Agent:         l.agentName(),
					Message:       fmt.Sprintf("Quota %s: %s", decision.Action, decision.Reason),
					QuotaDecision: decision,
				})
				initialQuotaLogged = true
			}
		} else if l.QuotaGate != nil {
			for {
				decision, release, quotaErr := l.QuotaGate.Acquire(ctx, func(d quota.Decision) {
					kind := LogInfo
					if d.Action == quota.ActionBlock {
						kind = LogError
					}
					decision := cloneQuotaDecision(&d)
					l.emit(LogEntry{
						Kind:          kind,
						Agent:         l.agentName(),
						Message:       fmt.Sprintf("Quota %s: %s", d.Action, d.Reason),
						QuotaDecision: decision,
					})
				})
				releaseAgentSlot = release
				if quotaErr != nil {
					kind := claude.OutcomeAgentFailure
					if ctx.Err() != nil {
						kind = claude.OutcomeCancelled
					}
					return &claude.OutcomeError{Outcome: claude.TerminalOutcome{Kind: kind, Message: quotaErr.Error()}}
				}
				if decision.Action != quota.ActionBlock {
					break
				}
				fallback, supportsFallback := l.Agent.(quotaFallbackAgent)
				from, to, switched := "", "", false
				if supportsFallback {
					from, to, switched = fallback.FailoverQuota()
				}
				if !switched {
					return &claude.OutcomeError{Outcome: claude.TerminalOutcome{
						Kind:     claude.OutcomeQuotaExhausted,
						Message:  decision.Reason,
						Provider: from,
						RetryAt:  decision.ResumeAt,
					}}
				}
				l.emit(LogEntry{
					Kind:     LogInfo,
					Agent:    l.agentName(),
					Provider: to,
					Message:  fmt.Sprintf("Provider %s quota admission blocked; switching %s to %s", from, l.agentName(), to),
				})
			}
		}

		cost, subtype, commitsProduced, dirty, iterErr := l.iterationWithRelease(ctx, i, maxIter, iterPrompt, branch, releaseAgentSlot)
		if iterErr != nil {
			return fmt.Errorf("loop: iteration %d: %w", i, iterErr)
		}
		totalCost += cost
		testsPassed := true
		if l.PostIteration != nil {
			testsPassed = l.PostIteration(ctx)
		} else if l.TestPlan != nil {
			testsPassed = l.runTestPlan(ctx)
		}

		// Spec completion detection: two independent structured success signals,
		// a clean worktree, completed declared tasks, and passing verification.
		rawCompletion := prevSubtype == "success" && subtype == "success" && !commitsProduced && !dirty
		if rawCompletion && !tasksComplete {
			l.emit(LogEntry{Kind: LogInfo, Agent: l.agentName(), Message: "Completion evidence rejected because declared tasks are incomplete or unavailable"})
		}
		completionCandidate := rawCompletion && tasksComplete
		if completionCandidate && testsPassed {
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
		if completionCandidate && !testsPassed {
			l.emit(LogEntry{
				Kind:    LogInfo,
				Message: "Completion evidence rejected because required tests did not pass",
				Agent:   l.agentName(),
			})
		}
		switch {
		case dirty:
			l.emit(LogEntry{
				Kind:    LogInfo,
				Message: "Worktree has uncommitted changes after iteration; not treating agent success as spec completion",
				Agent:   l.agentName(),
			})
			prevSubtype = ""
		case testsPassed:
			prevSubtype = subtype
		default:
			prevSubtype = ""
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
				return ErrOperatorStopped
			default:
			}
		}
	}

	if ludicrous {
		return &claude.OutcomeError{Outcome: claude.TerminalOutcome{
			Kind:    claude.OutcomeAgentFailure,
			Message: fmt.Sprintf("ludicrous mode exhausted %s iterations without verified goal-completion evidence", iterLabel(maxIter)),
		}}
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

func (l *Loop) iterationWithRelease(ctx context.Context, n, max int, prompt, branch string, release func()) (cost float64, subtype string, commitsProduced, dirty bool, err error) {
	defer release()
	return l.iteration(ctx, n, max, prompt, branch)
}
func (l *Loop) runTestPlan(ctx context.Context) bool {
	for _, step := range l.TestPlan.Steps {
		l.emit(LogEntry{Kind: LogInfo, Agent: l.agentName(), Message: fmt.Sprintf("Running tests: %s (%s)", step.Command, step.Dir)})
	}
	result := testplan.Run(ctx, *l.TestPlan)
	for _, step := range result.Steps {
		if step.Passed {
			continue
		}
		l.emit(LogEntry{Kind: LogError, Agent: l.agentName(), Message: fmt.Sprintf("Tests failed: %s\n%s", step.Step.Command, step.Output)})
		break
	}
	return result.Passed
}

// tasksPath resolves the active spec's tasks.md path relative to the loop dir.
func (l *Loop) tasksPath() string {
	if filepath.IsAbs(l.SpecDir) {
		return filepath.Join(l.SpecDir, "tasks.md")
	}
	return filepath.Join(l.Dir, l.SpecDir, "tasks.md")
}

// countTasks counts markdown checkbox items (- [ ], - [x], - [X]) in a
// tasks.md file. Returns an error when the file cannot be read.
func countTasks(path string) (checked, total int, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		t := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(t, "- [ ]"):
			total++
		case strings.HasPrefix(t, "- [x]"), strings.HasPrefix(t, "- [X]"):
			checked++
			total++
		}
	}
	return checked, total, nil
}

// appendTaskAccounting appends the live checkbox state and check-off
// instruction to an iteration prompt.
func appendTaskAccounting(prompt, specDir string, checked, total int) string {
	return fmt.Sprintf(`%s

## Task Accounting

%s/tasks.md: %d of %d tasks checked off.

tasks.md is the loop's bookkeeping, not a read-only artifact: whenever you complete a task, check its box ([ ] → [x]) in the same commit. Before declaring this spec complete, reconcile tasks.md so every completed task is checked.`,
		strings.TrimRight(prompt, "\n"), specDir, checked, total)
}

// drainSteers consumes all pending operator steering messages without
// blocking, emitting one LogSteer event per message. Returns the messages in
// arrival order.
func (l *Loop) drainSteers() []string {
	if l.Steer == nil {
		return nil
	}
	var msgs []string
	for {
		select {
		case msg, ok := <-l.Steer:
			if !ok {
				return msgs
			}
			msg = strings.TrimSpace(msg)
			if msg == "" {
				continue
			}
			msgs = append(msgs, msg)
			l.emit(LogEntry{
				Kind:    LogSteer,
				Message: msg,
				Agent:   l.agentName(),
			})
		default:
			return msgs
		}
	}
}

// appendSteerSection appends operator steering messages to a prompt so the
// agent sees them as explicit human direction for this iteration.
func appendSteerSection(prompt string, msgs []string) string {
	var b strings.Builder
	b.WriteString(strings.TrimRight(prompt, "\n"))
	b.WriteString("\n\n## User steering\n\nThe operator sent the following message(s) while the loop was running. Treat them as direct instructions for this iteration:\n")
	for _, msg := range msgs {
		b.WriteString("\n- ")
		b.WriteString(msg)
	}
	b.WriteString("\n")
	return b.String()
}

func (l *Loop) iteration(ctx context.Context, n, maxIter int, prompt, branch string) (cost float64, subtype string, commitsProduced bool, dirty bool, err error) {
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
		return 0, "", false, false, err
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

	// Capture HEAD before the agent runs to detect new commits afterward. An
	// unborn repository has no HEAD yet, so retain that state as unknown.
	headBefore, headBeforeErr := l.Git.LastCommit()

	// Run the selected agent.
	l.emit(LogEntry{
		Kind:    LogInfo,
		Agent:   l.agentName(),
		Message: fmt.Sprintf("Running %s...", l.agentName()),
	})
	agentStarted := l.nowTime()
	runOpts := claude.RunOptions{
		Model:                 l.agentModel(),
		MaxTurns:              l.agentMaxTurns(),
		DangerSkipPermissions: l.agentDangerSkipPermissions(),
		Dir:                   l.Dir,
	}
	if l.Config != nil && l.Config.Agent.StdinSteer {
		runOpts.Steer = l.Steer
	}
	events, agentErr := l.Agent.Run(ctx, prompt, runOpts)
	if agentErr != nil {
		return 0, "", false, false, fmt.Errorf("start %s: %w", l.agentName(), agentErr)
	}

	// Nested agents may emit their own progress; only a normalized terminal
	// outcome completes the Ralph iteration. A terminal failure always wins
	// over a success-looking event from the same invocation.
	var resultDuration float64
	var resultSeen bool
	var provider string
	var terminal *claude.TerminalOutcome
	terminalCount := 0
	for ev := range events {
		if ev.Provider != "" {
			provider = ev.Provider
		}
		switch ev.Type {
		case claude.EventToolUse:
			l.emit(LogEntry{
				Kind:      LogToolUse,
				Message:   fmt.Sprintf("tool: %s  %s", ev.ToolName, summarizeInput(ev.ToolInput)),
				Agent:     l.agentName(),
				Provider:  provider,
				ToolName:  ev.ToolName,
				ToolInput: summarizeInput(ev.ToolInput),
			})
		case claude.EventText:
			if ev.Text != "" {
				l.emit(LogEntry{
					Kind:     LogText,
					Agent:    l.agentName(),
					Provider: provider,
					Message:  ev.Text,
				})
			}
		case claude.EventResult:
			cost = ev.CostUSD
			subtype = ev.Subtype
			resultDuration = ev.Duration
			resultSeen = true
		case claude.EventError:
			l.emit(LogEntry{
				Kind:     LogError,
				Agent:    l.agentName(),
				Provider: provider,
				Message:  fmt.Sprintf("Error: %s", ev.Error),
			})
		}
		if ev.Outcome != nil {
			terminalCount++
			outcome := *ev.Outcome
			if terminal != nil {
				outcome = claude.PreferOutcome(*terminal, outcome)
			}
			terminal = &outcome
			if !outcome.Successful() && ev.Type == claude.EventResult {
				l.emit(LogEntry{
					Kind:     LogError,
					Agent:    l.agentName(),
					Provider: provider,
					Message:  fmt.Sprintf("Error: %s", outcome.Message),
				})
			}
		}
	}
	if terminalCount > 1 {
		l.emit(LogEntry{
			Kind:     LogError,
			Agent:    l.agentName(),
			Provider: provider,
			Message:  fmt.Sprintf("%s emitted %d terminal events; highest-priority outcome used", l.agentName(), terminalCount),
		})
	}
	if terminal != nil && !terminal.Successful() {
		return cost, subtype, false, false, &claude.OutcomeError{Outcome: *terminal}
	}
	if terminal == nil {
		outcome := claude.TerminalOutcome{
			Kind:    claude.OutcomeAgentFailure,
			Message: fmt.Sprintf("%s stream closed without a terminal outcome", l.agentName()),
		}
		if ctx.Err() != nil {
			outcome.Kind = claude.OutcomeCancelled
			outcome.Message = ctx.Err().Error()
		}
		return cost, subtype, false, false, &claude.OutcomeError{Outcome: outcome}
	}
	if resultSeen {
		if resultDuration <= 0 {
			resultDuration = l.nowTime().Sub(agentStarted).Seconds()
			if resultDuration < 0 {
				resultDuration = 0
			}
		}
		msg := fmt.Sprintf("Iteration %d complete — $%.2f — %.1fs", n, cost, resultDuration)
		if subtype != "" {
			msg += fmt.Sprintf(" — %s", subtype)
		}
		l.emit(LogEntry{
			Kind:      LogIterComplete,
			Message:   msg,
			Agent:     l.agentName(),
			Provider:  provider,
			Iteration: n,
			CostUSD:   cost,
			Duration:  resultDuration,
			Subtype:   subtype,
		})
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

	// Detect whether the agent produced new commits during this iteration.
	headAfter, headErr := l.Git.LastCommit()
	switch {
	case headErr == nil && headBeforeErr == nil:
		commitsProduced = headBefore != headAfter
	case headErr == nil:
		commitsProduced = headAfter != ""
	case headBeforeErr == nil:
		return cost, subtype, false, false, fmt.Errorf("get commit after %s: %w", l.agentName(), headErr)
	}
	dirty, dirtyErr := l.Git.HasUncommittedChanges()
	if dirtyErr != nil {
		return cost, subtype, commitsProduced, false, fmt.Errorf("check changes after %s: %w", l.agentName(), dirtyErr)
	}

	return cost, subtype, commitsProduced, dirty, nil
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
		created, stashErr := l.Git.Stash()
		if stashErr != nil {
			return false, fmt.Errorf("stash: %w", stashErr)
		}
		return created, nil
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
		entry.Timestamp = l.nowTime()
	}
	if entry.Provider == "" {
		if agent, ok := l.Agent.(providerAgent); ok {
			entry.Provider = agent.CurrentProvider()
		}
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

func (l *Loop) nowTime() time.Time {
	if l.now != nil {
		return l.now()
	}
	return time.Now()
}

func (l *Loop) modeConfig(mode Mode) (promptFile string, maxIter int) {
	if l.Roam {
		return l.Config.Roam.PromptFile, l.Config.Roam.MaxIterations
	}
	switch mode {
	case ModeBuild:
		return l.Config.Build.PromptFile, l.Config.Build.MaxIterations
	default:
		return l.Config.Build.PromptFile, l.Config.Build.MaxIterations
	}
}

func cloneQuotaDecision(decision *quota.Decision) *quota.Decision {
	if decision == nil {
		return nil
	}
	cloned := *decision
	if decision.Snapshot != nil {
		snapshot := *decision.Snapshot
		snapshot.Windows = append([]quota.Window(nil), decision.Snapshot.Windows...)
		cloned.Snapshot = &snapshot
	}
	return &cloned
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
	switch l.agentName() {
	case config.AgentCodex:
		return l.Config.Codex.Model
	default:
		return l.Config.Claude.Model
	}
}

func (l *Loop) agentMaxTurns() int {
	if l.agentName() != config.AgentClaude {
		return 0
	}
	return l.Config.Claude.MaxTurns
}

func (l *Loop) agentDangerSkipPermissions() bool {
	switch l.agentName() {
	case config.AgentCodex:
		return false
	default:
		return l.Config.Claude.DangerSkipPermissions
	}
}

// augmentPrompt appends a ## Spec Context section to the prompt when applicable.
// In roam mode, the ROAM.md prompt already defines the codebase-wide mission.
// When a spec name is set (and roam is false), the section names the active spec
// and its directory to keep Claude focused on the spec boundary.
// When focus is non-empty, a focus directive is appended to constrain the topic.
// When neither applies, the prompt is returned unchanged.
func augmentPrompt(prompt, spec, specDir string, roam bool, focus string) string {
	if roam {
		if focus != "" {
			return prompt + fmt.Sprintf("\n\n## Roam Focus\n\nFocus your work on: %s. Prioritize changes related to this area over other improvements.", focus)
		}
		return prompt
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
