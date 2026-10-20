package app

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/anopheles/navigo/internal/library"
	"github.com/anopheles/navigo/internal/player"
)

func tracks(n int) []library.Track {
	out := make([]library.Track, n)
	for i := range out {
		out[i] = library.Track{ID: library.TrackID(fmt.Sprintf("t%d", i+1)), Title: fmt.Sprintf("Track %d", i+1), Duration: 100 * time.Second}
	}
	return out
}

// run applies events in order and returns the state and all effects but
// PublishNowPlaying, which publish (and TestPublishNowPlaying) checks.
func run(s State, events ...Event) (State, []Effect) {
	s, all := publish(s, events...)
	return s, slices.DeleteFunc(all, func(e Effect) bool { _, ok := e.(PublishNowPlaying); return ok })
}

// publish is run with every effect.
func publish(s State, events ...Event) (State, []Effect) {
	var all []Effect
	for _, e := range events {
		var out []Effect
		s, out = Apply(s, e)
		all = append(all, out...)
	}
	return s, all
}

// entry is the player entry for queue entry id of track "tN", whose
// title is "Track N".
func entry(id player.EntryID, track string) player.Entry {
	return player.Entry{ID: id, Track: library.TrackID(track), Title: "Track " + strings.TrimPrefix(track, "t")}
}

func only[T Effect](effects []Effect) []T {
	var out []T
	for _, e := range effects {
		if t, ok := e.(T); ok {
			out = append(out, t)
		}
	}
	return out
}

func snap(name string) *library.Snapshot {
	return library.Build("https://m.example", "ann", time.Time{}, nil, []library.Album{{ID: "al", Name: name}}, nil, nil, nil)
}

func TestStartupSync(t *testing.T) {
	cached := snap("cached")
	// Logging in before the cache is checked waits for it.
	s, out := run(New(), Start{}, LoggedIn{})
	if !reflect.DeepEqual(out, []Effect{LoadCache{}}) {
		t.Fatalf("effects = %#v, want only LoadCache", out)
	}
	s, out = run(s, CacheLoaded{Library: cached})
	if s.Library != cached || !reflect.DeepEqual(out, []Effect{StartSync{Old: cached}}) || !s.Sync.Running {
		t.Fatalf("after the cache: library %v, effects %#v", s.Library, out)
	}
	s, _ = run(s, SyncProgress{Done: 500, Total: 547})
	if s.Sync.Done != 500 || s.Sync.Total != 547 {
		t.Errorf("progress = %+v", s.Sync)
	}
	fresh := snap("fresh")
	s, out = run(s, Synced{Library: fresh})
	if s.Library != fresh || s.Sync.Running || !reflect.DeepEqual(out, []Effect{SaveCache{Library: fresh}}) {
		t.Errorf("after the sync: %v, %#v", s.Sync, out)
	}
	// A cache that loads late never replaces a synced library.
	if s, _ = run(s, CacheLoaded{Library: cached}); s.Library != fresh {
		t.Error("a late cache replaced the synced library")
	}
}

func TestSyncFailureKeepsTheLibrary(t *testing.T) {
	cached := snap("cached")
	s, _ := run(New(), Start{}, CacheLoaded{Library: cached}, LoggedIn{})
	s, out := run(s, Synced{Err: errors.New("offline")})
	if s.Library != cached || s.Sync.Running {
		t.Error("a failed sync dropped the cached library")
	}
	if n := only[Notice](out); len(n) != 1 || !n[0].Err {
		t.Errorf("effects = %#v", out)
	}
}

func TestSyncNow(t *testing.T) {
	if _, out := run(New(), SyncNow{}); !only[Notice](out)[0].Err {
		t.Error(":sync before login should say so")
	}
	s, _ := run(New(), Start{}, CacheLoaded{}, LoggedIn{})
	if _, out := run(s, SyncNow{}); only[Notice](out)[0].Text != "already syncing" {
		t.Error(":sync during a sync")
	}
	s, _ = run(s, Synced{Library: snap("x")})
	s, out := run(s, SyncNow{})
	if st := only[StartSync](out); len(st) != 1 || !st[0].Full || !s.Sync.Running {
		t.Errorf(":sync = %#v", out)
	}
}

func TestPlayNow(t *testing.T) {
	s, out := run(New(), PlayNow{Tracks: tracks(3), Start: 1})
	want := Play{Span: player.Span{Prev: entry(1, "t1"), Current: entry(2, "t2"), Next: entry(3, "t3")}}
	if p := only[Play](out); len(p) != 1 || p[0] != want {
		t.Fatalf("play = %#v, want %#v", p, want)
	}
	if n := only[Notice](out); n[0].Text != "playing · 3 tracks in the queue" || n[0].Command != "" {
		t.Errorf("notice = %#v", n)
	}
	if s.Current != 1 || len(s.Queue) != 3 {
		t.Errorf("current %d, queue %d", s.Current, len(s.Queue))
	}

	// Replacing a queue offers undo.
	s, out = run(s, PlayNow{Tracks: tracks(2), Start: 0})
	if n := only[Notice](out); n[0].Text != "playing · queue replaced with 2 tracks" || n[0].Command != "undo" {
		t.Errorf("notice = %#v", n)
	}
	if s.Queue[0].ID != 4 {
		t.Errorf("new entries reuse IDs: %d", s.Queue[0].ID)
	}
}

func TestTrackStartedSendsNeighboursAndScrobble(t *testing.T) {
	s, _ := run(New(), PlayNow{Tracks: tracks(3), Start: 0})
	s, out := run(s, FromPlayer{player.TrackStarted{Entry: 1}})
	want := []Effect{SetNeighbours{Next: entry(2, "t2")}, Scrobble{Track: "t1"}}
	if !reflect.DeepEqual(out, want) {
		t.Errorf("effects = %#v, want %#v", out, want)
	}
	// mpv moves on by itself.
	s, out = run(s, FromPlayer{player.TrackEnded{Entry: 1, Reason: player.Finished}}, FromPlayer{player.TrackStarted{Entry: 2}})
	if s.Current != 1 || !s.Player.Playing {
		t.Errorf("current %d playing %v", s.Current, s.Player.Playing)
	}
	if sn := only[SetNeighbours](out); len(sn) != 1 || sn[0] != (SetNeighbours{Prev: entry(1, "t1"), Next: entry(3, "t3")}) {
		t.Errorf("neighbours = %#v", sn)
	}
	// The last one ends the queue.
	s, _ = run(s, FromPlayer{player.TrackStarted{Entry: 3}}, FromPlayer{player.TrackEnded{Entry: 3, Reason: player.Finished}})
	if s.Player.Playing || s.Current != 2 {
		t.Errorf("after the end: playing %v current %d", s.Player.Playing, s.Current)
	}
}

func TestScrobbleSubmission(t *testing.T) {
	s, _ := run(New(), PlayNow{Tracks: tracks(1), Start: 0}, FromPlayer{player.TrackStarted{Entry: 1}})
	var out []Effect
	// A seek to 45s doesn't count as listening.
	s, out = run(s, FromPlayer{player.Position{Entry: 1, Position: 45 * time.Second, Duration: 100 * time.Second}})
	for sec := 46; sec <= 95; sec++ {
		var o []Effect
		s, o = run(s, FromPlayer{player.Position{Entry: 1, Position: time.Duration(sec) * time.Second, Duration: 100 * time.Second}})
		out = append(out, o...)
	}
	sub := only[Scrobble](out)
	if len(sub) != 1 || !sub[0].Submission {
		t.Fatalf("scrobbles = %#v, want one submission", sub)
	}
	// Submitted after 50s of listening, at 95s, not at the 50s mark.
	if s.listened < 50*time.Second {
		t.Errorf("listened %v", s.listened)
	}
}

func TestAddEndAndNext(t *testing.T) {
	s, _ := run(New(), PlayNow{Tracks: tracks(2), Start: 0}, FromPlayer{player.TrackStarted{Entry: 1}})
	s, out := run(s, AddNext{Tracks: tracks(1)})
	if s.Queue[1].ID != 3 {
		t.Errorf("queue = %v, want the new entry second", s.Queue)
	}
	if sn := only[SetNeighbours](out); len(sn) != 1 || sn[0].Next.ID != 3 {
		t.Errorf("neighbours = %#v, want the new entry next", sn)
	}
	if n := only[Notice](out); n[0].Text != "1 track plays next" {
		t.Errorf("notice = %q", n[0].Text)
	}
	s, out = run(s, AddEnd{Tracks: tracks(2)})
	if n := only[Notice](out); n[0].Text != "2 tracks added to the end of the queue (5 in queue)" {
		t.Errorf("notice = %q", n[0].Text)
	}
	if len(s.Queue) != 5 || s.Current != 0 {
		t.Errorf("queue %d current %d", len(s.Queue), s.Current)
	}
}

func TestUndoRedo(t *testing.T) {
	s, _ := run(New(), PlayNow{Tracks: tracks(2), Start: 0}, FromPlayer{player.TrackStarted{Entry: 1}}, AddEnd{Tracks: tracks(1)})
	s, out := run(s, Undo{})
	if len(s.Queue) != 2 || s.Current != 0 {
		t.Errorf("after undo: %d entries, current %d", len(s.Queue), s.Current)
	}
	if n := only[Notice](out); n[0].Command != "redo" {
		t.Errorf("notice = %#v", n)
	}
	s, _ = run(s, Redo{})
	if len(s.Queue) != 3 {
		t.Errorf("after redo: %d entries", len(s.Queue))
	}
	// Undoing the first PlayNow while its entry plays: the queue is empty
	// and the player is told there are no neighbours.
	s, out = run(s, Undo{}, Undo{})
	if len(s.Queue) != 0 || s.Current != -1 {
		t.Errorf("after undoing everything: %v current %d", s.Queue, s.Current)
	}
	if sn := only[SetNeighbours](out); sn[len(sn)-1] != (SetNeighbours{}) {
		t.Errorf("neighbours = %#v", sn)
	}
	if _, out = run(s, Undo{}); only[Notice](out)[0].Text != "nothing to undo" {
		t.Error("undo with no history")
	}
}

func TestUndoDoesNotChangeTheGivenState(t *testing.T) {
	s0, _ := run(New(), PlayNow{Tracks: tracks(2), Start: 0})
	before := fmt.Sprint(s0.Queue)
	run(s0, AddNext{Tracks: tracks(3)}, Undo{}, AddEnd{Tracks: tracks(1)})
	if fmt.Sprint(s0.Queue) != before {
		t.Error("Apply changed a State it was given")
	}
}

func TestTransport(t *testing.T) {
	s, out := run(New(), PlayPause{})
	if only[Notice](out)[0].Text != "the queue is empty" {
		t.Error("play-pause on an empty queue")
	}
	s, _ = run(s, PlayNow{Tracks: tracks(3), Start: 0}, FromPlayer{player.TrackStarted{Entry: 1}})
	if _, out = run(s, PlayPause{}); !reflect.DeepEqual(out, []Effect{Pause{}}) {
		t.Errorf("pause: %#v", out)
	}
	s, _ = run(s, FromPlayer{player.Paused{Paused: true}})
	if _, out = run(s, PlayPause{}); !reflect.DeepEqual(out, []Effect{Resume{}}) {
		t.Errorf("resume: %#v", out)
	}
	if _, out = run(s, SeekBy{D: 10 * time.Second}); !reflect.DeepEqual(out, []Effect{Seek{By: 10 * time.Second}}) {
		t.Errorf("seek: %#v", out)
	}
	s, out = run(s, NextTrack{})
	if p := only[Play](out); len(p) != 1 || p[0].Span.Current.ID != 2 {
		t.Errorf("next: %#v", out)
	}
	s, _ = run(s, FromPlayer{player.TrackStarted{Entry: 2}}, FromPlayer{player.Position{Entry: 2, Position: 10 * time.Second}})
	if _, out = run(s, PrevTrack{}); !reflect.DeepEqual(out, []Effect{SeekTo{}}) {
		t.Errorf("prev after 10s restarts the track: %#v", out)
	}
	s, out = run(s, StopPlayback{})
	if !reflect.DeepEqual(out, []Effect{Stop{}}) || s.Player.Playing {
		t.Errorf("stop: %#v playing %v", out, s.Player.Playing)
	}
	// The stopped track's end changes nothing; play-pause starts again.
	s, _ = run(s, FromPlayer{player.TrackEnded{Entry: 2, Reason: player.Stopped}})
	if _, out = run(s, PlayPause{}); len(only[Play](out)) != 1 {
		t.Errorf("play after stop: %#v", out)
	}
}

func TestPlayerDied(t *testing.T) {
	s, _ := run(New(), PlayNow{Tracks: tracks(1), Start: 0}, FromPlayer{player.TrackStarted{Entry: 1}})
	s, out := run(s, FromPlayer{player.Died{Err: errors.New("exit 1")}})
	if s.Player.Playing {
		t.Error("still playing after the player died")
	}
	if n := only[Notice](out); len(n) != 1 || n[0].Command != "play-pause" {
		t.Errorf("notice = %#v", n)
	}
}
