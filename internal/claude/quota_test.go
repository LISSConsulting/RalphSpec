package claude

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestReadQuotaSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "quota.json")
	reset := time.Now().Add(time.Hour).Unix()
	data := `{"rate_limits":{"five_hour":{"used_percentage":23.5,"resets_at":` + strconv.FormatInt(reset, 10) + `},"seven_day":{"used_percentage":41.2,"resets_at":"2030-01-02T03:04:05Z"}}}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}

	snapshot, err := ReadQuotaSnapshot(path, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Agent != "claude" || len(snapshot.Windows) != 2 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if got := snapshot.Windows[0]; got.Name != "five_hour" || got.UsedPercent != 23.5 || got.ResetAt.Unix() != reset {
		t.Fatalf("five-hour window = %#v", got)
	}
}

func TestReadQuotaSnapshotRejectsStale(t *testing.T) {
	path := filepath.Join(t.TempDir(), "quota.json")
	if err := os.WriteFile(path, []byte(`{"rate_limits":{"five_hour":{"used_percentage":1}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadQuotaSnapshot(path, time.Minute); err == nil {
		t.Fatal("expected stale snapshot error")
	}
}

func TestReadQuotaSnapshotRequiresWindow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "quota.json")
	if err := os.WriteFile(path, []byte(`{"rate_limits":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadQuotaSnapshot(path, 0); err == nil {
		t.Fatal("expected missing-window error")
	}
}
