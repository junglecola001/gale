package engine

import (
	"sync/atomic"

	"gale/internal/model"
)

// chunkSize is how much a segment reads before it looks at splitRequest and
// the limiter again.
const chunkSize = 64 << 10

// initialSegments is how many segments a download starts with. Fewer than
// its connections, so that the idle workers have something to split.
const initialSegments = 4

// segmentState is one range of a download while it runs. pos and end are
// atomics: the segment's worker moves pos, and the workers looking for work
// read both.
type segmentState struct {
	index int
	start int64
	end   atomic.Int64
	pos   atomic.Int64

	taken    bool // in a worker's hands; guarded by task.mu
	splitReq atomic.Bool
	tailCh   chan *segmentState
}

func newSegmentState(seg model.Segment) *segmentState {
	s := &segmentState{index: seg.Index, start: seg.Start, tailCh: make(chan *segmentState)}
	s.end.Store(seg.End)
	s.pos.Store(seg.Pos)
	return s
}

// model is the segment as the store and the page see it.
func (s *segmentState) model() model.Segment {
	return model.Segment{Index: s.index, Start: s.start, End: s.end.Load(), Pos: s.pos.Load()}
}

// done reports whether the segment has all its bytes. An unknown length
// (end < 0) is never done: only the end of the response is.
func (s *segmentState) done() bool {
	end := s.end.Load()
	return end >= 0 && s.pos.Load() > end
}

func (s *segmentState) remaining() int64 {
	end := s.end.Load()
	if end < 0 || s.pos.Load() > end {
		return 0
	}
	return end - s.pos.Load() + 1
}

// downloaded is how many bytes of the segment are on disk.
func (s *segmentState) downloaded() int64 {
	pos := s.pos.Load()
	got := pos - s.start
	if got < 0 {
		return 0
	}
	if end := s.end.Load(); end >= 0 && pos > end+1 {
		return end + 1 - s.start
	}
	return got
}

// targetConnections is how many segments a download of total bytes may use:
// the configured connections, but not more than the file can usefully fill.
func targetConnections(total int64, connections int, minSeg int64) int {
	if connections < 1 {
		connections = 1
	}
	if total <= 0 {
		return 1
	}
	useful := total / minSeg
	if useful < 1 {
		useful = 1
	}
	if int64(connections) > useful {
		return int(useful)
	}
	return connections
}

// planSegments splits total into the first segments of a download, evenly,
// the last one taking the remainder.
func planSegments(total int64, connections, minSeg int64) []model.Segment {
	target := targetConnections(total, int(connections), minSeg)
	initial := initialSegments
	if target < initial {
		initial = target
	}
	if initial < 1 {
		initial = 1
	}
	segs := make([]model.Segment, 0, initial)
	per := total / int64(initial)
	for i := range initial {
		start := int64(i) * per
		end := start + per - 1
		if i == initial-1 {
			end = total - 1
		}
		segs = append(segs, model.Segment{Index: i, Start: start, End: end, Pos: start})
	}
	return segs
}
