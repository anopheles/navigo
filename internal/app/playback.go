package app

import (
	"fmt"
	"math/rand/v2"
	"slices"
)

// Repeat is what happens at the end of a track or of the queue.
type Repeat int

const (
	RepeatOff Repeat = iota
	RepeatAll        // start the queue again
	RepeatOne        // play the same queue entry again
)

func (r Repeat) String() string {
	return [...]string{"off", "all", "one"}[r]
}

// Events for volume, shuffle and repeat.
type (
	// ChangeVolume moves the volume By points, or sets it To when Set.
	ChangeVolume struct {
		By, To int
		Set    bool
	}
	// Shuffle reorders the queue at random from Seed (ADR 0004): the
	// current entry first, the rest after it.
	Shuffle     struct{ Seed uint64 }
	CycleRepeat struct{}
)

// SetVolume is the effect that sets the player's volume.
type SetVolume struct{ Percent int }

func (ChangeVolume) isEvent() {}
func (Shuffle) isEvent()      {}
func (CycleRepeat) isEvent()  {}
func (SetVolume) isEffect()   {}

func (s State) applyPlayback(e Event) (State, []Effect) {
	switch e := e.(type) {
	case ChangeVolume:
		v := s.Player.Volume + e.By
		if e.Set {
			v = e.To
		}
		s.Player.Volume = min(max(v, 0), 100)
		return s, []Effect{SetVolume{Percent: s.Player.Volume}, Notice{Text: fmt.Sprintf("volume %d%%", s.Player.Volume)}}

	case Shuffle:
		if len(s.Queue) < 2 {
			return s, []Effect{Notice{Text: "nothing to shuffle"}}
		}
		q := slices.Clone(s.Queue)
		start := 0
		if s.Current >= 0 && s.Current < len(q) {
			cur := q[s.Current]
			q = append([]QueueEntry{cur}, slices.Delete(q, s.Current, s.Current+1)...)
			start = 1
		}
		r := rand.New(rand.NewPCG(e.Seed, e.Seed^0x9e3779b97f4a7c15))
		rest := q[start:]
		r.Shuffle(len(rest), func(i, j int) { rest[i], rest[j] = rest[j], rest[i] })
		s = s.editRows(q, -1)
		return s, append(s.neighbours(), Notice{Text: "queue shuffled", Command: "undo", Hint: "undoes"})

	case CycleRepeat:
		s.Repeat = (s.Repeat + 1) % 3
		return s, append(s.neighbours(), Notice{Text: "repeat " + s.Repeat.String()})
	}
	return s, nil
}

// nextRow is the queue row that plays after row i, or -1: the next one,
// or the first again under repeat all. Under repeat one nothing follows
// in the player; the core plays the entry again when it ends.
func (s State) nextRow(i int) int {
	switch {
	case s.Repeat == RepeatOne:
		return -1
	case i+1 < len(s.Queue):
		return i + 1
	case s.Repeat == RepeatAll && len(s.Queue) > 1:
		return 0
	}
	return -1
}

// replays reports whether row i starts again when it ends: repeat one,
// or repeat all with only that row.
func (s State) replays() bool {
	return s.Repeat == RepeatOne || s.Repeat == RepeatAll && len(s.Queue) == 1
}
