package app

import (
	"fmt"
	"strings"

	"github.com/anopheles/navigo/internal/library"
)

// Rating: the UI says which tracks get which rating; the server call is a
// WriteRating effect, and the library takes the change once it succeeds.
type (
	// SetRating rates tracks 1–5 stars, or clears their rating with 0.
	// Name is what the message calls them, such as "Low Tide".
	SetRating struct {
		Tracks []library.TrackID
		Rating int
		Name   string
	}
	RatingWritten struct {
		Write       WriteRating
		Err         error
		Unreachable bool
	}
)

// WriteRating calls setRating for each track.
type WriteRating struct {
	Tracks []library.TrackID
	Rating int
	Done   Notice
}

func (SetRating) isEvent()     {}
func (RatingWritten) isEvent() {}
func (WriteRating) isEffect()  {}

func (s State) applyRating(e Event) (State, []Effect) {
	switch e := e.(type) {
	case SetRating:
		if len(e.Tracks) == 0 || e.Rating < 0 || e.Rating > 5 {
			return s, nil
		}
		if s.Library == nil {
			return s, []Effect{s.Unsynced("can't rate yet")}
		}
		if s.Offline {
			return s, []Effect{
				Notice{Text: "offline · ratings can be changed once the server is back", Err: true},
			}
		}
		text := "rating cleared for " + e.Name
		if e.Rating > 0 {
			text = "rated " + e.Name + " " + strings.Repeat("★", e.Rating)
		}
		return s, []Effect{
			WriteRating{Tracks: e.Tracks, Rating: e.Rating, Done: Notice{Text: text}},
		}

	case RatingWritten:
		if e.Err != nil {
			var out []Effect
			if e.Unreachable {
				s, out = s.applyOffline(Unreachable{Err: e.Err})
			}
			return s, append(
				out,
				Notice{Text: fmt.Sprintf("couldn't change the rating: %v", e.Err), Err: true},
			)
		}
		if s.Library == nil {
			return s, nil
		}
		s.Library = s.Library.WithRating(e.Write.Tracks, e.Write.Rating)
		return s, []Effect{SaveCache{Library: s.Library}, e.Write.Done}
	}
	return s, nil
}
