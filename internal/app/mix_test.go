package app

import (
	"fmt"
	"testing"
	"time"

	"github.com/anopheles/navigo/internal/library"
)

// mixable is n tracks, each by its own artist, so a Mix can take them all.
func mixable(n int) *library.Snapshot {
	ts := tracks(n)
	for i := range ts {
		ts[i].ArtistID = library.ArtistID(fmt.Sprintf("a%d", i))
		ts[i].AlbumID = library.AlbumID(fmt.Sprintf("b%d", i))
	}
	return library.Build("s", "u", time.Time{}, nil, nil, ts, nil, nil)
}

func TestMix(t *testing.T) {
	at := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	s, out := run(New(), CacheLoaded{Library: mixable(80)}, Mix{N: 50, Seed: 7, At: at})
	if len(s.Queue) != 50 || s.Current != 0 {
		t.Fatalf("queue %d current %d, want 50 playing from the first", len(s.Queue), s.Current)
	}
	if p := only[Play](out); len(p) != 1 {
		t.Errorf("plays = %#v", p)
	}
	if n := only[Notice](out); len(n) != 1 || n[0].Text != "mixed 50 tracks" || n[0].Command != "" {
		t.Errorf("notices = %#v", n)
	}
	first := s.Queue[0].Track.ID

	// The same seed and time give the same mix; a second one replaces
	// the queue and undo brings the first back.
	again, _ := run(New(), CacheLoaded{Library: mixable(80)}, Mix{N: 50, Seed: 7, At: at})
	if again.Queue[0].Track.ID != first {
		t.Error("the same seed made another mix")
	}
	s, out = run(s, Mix{N: 50, Seed: 8, At: at})
	if n := only[Notice](out); n[0].Command != "undo" {
		t.Errorf("second mix notice = %#v, want an undo hint", n[0])
	}
	if s, _ = run(s, Undo{}); s.Queue[0].Track.ID != first {
		t.Error("undo didn't bring the first mix back")
	}
}

func TestMixShortAndEmpty(t *testing.T) {
	s, out := run(New(), CacheLoaded{Library: mixable(5)}, Mix{N: 50, Seed: 1})
	if n := only[Notice](out); len(s.Queue) != 5 || n[0].Text != "mixed 5 of 50 tracks" {
		t.Errorf("queue %d, notices %#v", len(s.Queue), n)
	}

	s, out = run(New(), Mix{N: 50, Seed: 1})
	if n := only[Notice](out); len(s.Queue) != 0 || !n[0].Err {
		t.Errorf("no library: queue %d, notices %#v", len(s.Queue), n)
	}
	lib := mixable(2)
	for i := range lib.Tracks {
		lib.Tracks[i].Rating = 1
	}
	s, out = run(New(), CacheLoaded{Library: lib}, Mix{N: 50, Seed: 1})
	if n := only[Notice](
		out,
	); len(s.Queue) != 0 ||
		n[0].Text != "nothing to mix · every track is rated one star" {
		t.Errorf("all one star: queue %d, notices %#v", len(s.Queue), n)
	}
}
