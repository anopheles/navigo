package app

import (
	"errors"
	"slices"
	"time"

	"github.com/anopheles/navigo/internal/player"
)

// mpv's health: the startup check, a death that <Space> recovers from by
// resuming where it stopped, and the cap on restarts after 3 deaths in a
// minute.
type (
	// PlayerChecked is the startup check's result. Hint is how to install
	// mpv on this OS.
	PlayerChecked struct {
		Err  error
		Hint string
	}
	// Uncap ends the minute without restarts after repeated crashes.
	Uncap struct{}
	// PlayerDied is the Player's Died with when it came, which the crash
	// cap counts with; the UI sends it for each Died.
	PlayerDied struct {
		Err error
		At  time.Time
	}
)

// ScheduleUncap asks for Uncap after a while.
type ScheduleUncap struct{ After time.Duration }

func (PlayerChecked) isEvent()  {}
func (Uncap) isEvent()          {}
func (PlayerDied) isEvent()     {}
func (ScheduleUncap) isEffect() {}

const (
	crashCap    = 3           // deaths within crashWindow…
	crashWindow = time.Minute // …stop restarts for crashWindow
)

func (s State) applyMPV(e Event) (State, []Effect) {
	switch e := e.(type) {
	case PlayerChecked:
		s.installHint = e.Hint
		switch {
		case e.Err == nil:
			s.PlayerProblem = ""
		case errors.Is(e.Err, player.ErrNoMediaControls):
			// It plays; only the OS's controls are missing. Said once.
			s.PlayerProblem = ""
			return s, []Effect{Notice{Text: "mpv plays, but " + e.Err.Error()}}
		case errors.Is(e.Err, player.ErrMPVTooOld):
			s.PlayerProblem = "mpv too old · install mpv 0.38 or newer to play"
		case errors.Is(e.Err, player.ErrMPVMissing):
			s.PlayerProblem = "mpv not found · install mpv 0.38 or newer to play"
		default:
			s.PlayerProblem = "mpv can't start · " + e.Err.Error()
		}
	case Uncap:
		s.PlayerCapped = false
	}
	return s, nil
}

// died handles mpv exiting: what played is remembered for <Space>, and
// the third death in a minute stops restarts for a minute.
func (s State) died(err error, at time.Time) (State, []Effect) {
	if s.playing != 0 {
		if i := s.index(s.playing); i >= 0 {
			s.resume = &resumePoint{entry: s.playing, pos: s.Player.Position}
		}
	}
	s.playing = 0
	s.Player = PlayerState{Volume: s.Player.Volume}
	s.PlayerDown, s.PlayerStuck = true, false
	s.deaths = append(
		slices.DeleteFunc(
			slices.Clone(s.deaths),
			func(t time.Time) bool { return at.Sub(t) >= crashWindow },
		),
		at,
	)
	if len(s.deaths) >= crashCap {
		s.deaths, s.PlayerCapped = nil, true
		return s, []Effect{
			ScheduleUncap{After: crashWindow},
			Notice{
				Text:       "mpv crashed 3 times in a minute · it can start again in a minute, or now with :reconnect",
				Err:        true,
				Unexpected: true,
			},
		}
	}
	return s, []Effect{
		Notice{
			Text:    "the player stopped (" + err.Error() + ")",
			Command: "play-pause",
			Hint:    "starts it again where it stopped",
			Err:     true,
		},
	}
}

// resumePoint is where playback was when mpv died.
type resumePoint struct {
	entry player.EntryID
	pos   time.Duration
}

// cantPlay is why nothing can play now, or nil.
func (s State) cantPlay() []Effect {
	switch {
	case s.PlayerProblem != "":
		text := s.PlayerProblem
		if s.installHint != "" {
			text += " · " + s.installHint
		}
		return []Effect{Notice{Text: text, Err: true}}
	case s.PlayerCapped:
		return []Effect{
			Notice{
				Text: "mpv crashed 3 times in a minute · it can start again in a minute, or now with :reconnect",
				Err:  true,
			},
		}
	}
	return nil
}
