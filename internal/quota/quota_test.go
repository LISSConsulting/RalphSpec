package quota

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LISSConsulting/RalphSpec/internal/config"
)

type sequenceProvider struct {
	snapshots []Snapshot
	err       error
	calls     int
}

func (p *sequenceProvider) Quota(context.Context) (Snapshot, error) {
	p.calls++
	if p.err != nil {
		return Snapshot{}, p.err
	}
	index := p.calls - 1
	if index >= len(p.snapshots) {
		index = len(p.snapshots) - 1
	}
	return p.snapshots[index], nil
}

func policy() config.QuotaConfig {
	return config.QuotaConfig{
		Enabled:         true,
		ReservePercent:  10,
		ExhaustedPolicy: config.QuotaFailClosed,
		UnknownPolicy:   config.QuotaAllow,
		MaxWaitSeconds:  3600,
		MaxParallel:     1,
	}
}

func TestGateBlocksAtReserve(t *testing.T) {
	provider := &sequenceProvider{snapshots: []Snapshot{{Windows: []Window{{Name: "weekly", UsedPercent: 90}}}}}
	gate := NewGate(policy(), provider)
	decision, err := gate.Admit(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != ActionBlock || provider.calls != 1 {
		t.Fatalf("decision = %#v, calls = %d; want block", decision, provider.calls)
	}
}

func TestGateUnknownPolicy(t *testing.T) {
	for _, tt := range []struct {
		name   string
		policy string
		want   Action
	}{
		{"allow", config.QuotaAllow, ActionAllow},
		{"fail closed", config.QuotaFailClosed, ActionBlock},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := policy()
			cfg.UnknownPolicy = tt.policy
			gate := NewGate(cfg, nil)
			decision, err := gate.Admit(context.Background())
			if err != nil || decision.Action != tt.want {
				t.Fatalf("decision = %#v, err = %v; want %s", decision, err, tt.want)
			}
		})
	}
}

func TestGateProbeFailureIsUnknown(t *testing.T) {
	cfg := policy()
	cfg.UnknownPolicy = config.QuotaFailClosed
	gate := NewGate(cfg, &sequenceProvider{err: errors.New("probe failed")})
	decision, err := gate.Admit(context.Background())
	if err != nil || decision.Action != ActionBlock {
		t.Fatalf("decision = %#v, err = %v; want block", decision, err)
	}
}

func TestGateWaitsThenRechecks(t *testing.T) {
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	provider := &sequenceProvider{snapshots: []Snapshot{
		{Windows: []Window{{Name: "5h", UsedPercent: 99, ResetAt: now.Add(time.Minute)}}},
		{Windows: []Window{{Name: "5h", UsedPercent: 1, ResetAt: now.Add(5 * time.Hour)}}},
	}}
	cfg := policy()
	cfg.ExhaustedPolicy = config.QuotaWait
	gate := NewGate(cfg, provider)
	gate.now = func() time.Time { return now }
	waits := 0
	gate.wait = func(context.Context, time.Duration) error {
		waits++
		now = now.Add(time.Minute)
		return nil
	}
	var actions []Action

	decision, err := gate.Admit(context.Background(), func(d Decision) { actions = append(actions, d.Action) })
	if err != nil || decision.Action != ActionAllow || waits != 1 || provider.calls != 2 {
		t.Fatalf("decision = %#v, err = %v, waits = %d, calls = %d", decision, err, waits, provider.calls)
	}
	if len(actions) != 2 || actions[0] != ActionWait || actions[1] != ActionAllow {
		t.Fatalf("actions = %v, want [wait allow]", actions)
	}
}

func TestGateRejectsResetOutsideBound(t *testing.T) {
	now := time.Now()
	provider := &sequenceProvider{snapshots: []Snapshot{{Windows: []Window{{Name: "weekly", UsedPercent: 100, ResetAt: now.Add(2 * time.Hour)}}}}}
	cfg := policy()
	cfg.ExhaustedPolicy = config.QuotaWait
	cfg.MaxWaitSeconds = 60
	gate := NewGate(cfg, provider)
	gate.now = func() time.Time { return now }
	decision, err := gate.Admit(context.Background())
	if err != nil || decision.Action != ActionBlock {
		t.Fatalf("decision = %#v, err = %v; want block", decision, err)
	}
}

func TestAcquireLimitsParallelInvocations(t *testing.T) {
	provider := &sequenceProvider{snapshots: []Snapshot{{Windows: []Window{{Name: "weekly", UsedPercent: 10}}}}}
	gate := NewGate(policy(), provider)
	decision, release, err := gate.Acquire(context.Background())
	if err != nil || decision.Action != ActionAllow {
		t.Fatalf("first acquire = %#v, %v", decision, err)
	}

	waitCtx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, _, err := gate.Acquire(waitCtx); err == nil {
		t.Fatal("second acquire should wait for the only slot")
	}

	release()
	decision, release, err = gate.Acquire(context.Background())
	if err != nil || decision.Action != ActionAllow {
		t.Fatalf("acquire after release = %#v, %v", decision, err)
	}
	release()
}
