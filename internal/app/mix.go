package app

import (
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/anopheles/navigo/internal/player"
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

// mixWait is a Mix asked for before the first sync was done, with what
// the queue was then. It runs when the sync is done, unless the queue or
// what plays changed since: then the listener has moved on.
type mixWait struct {
	mix             Mix
	nextID, playing player.EntryID
	queueLen        int
}

func (w *mixWait) still(s State) bool {
	return w.nextID == s.nextID && w.playing == s.playing && w.queueLen == len(s.Queue)
}

// MixWaiting reports whether a Mix waits for the first sync.
func (s State) MixWaiting() bool { return s.mixWait != nil }

func (s State) applyMix(e Mix) (State, []Effect) {
	if s.Library == nil {
		if s.Sync.Running && !s.Offline {
			s.mixWait = &mixWait{
				mix:      e,
				nextID:   s.nextID,
				playing:  s.playing,
				queueLen: len(s.Queue),
			}
			return s, []Effect{
				Notice{Text: "the library is syncing · the mix starts once it's done"},
			}
		}
		return s, []Effect{s.unsynced("nothing to mix")}
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

// unsynced says why there is no library yet, for a command that needs
// one: what is what the command couldn't do, such as "nothing to mix".
func (s State) unsynced(what string) Notice {
	n := Notice{Err: true}
	switch {
	case s.Offline:
		n.Text = what + " · offline, and the library hasn't synced yet"
	case s.Sync.Running:
		n.Text = what + " · the library is still syncing"
	case s.Sync.Failed:
		n.Text = what + " · the library couldn't sync"
		n.Command, n.Hint = "sync", "tries again"
	default:
		n.Text = what + " · the library is still loading"
	}
	return n
}
