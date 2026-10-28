package app

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/anopheles/navigo/internal/library"
	"github.com/anopheles/navigo/internal/player"
)

// Mix replaces the queue with N tracks picked from the whole library
// (library.Snapshot.Mix), or around a seed when Around is set
// (library.Snapshot.MixAround), and plays the first. Seed makes the pick
// and At is the time "played lately" counts back from, so Apply stays
// pure.
type Mix struct {
	N      int
	Seed   uint64
	At     time.Time
	Around *library.Seed
}

// from is " from <name>" for a Mix around a seed, for its messages.
func (e Mix) from() string {
	if e.Around == nil {
		return ""
	}
	return " from " + e.Around.Name
}

func (Mix) isEvent() {}

// mixLookups is how many seed artists a Mix asks similar artists for.
const mixLookups = 3

// The similar-artist lookups: how many artists each asks for, and how
// long they may take together. The server is slow the first time it
// looks an artist up, so running out of time doesn't mean it's gone.
const (
	SimilarCount = 20
	SimilarLimit = 2 * time.Second
)

// FetchSimilar asks the server for the similar artists of each of
// Artists, in parallel and within a short time limit, and reports
// SimilarLoaded with Seq.
type FetchSimilar struct {
	Seq     int
	Artists []library.ArtistID
}

// SimilarLoaded is the answer to FetchSimilar: one list per artist asked
// for, closest first, empty where a lookup failed or ran out of time.
// Unreachable says one couldn't reach the server.
type SimilarLoaded struct {
	Seq         int
	Lists       [][]library.ArtistRef
	Unreachable bool
}

func (FetchSimilar) isEffect() {}
func (SimilarLoaded) isEvent() {}

// mixWait is a Mix asked for before the first sync was done, with what
// the queue was then. It runs when the sync is done, unless the queue or
// what plays changed since: then the listener has moved on.
type mixWait struct {
	mix             Mix
	seq             int // the lookup's, for a Mix waiting on similar artists
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
			s.mixWait = s.waiting(e)
			n := Notice{Text: "the library is syncing · the mix starts once it's done"}
			if e.Around != nil {
				n.Text = "mixing" + e.from() + " once the library has synced"
			}
			return s, []Effect{n}
		}
		return s, []Effect{s.Unsynced("nothing to mix" + e.from())}
	}
	s.mixLookup = nil // a newer Mix replaces one waiting on the server
	if e.Around != nil && !s.Offline {
		if artists := s.Library.SeedArtists(*e.Around, mixLookups); len(artists) > 0 {
			s.mixSeq++
			s.mixLookup = s.waiting(e)
			s.mixLookup.seq = s.mixSeq
			return s, []Effect{
				FetchSimilar{Seq: s.mixSeq, Artists: artists},
				Notice{Text: "mixing" + e.from() + "…"},
			}
		}
	}
	return s.mix(e, nil)
}

// waiting is e waiting for something, with what the queue is now.
func (s State) waiting(e Mix) *mixWait {
	return &mixWait{mix: e, nextID: s.nextID, playing: s.playing, queueLen: len(s.Queue)}
}

// applySimilar makes the Mix that waited on similar artists, unless a
// newer Mix replaced it or the queue or what plays changed meanwhile. A
// lookup that couldn't reach the server takes navigo offline, but the
// Mix is still made, from the cache.
func (s State) applySimilar(e SimilarLoaded) (State, []Effect) {
	if s, out, ok := s.radioSimilar(e); ok {
		return s, out
	}
	w := s.mixLookup
	if w == nil || w.seq != e.Seq {
		return s, nil
	}
	s.mixLookup = nil
	var out []Effect
	if e.Unreachable {
		s, out = s.applyOffline(
			Unreachable{Err: errors.New("similar artists: can't reach the server")},
		)
	}
	if !w.still(s) {
		return s, out
	}
	s, more := s.mix(w.mix, e.Lists)
	return s, append(more, out...)
}

// mix makes Mix e, around its seed with the given similar artists.
func (s State) mix(e Mix, similar [][]library.ArtistRef) (State, []Effect) {
	r := rand.New(rand.NewPCG(e.Seed, e.Seed^0x9e3779b97f4a7c15))
	var picked []library.Track
	if e.Around != nil {
		picked = s.Library.MixAround(*e.Around, similar, e.N, e.At, r, s.recs.matched)
		if len(picked) == 0 {
			return s, []Effect{Notice{Text: "nothing to mix" + e.from(), Err: true}}
		}
	} else {
		picked = s.Library.Mix(e.N, e.At, r, s.recs.matched)
	}
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
	n := Notice{Text: "mixed " + count(len(picked)) + e.from()}
	if len(picked) < e.N {
		n.Text = fmt.Sprintf("mixed %d of %d tracks%s", len(picked), e.N, e.from())
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

// Unsynced says why there is no library yet, for a command that needs
// one: what is what the command couldn't do, such as "nothing to mix".
func (s State) Unsynced(what string) Notice {
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
