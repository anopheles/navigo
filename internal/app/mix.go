package app

import (
	"fmt"
	"math/rand/v2"
	"time"
)

// Mix replaces the queue with N tracks picked from the whole library
// (library.Snapshot.Mix) and plays the first. Seed makes the pick and At
// is the time "played lately" counts back from, so Apply stays pure.
type Mix struct {
	N    int
	Seed uint64
	At   time.Time
}

func (Mix) isEvent() {}

func (s State) applyMix(e Mix) (State, []Effect) {
	if s.Library == nil {
		return s, []Effect{
			Notice{Text: "nothing to mix · the library hasn't synced yet", Err: true},
		}
	}
	picked := s.Library.Mix(e.N, e.At, rand.New(rand.NewPCG(e.Seed, e.Seed^0x9e3779b97f4a7c15)))
	if len(picked) == 0 {
		why := "every track is rated one star"
		if len(s.Library.Tracks) == 0 {
			why = "the library has no tracks"
		}
		return s, []Effect{Notice{Text: "nothing to mix · " + why, Err: true}}
	}
	had := len(s.Queue) > 0
	s, played := apply(s, PlayNow{Tracks: picked})
	// The mix's own message replaces "playing"; an error saying why
	// nothing plays comes after it, so it stays on the status line.
	n := Notice{Text: "mixed " + count(len(picked))}
	if len(picked) < e.N {
		n.Text = fmt.Sprintf("mixed %d of %d tracks", len(picked), e.N)
	}
	if had {
		n.Command, n.Hint = "undo", "brings the old queue back"
	}
	out := []Effect{n}
	for _, f := range played {
		if p, ok := f.(Notice); !ok || p.Err {
			out = append(out, f)
		}
	}
	return s, out
}
