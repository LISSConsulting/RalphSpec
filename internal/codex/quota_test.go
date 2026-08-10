package codex

import (
	"bufio"
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestQuotaReadsAppServerWindows(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	reset := time.Now().Add(time.Hour).Unix()
	output := strings.Join([]string{
		`{"id":1,"result":{"userAgent":"test"}}`,
		`{"method":"account/rateLimits/updated","params":{"rateLimits":{}}}`,
		`{"id":2,"result":{"rateLimits":{"primary":{"usedPercent":25,"windowDurationMins":300,"resetsAt":` + formatInt(reset) + `},"secondary":{"usedPercent":80,"windowDurationMins":10080,"resetsAt":` + formatInt(reset+60) + `},"individualLimit":{"remainingPercent":40,"resetsAt":` + formatInt(reset+120) + `},"rateLimitReachedType":null}}}`,
	}, "\n")
	agent := setUpFakeCodex(t, exe, 0, output, "")

	snapshot, err := agent.Quota(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Agent != "codex" || snapshot.Provider != "openai" || len(snapshot.Windows) != 3 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if got := snapshot.Windows[0]; got.Name != "primary" || got.UsedPercent != 25 || got.Duration != 5*time.Hour || got.ResetAt.Unix() != reset {
		t.Fatalf("primary = %#v", got)
	}
	if got := snapshot.Windows[2]; got.Name != "individual_spend_control" || got.UsedPercent != 60 || got.ResetAt.Unix() != reset+120 {
		t.Fatalf("individual spend control = %#v", got)
	}
}

func TestQuotaMapsReachedTypeToBlockedState(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	output := strings.Join([]string{
		`{"id":1,"result":{}}`,
		`{"id":2,"result":{"rateLimits":{"rateLimitReachedType":"workspace_owner_credits_depleted"}}}`,
	}, "\n")
	agent := setUpFakeCodex(t, exe, 0, output, "")
	snapshot, err := agent.Quota(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.LimitReached != "workspace_owner_credits_depleted" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestReadRPCResponseReportsError(t *testing.T) {
	scanner := bufioNewScanner(strings.NewReader(`{"id":2,"error":{"code":-32001,"message":"Server overloaded; retry later."}}`))
	_, err := readRPCResponse(scanner, 2)
	if err == nil || !strings.Contains(err.Error(), "Server overloaded") {
		t.Fatalf("err = %v", err)
	}
}

func formatInt(value int64) string {
	return strconv.FormatInt(value, 10)
}

func bufioNewScanner(reader *strings.Reader) *bufio.Scanner {
	return bufio.NewScanner(reader)
}
