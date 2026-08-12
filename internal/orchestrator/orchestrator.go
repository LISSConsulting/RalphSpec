package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/LISSConsulting/RalphSpec/internal/claude"
	"github.com/LISSConsulting/RalphSpec/internal/codex"
	"github.com/LISSConsulting/RalphSpec/internal/config"
	"github.com/LISSConsulting/RalphSpec/internal/git"
	"github.com/LISSConsulting/RalphSpec/internal/loop"
	"github.com/LISSConsulting/RalphSpec/internal/quota"
	"github.com/LISSConsulting/RalphSpec/internal/regent"
	"github.com/LISSConsulting/RalphSpec/internal/testplan"
	"github.com/LISSConsulting/RalphSpec/internal/worktree"
)

const mergedEventsBuf = 256

type quotaFallbackAgent interface {
	FailoverQuota() (from, to string, ok bool)
}

// Orchestrator manages the lifecycle of multiple WorktreeAgents.
type Orchestrator struct {
	mu               sync.Mutex
	agents           map[string]*WorktreeAgent // keyed by branch name
	fanInWg          sync.WaitGroup
	MaxParallel      int
	AutoMerge        bool
	MergeTarget      string
	WorktreeOps      worktree.WorktreeOps
	cfg              *config.Config
	quotaGates       map[string]*quota.Gate
	claudeRouter     *claude.ProviderRouter
	claudeRouterErr  error
	claudeRouterOnce sync.Once

	// MergedEvents receives tagged events from all agent fan-in goroutines.
	// Consumers (e.g. TUI) read from this channel.
	MergedEvents chan TaggedLogEntry

	// NotificationHook, if set, is called for synthesised merge-result log
	// entries so the external notification system can fire webhooks.
	NotificationHook func(loop.LogEntry)
}

// New creates an Orchestrator with the given settings.
func New(cfg *config.Config, ops worktree.WorktreeOps) *Orchestrator {
	if cfg == nil {
		cfg = &config.Config{}
	}
	maxParallel := cfg.Worktree.MaxParallel
	if maxParallel < 1 {
		maxParallel = 1
	}
	return &Orchestrator{
		agents:       make(map[string]*WorktreeAgent),
		MaxParallel:  maxParallel,
		AutoMerge:    cfg.Worktree.AutoMerge,
		MergeTarget:  cfg.Worktree.MergeTarget,
		WorktreeOps:  ops,
		cfg:          cfg,
		quotaGates:   make(map[string]*quota.Gate),
		MergedEvents: make(chan TaggedLogEntry, mergedEventsBuf),
	}
}

// ActiveAgents returns a snapshot of agents that have not been removed.
func (o *Orchestrator) ActiveAgents() []*WorktreeAgent {
	o.mu.Lock()
	defer o.mu.Unlock()

	var result []*WorktreeAgent
	for _, a := range o.agents {
		if a.State != StateRemoved {
			result = append(result, a)
		}
	}
	return result
}

// RunningCount returns the number of agents currently in StateRunning.
func (o *Orchestrator) RunningCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()

	count := 0
	for _, a := range o.agents {
		if a.State == StateRunning {
			count++
		}
	}
	return count
}

// AgentByBranch returns the agent for the given branch, or nil if not found.
func (o *Orchestrator) AgentByBranch(branch string) *WorktreeAgent {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.agents[branch]
}

// AgentState returns the current state of the agent for the given branch
// under the orchestrator lock. Returns StateCreating and false if not found.
func (o *Orchestrator) AgentState(branch string) (AgentState, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	a, ok := o.agents[branch]
	if !ok {
		return StateCreating, false
	}
	return a.State, true
}

// Launch creates a worktree for branch, starts a loop inside it, and registers
// the agent with the fan-in multiplexer. The loop runs in a background
// goroutine.
//
// Returns an error if:
//   - max_parallel is already reached
//   - an agent for branch already exists and is not in a terminal state
//   - WorktreeOps.Switch() fails
func (o *Orchestrator) Launch(ctx context.Context, branch, specName, specDir string, mode loop.Mode, maxOverride int) error {
	agentType, agentImpl, err := o.buildAgent()
	if err != nil {
		return err
	}
	sharedQuotaGate := o.quotaGateFor(agentType, agentImpl)
	var initialQuotaRelease func()
	var initialQuotaDecision *quota.Decision
	if sharedQuotaGate != nil {
		for {
			decision, release, quotaErr := sharedQuotaGate.Acquire(ctx, func(d quota.Decision) {
				decisionCopy := d
				entry := TaggedLogEntry{Branch: branch, Agent: agentType, Entry: loop.LogEntry{
					Kind:          loop.LogInfo,
					Agent:         agentType,
					Branch:        branch,
					Message:       fmt.Sprintf("Quota %s before worker launch: %s", d.Action, d.Reason),
					QuotaDecision: &decisionCopy,
				}}
				select {
				case o.MergedEvents <- entry:
				case <-ctx.Done():
				}
			})
			if quotaErr != nil {
				return fmt.Errorf("orchestrator: quota admission: %w", quotaErr)
			}
			if decision.Action != quota.ActionBlock {
				initialQuotaRelease = release
				decisionCopy := decision
				initialQuotaDecision = &decisionCopy
				break
			}
			if release != nil {
				release()
			}
			fallback, supportsFallback := agentImpl.(quotaFallbackAgent)
			from, to, switched := "", "", false
			if supportsFallback {
				from, to, switched = fallback.FailoverQuota()
			}
			if !switched {
				return fmt.Errorf("orchestrator: quota admission blocked: %s", decision.Reason)
			}
			entry := TaggedLogEntry{Branch: branch, Agent: agentType, Entry: loop.LogEntry{
				Kind:     loop.LogInfo,
				Agent:    agentType,
				Provider: to,
				Branch:   branch,
				Message:  fmt.Sprintf("Provider %s quota admission blocked; switching %s to %s", from, agentType, to),
			}}
			select {
			case o.MergedEvents <- entry:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	o.mu.Lock()

	// Reject duplicate branches (non-terminal state).
	if existing, ok := o.agents[branch]; ok {
		switch existing.State {
		case StateRunning, StateCreating:
			o.mu.Unlock()
			if initialQuotaRelease != nil {
				initialQuotaRelease()
			}
			return fmt.Errorf("orchestrator: agent already running on branch %s", branch)
		}
	}

	if o.runningCount() >= o.MaxParallel {
		o.mu.Unlock()
		if initialQuotaRelease != nil {
			initialQuotaRelease()
		}
		return fmt.Errorf("orchestrator: max parallel agents (%d) reached", o.MaxParallel)
	}

	// events is the outward-facing channel consumed by the fan-in goroutine.
	// loopEvents is the internal channel the loop writes to; a drain goroutine
	// bridges the two and calls rgt.UpdateState() to keep the hang timer alive.
	events := make(chan loop.LogEntry, 128)
	loopEvents := make(chan loop.LogEntry, 128)
	stopCh := make(chan struct{})

	agent := &WorktreeAgent{
		Branch:   branch,
		SpecName: specName,
		SpecDir:  specDir,
		Agent:    agentType,
		State:    StateCreating,
		Events:   events,
		StopCh:   stopCh,
	}
	o.agents[branch] = agent
	o.mu.Unlock()

	// Create/switch worktree outside the lock (subprocess call).
	// Try reuse first (branch already exists as a feature branch), then create.
	wtPath, err := o.WorktreeOps.Switch(branch, false)
	if err != nil {
		wtPath, err = o.WorktreeOps.Switch(branch, true)
	}
	if err != nil {
		o.mu.Lock()
		agent.State = StateFailed
		agent.Error = err
		o.mu.Unlock()
		close(events)
		if initialQuotaRelease != nil {
			initialQuotaRelease()
		}
		return fmt.Errorf("orchestrator: create worktree for %s: %w", branch, err)
	}
	var discoveredPlan *testplan.Plan
	if o.cfg.Regent.AutoDiscoverTests || strings.TrimSpace(o.cfg.Regent.TestCommand) != "" {
		plan, discoverErr := testplan.Discover(wtPath, o.cfg.Regent.TestCommand)
		if discoverErr != nil || len(plan.Steps) == 0 {
			if discoverErr == nil {
				discoverErr = fmt.Errorf("no high-confidence test command found")
			}
			o.mu.Lock()
			agent.State = StateFailed
			agent.Error = discoverErr
			o.mu.Unlock()
			close(events)
			if initialQuotaRelease != nil {
				initialQuotaRelease()
			}
			return fmt.Errorf("orchestrator: test plan discovery: %w", discoverErr)
		}
		discoveredPlan = &plan
	}

	o.mu.Lock()
	agent.WorktreePath = wtPath
	agent.State = StateRunning
	o.mu.Unlock()

	// Register with fan-in so events reach MergedEvents.
	// The onEntry callback updates per-agent stats under the lock.
	startFanIn(branch, agentType, events, o.MergedEvents, func(e loop.LogEntry) {
		if e.Kind == loop.LogIterComplete {
			o.mu.Lock()
			agent.Iterations++
			agent.TotalCost += e.CostUSD
			o.mu.Unlock()
		}
	}, &o.fanInWg)

	// Build the loop for this worktree.
	lp := &loop.Loop{
		Agent:                agentImpl,
		AgentType:            agentType,
		Git:                  git.NewRunner(wtPath),
		Config:               o.cfg,
		Dir:                  wtPath,
		Events:               loopEvents,
		Spec:                 specName,
		SpecDir:              specDir,
		StopAfter:            stopCh,
		QuotaGate:            sharedQuotaGate,
		InitialQuotaRelease:  initialQuotaRelease,
		InitialQuotaDecision: initialQuotaDecision,
		TestPlan:             discoveredPlan,
	}

	go func() {
		// Each agent runs its own Regent instance for independent supervision
		// (crash detection, hang detection, per-worktree rollback). Creating one
		// Regent per agent satisfies FR-019/FR-020: failures are isolated — a
		// hanging or crashing agent does not affect any peer.
		//
		// The Regent emits its own messages (start, retry, hang) directly to the
		// events channel. Loop output flows through loopEvents → drain goroutine
		// → events, with the drain calling rgt.UpdateState() to reset the hang
		// timer on every loop event.
		var runErr error
		if o.cfg.Regent.Enabled {
			rgt := regent.New(o.cfg.Regent, wtPath, git.NewRunner(wtPath), events)
			rgt.SetTestPlan(discoveredPlan)
			rgt.SetLudicrous(o.cfg.Build.Ludicrous)
			lp.PostIteration = rgt.RunPostIterationTests

			// Drain loop events → regent state update → fan-in channel.
			drainDone := make(chan struct{})
			go func() {
				defer close(drainDone)
				for entry := range loopEvents {
					if entry.Kind != loop.LogRegent {
						rgt.UpdateState(entry)
					}
					select {
					case events <- entry:
					default:
					}
				}
			}()

			runErr = rgt.Supervise(ctx, func(ctx context.Context) error {
				return lp.Run(ctx, mode, maxOverride)
			})
			close(loopEvents)
			<-drainDone
		} else {
			// No Regent — forward loop events directly.
			drainDone := make(chan struct{})
			go func() {
				defer close(drainDone)
				for entry := range loopEvents {
					select {
					case events <- entry:
					default:
					}
				}
			}()

			runErr = lp.Run(ctx, mode, maxOverride)
			close(loopEvents)
			<-drainDone
		}

		o.mu.Lock()
		switch {
		case errors.Is(runErr, loop.ErrOperatorStopped):
			agent.State = StateStopped
			agent.Error = nil
		case runErr != nil && !errors.Is(runErr, context.Canceled):
			agent.State = StateFailed
			agent.Error = runErr
		case agent.State == StateRunning:
			agent.State = StateCompleted
		}
		finalState := agent.State
		o.mu.Unlock()

		close(events)

		if finalState == StateCompleted {
			o.autoMergeIfNeeded(agent, branch)
		}
	}()

	return nil
}

func (o *Orchestrator) quotaGateFor(agentType string, agentImpl claude.Agent) *quota.Gate {
	if !o.cfg.Quota.Enabled {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if gate := o.quotaGates[agentType]; gate != nil {
		return gate
	}
	var provider quota.Provider
	if supported, ok := agentImpl.(quota.Provider); ok {
		provider = supported
	}
	gate := quota.NewGate(o.cfg.Quota, provider)
	o.quotaGates[agentType] = gate
	return gate
}

// Stop requests a graceful stop for the agent on branch by closing its StopCh.
func (o *Orchestrator) Stop(branch string) error {
	o.mu.Lock()
	agent, ok := o.agents[branch]
	if !ok {
		o.mu.Unlock()
		return fmt.Errorf("orchestrator: no agent for branch %s", branch)
	}
	if agent.State != StateRunning {
		o.mu.Unlock()
		return fmt.Errorf("orchestrator: agent %s is not running (state: %s)", branch, agent.State)
	}
	o.mu.Unlock()

	// Close stop channel; loop checks it after each iteration.
	func() {
		defer func() { _ = recover() }()
		close(agent.StopCh)
	}()

	o.mu.Lock()
	if agent.State == StateRunning {
		agent.State = StateStopped
	}
	o.mu.Unlock()
	return nil
}

// StopAll stops every currently running agent.
func (o *Orchestrator) StopAll() {
	o.mu.Lock()
	branches := make([]string, 0, len(o.agents))
	for b, a := range o.agents {
		if a.State == StateRunning {
			branches = append(branches, b)
		}
	}
	o.mu.Unlock()

	for _, b := range branches {
		_ = o.Stop(b)
	}
}

// Merge merges a completed/stopped worktree branch into the merge target.
func (o *Orchestrator) Merge(branch string) error {
	o.mu.Lock()
	agent, ok := o.agents[branch]
	if !ok {
		o.mu.Unlock()
		return fmt.Errorf("orchestrator: no agent for branch %s", branch)
	}
	if agent.State == StateRunning || agent.State == StateCreating {
		o.mu.Unlock()
		return fmt.Errorf("orchestrator: cannot merge running agent %s — stop it first", branch)
	}
	agent.State = StateMerging
	o.mu.Unlock()

	if err := o.WorktreeOps.Merge(branch, o.MergeTarget); err != nil {
		o.mu.Lock()
		agent.State = StateMergeFailed
		agent.Error = err
		o.mu.Unlock()
		return fmt.Errorf("orchestrator: merge %s: %w", branch, err)
	}

	o.mu.Lock()
	agent.State = StateMerged
	o.mu.Unlock()
	return nil
}

// Clean removes a non-running worktree agent via worktrunk.
func (o *Orchestrator) Clean(branch string) error {
	o.mu.Lock()
	agent, ok := o.agents[branch]
	if !ok {
		o.mu.Unlock()
		return fmt.Errorf("orchestrator: no agent for branch %s", branch)
	}
	if agent.State == StateRunning || agent.State == StateCreating {
		o.mu.Unlock()
		return fmt.Errorf("orchestrator: cannot clean running agent %s — stop it first", branch)
	}
	o.mu.Unlock()

	if err := o.WorktreeOps.Remove(branch); err != nil {
		return fmt.Errorf("orchestrator: clean %s: %w", branch, err)
	}

	o.mu.Lock()
	agent.State = StateRemoved
	o.mu.Unlock()
	return nil
}

// WorktreePaths returns the working directory path of every non-removed agent.
func (o *Orchestrator) WorktreePaths() []string {
	o.mu.Lock()
	defer o.mu.Unlock()

	var paths []string
	for _, a := range o.agents {
		if a.State != StateRemoved && a.WorktreePath != "" {
			paths = append(paths, a.WorktreePath)
		}
	}
	return paths
}

// runningCount returns the count of StateRunning agents; callers must hold o.mu.
func (o *Orchestrator) runningCount() int {
	count := 0
	for _, a := range o.agents {
		if a.State == StateRunning {
			count++
		}
	}
	return count
}

// autoMergeIfNeeded runs optional test-gating and then merges the branch when
// AutoMerge is enabled and the agent completed successfully.  It is called
// from the Launch goroutine after the loop exits with StateCompleted.
//
//   - If AutoMerge is false, this is a no-op.
//   - If regent.test_command is configured, the command is executed inside the
//     worktree directory.  A failing test run skips the merge and logs a warning.
//   - If wt merge fails the agent transitions to StateMergeFailed.
//   - Merge result events are emitted to MergedEvents and to NotificationHook.
func (o *Orchestrator) autoMergeIfNeeded(agent *WorktreeAgent, branch string) {
	if !o.AutoMerge {
		return
	}

	// Optional test-gating (uses regent.RunTests which supports Windows cmd /C).
	if tc := o.cfg.Regent.TestCommand; tc != "" {
		res, err := regent.RunTests(agent.WorktreePath, tc)
		if err != nil {
			o.emitToMerged(branch, loop.LogEntry{
				Kind:    loop.LogError,
				Message: fmt.Sprintf("worktree %s: could not run tests: %v — skipping auto-merge", branch, err),
			})
			return
		}
		if !res.Passed {
			o.emitToMerged(branch, loop.LogEntry{
				Kind:    loop.LogInfo,
				Message: fmt.Sprintf("worktree %s: tests failed — skipping auto-merge\n%s", branch, res.Output),
			})
			return
		}
	}

	if err := o.Merge(branch); err != nil {
		o.emitToMerged(branch, loop.LogEntry{
			Kind:    loop.LogError,
			Agent:   agent.Agent,
			Message: fmt.Sprintf("worktree %s: auto-merge failed: %v", branch, err),
		})
	} else {
		o.emitToMerged(branch, loop.LogEntry{
			Kind:    loop.LogInfo,
			Agent:   agent.Agent,
			Message: fmt.Sprintf("worktree %s: auto-merge completed successfully", branch),
		})
	}
}

// emitToMerged sends a synthesised log entry to MergedEvents (non-blocking)
// and fires NotificationHook if configured.
func (o *Orchestrator) emitToMerged(branch string, entry loop.LogEntry) {
	agent := ""
	o.mu.Lock()
	if a, ok := o.agents[branch]; ok {
		agent = a.Agent
	}
	o.mu.Unlock()
	if entry.Agent == "" {
		entry.Agent = agent
	}
	select {
	case o.MergedEvents <- TaggedLogEntry{Branch: branch, Agent: entry.Agent, Entry: entry}:
	default:
	}
	if o.NotificationHook != nil {
		o.NotificationHook(entry)
	}
}

func (o *Orchestrator) buildAgent() (string, claude.Agent, error) {
	agentType := config.AgentClaude
	if o.cfg != nil && o.cfg.Agent.Type != "" {
		agentType = o.cfg.Agent.Type
	}
	harness := config.HarnessConfig{Claude: "claude", Codex: "codex"}
	if o.cfg != nil {
		if o.cfg.Harness.Claude != "" {
			harness.Claude = o.cfg.Harness.Claude
		}
		if o.cfg.Harness.Codex != "" {
			harness.Codex = o.cfg.Harness.Codex
		}
	}
	switch agentType {
	case config.AgentClaude:
		o.claudeRouterOnce.Do(func() {
			o.claudeRouter, o.claudeRouterErr = claude.LoadProviderRouter(
				o.cfg.Claude.Provider,
				o.cfg.Claude.FallbackProviders,
				o.cfg.Claude.ProviderConfigFile,
				os.Environ(),
			)
		})
		if o.claudeRouterErr != nil {
			return "", nil, fmt.Errorf("claude provider fallback: %w", o.claudeRouterErr)
		}
		return agentType, &loop.ClaudeAgent{
			Executable:          harness.Claude,
			QuotaSnapshotFile:   o.cfg.Claude.QuotaSnapshotFile,
			QuotaSnapshotMaxAge: time.Duration(o.cfg.Claude.QuotaSnapshotMaxAgeSeconds) * time.Second,
			ProviderRouter:      o.claudeRouter,
		}, nil
	case config.AgentCodex:
		if err := codex.CheckAvailable(harness.Codex); err != nil {
			return "", nil, fmt.Errorf("codex agent unavailable: %w", err)
		}
		return agentType, &codex.Agent{Executable: harness.Codex}, nil
	default:
		return "", nil, fmt.Errorf("agent.type must be one of %s,%s", config.AgentClaude, config.AgentCodex)
	}
}
