package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/anopheles/navigo/internal/app"
	"github.com/anopheles/navigo/internal/mediactl"
)

type mediaMsg struct{ mediactl.Command }

// MediaMsg wraps a command from the OS's media controls for Program.Send.
func MediaMsg(c mediactl.Command) tea.Msg { return mediaMsg{c} }

// mediaStatus is what the media controls show, from the core's.
func mediaStatus(np app.NowPlayingInfo) mediactl.Status {
	s := mediactl.Status{
		Playing:  np.Playing,
		Paused:   np.Paused,
		Position: np.Position,
		Length:   np.Length,
		CanNext:  np.CanNext,
		CanPrev:  np.CanPrev,
		CanPlay:  np.CanPlay,
		Volume:   np.Volume,
		Loop:     [...]mediactl.Loop{app.RepeatOff: mediactl.LoopNone, app.RepeatAll: mediactl.LoopPlaylist, app.RepeatOne: mediactl.LoopTrack}[np.Repeat],
	}
	if t := np.Track; t.ID != "" {
		s.TrackID, s.Title, s.Album = string(t.ID), t.Title, t.Album
		for _, a := range t.Artists {
			s.Artists = append(s.Artists, a.Name)
		}
		if len(s.Artists) == 0 && t.Artist != "" {
			s.Artists = []string{t.Artist}
		}
		if s.Length == 0 {
			s.Length = t.Duration
		}
	}
	return s
}

// mediaCommand turns a media key or widget into what the same keys in
// navigo do. Play and Pause only act when they change something.
func (m *Model) mediaCommand(c mediactl.Command) tea.Cmd {
	p := m.state.Player
	switch c.Kind {
	case mediactl.Play:
		if p.Playing && !p.Paused {
			return nil
		}
		return m.dispatch(app.PlayPause{})
	case mediactl.Pause:
		if !p.Playing || p.Paused {
			return nil
		}
		return m.dispatch(app.PlayPause{})
	case mediactl.PlayPause:
		return m.dispatch(app.PlayPause{})
	case mediactl.Stop:
		return m.dispatch(app.StopPlayback{})
	case mediactl.Next:
		return m.dispatch(app.NextTrack{})
	case mediactl.Previous:
		return m.dispatch(app.PrevTrack{})
	case mediactl.SeekBy:
		return m.dispatch(app.SeekBy{D: c.Offset})
	case mediactl.SeekTo:
		return m.dispatch(app.SeekAt{Pos: c.Position})
	case mediactl.SetVolume:
		return m.dispatch(app.ChangeVolume{Set: true, To: c.Volume})
	case mediactl.SetLoop:
		r := map[mediactl.Loop]app.Repeat{mediactl.LoopNone: app.RepeatOff, mediactl.LoopPlaylist: app.RepeatAll, mediactl.LoopTrack: app.RepeatOne}[c.Loop]
		return m.dispatch(app.SetRepeat{Repeat: r})
	}
	return nil
}
