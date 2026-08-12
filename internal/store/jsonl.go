package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/LISSConsulting/RalphSpec/internal/loop"
)

// JSONL is a Store backed by an append-only JSONL file. Each line is a
// JSON-serialized loop.LogEntry. The file is synced after every Append to
// guarantee durability across Regent kills.
//
// Session identity: "<unix-timestamp>-<pid>.jsonl". The Regent restarts
// Ralph within the same OS process, so PID and start timestamp are stable
// across restarts, ensuring all restarts append to the same file.
type JSONL struct {
	file       *os.File
	mu         sync.Mutex
	idx        *fileIndex
	sessionID  string
	startedAt  time.Time
	pos        int64 // current write position in the file
	branch     string
	lastCommit string
	agent      string
	provider   string
}

// NewJSONL creates (or reopens) the session JSONL log in dir. dir is created
// with os.MkdirAll if it does not exist. The session ID is derived from
// the current Unix timestamp and PID, making it stable within one process.
func NewJSONL(dir string) (*JSONL, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("store: mkdir %q: %w", dir, err)
	}
	now := time.Now()
	sessionID := fmt.Sprintf("%d-%d", now.Unix(), os.Getpid())
	path := filepath.Join(dir, sessionID+".jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("store: open %q: %w", path, err)
	}
	// Seek to end in case the file already has content (same-process restart).
	pos, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("store: seek: %w", err)
	}
	return &JSONL{
		file:      f,
		idx:       newFileIndex(),
		sessionID: sessionID,
		startedAt: now,
		pos:       pos,
	}, nil
}

// Append serializes entry as a JSON line, writes it to the file, and syncs.
// It is safe to call from multiple goroutines.
func (j *JSONL) Append(entry loop.LogEntry) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("store: marshal: %w", err)
	}
	data = append(data, '\n')

	j.mu.Lock()
	defer j.mu.Unlock()

	lineOffset := j.pos
	if _, err := j.file.Write(data); err != nil {
		return fmt.Errorf("store: write: %w", err)
	}
	if err := j.file.Sync(); err != nil {
		return fmt.Errorf("store: sync: %w", err)
	}
	lineLen := int64(len(data))
	j.pos += lineLen
	j.idx.onAppend(entry, lineOffset, lineLen)
	if entry.Branch != "" {
		j.branch = entry.Branch
	}
	if entry.Commit != "" {
		j.lastCommit = entry.Commit
	}
	if entry.Agent != "" {
		j.agent = entry.Agent
	}
	if entry.Provider != "" {
		j.provider = entry.Provider
	}
	return nil
}

// Close flushes any pending state and closes the underlying file.
func (j *JSONL) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.file.Close()
}

// Iterations returns summaries for all completed iterations in this session.
// The returned slice is a copy and safe to mutate.
func (j *JSONL) Iterations() ([]IterationSummary, error) {
	j.mu.Lock()
	result := make([]IterationSummary, len(j.idx.summaries))
	copy(result, j.idx.summaries)
	j.mu.Unlock()
	return result, nil
}

// IterationLog returns the full event log for a completed iteration, reading
// from the JSONL file using the in-memory byte-offset index. Returns an error
// if iteration n has not completed (or was never started).
func (j *JSONL) IterationLog(n int) ([]loop.LogEntry, error) {
	j.mu.Lock()
	r, ok := j.idx.ranges[n]
	j.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("store: iteration %d not found", n)
	}
	size := r.end - r.start
	if size <= 0 {
		return nil, nil
	}
	buf := make([]byte, size)
	if _, err := j.file.ReadAt(buf, r.start); err != nil {
		return nil, fmt.Errorf("store: read iteration %d: %w", n, err)
	}
	var entries []loop.LogEntry
	for _, line := range bytes.Split(buf, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var e loop.LogEntry
		if err := json.Unmarshal(line, &e); err != nil {
			log.Printf("store: skipping malformed line in iteration %d: %v", n, err)
			continue
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// EnforceRetention removes the oldest session log files in dir, keeping at most
// maxKeep files. If maxKeep is 0, no files are removed. Returns nil if dir does
// not exist or is empty.
func EnforceRetention(dir string, maxKeep int) error {
	if maxKeep <= 0 {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("store: read dir %q: %w", dir, err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
			files = append(files, e.Name())
		}
	}

	sort.Strings(files) // timestamp-prefixed names sort chronologically

	toDelete := len(files) - maxKeep
	for i := 0; i < toDelete; i++ {
		path := filepath.Join(dir, files[i])
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("store: remove %q: %w", path, err)
		}
	}
	return nil
}

// OpenSession opens an existing session JSONL log read-only and rebuilds the
// in-memory index by scanning the file. The returned reader must not be used
// for writes. Caller must Close.
func OpenSession(path string) (*JSONL, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("store: open session %q: %w", path, err)
	}
	j := &JSONL{
		file:      f,
		idx:       newFileIndex(),
		sessionID: strings.TrimSuffix(filepath.Base(path), ".jsonl"),
	}
	if err := j.rebuildIndex(); err != nil {
		_ = f.Close()
		return nil, err
	}
	return j, nil
}

// rebuildIndex scans the whole file and replays each entry through the index,
// tracking byte offsets so IterationLog ReadAt lookups work. It also derives
// session metadata (startedAt, branch, agent, lastCommit).
func (j *JSONL) rebuildIndex() error {
	if _, err := j.file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("store: seek: %w", err)
	}
	data, err := io.ReadAll(j.file)
	if err != nil {
		return fmt.Errorf("store: read session: %w", err)
	}
	var offset int64
	for _, line := range bytes.Split(data, []byte("\n")) {
		lineLen := int64(len(line)) + 1
		if len(line) > 0 {
			var e loop.LogEntry
			if err := json.Unmarshal(line, &e); err == nil {
				j.idx.onAppend(e, offset, lineLen)
				if j.startedAt.IsZero() && !e.Timestamp.IsZero() {
					j.startedAt = e.Timestamp
				}
				if e.Branch != "" {
					j.branch = e.Branch
				}
				if e.Commit != "" {
					j.lastCommit = e.Commit
				}
				if e.Agent != "" {
					j.agent = e.Agent
				}
				if e.Provider != "" {
					j.provider = e.Provider
				}
			}
		}
		offset += lineLen
	}
	return nil
}

// ListSessions returns summaries for every session log in dir, newest first.
// Unreadable or malformed files are skipped. Returns nil when dir does not
// exist or contains no logs.
func ListSessions(dir string) ([]SessionSummary, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: read dir %q: %w", dir, err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
			files = append(files, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(files))) // timestamp-prefixed → newest first

	var sessions []SessionSummary
	for _, name := range files {
		j, err := OpenSession(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		sum, err := j.SessionSummary()
		_ = j.Close()
		if err != nil {
			continue
		}
		sessions = append(sessions, sum)
	}
	return sessions, nil
}

// SessionSummary returns metadata about the current session derived from
// the in-memory iteration index.
func (j *JSONL) SessionSummary() (SessionSummary, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	var total float64
	for _, s := range j.idx.summaries {
		total += s.CostUSD
	}
	return SessionSummary{
		SessionID:  j.sessionID,
		StartedAt:  j.startedAt,
		Agent:      j.agent,
		Provider:   j.provider,
		TotalCost:  total,
		Iterations: len(j.idx.summaries),
		LastCommit: j.lastCommit,
		Branch:     j.branch,
	}, nil
}
