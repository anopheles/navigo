package ui

import (
	"errors"
	"fmt"
	"strings"

	"github.com/anopheles/navigo/internal/app"
	"github.com/anopheles/navigo/internal/command"
	"github.com/anopheles/navigo/internal/config"
	"github.com/anopheles/navigo/internal/player"
)

// gainOf is the ReplayGain the config asks for.
func gainOf(p config.Player) player.ReplayGain {
	return player.ReplayGain{Mode: player.GainMode(p.GainMode()), Preamp: p.ReplayGainPreamp}
}

func replaygainArgs(a []string) error {
	if len(a) > 1 || len(a) == 1 && a[0] != "off" && a[0] != "track" && a[0] != "album" {
		return errors.New("replaygain: give off, track or album")
	}
	return nil
}

// replaygain is :replaygain: set the mode for this session, or with no
// argument move to the next one.
func replaygain(m *Model, c command.Call) error {
	if err := replaygainArgs(c.Args); err != nil {
		return err
	}
	if len(c.Args) == 0 {
		m.event(app.SetReplayGainMode{Cycle: true})
	} else {
		m.event(app.SetReplayGainMode{Mode: player.GainMode(c.Args[0])})
	}
	return nil
}

func gainWords(*Model) []string { return []string{"off", "track", "album"} }

// gainIndicator is the ReplayGain tag of the now-playing bar, empty when off.
func (m *Model) gainIndicator() string {
	switch m.state.ReplayGain.Mode {
	case player.GainTrack:
		return "RG track"
	case player.GainAlbum:
		return "RG album"
	}
	return ""
}

// gainText is what the info popup says about the playing file's tags.
func (m *Model) gainText() string {
	g, ok := m.state.PlayingGain()
	if !ok {
		return ""
	}
	var parts []string
	if g.Track != nil {
		parts = append(parts, fmt.Sprintf("track %+.2f dB", *g.Track))
	}
	if g.Album != nil {
		parts = append(parts, fmt.Sprintf("album %+.2f dB", *g.Album))
	}
	if len(parts) == 0 {
		return "no replaygain tags"
	}
	text := strings.Join(parts, " · ")
	if m.state.ReplayGain.Mode == player.GainOff {
		text += " (off)"
	}
	return text
}

// newState is the core's state at the start of a session, with what the
// config sets: ReplayGain and Resume.
func newState(cfg config.Config) app.State {
	s := app.New()
	s.ReplayGain = gainOf(cfg.Player)
	s.ResumeOn, s.SyncOn = cfg.ResumeOn(), cfg.SyncQueue.On
	return s.WithListenBrainz(lbOf(cfg.ListenBrainz))
}
