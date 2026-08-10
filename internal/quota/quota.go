// Package quota implements provider-neutral quota admission for new agent work.
package quota

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/LISSConsulting/RalphSpec/internal/config"
)

// Window is one provider-reported usage window.
type Window struct {
	Name        string        `json:"name"`
	UsedPercent float64       `json:"used_percent"`
	ResetAt     time.Time     `json:"reset_at,omitempty"`
	Duration    time.Duration `json:"duration,omitempty"`
}

func (w Window) RemainingPercent() float64 {
	return math.Max(0, math.Min(100, 100-w.UsedPercent))
}

// Snapshot is one provider observation. An empty window list is unknown.
type Snapshot struct {
	Agent               string    `json:"agent"`
	Provider            string    `json:"provider,omitempty"`
	ObservedAt          time.Time `json:"observed_at"`
	Windows             []Window  `json:"windows,omitempty"`
	SpendControlReached *bool     `json:"spend_control_reached,omitempty"`
	LimitReached        string    `json:"limit_reached,omitempty"`
}

// Provider is implemented by harnesses with a documented quota interface.
type Provider interface {
	Quota(context.Context) (Snapshot, error)
}

type Action string

const (
	ActionAllow Action = "allow"
	ActionWait  Action = "wait"
	ActionBlock Action = "block"
)

// Decision records exactly why a launch was admitted, delayed, or blocked.
type Decision struct {
	Action   Action    `json:"action"`
	Reason   string    `json:"reason"`
	ResumeAt time.Time `json:"resume_at,omitempty"`
	Snapshot *Snapshot `json:"snapshot,omitempty"`
}

// Gate serializes quota checks and waits across workers sharing one provider.
type Gate struct {
	policy   config.QuotaConfig
	provider Provider
	mu       sync.Mutex
	slots    chan struct{}
	now      func() time.Time
	wait     func(context.Context, time.Duration) error
}

func NewGate(policy config.QuotaConfig, provider Provider) *Gate {
	maxParallel := policy.MaxParallel
	if maxParallel < 1 {
		maxParallel = 1
	}
	return &Gate{
		policy:   policy,
		provider: provider,
		slots:    make(chan struct{}, maxParallel),
		now:      time.Now,
		wait:     waitContext,
	}
}

// Acquire reserves one configured concurrency slot, performs quota admission,
// and returns a release function that the caller must invoke when agent work
// ends. Blocked decisions never retain a slot.
func (g *Gate) Acquire(ctx context.Context, observers ...func(Decision)) (Decision, func(), error) {
	if g == nil || !g.policy.Enabled {
		return Decision{Action: ActionAllow, Reason: "quota admission disabled"}, func() {}, nil
	}
	select {
	case g.slots <- struct{}{}:
	case <-ctx.Done():
		return Decision{Action: ActionBlock, Reason: ctx.Err().Error()}, func() {}, ctx.Err()
	}
	var once sync.Once
	release := func() {
		once.Do(func() { <-g.slots })
	}
	decision, err := g.Admit(ctx, observers...)
	if err != nil || decision.Action == ActionBlock {
		release()
		return decision, func() {}, err
	}
	return decision, release, nil
}

// Admit checks fresh quota and, for bounded wait policy, waits and rechecks.
// The mutex stays held through a wait so parallel workers cannot all reserve
// the same remaining window concurrently.
func (g *Gate) Admit(ctx context.Context, observers ...func(Decision)) (Decision, error) {
	if g == nil || !g.policy.Enabled {
		return Decision{Action: ActionAllow, Reason: "quota admission disabled"}, nil
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	started := g.now()
	for {
		decision := g.check(ctx, started)
		emitDecision(observers, decision)
		if decision.Action != ActionWait {
			return decision, nil
		}
		delay := decision.ResumeAt.Sub(g.now())
		if delay <= 0 {
			continue
		}
		if err := g.wait(ctx, delay); err != nil {
			return Decision{Action: ActionBlock, Reason: err.Error(), Snapshot: decision.Snapshot}, err
		}
	}
}

func (g *Gate) check(ctx context.Context, started time.Time) Decision {
	if g.provider == nil {
		return g.unknown("selected harness has no quota snapshot capability")
	}
	snapshot, err := g.provider.Quota(ctx)
	if err != nil {
		return g.unknown(fmt.Sprintf("quota snapshot unavailable: %v", err))
	}
	if snapshot.ObservedAt.IsZero() {
		snapshot.ObservedAt = g.now()
	}
	if snapshot.LimitReached != "" {
		return Decision{Action: ActionBlock, Reason: "provider limit reached: " + snapshot.LimitReached, Snapshot: &snapshot}
	}
	if snapshot.SpendControlReached != nil && *snapshot.SpendControlReached {
		return Decision{Action: ActionBlock, Reason: "provider spend control reached", Snapshot: &snapshot}
	}
	if len(snapshot.Windows) == 0 {
		decision := g.unknown("provider returned no quota windows")
		decision.Snapshot = &snapshot
		return decision
	}

	var blocking *Window
	for i := range snapshot.Windows {
		window := &snapshot.Windows[i]
		if window.RemainingPercent() <= g.policy.ReservePercent &&
			(blocking == nil || window.RemainingPercent() < blocking.RemainingPercent()) {
			blocking = window
		}
	}
	if blocking == nil {
		return Decision{
			Action:   ActionAllow,
			Reason:   fmt.Sprintf("quota above %.1f%% reserve", g.policy.ReservePercent),
			Snapshot: &snapshot,
		}
	}

	reason := fmt.Sprintf("%s quota has %.1f%% remaining, at or below %.1f%% reserve",
		blocking.Name, blocking.RemainingPercent(), g.policy.ReservePercent)
	if g.policy.ExhaustedPolicy != config.QuotaWait || blocking.ResetAt.IsZero() {
		return Decision{Action: ActionBlock, Reason: reason, ResumeAt: blocking.ResetAt, Snapshot: &snapshot}
	}
	maxWait := time.Duration(g.policy.MaxWaitSeconds) * time.Second
	if !blocking.ResetAt.After(g.now()) || blocking.ResetAt.Sub(started) > maxWait {
		return Decision{Action: ActionBlock, Reason: reason + "; reset is outside the allowed wait", ResumeAt: blocking.ResetAt, Snapshot: &snapshot}
	}
	return Decision{Action: ActionWait, Reason: reason, ResumeAt: blocking.ResetAt, Snapshot: &snapshot}
}

func (g *Gate) unknown(reason string) Decision {
	if g.policy.UnknownPolicy == config.QuotaFailClosed {
		return Decision{Action: ActionBlock, Reason: reason + "; unknown quota policy is fail_closed"}
	}
	return Decision{Action: ActionAllow, Reason: reason + "; unknown quota policy allows launch"}
}

func emitDecision(observers []func(Decision), decision Decision) {
	for _, observer := range observers {
		if observer != nil {
			observer(decision)
		}
	}
}

func waitContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
