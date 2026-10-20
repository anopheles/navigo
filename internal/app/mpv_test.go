package app

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/anopheles/navigo/internal/player"
)

func TestMissingMPV(t *testing.T) {
	s, _ := run(
		New(),
		PlayerChecked{
			Err:  fmt.Errorf("%w at mpv", player.ErrMPVMissing),
			Hint: "sudo apt install mpv",
		},
	)
	if s.PlayerProblem != "mpv not found · install mpv 0.38 or newer to play" {
		t.Fatalf("problem %q", s.PlayerProblem)
	}
	// The queue fills, but nothing plays, and play keys say why.
	s, out := run(s, PlayNow{Tracks: tracks(3), Start: 0})
	if len(s.Queue) != 3 || len(only[Play](out)) != 0 {
		t.Errorf("queue %d, plays %#v", len(s.Queue), only[Play](out))
	}
	if n := only[Notice](
		out,
	); len(n) != 1 ||
		n[0].Text != "mpv not found · install mpv 0.38 or newer to play · sudo apt install mpv" {
		t.Errorf("notice = %#v", n)
	}
	if _, out = run(s, PlayPause{}); len(only[Play](out)) != 0 || !only[Notice](out)[0].Err {
		t.Errorf("<Space>: %#v", out)
	}

	s, _ = run(New(), PlayerChecked{Err: fmt.Errorf("%w, found mpv v0.37.0", player.ErrMPVTooOld)})
	if s.PlayerProblem != "mpv too old · install mpv 0.38 or newer to play" {
		t.Errorf("problem %q", s.PlayerProblem)
	}
}

func TestResumeAfterDied(t *testing.T) {
	s := playing4(t)
	s, _ = run(
		s,
		FromPlayer{
			player.Position{Entry: 2, Position: 42 * time.Second, Duration: 100 * time.Second},
		},
	)
	s, out := run(s, PlayerDied{Err: errors.New("exit 1"), At: time.Unix(0, 0)})
	if s.Player.Playing || !s.PlayerDown {
		t.Fatalf("after Died: playing %v down %v", s.Player.Playing, s.PlayerDown)
	}
	if n := only[Notice](
		out,
	); n[0].Command != "play-pause" ||
		n[0].Hint != "starts it again where it stopped" {
		t.Errorf("notice = %#v", n)
	}
	s, out = run(s, PlayPause{})
	p := only[Play](out)
	if len(p) != 1 || p[0].Span.Current.ID != 2 || p[0].Span.Start != 42*time.Second {
		t.Fatalf("<Space> after Died = %#v", p)
	}
	s, _ = run(s, FromPlayer{player.TrackStarted{Entry: 2}})
	if s.PlayerDown {
		t.Error("still down after the track started")
	}
	// Played again from its start the next time.
	s, _ = run(s, StopPlayback{})
	if _, out = run(s, PlayPause{}); only[Play](out)[0].Span.Start != 0 {
		t.Errorf("a later play resumed too: %#v", only[Play](out))
	}
}

func TestCrashCap(t *testing.T) {
	s := playing4(t)
	t0 := time.Unix(1000, 0)
	die := func(s State, at time.Time) (State, []Effect) {
		s, _ = run(s, PlayPause{}, FromPlayer{player.TrackStarted{Entry: 2}})
		return run(s, PlayerDied{Err: errors.New("crash"), At: at})
	}
	s, _ = die(s, t0)
	s, _ = die(s, t0.Add(20*time.Second))
	if s.PlayerCapped {
		t.Fatal("capped after two deaths")
	}
	// A death a minute after the first doesn't count it.
	s, _ = die(s, t0.Add(61*time.Second))
	if s.PlayerCapped {
		t.Fatal("capped counting a death more than a minute old")
	}
	s, out := die(s, t0.Add(70*time.Second))
	if !s.PlayerCapped {
		t.Fatal("not capped after three deaths in a minute")
	}
	if u := only[ScheduleUncap](out); len(u) != 1 || u[0].After != time.Minute {
		t.Errorf("uncap = %#v", u)
	}
	if n := only[Notice](
		out,
	); !n[0].Unexpected ||
		n[0].Text != "mpv crashed 3 times in a minute · it can start again in a minute, or now with :reconnect" {
		t.Errorf("notice = %#v", n)
	}
	// Capped: <Space> doesn't restart it.
	if _, out = run(s, PlayPause{}); len(only[Play](out)) != 0 {
		t.Errorf("restarted while capped: %#v", out)
	}
	// After the minute, or :reconnect, it can.
	if u, out := run(s, Uncap{}, PlayPause{}); u.PlayerCapped || len(only[Play](out)) != 1 {
		t.Errorf("after Uncap: capped %v, %#v", u.PlayerCapped, out)
	}
	if r, out := run(s, Reconnect{}, PlayPause{}); r.PlayerCapped || len(only[Play](out)) != 1 {
		t.Errorf("after :reconnect: capped %v, %#v", r.PlayerCapped, out)
	}
}

func TestOldMPVWithoutMediaControls(t *testing.T) {
	s, out := run(
		New(),
		PlayerChecked{Err: fmt.Errorf("%w; found mpv v0.38.0", player.ErrNoMediaControls)},
	)
	if s.PlayerProblem != "" {
		t.Errorf("a problem for a warning: %q", s.PlayerProblem)
	}
	if n := only[Notice](
		out,
	); len(n) != 1 ||
		n[0].Text != "mpv plays, but the OS's media controls need mpv 0.39 or newer; found mpv v0.38.0" {
		t.Errorf("notice = %#v", n)
	}
	if _, out = run(s, PlayNow{Tracks: tracks(1), Start: 0}); len(only[Play](out)) != 1 {
		t.Error("it doesn't play")
	}
}
