package store_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/LISSConsulting/RalphSpec/internal/loop"
	"github.com/LISSConsulting/RalphSpec/internal/store"
)

// Compile-time check: *JSONL implements Store.
var _ store.Store = (*store.JSONL)(nil)

func TestNewJSONL_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	s, err := store.NewJSONL(dir)
	if err != nil {
		t.Fatalf("NewJSONL: %v", err)
	}
	defer func() { _ = s.Close() }()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 file in dir, got %d", len(entries))
	}
	if ext := filepath.Ext(entries[0].Name()); ext != ".jsonl" {
		t.Errorf("expected .jsonl extension, got %q", ext)
	}
}

func TestNewJSONL_CreatesDir(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "subdir", "logs")
	s, err := store.NewJSONL(dir)
	if err != nil {
		t.Fatalf("NewJSONL on non-existent dir: %v", err)
	}
	defer func() { _ = s.Close() }()

	if _, err := os.Stat(dir); err != nil {
		t.Errorf("expected dir to exist after NewJSONL: %v", err)
	}
}

func TestAppendAndIterationLog(t *testing.T) {
	dir := t.TempDir()
	s, err := store.NewJSONL(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	now := time.Now()
	entries := []loop.LogEntry{
		{Kind: loop.LogIterStart, Timestamp: now, Iteration: 1, Agent: "codex", Mode: "build", Branch: "main"},
		{Kind: loop.LogToolUse, Timestamp: now, Iteration: 1, ToolName: "Read", ToolInput: "main.go"},
		{Kind: loop.LogIterComplete, Timestamp: now, Iteration: 1, CostUSD: 0.05, Duration: 1.2, Subtype: "success"},
	}
	for _, e := range entries {
		if err := s.Append(e); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	got, err := s.IterationLog(1)
	if err != nil {
		t.Fatalf("IterationLog(1): %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(got))
	}
	if got[0].Kind != loop.LogIterStart {
		t.Errorf("got[0].Kind: expected LogIterStart, got %v", got[0].Kind)
	}
	if got[1].Kind != loop.LogToolUse {
		t.Errorf("got[1].Kind: expected LogToolUse, got %v", got[1].Kind)
	}
	if got[1].ToolName != "Read" {
		t.Errorf("got[1].ToolName: expected %q, got %q", "Read", got[1].ToolName)
	}
	if got[2].Kind != loop.LogIterComplete {
		t.Errorf("got[2].Kind: expected LogIterComplete, got %v", got[2].Kind)
	}
	if got[2].CostUSD != 0.05 {
		t.Errorf("got[2].CostUSD: expected 0.05, got %v", got[2].CostUSD)
	}
}

func TestIterations(t *testing.T) {
	dir := t.TempDir()
	s, err := store.NewJSONL(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	now := time.Now()
	for i := 1; i <= 3; i++ {
		_ = s.Append(loop.LogEntry{Kind: loop.LogIterStart, Iteration: i, Mode: "build", Branch: "main", Timestamp: now})
		_ = s.Append(loop.LogEntry{Kind: loop.LogIterComplete, Iteration: i, Agent: "claude", Provider: "kimi", CostUSD: float64(i) * 0.01, Duration: float64(i), Subtype: "success", Timestamp: now})
	}

	iters, err := s.Iterations()
	if err != nil {
		t.Fatal(err)
	}
	if len(iters) != 3 {
		t.Fatalf("expected 3 iterations, got %d", len(iters))
	}
	for i, it := range iters {
		wantNum := i + 1
		if it.Number != wantNum {
			t.Errorf("iters[%d].Number: expected %d, got %d", i, wantNum, it.Number)
		}
		if it.Mode != "build" {
			t.Errorf("iters[%d].Mode: expected build, got %q", i, it.Mode)
		}
		if it.Agent != "claude" {
			t.Errorf("iters[%d].Agent: expected claude, got %q", i, it.Agent)
		}
		if it.Provider != "kimi" {
			t.Errorf("iters[%d].Provider: expected kimi, got %q", i, it.Provider)
		}
		wantCost := float64(wantNum) * 0.01
		if it.CostUSD != wantCost {
			t.Errorf("iters[%d].CostUSD: expected %.2f, got %.2f", i, wantCost, it.CostUSD)
		}
		if it.Subtype != "success" {
			t.Errorf("iters[%d].Subtype: expected success, got %q", i, it.Subtype)
		}
	}
}

func TestIterationsReturnsCopy(t *testing.T) {
	dir := t.TempDir()
	s, err := store.NewJSONL(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	now := time.Now()
	_ = s.Append(loop.LogEntry{Kind: loop.LogIterStart, Iteration: 1, Timestamp: now})
	_ = s.Append(loop.LogEntry{Kind: loop.LogIterComplete, Iteration: 1, CostUSD: 0.05, Timestamp: now})

	first, _ := s.Iterations()
	first[0].CostUSD = 999.0 // mutate the returned copy

	second, _ := s.Iterations()
	if second[0].CostUSD == 999.0 {
		t.Error("Iterations should return a copy; mutation affected internal state")
	}
}

func TestOpenSession_ReadsPastLog(t *testing.T) {
	dir := t.TempDir()
	s, err := store.NewJSONL(dir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	_ = s.Append(loop.LogEntry{Kind: loop.LogInfo, Agent: "claude", Provider: "anthropic", Branch: "main", Timestamp: now})
	_ = s.Append(loop.LogEntry{Kind: loop.LogIterStart, Iteration: 1, Agent: "claude", Provider: "anthropic", Timestamp: now})
	_ = s.Append(loop.LogEntry{Kind: loop.LogText, Iteration: 1, Provider: "kimi", Message: "thinking", Timestamp: now})
	_ = s.Append(loop.LogEntry{Kind: loop.LogIterComplete, Iteration: 1, Provider: "kimi", CostUSD: 0.5, Timestamp: now})
	sum, err := s.SessionSummary()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	r, err := store.OpenSession(filepath.Join(dir, sum.SessionID+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()

	iters, err := r.Iterations()
	if err != nil {
		t.Fatal(err)
	}
	if len(iters) != 1 || iters[0].Number != 1 {
		t.Fatalf("Iterations: got %+v", iters)
	}

	entries, err := r.IterationLog(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("IterationLog: expected 3 entries, got %d", len(entries))
	}
	if entries[1].Kind != loop.LogText || entries[1].Message != "thinking" {
		t.Errorf("IterationLog entry[1]: got %+v", entries[1])
	}

	got, err := r.SessionSummary()
	if err != nil {
		t.Fatal(err)
	}
	if got.SessionID != sum.SessionID {
		t.Errorf("SessionID: got %q, want %q", got.SessionID, sum.SessionID)
	}
	if got.Agent != "claude" || got.Branch != "main" {
		t.Errorf("metadata: got agent=%q branch=%q", got.Agent, got.Branch)
	}
	if got.Provider != "kimi" {
		t.Errorf("Provider: got %q, want kimi", got.Provider)
	}
	if got.TotalCost != 0.5 {
		t.Errorf("TotalCost: got %v, want 0.5", got.TotalCost)
	}
	if got.StartedAt.IsZero() {
		t.Error("StartedAt should be derived from the first entry")
	}
}

func TestListSessions(t *testing.T) {
	dir := t.TempDir()

	sessions, err := store.ListSessions(dir)
	if err != nil {
		t.Fatal(err)
	}
	if sessions != nil {
		t.Errorf("expected nil for empty dir, got %v", sessions)
	}

	// Two session files: rename the first (same-second, same-pid names
	// collide) so the second gets its own file.
	first, err := store.NewJSONL(dir)
	if err != nil {
		t.Fatal(err)
	}
	_ = first.Append(loop.LogEntry{Kind: loop.LogInfo, Agent: "claude", Timestamp: time.Now()})
	sum1, err := first.SessionSummary()
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(dir, sum1.SessionID+".jsonl"), filepath.Join(dir, "0000000001-1.jsonl")); err != nil {
		t.Fatal(err)
	}

	second, err := store.NewJSONL(dir)
	if err != nil {
		t.Fatal(err)
	}
	_ = second.Append(loop.LogEntry{Kind: loop.LogInfo, Agent: "codex", Timestamp: time.Now()})
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}

	sessions, err = store.ListSessions(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
	// Newest first: the real-timestamp file sorts after the renamed 0000000001 file.
	if sessions[0].Agent != "codex" {
		t.Errorf("newest session should be codex, got %q", sessions[0].Agent)
	}
	if sessions[1].SessionID != "0000000001-1" {
		t.Errorf("oldest session ID: got %q", sessions[1].SessionID)
	}
}

func TestSessionSummary(t *testing.T) {
	dir := t.TempDir()
	s, err := store.NewJSONL(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	now := time.Now()
	_ = s.Append(loop.LogEntry{Kind: loop.LogInfo, Agent: "codex", Branch: "feat/test", Commit: "abc123", Timestamp: now})
	_ = s.Append(loop.LogEntry{Kind: loop.LogIterStart, Iteration: 1, Agent: "codex", Timestamp: now})
	_ = s.Append(loop.LogEntry{Kind: loop.LogIterComplete, Iteration: 1, CostUSD: 1.0, Timestamp: now})
	_ = s.Append(loop.LogEntry{Kind: loop.LogIterStart, Iteration: 2, Agent: "codex", Timestamp: now})
	_ = s.Append(loop.LogEntry{Kind: loop.LogIterComplete, Iteration: 2, CostUSD: 2.0, Timestamp: now})

	sum, err := s.SessionSummary()
	if err != nil {
		t.Fatal(err)
	}
	if sum.Iterations != 2 {
		t.Errorf("Iterations: expected 2, got %d", sum.Iterations)
	}
	if sum.TotalCost != 3.0 {
		t.Errorf("TotalCost: expected 3.0, got %v", sum.TotalCost)
	}
	if sum.Branch != "feat/test" {
		t.Errorf("Branch: expected %q, got %q", "feat/test", sum.Branch)
	}
	if sum.Agent != "codex" {
		t.Errorf("Agent: expected %q, got %q", "codex", sum.Agent)
	}
	if sum.LastCommit != "abc123" {
		t.Errorf("LastCommit: expected %q, got %q", "abc123", sum.LastCommit)
	}
	if sum.SessionID == "" {
		t.Error("SessionID should not be empty")
	}
	if sum.StartedAt.IsZero() {
		t.Error("StartedAt should not be zero")
	}
}

func TestSessionSummary_Empty(t *testing.T) {
	dir := t.TempDir()
	s, err := store.NewJSONL(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	sum, err := s.SessionSummary()
	if err != nil {
		t.Fatal(err)
	}
	if sum.Iterations != 0 {
		t.Errorf("expected 0 iterations, got %d", sum.Iterations)
	}
	if sum.TotalCost != 0 {
		t.Errorf("expected 0 total cost, got %.2f", sum.TotalCost)
	}
	if sum.SessionID == "" {
		t.Error("SessionID should not be empty even with no entries")
	}
}

func TestIterationLog_NotFound(t *testing.T) {
	dir := t.TempDir()
	s, err := store.NewJSONL(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	_, err = s.IterationLog(42)
	if err == nil {
		t.Fatal("expected error for nonexistent iteration")
	}
}

func TestIterationLog_InProgress(t *testing.T) {
	dir := t.TempDir()
	s, err := store.NewJSONL(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	// Write LogIterStart but not LogIterComplete
	now := time.Now()
	_ = s.Append(loop.LogEntry{Kind: loop.LogIterStart, Iteration: 1, Timestamp: now})
	_ = s.Append(loop.LogEntry{Kind: loop.LogToolUse, Iteration: 1, ToolName: "Read", Timestamp: now})

	// Iteration 1 is in progress — not in the completed index
	_, err = s.IterationLog(1)
	if err == nil {
		t.Fatal("expected error for in-progress (incomplete) iteration")
	}
}

func TestIterationLog_EntriesBetweenIterations(t *testing.T) {
	dir := t.TempDir()
	s, err := store.NewJSONL(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	// Write entries interleaved with info messages outside iterations
	now := time.Now()
	all := []loop.LogEntry{
		{Kind: loop.LogInfo, Message: "session started", Timestamp: now},
		{Kind: loop.LogIterStart, Iteration: 1, Mode: "build", Timestamp: now},
		{Kind: loop.LogToolUse, Iteration: 1, ToolName: "Read", Timestamp: now},
		{Kind: loop.LogGitPull, Iteration: 1, Message: "pulled", Timestamp: now},
		{Kind: loop.LogIterComplete, Iteration: 1, CostUSD: 0.10, Subtype: "success", Timestamp: now},
		{Kind: loop.LogInfo, Message: "between iterations", Timestamp: now},
		{Kind: loop.LogIterStart, Iteration: 2, Mode: "build", Timestamp: now},
		{Kind: loop.LogToolUse, Iteration: 2, ToolName: "Write", Timestamp: now},
		{Kind: loop.LogIterComplete, Iteration: 2, CostUSD: 0.20, Subtype: "success", Timestamp: now},
	}
	for _, e := range all {
		if err := s.Append(e); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	log1, err := s.IterationLog(1)
	if err != nil {
		t.Fatalf("IterationLog(1): %v", err)
	}
	// Should contain LogIterStart + LogToolUse + LogGitPull + LogIterComplete
	if len(log1) != 4 {
		t.Errorf("iter 1: expected 4 entries, got %d", len(log1))
	}

	log2, err := s.IterationLog(2)
	if err != nil {
		t.Fatalf("IterationLog(2): %v", err)
	}
	// Should contain LogIterStart + LogToolUse + LogIterComplete
	if len(log2) != 3 {
		t.Errorf("iter 2: expected 3 entries, got %d", len(log2))
	}

	// The LogInfo entries between/outside iterations must not appear in either log
	for i, e := range log1 {
		if e.Kind == loop.LogInfo {
			t.Errorf("iter 1 log1[%d] is LogInfo (should not be in iteration range)", i)
		}
	}
	for i, e := range log2 {
		if e.Kind == loop.LogInfo {
			t.Errorf("iter 2 log2[%d] is LogInfo (should not be in iteration range)", i)
		}
	}
}

func TestIterations_Empty(t *testing.T) {
	dir := t.TempDir()
	s, err := store.NewJSONL(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	iters, err := s.Iterations()
	if err != nil {
		t.Fatal(err)
	}
	if len(iters) != 0 {
		t.Fatalf("expected 0 iterations, got %d", len(iters))
	}
}

func TestNewJSONL_DirIsFile(t *testing.T) {
	base := t.TempDir()
	file := filepath.Join(base, "notadir")
	if err := os.WriteFile(file, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	// "file" exists as a regular file; MkdirAll should fail.
	_, err := store.NewJSONL(file)
	if err == nil {
		t.Fatal("expected error when dir argument is an existing file")
	}
}

func TestAppend_AfterClose(t *testing.T) {
	dir := t.TempDir()
	s, err := store.NewJSONL(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	err = s.Append(loop.LogEntry{Kind: loop.LogInfo, Timestamp: time.Now()})
	if err == nil {
		t.Fatal("expected error when appending to a closed store")
	}
}

func TestIterationLog_CompleteWithoutStart(t *testing.T) {
	dir := t.TempDir()
	s, err := store.NewJSONL(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	// Write LogIterComplete without a preceding LogIterStart.
	// The index should ignore this entry (no pending iteration).
	now := time.Now()
	_ = s.Append(loop.LogEntry{Kind: loop.LogIterComplete, Iteration: 5, CostUSD: 0.01, Timestamp: now})

	_, err = s.IterationLog(5)
	if err == nil {
		t.Fatal("expected error: iteration 5 was never started")
	}

	iters, _ := s.Iterations()
	if len(iters) != 0 {
		t.Errorf("expected 0 completed iterations, got %d", len(iters))
	}
}

func TestEnforceRetention(t *testing.T) {
	// createFiles creates n fake .jsonl files named 0000000000-N.jsonl
	// (stable lexicographic = chronological order) and returns the dir.
	createFiles := func(t *testing.T, n int) string {
		t.Helper()
		dir := t.TempDir()
		for i := 0; i < n; i++ {
			name := fmt.Sprintf("%010d-%d.jsonl", i, i)
			if err := os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0644); err != nil {
				t.Fatal(err)
			}
		}
		return dir
	}

	countFiles := func(t *testing.T, dir string) int {
		t.Helper()
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		count := 0
		for _, e := range entries {
			if filepath.Ext(e.Name()) == ".jsonl" {
				count++
			}
		}
		return count
	}

	tests := []struct {
		name      string
		nFiles    int
		maxKeep   int
		wantFiles int
	}{
		{"zero files, keep 20", 0, 20, 0},
		{"fewer than limit", 5, 20, 5},
		{"exactly at limit", 20, 20, 20},
		{"one over limit", 21, 20, 20},
		{"many over limit", 50, 20, 20},
		{"keep 0 means unlimited", 50, 0, 50},
		{"keep 1 keeps newest", 5, 1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := createFiles(t, tt.nFiles)
			if err := store.EnforceRetention(dir, tt.maxKeep); err != nil {
				t.Fatalf("EnforceRetention: %v", err)
			}
			got := countFiles(t, dir)
			if got != tt.wantFiles {
				t.Errorf("want %d files remaining, got %d", tt.wantFiles, got)
			}
		})
	}

	t.Run("non-existent dir returns nil", func(t *testing.T) {
		err := store.EnforceRetention(filepath.Join(t.TempDir(), "no-such-dir"), 5)
		if err != nil {
			t.Errorf("expected nil for missing dir, got: %v", err)
		}
	})

	t.Run("oldest files are deleted", func(t *testing.T) {
		dir := createFiles(t, 5)
		if err := store.EnforceRetention(dir, 2); err != nil {
			t.Fatal(err)
		}
		// After keeping 2, the oldest 3 (0000000000-0, ...-1, ...-2) should be gone.
		for i := 0; i < 3; i++ {
			name := fmt.Sprintf("%010d-%d.jsonl", i, i)
			if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
				t.Errorf("expected file %s to be deleted", name)
			}
		}
		// Newest 2 (...-3, ...-4) should remain.
		for i := 3; i < 5; i++ {
			name := fmt.Sprintf("%010d-%d.jsonl", i, i)
			if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
				t.Errorf("expected file %s to remain: %v", name, err)
			}
		}
	})
}

func TestIterationLog_MalformedLineSkipped(t *testing.T) {
	dir := t.TempDir()
	s, err := store.NewJSONL(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	now := time.Now()
	_ = s.Append(loop.LogEntry{Kind: loop.LogIterStart, Iteration: 1, Timestamp: now})
	// Append two tool entries so we can corrupt the first and still get the second.
	_ = s.Append(loop.LogEntry{Kind: loop.LogToolUse, Iteration: 1, ToolName: "Read", Timestamp: now})
	_ = s.Append(loop.LogEntry{Kind: loop.LogToolUse, Iteration: 1, ToolName: "Write", Timestamp: now})
	_ = s.Append(loop.LogEntry{Kind: loop.LogIterComplete, Iteration: 1, CostUSD: 0.01, Timestamp: now})

	// Locate the JSONL file written by this store.
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) == 0 {
		t.Fatalf("expected at least 1 file in dir: %v", err)
	}
	path := filepath.Join(dir, entries[0].Name())

	// Read file contents; Go opens with FILE_SHARE_READ so this succeeds even
	// while the store still holds an open handle.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	// Split into lines and corrupt line index 1 (first ToolUse entry) with
	// garbage of the EXACT SAME BYTE LENGTH so that all byte-offset indices
	// tracked by the in-memory index remain valid.
	lines := splitLines(data) // [[bytes...], ...]
	if len(lines) < 4 {
		t.Fatalf("expected ≥4 lines in JSONL, got %d", len(lines))
	}
	targetLine := lines[1] // first ToolUse line (not including the trailing \n)
	if len(targetLine) < 1 {
		t.Fatalf("target line is empty")
	}
	// Build a same-length replacement that is invalid JSON.
	replacement := make([]byte, len(targetLine))
	copy(replacement, []byte("{BADLINE"))
	for i := 8; i < len(replacement); i++ {
		replacement[i] = 'X'
	}
	// replacement does NOT end with '}', so it is invalid JSON regardless of length.

	// Compute byte offset of line 1 (after line 0 and its newline).
	offset := int64(len(lines[0]) + 1) // +1 for the '\n' after line 0

	// Open a second handle to the file and overwrite line 1 in place.
	f2, openErr := os.OpenFile(path, os.O_RDWR, 0644)
	if openErr != nil {
		t.Skipf("cannot open file with second handle (file locking?): %v", openErr)
	}
	if _, writeErr := f2.WriteAt(replacement, offset); writeErr != nil {
		_ = f2.Close()
		t.Skipf("WriteAt failed (file locking?): %v", writeErr)
	}
	_ = f2.Close()

	// IterationLog re-reads the byte range from disk via ReadAt. The malformed
	// line should be logged and skipped; the remaining 3 valid entries survive.
	got, err := s.IterationLog(1)
	if err != nil {
		t.Fatalf("IterationLog with malformed line: %v", err)
	}
	// Expected: LogIterStart + LogToolUse(Write) + LogIterComplete = 3 entries.
	// (The malformed Read line is silently skipped.)
	if len(got) != 3 {
		t.Errorf("expected 3 entries (malformed line skipped), got %d", len(got))
	}
}

func TestIterationSummary_CommitFromComplete(t *testing.T) {
	// The onAppend index updates Commit on the summary when LogIterComplete
	// carries a non-empty Commit (e.g. after the loop commits to git).
	dir := t.TempDir()
	s, err := store.NewJSONL(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	now := time.Now()
	_ = s.Append(loop.LogEntry{Kind: loop.LogIterStart, Iteration: 1, Timestamp: now})
	_ = s.Append(loop.LogEntry{
		Kind:      loop.LogIterComplete,
		Iteration: 1,
		CostUSD:   0.01,
		Commit:    "deadbeef",
		Timestamp: now,
	})

	iters, err := s.Iterations()
	if err != nil {
		t.Fatal(err)
	}
	if len(iters) != 1 {
		t.Fatalf("expected 1 iteration, got %d", len(iters))
	}
	if iters[0].Commit != "deadbeef" {
		t.Errorf("Commit: expected %q, got %q", "deadbeef", iters[0].Commit)
	}
}

// TestEnforceRetention_ReadOnlyDir verifies that EnforceRetention returns an
// error when os.Remove fails because the directory is read-only.
func TestEnforceRetention_ReadOnlyDir(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("cannot test permission errors as root")
	}
	if runtime.GOOS == "windows" {
		t.Skip("chmod read-only directory is not reliably enforced on Windows")
	}

	dir := t.TempDir()
	// Create two .jsonl files so EnforceRetention will try to delete the oldest.
	for i := 0; i < 2; i++ {
		name := fmt.Sprintf("%010d-%d.jsonl", i, i)
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Make the directory read-only so os.Remove will fail.
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0755) })

	err := store.EnforceRetention(dir, 1)
	if err == nil {
		t.Fatal("expected error when directory is read-only and remove fails")
	}
}

func TestIterationLog_ReadError(t *testing.T) {
	dir := t.TempDir()
	s, err := store.NewJSONL(dir)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	_ = s.Append(loop.LogEntry{Kind: loop.LogIterStart, Iteration: 1, Timestamp: now})
	_ = s.Append(loop.LogEntry{Kind: loop.LogIterComplete, Iteration: 1, CostUSD: 0.01, Timestamp: now})

	// Close the store so the underlying file handle is invalid.
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	// IterationLog now attempts ReadAt on a closed file — must return an error.
	_, err = s.IterationLog(1)
	if err == nil {
		t.Fatal("expected error when reading from a closed store")
	}
}

func TestEnforceRetention_DirIsFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		// On Windows, os.ReadDir on a regular file returns an IsNotExist error,
		// so EnforceRetention returns nil — the non-IsNotExist ReadDir error
		// branch is a Windows ceiling.
		t.Skip("os.ReadDir on a file returns IsNotExist on Windows")
	}
	base := t.TempDir()
	file := filepath.Join(base, "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	// Passing a regular file as dir: os.ReadDir returns ENOTDIR (non-IsNotExist).
	err := store.EnforceRetention(file, 5)
	if err == nil {
		t.Fatal("expected error when dir argument is a regular file")
	}
}

// splitLines returns the byte content of each line (without the trailing '\n')
// from data. An empty trailing line from a final '\n' is omitted.
func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, data[start:i])
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}

func TestAppend_RoundTripsAllFields(t *testing.T) {
	dir := t.TempDir()
	s, err := store.NewJSONL(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	now := time.Now().Truncate(time.Millisecond) // JSON uses millisecond precision
	orig := loop.LogEntry{
		Kind:      loop.LogToolUse,
		Timestamp: now,
		Message:   "tool message",
		ToolName:  "Bash",
		ToolInput: "echo hello",
		CostUSD:   0.001,
		Duration:  0.5,
		TotalCost: 1.23,
		Subtype:   "success",
		Iteration: 3,
		MaxIter:   10,
		Branch:    "feature",
		Commit:    "deadbeef",
		Mode:      "build",
	}
	// Write without an iteration wrapper to test raw field round-trip
	_ = s.Append(loop.LogEntry{Kind: loop.LogIterStart, Iteration: 3, Timestamp: now})
	if err := s.Append(orig); err != nil {
		t.Fatalf("Append: %v", err)
	}
	_ = s.Append(loop.LogEntry{Kind: loop.LogIterComplete, Iteration: 3, Timestamp: now})

	got, err := s.IterationLog(3)
	if err != nil {
		t.Fatal(err)
	}
	// got[0] = LogIterStart, got[1] = our entry, got[2] = LogIterComplete
	if len(got) < 2 {
		t.Fatalf("expected at least 2 entries, got %d", len(got))
	}
	e := got[1]
	if e.Kind != orig.Kind {
		t.Errorf("Kind: want %v, got %v", orig.Kind, e.Kind)
	}
	if !e.Timestamp.Equal(orig.Timestamp) {
		t.Errorf("Timestamp: want %v, got %v", orig.Timestamp, e.Timestamp)
	}
	if e.ToolName != orig.ToolName {
		t.Errorf("ToolName: want %q, got %q", orig.ToolName, e.ToolName)
	}
	if e.ToolInput != orig.ToolInput {
		t.Errorf("ToolInput: want %q, got %q", orig.ToolInput, e.ToolInput)
	}
	if e.Branch != orig.Branch {
		t.Errorf("Branch: want %q, got %q", orig.Branch, e.Branch)
	}
	if e.Commit != orig.Commit {
		t.Errorf("Commit: want %q, got %q", orig.Commit, e.Commit)
	}
	if e.Mode != orig.Mode {
		t.Errorf("Mode: want %q, got %q", orig.Mode, e.Mode)
	}
}
