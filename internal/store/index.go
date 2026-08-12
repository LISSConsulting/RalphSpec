package store

import "github.com/LISSConsulting/RalphSpec/internal/loop"

// iterRange is the [start, end) byte range of one iteration in the JSONL file.
// start is the offset of the LogIterStart line; end is the offset of the first
// byte after the LogIterComplete line (i.e. start of the next line).
type iterRange struct {
	start int64
	end   int64
}

// fileIndex maintains in-memory byte-offset bookmarks per completed iteration.
// It is updated by onAppend as each LogEntry is written and provides O(1)
// lookup for IterationLog reads via file.ReadAt.
type fileIndex struct {
	summaries []IterationSummary // ordered by completion time
	ranges    map[int]iterRange  // iteration Number → byte range
	pending   *pendingIter       // open iteration being built (nil if none)
}

// pendingIter accumulates state for the iteration currently being written.
type pendingIter struct {
	startOffset int64
	summary     IterationSummary
}

func newFileIndex() *fileIndex {
	return &fileIndex{ranges: make(map[int]iterRange)}
}

// onAppend updates the index when a LogEntry line has been appended.
// lineOffset is the byte offset of the first byte of the written line;
// lineLen is the total bytes written (including the trailing newline).
func (idx *fileIndex) onAppend(entry loop.LogEntry, lineOffset, lineLen int64) {
	switch entry.Kind {
	case loop.LogIterStart:
		idx.pending = &pendingIter{
			startOffset: lineOffset,
			summary: IterationSummary{
				Number:   entry.Iteration,
				Agent:    entry.Agent,
				Provider: entry.Provider,
				Mode:     entry.Mode,
				StartAt:  entry.Timestamp,
				Commit:   entry.Commit,
			},
		}
	case loop.LogIterComplete:
		if idx.pending == nil {
			return
		}
		s := idx.pending.summary
		s.CostUSD = entry.CostUSD
		s.Duration = entry.Duration
		s.Subtype = entry.Subtype
		s.EndAt = entry.Timestamp
		if entry.Agent != "" {
			s.Agent = entry.Agent
		}
		if entry.Provider != "" {
			s.Provider = entry.Provider
		}
		if entry.Commit != "" {
			s.Commit = entry.Commit
		}
		idx.ranges[s.Number] = iterRange{
			start: idx.pending.startOffset,
			end:   lineOffset + lineLen,
		}
		idx.summaries = append(idx.summaries, s)
		idx.pending = nil
	}
}
