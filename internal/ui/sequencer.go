package ui

import "sync"

// sequencer runs functions in ticket order. Tickets are taken in Update,
// one goroutine, so player calls happen in the order the core asked for
// them even though each runs in its own tea.Cmd goroutine.
type sequencer struct {
	mu         sync.Mutex
	cond       *sync.Cond
	next, done uint64
}

func (s *sequencer) init() { s.cond = sync.NewCond(&s.mu) }

func (s *sequencer) ticket() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.next
	s.next++
	return t
}

func (s *sequencer) run(ticket uint64, f func()) {
	s.mu.Lock()
	for s.done != ticket {
		s.cond.Wait()
	}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.done++
		s.cond.Broadcast()
		s.mu.Unlock()
	}()
	f()
}
