package app

import (
	"strings"
	"testing"
	"time"

	"github.com/anopheles/navigo/internal/library"
)

var mixAddAt = time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

func mixAddEvent(n int, place MixPlace) Mix {
	return Mix{N: n, Seed: 7, At: mixAddAt, Place: place}
}

func TestMixAddEndKeepsTheQueueAndWhatPlays(t *testing.T) {
	lib := mixable(40)
	s, _ := run(New(), CacheLoaded{Library: lib}, PlayNow{Tracks: lib.Tracks[:2]})
	playing, current := s.playing, s.Current
	s, out := run(s, mixAddEvent(5, MixAddEnd))
	if len(s.Queue) != 7 || s.Current != current || s.playing != playing {
		t.Fatalf("queue %d current %d", len(s.Queue), s.Current)
	}
	if p := only[Play](out); len(p) != 0 {
		t.Errorf("plays = %#v", p)
	}
	n := only[Notice](out)
	if len(n) != 1 || n[0].Text != "added 5 tracks" || n[0].Command != "undo" {
		t.Errorf("notices = %#v", n)
	}
	seen := map[library.TrackID]bool{}
	for _, q := range s.Queue {
		if seen[q.Track.ID] {
			t.Errorf("%s is in the queue twice", q.Track.ID)
		}
		seen[q.Track.ID] = true
		if q.Radio {
			t.Errorf("%s is marked as the radio's", q.Track.ID)
		}
	}
	// One undo takes the whole batch away.
	if s, _ = run(s, Undo{}); len(s.Queue) != 2 {
		t.Errorf("after undo, queue %d, want 2", len(s.Queue))
	}
}

func TestMixAddNextGoesAfterTheCurrentTrack(t *testing.T) {
	lib := mixable(40)
	s, _ := run(New(), CacheLoaded{Library: lib}, PlayNow{Tracks: lib.Tracks[:2]})
	second := s.Queue[1].Track.ID
	s, _ = run(s, mixAddEvent(5, MixAddNext))
	if len(s.Queue) != 7 || s.Queue[6].Track.ID != second || s.Current != 0 {
		t.Fatalf("queue %d, last %s, current %d", len(s.Queue), s.Queue[6].Track.ID, s.Current)
	}
}

func TestMixAddOnAnEmptyQueueDoesntPlay(t *testing.T) {
	for _, place := range []MixPlace{MixAddEnd, MixAddNext} {
		s, out := run(New(), CacheLoaded{Library: mixable(40)}, mixAddEvent(5, place))
		if len(s.Queue) != 5 || len(only[Play](out)) != 0 {
			t.Errorf("place %d: queue %d, plays %#v", place, len(s.Queue), only[Play](out))
		}
	}
}

func TestMixAddLeavesOutATrackSeed(t *testing.T) {
	lib := genreLib(30)
	seedTrack := lib.Tracks[3]
	seed := &library.Seed{Kind: library.SeedTrack, ID: string(seedTrack.ID), Name: seedTrack.Title}
	s, _ := run(New(), CacheLoaded{Library: lib}, Unreachable{Err: errDown})
	s, out := run(s, Mix{N: 5, Seed: 7, At: mixAddAt, Around: seed, Place: MixAddEnd})
	if len(s.Queue) == 0 {
		t.Fatalf("nothing added: %#v", only[Notice](out))
	}
	for _, q := range s.Queue {
		if q.Track.ID == seedTrack.ID {
			t.Error("added the seed track itself")
		}
	}
	if n := only[Notice](out); !strings.HasSuffix(n[0].Text, "from "+seedTrack.Title) {
		t.Errorf("notices = %#v", n)
	}
}

// A pending mix-add survives a change to the queue, and "next" means after
// what plays when its tracks arrive. A newer Mix still replaces it.
func TestPendingMixAdd(t *testing.T) {
	lib := genreLib(30)
	seed := &library.Seed{Kind: library.SeedAlbum, ID: "a3-b1", Name: "Album 3"}
	ask := Mix{N: 5, Seed: 7, At: mixAddAt, Around: seed, Place: MixAddNext}
	s, out := run(New(), CacheLoaded{Library: lib}, ask)
	seq := only[FetchSimilar](out)[0].Seq

	s, _ = run(s, PlayNow{Tracks: tracks(3)}, NextTrack{})
	current := s.Current
	s, _ = run(s, SimilarLoaded{Seq: seq})
	if len(s.Queue) <= 3 {
		t.Fatalf("queue %d; the mix-add was dropped", len(s.Queue))
	}
	if got, old := s.Queue[current+1].Track.ID, tracks(3)[current+1].ID; got == old {
		t.Errorf("the batch isn't right after what plays: next is %s", got)
	}

	s, out = run(New(), CacheLoaded{Library: lib}, ask)
	seq = only[FetchSimilar](out)[0].Seq
	s, _ = run(s, Mix{N: 5, Seed: 8, At: mixAddAt})
	want := len(s.Queue)
	if s, _ = run(s, SimilarLoaded{Seq: seq}); len(s.Queue) != want {
		t.Errorf("queue %d, want %d; the replaced mix-add ran", len(s.Queue), want)
	}
}

func TestMixAddTooFewAndNothing(t *testing.T) {
	lib := mixable(3)
	s, out := run(New(), CacheLoaded{Library: lib}, mixAddEvent(5, MixAddEnd))
	if n := only[Notice](out); n[0].Text != "added 3 of 5 tracks" {
		t.Errorf("notices = %#v", n)
	}
	s, out = run(s, mixAddEvent(5, MixAddEnd))
	if n := only[Notice](out); !n[0].Err || !strings.HasPrefix(n[0].Text, "nothing to add") {
		t.Errorf("notices = %#v", n)
	}
	if len(s.Queue) != 3 {
		t.Errorf("queue %d, want it left alone", len(s.Queue))
	}
}
