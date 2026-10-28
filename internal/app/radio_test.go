package app

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/anopheles/navigo/internal/library"
	"github.com/anopheles/navigo/internal/player"
)

var radioNow = time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)

// radioLibrary has 12 artists of 6 rock tracks each.
func radioLibrary() []library.Track {
	var out []library.Track
	for a := range 12 {
		for n := range 6 {
			out = append(out, library.Track{
				ID:       library.TrackID(fmt.Sprintf("a%d-t%d", a, n)),
				Title:    fmt.Sprintf("Song %d of %d", n, a),
				Artist:   fmt.Sprintf("Artist %d", a),
				ArtistID: library.ArtistID(fmt.Sprintf("a%d", a)),
				AlbumID:  library.AlbumID(fmt.Sprintf("a%d-b", a)),
				Genre:    "Rock",
				Duration: 100 * time.Second,
			})
		}
	}
	return out
}

// radioPlaying is online with the radio on and two tracks queued, the
// first playing; the radio has asked for its first top-up.
func radioPlaying(t *testing.T) (State, RadioDue) {
	t.Helper()
	ts := radioLibrary()
	s := online()
	s.Library = library.Build("s", "u", time.Time{}, nil, nil, ts, nil, nil)
	s, _ = run(s, SetRadio{On: true}, PlayNow{Tracks: ts[:2]})
	s, out := run(s, FromPlayer{player.TrackStarted{Entry: s.Queue[0].ID}})
	due := only[RadioDue](out)
	if len(due) != 1 {
		t.Fatalf("effects %v: want one RadioDue", out)
	}
	return s, due[0]
}

// topUp answers due, and the similar-artist lookup with none.
func topUp(t *testing.T, s State, due RadioDue) State {
	t.Helper()
	s, out := run(s, RadioTopUp{Seq: due.Seq, Seed: 7, At: radioNow})
	if f := only[FetchSimilar](out); len(f) == 1 {
		s, _ = run(s, SimilarLoaded{Seq: f[0].Seq})
	}
	return s
}

func TestRadioTopsUpOutsideUndo(t *testing.T) {
	s, due := radioPlaying(t)
	undo := len(s.undo)
	s = topUp(t, s, due)
	if len(s.Queue) != 2+library.RadioBatch {
		t.Fatalf("queue has %d, want %d", len(s.Queue), 2+library.RadioBatch)
	}
	seen := map[library.TrackID]bool{}
	for i, q := range s.Queue {
		if q.Radio != (i >= 2) {
			t.Errorf("row %d radio = %v", i, q.Radio)
		}
		if seen[q.Track.ID] {
			t.Errorf("%s twice in the queue", q.Track.ID)
		}
		seen[q.Track.ID] = true
	}
	if len(s.undo) != undo {
		t.Errorf("undo history went from %d to %d", undo, len(s.undo))
	}
	// Far from the end now, a track starting asks for nothing.
	s, out := run(s, FromPlayer{player.TrackStarted{Entry: s.Queue[1].ID}})
	if d := only[RadioDue](out); len(d) != 0 {
		t.Errorf("asked again with %d rows ahead", len(s.Queue)-2)
	}
}

func TestRadioNeverAddsWhatPlayed(t *testing.T) {
	s, due := radioPlaying(t)
	s = topUp(t, s, due)
	for _, q := range s.Queue[2:] {
		if slices.ContainsFunc(s.Played, func(p library.Track) bool { return p.ID == q.Track.ID }) {
			t.Errorf("added %s, which played", q.Track.ID)
		}
	}
}

func TestRadioOfflineFromTheCache(t *testing.T) {
	s, due := radioPlaying(t)
	s.Offline = true
	s, out := run(s, RadioTopUp{Seq: due.Seq, Seed: 7, At: radioNow})
	if len(only[FetchSimilar](out)) != 0 || len(s.Queue) != 2+library.RadioBatch {
		t.Errorf("offline: %d queued, effects %v", len(s.Queue), out)
	}
}

func TestRadioStaleAnswerIgnored(t *testing.T) {
	s, due := radioPlaying(t)
	s, _ = run(s, SetRadio{On: false, Quiet: true})
	s, out := run(s, RadioTopUp{Seq: due.Seq, Seed: 7, At: radioNow})
	if len(s.Queue) != 2 || len(out) != 0 {
		t.Errorf("radio off: %d queued, effects %v", len(s.Queue), out)
	}
}

func TestSteerLessDropsTheArtistAndSkips(t *testing.T) {
	s, due := radioPlaying(t)
	s = topUp(t, s, due)
	target := s.Queue[3].Track // a radio track, not playing
	s, out := run(s, SteerRadio{Track: target})
	for _, q := range s.Queue[1:] {
		if q.Radio && q.Track.ArtistID == target.ArtistID {
			t.Errorf("%s by the turned-down artist is still queued", q.Track.ID)
		}
	}
	if n := only[Notice](
		out,
	); len(n) == 0 ||
		n[0].Text != "no more "+target.Artist+" on the radio" {
		t.Errorf("notices %v", n)
	}

	// Less like what plays skips it.
	playing := s.Queue[s.Current].Track
	s, out = run(s, SteerRadio{Track: playing})
	if len(only[Play](out)) != 1 || s.Queue[s.Current].Track.ID == playing.ID {
		t.Errorf("didn't skip %s: current %d, effects %v", playing.ID, s.Current, out)
	}
}

func TestSteerMoreSeedsTheRadio(t *testing.T) {
	s, _ := radioPlaying(t)
	liked := radioLibrary()[40]
	s, _ = run(s, SteerRadio{Track: liked, More: true})
	if !s.radio.more[liked.ArtistID] || !slices.Contains(s.radioSeed().Tracks, liked.ID) {
		t.Errorf("more like %s: more %v, seed %v", liked.ID, s.radio.more, s.radioSeed().Tracks)
	}
}

func TestSteerNeedsTheRadio(t *testing.T) {
	s := online()
	_, out := run(s, SteerRadio{Track: tracks(1)[0], More: true})
	if n := only[Notice](out); len(n) != 1 || !n[0].Err {
		t.Errorf("notices %v", n)
	}
}

func TestRadioAndRepeatAllExclude(t *testing.T) {
	s, _ := radioPlaying(t)
	s, _ = run(s, SetRepeat{Repeat: RepeatAll})
	if s.Radio {
		t.Error("repeat all left the radio on")
	}
	s, _ = run(s, SetRadio{On: true})
	if s.Repeat != RepeatOff {
		t.Errorf("radio on left repeat %v", s.Repeat)
	}
}
