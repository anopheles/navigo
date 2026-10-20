package app

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/anopheles/navigo/internal/player"
)

var errDown = errors.New("connection refused")

// online is a logged-in session with a synced library.
func online() State {
	s, _ := run(
		New(),
		Start{},
		CacheLoaded{Library: snap("x")},
		LoggedIn{},
		Synced{Library: snap("x")},
	)
	return s
}

func TestGoOfflineOnce(t *testing.T) {
	s, out := run(online(), Unreachable{Err: errDown})
	if !s.Offline {
		t.Fatal("not offline after an unreachable request")
	}
	want := []Effect{
		ScheduleRetry{After: 2 * time.Second, Seq: 1},
		Notice{Text: "can't reach the server · browsing from the cache"},
	}
	if !reflect.DeepEqual(out, want) {
		t.Errorf("effects = %#v, want %#v", out, want)
	}
	if _, out = run(s, Unreachable{Err: errDown}); out != nil {
		t.Errorf("a second failure while offline: %#v", out)
	}
}

func TestBackoff(t *testing.T) {
	s, _ := run(online(), Unreachable{Err: errDown})
	var waits []time.Duration
	for range 8 {
		var out []Effect
		s, out = run(s, RetryNow{Seq: s.retrySeq})
		if p := only[Probe](out); len(p) != 1 || p[0].Why != ProbeRetry {
			t.Fatalf("retry didn't probe: %#v", out)
		}
		s, out = run(s, ProbeResult{Why: ProbeRetry, Err: errDown})
		waits = append(waits, only[ScheduleRetry](out)[0].After)
	}
	want := []time.Duration{4, 8, 16, 32, 60, 60, 60, 60}
	for i := range want {
		want[i] *= time.Second
	}
	if !reflect.DeepEqual(waits, want) {
		t.Errorf("backoff = %v, want %v", waits, want)
	}
	// A stale timer does nothing.
	if _, out := run(s, RetryNow{Seq: 1}); out != nil {
		t.Errorf("stale retry: %#v", out)
	}
}

func TestBackOnline(t *testing.T) {
	s, _ := run(online(), Unreachable{Err: errDown}, RetryNow{Seq: 1})
	s, out := run(s, ProbeResult{Why: ProbeRetry})
	if s.Offline || s.Probing {
		t.Error("still offline after the server answered")
	}
	if n := only[Notice](out); len(n) != 1 || n[0].Text != "back online" {
		t.Errorf("notices = %#v", n)
	}
	if st := only[StartSync](out); len(st) != 1 || st[0].Full {
		t.Errorf("want the startup sync check, got %#v", out)
	}
}

func TestProbeFindsWrongCredentials(t *testing.T) {
	s, _ := run(online(), Unreachable{Err: errDown}, RetryNow{Seq: 1})
	s, out := run(s, ProbeResult{Why: ProbeRetry, WrongCredentials: true})
	if s.Offline || !reflect.DeepEqual(out, []Effect{OpenLogin{}}) {
		t.Errorf("offline %v, effects %#v", s.Offline, out)
	}
}

func TestPlayWhileOffline(t *testing.T) {
	s, _ := run(online(), Unreachable{Err: errDown})
	s, out := run(s, PlayNow{Tracks: tracks(3), Start: 1})
	if len(s.Queue) != 3 || s.Current != 1 {
		t.Errorf("the queue didn't change offline: %d entries, current %d", len(s.Queue), s.Current)
	}
	if len(only[Play](out)) != 0 {
		t.Error("played while offline")
	}
	n := only[Notice](out)
	if len(n) != 1 || n[0].Text != "offline · the queue is ready" || n[0].Command != "play-pause" {
		t.Errorf("notices = %#v", n)
	}
	if p := only[Probe](out); len(p) != 1 || p[0].Why != ProbeUser {
		t.Errorf("playing offline should check the server now: %#v", out)
	}
	// Coming back doesn't start playback by itself.
	s, out = run(s, ProbeResult{Why: ProbeUser})
	if len(only[Play](out)) != 0 || s.Player.Playing {
		t.Error("playback started on reconnect")
	}
	if _, out = run(s, PlayPause{}); len(only[Play](out)) != 1 {
		t.Error("<Space> after reconnect didn't play")
	}
}

func TestOfflineDropsScrobblesAndRefusesSync(t *testing.T) {
	s, _ := run(online(), PlayNow{Tracks: tracks(2), Start: 0}, Unreachable{Err: errDown})
	// The current track plays on from mpv's buffer.
	s, out := run(s, FromPlayer{player.TrackStarted{Entry: 1}})
	if len(only[Scrobble](out)) != 0 {
		t.Error("scrobbled while offline")
	}
	if _, out = run(
		s,
		SyncNow{},
	); only[Notice](out)[0].Text != "offline · the library syncs once the server is back" {
		t.Errorf(":sync offline = %#v", out)
	}
}

func TestStreamFailureStopsAndProbes(t *testing.T) {
	s, _ := run(
		online(),
		PlayNow{Tracks: tracks(3), Start: 0},
		FromPlayer{player.TrackStarted{Entry: 1}},
	)
	s, out := run(
		s,
		FromPlayer{player.TrackEnded{Entry: 1, Reason: player.Failed, Err: "loading failed"}},
	)
	if !reflect.DeepEqual(out, []Effect{Stop{}, Probe{Why: ProbeStream}}) {
		t.Fatalf("effects = %#v", out)
	}
	// mpv ran on to the next entry before it stopped: ignored.
	s, out = run(
		s,
		FromPlayer{player.TrackStarted{Entry: 2}},
		FromPlayer{player.TrackEnded{Entry: 2, Reason: player.Failed}},
	)
	if out != nil || s.Current != 0 || s.Player.Playing {
		t.Errorf("events after the failure changed things: %#v, current %d", out, s.Current)
	}
	// The server is gone: offline, still on the failed track.
	off, out := run(s, ProbeResult{Why: ProbeStream, Err: errDown})
	if !off.Offline || off.Current != 0 || len(only[Play](out)) != 0 {
		t.Errorf("offline %v current %d effects %#v", off.Offline, off.Current, out)
	}
}

func TestBrokenFilesAreSkippedThreeTimes(t *testing.T) {
	s, _ := run(online(), PlayNow{Tracks: tracks(5), Start: 0})
	var notices []string
	for i := range 3 {
		var out []Effect
		id := s.Queue[s.Current].ID
		s, _ = run(s, FromPlayer{player.TrackStarted{Entry: id}})
		s, _ = run(
			s,
			FromPlayer{player.TrackEnded{Entry: id, Reason: player.Failed, Err: "loading failed"}},
		)
		s, out = run(s, ProbeResult{Why: ProbeStream})
		notices = append(notices, only[Notice](out)[0].Text)
		if i < 2 && (len(only[Play](out)) != 1 || s.Current != i+1) {
			t.Fatalf("skip %d: current %d, effects %#v", i, s.Current, out)
		}
	}
	want := []string{
		"couldn't play Track 1 (loading failed) · skipped",
		"couldn't play Track 2 (loading failed) · skipped",
		"couldn't play Track 3 (loading failed) · 3 tracks in a row failed, so playback stopped",
	}
	if !reflect.DeepEqual(notices, want) {
		t.Errorf("notices = %q", notices)
	}
	if s.Current != 2 {
		t.Errorf("stopped on row %d, want the third track", s.Current)
	}
	// Playing the track mpv had run on to works, and a track that really
	// plays resets the count.
	s, _ = run(s, PlayRow{Index: 3})
	s, _ = run(
		s,
		FromPlayer{player.TrackStarted{Entry: s.Queue[3].ID}},
		FromPlayer{player.Position{Entry: s.Queue[3].ID, Position: time.Second}},
	)
	if !s.Player.Playing || s.Current != 3 {
		t.Errorf("row 4 didn't play: current %d", s.Current)
	}
	if s.skips != 0 {
		t.Errorf("skips = %d after a track played", s.skips)
	}
}

func TestBufferingStall(t *testing.T) {
	s, _ := run(
		online(),
		PlayNow{Tracks: tracks(2), Start: 0},
		FromPlayer{player.TrackStarted{Entry: 1}},
	)
	s, out := run(s, BufferingStalled{})
	if !reflect.DeepEqual(out, []Effect{Probe{Why: ProbeStall}}) {
		t.Fatalf("stall = %#v", out)
	}
	// The server is just slow: keep waiting.
	if ok, out := run(s, ProbeResult{Why: ProbeStall}); ok.Offline || len(only[Stop](out)) != 0 {
		t.Errorf("a slow server stopped playback: %#v", out)
	}
	// The server is gone: stop and go offline.
	gone, out := run(s, ProbeResult{Why: ProbeStall, Err: errDown})
	if !gone.Offline || len(only[Stop](out)) != 1 || gone.Player.Playing {
		t.Errorf("gone: offline %v, effects %#v", gone.Offline, out)
	}
}

func TestStartedOffline(t *testing.T) {
	s, _ := run(New(), Start{}, CacheLoaded{Library: snap("cached")}, StartedOffline{})
	if !s.Offline || s.Library == nil {
		t.Fatal("started offline without the cache")
	}
	s, out := run(s, RetryNow{Seq: 1}, ProbeResult{Why: ProbeRetry})
	if !s.loggedIn || len(only[StartSync](out)) != 1 {
		t.Errorf("coming online after starting offline should sync: %#v", out)
	}
}

func TestReconnect(t *testing.T) {
	s, out := run(online(), Reconnect{})
	if p := only[Probe](out); len(p) != 1 || p[0].Why != ProbeReconnect {
		t.Fatalf("reconnect = %#v", out)
	}
	if _, out = run(
		s,
		ProbeResult{Why: ProbeReconnect},
	); only[Notice](out)[0].Text != "connected to the server" {
		t.Errorf("reconnect result = %#v", out)
	}
}

// mpv's late report about the entry it ran on to must not fail the track
// navigo replays in its place.
func TestLateEventsAfterASkipAreIgnored(t *testing.T) {
	s, _ := run(
		online(),
		PlayNow{Tracks: tracks(3), Start: 0},
		FromPlayer{player.TrackStarted{Entry: 1}},
	)
	s, _ = run(
		s,
		FromPlayer{player.TrackEnded{Entry: 1, Reason: player.Failed, Err: "loading failed"}},
	)
	s, out := run(s, ProbeResult{Why: ProbeStream})
	replay := only[Play](out)[0].Span.Current.ID
	if replay == 2 {
		t.Fatal("the skipped-to entry kept its old ID")
	}
	s, out = run(
		s,
		FromPlayer{player.TrackStarted{Entry: 2}},
		FromPlayer{player.TrackEnded{Entry: 2, Reason: player.Failed}},
	)
	if out != nil || s.skips != 1 {
		t.Errorf("late events for the old entry: %#v, skips %d", out, s.skips)
	}
	s, _ = run(s, FromPlayer{player.TrackStarted{Entry: replay}})
	if !s.Player.Playing || s.Current != 1 {
		t.Errorf("the replayed entry didn't start: current %d", s.Current)
	}
}

// mpv advancing gaplessly onto a neighbour that was stale still counts.
func TestReplayedNeighboursAreFresh(t *testing.T) {
	s, _ := run(
		online(),
		PlayNow{Tracks: tracks(3), Start: 0},
		FromPlayer{player.TrackStarted{Entry: 1}},
	)
	s, _ = run(
		s,
		FromPlayer{player.TrackEnded{Entry: 1, Reason: player.Failed, Err: "loading failed"}},
		ProbeResult{Why: ProbeStream, Err: errDown},
	)
	// Back online, the user plays row 1 again: its next (entry 2) was stale.
	s, _ = run(s, RetryNow{Seq: s.retrySeq}, ProbeResult{Why: ProbeRetry})
	s, out := run(s, PlayRow{Index: 0})
	sp := only[Play](out)[0].Span
	if sp.Current.ID == 1 || sp.Next.ID == 2 {
		t.Fatalf("span still has stale IDs: %+v", sp)
	}
	s, _ = run(
		s,
		FromPlayer{player.TrackStarted{Entry: sp.Current.ID}},
		FromPlayer{player.TrackEnded{Entry: sp.Current.ID, Reason: player.Finished}},
		FromPlayer{player.TrackStarted{Entry: sp.Next.ID}},
	)
	if s.Current != 1 || !s.Player.Playing {
		t.Errorf(
			"advancing to the fresh neighbour: current %d, playing %v",
			s.Current,
			s.Player.Playing,
		)
	}
}
