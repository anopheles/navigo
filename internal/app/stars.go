package app

import (
	"fmt"
	"slices"
)

// Starring: the UI says what to star or unstar; the server call is a
// WriteStar effect, and the library takes the change once it succeeds.
type (
	// SetStars stars (On) or unstars artists, albums and tracks by ID.
	// Name is what the message calls them, such as "Blue Hour".
	SetStars struct {
		On                      bool
		Artists, Albums, Tracks []string
		Name                    string
	}
	StarsWritten struct {
		Write       WriteStar
		Err         error
		Unreachable bool
	}
)

// WriteStar calls star or unstar.
type WriteStar struct {
	On                      bool
	Artists, Albums, Tracks []string
	Done                    Notice
}

func (SetStars) isEvent()     {}
func (StarsWritten) isEvent() {}
func (WriteStar) isEffect()   {}

func (s State) applyStars(e Event) (State, []Effect) {
	switch e := e.(type) {
	case SetStars:
		if len(e.Artists)+len(e.Albums)+len(e.Tracks) == 0 {
			return s, nil
		}
		if s.Library == nil {
			return s, []Effect{s.unsynced("can't star yet")}
		}
		if s.Offline {
			return s, []Effect{
				Notice{Text: "offline · stars can be changed once the server is back", Err: true},
			}
		}
		verb := "unstarred "
		if e.On {
			verb = "starred "
		}
		return s, []Effect{
			WriteStar{
				On:      e.On,
				Artists: e.Artists,
				Albums:  e.Albums,
				Tracks:  e.Tracks,
				Done:    Notice{Text: verb + e.Name},
			},
		}

	case StarsWritten:
		w := e.Write
		if e.Err != nil {
			var out []Effect
			if e.Unreachable {
				s, out = s.applyOffline(Unreachable{Err: e.Err})
			}
			return s, append(
				out,
				Notice{Text: fmt.Sprintf("couldn't change the stars: %v", e.Err), Err: true},
			)
		}
		if s.Library == nil {
			return s, nil
		}
		ids := slices.Concat(w.Artists, w.Albums, w.Tracks)
		starred := slices.DeleteFunc(
			slices.Clone(s.Library.Starred),
			func(id string) bool { return slices.Contains(ids, id) },
		)
		if w.On {
			starred = append(starred, ids...)
		}
		s.Library = s.Library.WithExtras(s.Library.Playlists, starred)
		return s, []Effect{SaveCache{Library: s.Library}, w.Done}
	}
	return s, nil
}
