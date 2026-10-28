package app

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/anopheles/navigo/internal/library"
	"github.com/anopheles/navigo/internal/player"
)

func TestShuffleAndUndo(t *testing.T) {
	s := playing4(t) // entry 2 plays
	s, out := run(s, Shuffle{Seed: 7})
	got := ids(s)
	if got[0] != 2 || s.Current != 0 {
		t.Fatalf("queue %v current %d: want the playing entry first", got, s.Current)
	}
	sorted := slices.Clone(got)
	slices.Sort(sorted)
	if !sameIDs(sorted, 1, 2, 3, 4) {
		t.Errorf("shuffled %v: entries lost or added", got)
	}
	if sn := only[SetNeighbours](out); len(sn) != 1 || sn[0].Next.ID != got[1] {
		t.Errorf("neighbours = %#v, want the new second row next", sn)
	}
	if n := only[Notice](out); n[0].Text != "queue shuffled" || n[0].Command != "undo" {
		t.Errorf("notice = %#v", n)
	}
	// The same seed shuffles the same way; undo puts it back.
	if again, _ := run(playing4(t), Shuffle{Seed: 7}); !sameIDs(ids(again), got...) {
		t.Errorf("seed 7 gave %v, then %v", got, ids(again))
	}
	s, _ = run(s, Undo{})
	if !sameIDs(ids(s), 1, 2, 3, 4) || s.Current != 1 {
		t.Errorf("after undo: %v current %d", ids(s), s.Current)
	}
	if _, out = run(New(), Shuffle{Seed: 1}); only[Notice](out)[0].Text != "nothing to shuffle" {
		t.Error("shuffling an empty queue")
	}
}

func TestRepeatAll(t *testing.T) {
	s, _ := run(
		New(),
		PlayNow{Tracks: tracks(3), Start: 2},
		FromPlayer{player.TrackStarted{Entry: 3}},
	)
	s, out := run(s, CycleRepeat{})
	if s.Repeat != RepeatAll || only[Notice](out)[0].Text != "repeat all" {
		t.Fatalf("repeat %v, %#v", s.Repeat, out)
	}
	// The last row's next is the first.
	if sn := only[SetNeighbours](
		out,
	); sn[0] != (SetNeighbours{Prev: entry(2, "t2"), Next: entry(1, "t1")}) {
		t.Errorf("neighbours = %#v", sn)
	}
	// At the end of the queue mpv runs on to it; the core keeps playing.
	s, _ = run(s, FromPlayer{player.TrackEnded{Entry: 3, Reason: player.Finished}})
	if !s.Player.Playing {
		t.Error("playback stopped at the end of the queue")
	}
	s, _ = run(s, FromPlayer{player.TrackStarted{Entry: 1}})
	if s.Current != 0 {
		t.Errorf("current %d after wrapping", s.Current)
	}
	// > at the end and < at the start go round too.
	s, out = run(s, PrevTrack{})
	if p := only[Play](out); len(p) != 1 || p[0].Span.Current.ID != 3 {
		t.Errorf("< from the first row = %#v", p)
	}
	s, _ = run(s, FromPlayer{player.TrackStarted{Entry: 3}})
	if _, out = run(s, NextTrack{}); only[Play](out)[0].Span.Current.ID != 1 {
		t.Errorf("> from the last row = %#v", only[Play](out))
	}
}

func TestRepeatOne(t *testing.T) {
	s, _ := run(
		New(),
		PlayNow{Tracks: tracks(3), Start: 0},
		FromPlayer{player.TrackStarted{Entry: 1}},
	)
	s, out := run(s, CycleRepeat{}, CycleRepeat{})
	if s.Repeat != RepeatOne {
		t.Fatalf("repeat %v", s.Repeat)
	}
	// Nothing follows in the player...
	if sn := only[SetNeighbours](out); sn[len(sn)-1] != (SetNeighbours{}) {
		t.Errorf("neighbours = %#v", sn)
	}
	// ...and at the end of the track it plays again, scrobbled again.
	s, out = run(s, FromPlayer{player.TrackEnded{Entry: 1, Reason: player.Finished}})
	if p := only[Play](out); len(p) != 1 || p[0].Span.Current.ID != 1 {
		t.Fatalf("at the end of the track: %#v", out)
	}
	s, out = run(s, FromPlayer{player.TrackStarted{Entry: 1}})
	if sc := only[Scrobble](out); len(sc) != 1 {
		t.Errorf("scrobbles = %#v", sc)
	}
	// Also at the end of the queue.
	s, _ = run(s, PlayRow{Index: 2}, FromPlayer{player.TrackStarted{Entry: 3}})
	if _, out = run(
		s,
		FromPlayer{player.TrackEnded{Entry: 3, Reason: player.Finished}},
	); only[Play](out)[0].Span.Current.ID != 3 {
		t.Errorf("at the end of the queue: %#v", out)
	}
	// Off again: the end of the queue ends playback.
	s, _ = run(s, CycleRepeat{}, FromPlayer{player.TrackStarted{Entry: 3}})
	if s, _ = run(
		s,
		FromPlayer{player.TrackEnded{Entry: 3, Reason: player.Finished}},
	); s.Player.Playing {
		t.Error("still playing with repeat off")
	}
}

func TestVolume(t *testing.T) {
	s, out := run(New(), ChangeVolume{By: -5})
	if s.Player.Volume != 95 || only[SetVolume](out)[0].Percent != 95 ||
		only[Notice](out)[0].Text != "volume 95%" {
		t.Errorf("volume %d, %#v", s.Player.Volume, out)
	}
	if s, _ = run(s, ChangeVolume{By: 50}); s.Player.Volume != 100 {
		t.Errorf("over 100: %d", s.Player.Volume)
	}
	if s, _ = run(s, ChangeVolume{Set: true, To: -3}); s.Player.Volume != 0 {
		t.Errorf("below 0: %d", s.Player.Volume)
	}
	if s, _ = run(s, FromPlayer{player.Volume{Percent: 40}}); s.Player.Volume != 40 {
		t.Errorf("the player's volume: %d", s.Player.Volume)
	}
}

func TestStars(t *testing.T) {
	s := New()
	s.Library = library.Build(
		"u",
		"ann",
		time.Time{},
		nil,
		nil,
		tracks(2),
		nil,
		[]library.Star{{ID: "t2"}},
	)
	s, out := run(s, SetStars{On: true, Tracks: []string{"t1"}, Name: "Track 1"})
	w := only[WriteStar](out)
	if len(w) != 1 || !w[0].On || s.Library.IsStarred("t1") {
		t.Fatalf("write = %#v; starred before the server agreed: %v", w, s.Library.IsStarred("t1"))
	}
	at := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	s, out = run(s, StarsWritten{Write: w[0], At: at})
	if !s.Library.IsStarred("t1") || !s.Library.IsStarred("t2") ||
		only[Notice](out)[0].Text != "starred Track 1" {
		t.Errorf("after: %v %v %#v", s.Library.IsStarred("t1"), s.Library.IsStarred("t2"), out)
	}
	// A new star is kept as starred when the server took it, newest first.
	if !s.Library.StarredAt("t1").Equal(at) || s.Library.StarredTracks()[0].ID != "t1" {
		t.Errorf(
			"t1 starred at %v, starred tracks %v",
			s.Library.StarredAt("t1"),
			s.Library.StarredTracks(),
		)
	}
	s, out = run(s, SetStars{Tracks: []string{"t2"}, Name: "Track 2"})
	s, _ = run(s, StarsWritten{Write: only[WriteStar](out)[0]})
	if s.Library.IsStarred("t2") {
		t.Error("t2 still starred")
	}

	// Refused offline; a failure changes nothing.
	off := s
	off.Offline = true
	if _, out = run(
		off,
		SetStars{On: true, Tracks: []string{"t2"}},
	); len(
		only[WriteStar](out),
	) != 0 {
		t.Error("starred while offline")
	}
	s, out = run(s, SetStars{On: true, Tracks: []string{"t2"}})
	s, _ = run(
		s,
		StarsWritten{Write: only[WriteStar](out)[0], Err: errors.New("gone"), Unreachable: true},
	)
	if s.Library.IsStarred("t2") || !s.Offline {
		t.Errorf("after a failure: starred %v offline %v", s.Library.IsStarred("t2"), s.Offline)
	}
}

func TestRatings(t *testing.T) {
	s := New()
	s.Library = library.Build("u", "ann", time.Time{}, nil, nil, tracks(3), nil, nil)
	rating := func(id library.TrackID) int { tr, _ := s.Library.Track(id); return tr.Rating }
	s, out := run(s, SetRating{Tracks: []library.TrackID{"t1", "t2"}, Rating: 4, Name: "2 tracks"})
	w := only[WriteRating](out)
	if len(w) != 1 || w[0].Rating != 4 || len(w[0].Tracks) != 2 || rating("t1") != 0 {
		t.Fatalf("write = %#v; rated before the server agreed: %d", w, rating("t1"))
	}
	s, out = run(s, RatingWritten{Write: w[0]})
	if rating("t1") != 4 || rating("t2") != 4 || rating("t3") != 0 ||
		only[Notice](out)[0].Text != "rated 2 tracks ★★★★" || len(only[SaveCache](out)) != 1 {
		t.Errorf("after: %d %d %d %#v", rating("t1"), rating("t2"), rating("t3"), out)
	}
	s, out = run(s, SetRating{Tracks: []library.TrackID{"t1"}, Rating: 0, Name: "Track 1"})
	s, out = run(s, RatingWritten{Write: only[WriteRating](out)[0]})
	if rating("t1") != 0 || only[Notice](out)[0].Text != "rating cleared for Track 1" {
		t.Errorf("cleared: %d %#v", rating("t1"), out)
	}

	// Refused offline; a failure changes nothing.
	off := s
	off.Offline = true
	if _, out = run(off, SetRating{Tracks: []library.TrackID{"t3"}, Rating: 2}); len(
		only[WriteRating](out),
	) != 0 {
		t.Error("rated while offline")
	}
	s, out = run(s, SetRating{Tracks: []library.TrackID{"t3"}, Rating: 2})
	s, _ = run(
		s,
		RatingWritten{Write: only[WriteRating](out)[0], Err: errors.New("gone"), Unreachable: true},
	)
	if rating("t3") != 0 || !s.Offline {
		t.Errorf("after a failure: rating %d offline %v", rating("t3"), s.Offline)
	}
}

func TestPublishNowPlaying(t *testing.T) {
	s, out := publish(
		New(),
		PlayNow{Tracks: tracks(3), Start: 0},
		FromPlayer{player.TrackStarted{Entry: 1}},
	)
	p := only[PublishNowPlaying](out)
	if len(p) == 0 {
		t.Fatal("nothing published")
	}
	last := p[len(p)-1]
	if last.Track.ID != "t1" || !last.Playing || last.Paused || !last.CanNext || !last.CanPlay ||
		last.Volume != 100 {
		t.Errorf("published %+v", last.NowPlayingInfo)
	}
	// Each change publishes once; nothing new publishes nothing.
	s, out = publish(s, FromPlayer{player.Paused{Paused: true}})
	if p := only[PublishNowPlaying](out); len(p) != 1 || !p[0].Paused {
		t.Errorf("after pausing: %+v", p)
	}
	if _, out = publish(
		s,
		FromPlayer{player.Paused{Paused: true}},
	); len(
		only[PublishNowPlaying](out),
	) != 0 {
		t.Error("published without a change")
	}
	// The last row can't go next, unless the queue repeats.
	s, _ = run(s, PlayRow{Index: 2}, FromPlayer{player.TrackStarted{Entry: 3}})
	if np := s.nowPlaying(); np.CanNext {
		t.Error("CanNext on the last row")
	}
	s, _ = run(s, SetRepeat{Repeat: RepeatAll})
	if np := s.nowPlaying(); !np.CanNext || np.Repeat != RepeatAll {
		t.Errorf("repeat all: %+v", np)
	}
	// SeekAt seeks only while playing.
	if _, out = run(s, SeekAt{Pos: time.Minute}); len(only[SeekTo](out)) != 1 {
		t.Errorf("SeekAt: %#v", out)
	}
}

func TestUpNext(t *testing.T) {
	if _, ok := New().UpNext(); ok {
		t.Error("something is up next with nothing playing")
	}
	s := playing4(t)
	if n, ok := s.UpNext(); !ok || n.ID != "t3" {
		t.Errorf("up next %q %v, want t3", n.ID, ok)
	}
	// Removed while it plays: the queue goes on with what followed it.
	r, _ := run(s, Remove{From: 1, To: 3})
	if n, ok := r.UpNext(); !ok || n.ID != "t4" {
		t.Errorf("after removing it: up next %q %v, want t4", n.ID, ok)
	}
	r, _ = run(s, Remove{From: 1, To: 4})
	if _, ok := r.UpNext(); ok {
		t.Error("something is up next with the rest removed")
	}
	// The last row: nothing, or the first under repeat all; repeat one
	// plays the same entry again.
	s, _ = run(s, FromPlayer{player.TrackStarted{Entry: 4}})
	if _, ok := s.UpNext(); ok {
		t.Error("something is up next after the last row")
	}
	s, _ = run(s, CycleRepeat{})
	if n, ok := s.UpNext(); !ok || n.ID != "t1" {
		t.Errorf("repeat all: up next %q %v, want t1", n.ID, ok)
	}
	s, _ = run(s, CycleRepeat{})
	if _, ok := s.UpNext(); ok {
		t.Error("something is up next under repeat one")
	}
}
