package app

import (
	"fmt"

	"github.com/anopheles/navigo/internal/player"
)

// Events for ReplayGain.
type (
	// SetReplayGainMode is :replaygain: it sets the mode for the session,
	// or moves to the next one when Cycle is set.
	SetReplayGainMode struct {
		Mode  player.GainMode
		Cycle bool
	}
	// ConfigureReplayGain puts the configured mode and preamp in use, at
	// startup and on :reload-config, dropping a session override.
	ConfigureReplayGain struct{ ReplayGain player.ReplayGain }
)

// SetReplayGain is the effect that sets the player's ReplayGain.
type SetReplayGain struct{ ReplayGain player.ReplayGain }

func (SetReplayGainMode) isEvent()   {}
func (ConfigureReplayGain) isEvent() {}
func (SetReplayGain) isEffect()      {}

var gainModes = []player.GainMode{player.GainOff, player.GainTrack, player.GainAlbum}

func (s State) applyReplayGain(e Event) (State, []Effect) {
	switch e := e.(type) {
	case SetReplayGainMode:
		mode := e.Mode
		if e.Cycle {
			next := 0
			for i, m := range gainModes {
				if m == s.ReplayGain.Mode {
					next = (i + 1) % len(gainModes)
				}
			}
			mode = gainModes[next]
		}
		s.ReplayGain.Mode = mode
		return s, []Effect{
			SetReplayGain{ReplayGain: s.ReplayGain},
			Notice{Text: fmt.Sprintf("replaygain %s", mode)},
		}
	case ConfigureReplayGain:
		s.ReplayGain = e.ReplayGain
		return s, []Effect{SetReplayGain{ReplayGain: s.ReplayGain}}
	}
	return s, nil
}

// PlayingGain returns the ReplayGain tags of the playing file. ok is
// false while nothing plays or the player hasn't read the tags yet.
func (s State) PlayingGain() (g player.GainTags, ok bool) {
	if !s.Player.Playing || s.playing == 0 || s.Gain.Entry != s.playing {
		return player.GainTags{}, false
	}
	return s.Gain, true
}
