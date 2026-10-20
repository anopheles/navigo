package app

import (
	"fmt"
	"slices"

	"github.com/anopheles/navigo/internal/library"
	"github.com/anopheles/navigo/internal/player"
)

// Events that edit the queue through the register (d, y, p, P, x, J, K).
// Rows are queue indexes; To is exclusive.
type (
	// Yank fills the register.
	Yank struct{ Tracks []library.Track }
	// Remove takes queue rows out, into the register.
	Remove struct{ From, To int }
	// Paste inserts the register Times times before row At.
	Paste struct{ At, Times int }
	// Move moves rows By places, down when positive. The UI clamps By
	// so the rows stay in the queue.
	Move struct{ From, To, By int }
)

func (Yank) isEvent()   {}
func (Remove) isEvent() {}
func (Paste) isEvent()  {}
func (Move) isEvent()   {}

func (s State) applyEdit(e Event) (State, []Effect) {
	switch e := e.(type) {
	case Yank:
		if len(e.Tracks) == 0 {
			return s, nil
		}
		s.Register = slices.Clone(e.Tracks)
		return s, []Effect{Notice{Text: count(len(e.Tracks)) + " yanked", Command: "paste-after", Hint: "pastes them"}}

	case Remove:
		from, to := max(e.From, 0), min(e.To, len(s.Queue))
		if from >= to {
			return s, nil
		}
		gone := s.Queue[from:to]
		s.Register = make([]library.Track, len(gone))
		for i, q := range gone {
			s.Register[i] = q.Track
		}
		if slices.ContainsFunc(gone, func(q QueueEntry) bool { return q.ID == s.playing }) && to < len(s.Queue) {
			// It plays on, then the queue goes on where it was.
			s.orphanNext = s.Queue[to].ID
		}
		s = s.editRows(slices.Concat(s.Queue[:from], s.Queue[to:]), from-1)
		text := fmt.Sprintf("%s removed (%d in queue)", count(len(gone)), len(s.Queue))
		return s, append(s.neighbours(), Notice{Text: text, Command: "undo", Hint: "undoes"})

	case Paste:
		if len(s.Register) == 0 {
			return s, []Effect{Notice{Text: "nothing to paste: yank or delete something first"}}
		}
		at := min(max(e.At, 0), len(s.Queue))
		var tracks []library.Track
		for range max(e.Times, 1) {
			tracks = append(tracks, s.Register...)
		}
		var added []QueueEntry
		added, s.nextID = entries(s.nextID, tracks)
		s = s.editRows(slices.Insert(slices.Clone(s.Queue), at, added...), -1)
		text := fmt.Sprintf("%s pasted (%d in queue)", count(len(added)), len(s.Queue))
		return s, append(s.neighbours(), Notice{Text: text, Command: "undo", Hint: "undoes"})

	case Move:
		from, to := max(e.From, 0), min(e.To, len(s.Queue))
		by := min(max(e.By, -from), len(s.Queue)-to)
		if from >= to || by == 0 {
			return s, nil
		}
		block := slices.Clone(s.Queue[from:to])
		rest := slices.Concat(s.Queue[:from], s.Queue[to:])
		s = s.editRows(slices.Insert(rest, from+by, block...), -1)
		dir := "down"
		if by < 0 {
			dir = "up"
		}
		text := fmt.Sprintf("moved %s %s", count(len(block)), dir)
		return s, append(s.neighbours(), Notice{Text: text, Command: "undo", Hint: "undoes"})
	}
	return s, nil
}

// editRows replaces the queue as one undoable step, keeping Current on
// the same entry. If that entry is gone, Current becomes gone: the row
// before the ones removed.
func (s State) editRows(q []QueueEntry, gone int) State {
	var cur player.EntryID
	if s.Current >= 0 && s.Current < len(s.Queue) {
		cur = s.Queue[s.Current].ID
	}
	return s.edit(func([]QueueEntry) ([]QueueEntry, int) {
		c := slices.IndexFunc(q, func(e QueueEntry) bool { return e.ID == cur })
		if c < 0 && cur != 0 {
			c = gone
		}
		return q, c
	})
}
