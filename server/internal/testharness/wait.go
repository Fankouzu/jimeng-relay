package testharness

import "sync"

type DeterministicWaiter struct {
	mu        sync.Mutex
	sequence  []StatusTransition
	nextIndex int
	pollCount int
}

func NewDeterministicWaiter(sequence []StatusTransition) *DeterministicWaiter {
	seq := append([]StatusTransition(nil), sequence...)
	if len(seq) == 0 {
		seq = StatusTransitions(StatusDone)
	}
	return &DeterministicWaiter{sequence: seq}
}

func (w *DeterministicWaiter) Next() StatusTransition {
	w.mu.Lock()
	defer w.mu.Unlock()

	idx := w.nextIndex
	if idx >= len(w.sequence) {
		idx = len(w.sequence) - 1
	}
	out := w.sequence[idx]
	w.pollCount++
	if w.nextIndex < len(w.sequence)-1 {
		w.nextIndex++
	}
	return out
}

func (w *DeterministicWaiter) PollCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.pollCount
}

type TransitionSequence struct {
	items []StatusTransition
}

func NewTransitionSequence() *TransitionSequence {
	return &TransitionSequence{items: make([]StatusTransition, 0, 4)}
}

func (s *TransitionSequence) InQueue() *TransitionSequence {
	s.items = append(s.items, StatusTransition{Status: StatusInQueue, Code: 10000, Message: "ok"})
	return s
}

func (s *TransitionSequence) Generating() *TransitionSequence {
	s.items = append(s.items, StatusTransition{Status: StatusGenerating, Code: 10000, Message: "ok"})
	return s
}

func (s *TransitionSequence) Done(videoURL string) *TransitionSequence {
	s.items = append(s.items, StatusTransition{Status: StatusDone, Code: 10000, Message: "ok", VideoURL: videoURL})
	return s
}

func (s *TransitionSequence) Failed(message string) *TransitionSequence {
	msg := message
	if msg == "" {
		msg = "failed"
	}
	s.items = append(s.items, StatusTransition{Status: StatusFailed, Code: 10000, Message: msg})
	return s
}

func (s *TransitionSequence) Build() []StatusTransition {
	if len(s.items) == 0 {
		return StatusTransitions(StatusDone)
	}
	return append([]StatusTransition(nil), s.items...)
}
