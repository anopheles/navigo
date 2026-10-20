package app

import (
	"time"

	"github.com/anopheles/navigo/internal/library"
)

// NowPlayingInfo is what the OS's media controls show: the track, whether
// it plays, and what the controls can do.
type NowPlayingInfo struct {
	Track    library.Track // zero ID: nothing plays
	Playing  bool
	Paused   bool
	Position time.Duration
	Length   time.Duration
	CanNext  bool
	CanPrev  bool
	CanPlay  bool
	Volume   int
	Repeat   Repeat
}

// PublishNowPlaying tells the media controls what plays, whenever any of
// it changes.
type PublishNowPlaying struct{ NowPlayingInfo }

func (PublishNowPlaying) isEffect() {}

// Events from the media controls that keys don't send.
type (
	// SeekAt seeks to a position in the track that plays.
	SeekAt struct{ Pos time.Duration }
	// SetRepeat sets the repeat mode.
	SetRepeat struct{ Repeat Repeat }
)

func (SeekAt) isEvent()    {}
func (SetRepeat) isEvent() {}

func (s State) nowPlaying() NowPlayingInfo {
	np := NowPlayingInfo{
		Volume:  s.Player.Volume,
		Repeat:  s.Repeat,
		CanPlay: len(s.Queue) > 0 && s.cantPlay() == nil,
	}
	if t, row, ok := s.NowPlaying(); ok {
		np.Track, np.Playing, np.Paused = t, true, s.Player.Paused
		np.Position, np.Length = s.Player.Position, s.Player.Duration
		np.CanNext = row >= 0 && s.Repeat != RepeatOne && s.nextRow(row) >= 0 ||
			row >= 0 && s.Repeat == RepeatOne && row+1 < len(s.Queue)
		np.CanPrev = true // < goes back, or to the start of the track
	}
	return np
}

func (s State) applyMedia(e Event) (State, []Effect) {
	switch e := e.(type) {
	case SeekAt:
		if !s.Player.Playing {
			return s, nil
		}
		return s, []Effect{SeekTo(e)}
	case SetRepeat:
		s.Repeat = e.Repeat
		return s, append(s.neighbours(), Notice{Text: "repeat " + s.Repeat.String()})
	}
	return s, nil
}
