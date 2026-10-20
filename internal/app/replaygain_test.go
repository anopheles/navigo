package app

import (
	"testing"

	"github.com/anopheles/navigo/internal/player"
)

func TestReplayGainMode(t *testing.T) {
	s := New()
	s, _ = Apply(
		s,
		ConfigureReplayGain{ReplayGain: player.ReplayGain{Mode: player.GainTrack, Preamp: 2}},
	)
	s, out := Apply(s, SetReplayGainMode{Mode: player.GainAlbum})
	want := player.ReplayGain{Mode: player.GainAlbum, Preamp: 2}
	if s.ReplayGain != want {
		t.Errorf("state = %+v, want %+v", s.ReplayGain, want)
	}
	if len(out) != 2 || out[0] != (SetReplayGain{ReplayGain: want}) ||
		out[1] != (Notice{Text: "replaygain album"}) {
		t.Errorf("effects = %+v", out)
	}
}

func TestReplayGainCycles(t *testing.T) {
	s := New()
	s, _ = Apply(s, ConfigureReplayGain{ReplayGain: player.ReplayGain{Mode: player.GainTrack}})
	var got []player.GainMode
	for range 4 {
		s, _ = Apply(s, SetReplayGainMode{Cycle: true})
		got = append(got, s.ReplayGain.Mode)
	}
	want := []player.GainMode{player.GainAlbum, player.GainOff, player.GainTrack, player.GainAlbum}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("cycle = %v, want %v", got, want)
		}
	}
}

func TestConfigureReplayGainDropsTheOverride(t *testing.T) {
	cfg := player.ReplayGain{Mode: player.GainTrack}
	s, _ := Apply(New(), ConfigureReplayGain{ReplayGain: cfg})
	s, _ = Apply(s, SetReplayGainMode{Mode: player.GainOff})
	s, out := Apply(s, ConfigureReplayGain{ReplayGain: cfg})
	if s.ReplayGain != cfg || len(out) != 1 || out[0] != (SetReplayGain{ReplayGain: cfg}) {
		t.Errorf("state %+v, effects %+v", s.ReplayGain, out)
	}
}

func TestGainTagsOnlyForThePlayingEntry(t *testing.T) {
	s := New()
	s.Player.Playing, s.playing = true, 7
	track := -6.5
	s, _ = Apply(s, FromPlayer{player.GainTags{Entry: 3, Track: &track}}) // a stale entry
	if _, ok := s.PlayingGain(); ok {
		t.Error("tags of another entry were kept")
	}
	s, _ = Apply(s, FromPlayer{player.GainTags{Entry: 7, Track: &track}})
	if g, ok := s.PlayingGain(); !ok || *g.Track != -6.5 || g.Album != nil {
		t.Errorf("PlayingGain = %+v, %v", g, ok)
	}
	s.playing = 8 // the next entry starts: its tags aren't read yet
	if _, ok := s.PlayingGain(); ok {
		t.Error("the old entry's tags show for the new one")
	}
}
