package app

import (
	"reflect"
	"testing"
	"time"

	"github.com/anopheles/navigo/internal/player"
)

// No audio output isn't the file's fault: playback stops at once, nothing
// is skipped, and NoAudio stays until a track really plays.
func TestNoAudioOutputStopsAtOnce(t *testing.T) {
	s, _ := run(online(), PlayNow{Tracks: tracks(3)}, FromPlayer{player.TrackStarted{Entry: 1}})
	s, out := run(s, FromPlayer{player.TrackEnded{Entry: 1, Reason: player.NoAudio}})
	want := []Effect{
		Stop{},
		Notice{
			Text:    "couldn't play Track 1 · no audio output · check the system's sound",
			Command: "play-pause", Hint: "tries again", Err: true,
		},
	}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("effects = %#v", out)
	}
	if !s.NoAudio || s.Current != 0 || s.Player.Playing {
		t.Fatalf("no audio %v, current %d, playing %v", s.NoAudio, s.Current, s.Player.Playing)
	}
	// mpv ran on to the next entry: ignored, as after any failure.
	if s, out = run(s, FromPlayer{player.TrackEnded{Entry: 2, Reason: player.NoAudio}}); out != nil {
		t.Errorf("a later entry's failure did %#v", out)
	}

	// <Space> tries the same track again; NoAudio holds until it plays.
	s, out = run(s, PlayPause{})
	if p := only[Play](out); len(p) != 1 || p[0].Span.Current.ID != s.Queue[0].ID {
		t.Fatalf("play-pause = %#v", out)
	}
	s, _ = run(s, FromPlayer{player.TrackStarted{Entry: p0(out)}})
	if !s.NoAudio {
		t.Error("NoAudio cleared before any sound")
	}
	s, _ = run(s, FromPlayer{player.Position{Entry: p0(out), Position: time.Second}})
	if s.NoAudio {
		t.Error("NoAudio still set while it plays")
	}
}

func p0(out []Effect) player.EntryID { return only[Play](out)[0].Span.Current.ID }

// A player call unanswered for a while marks mpv unresponsive, until it
// answers; mpv dying clears it too.
func TestPlayerUnresponsive(t *testing.T) {
	s, _ := run(online(), PlayerUnresponsive{Stuck: true})
	if !s.PlayerStuck {
		t.Fatal("not marked unresponsive")
	}
	if s, _ = run(s, PlayerUnresponsive{}); s.PlayerStuck {
		t.Error("still unresponsive after it answered")
	}
	s, _ = run(s, PlayerUnresponsive{Stuck: true}, PlayerDied{Err: errDown})
	if s.PlayerStuck {
		t.Error("still unresponsive after mpv died")
	}
}
