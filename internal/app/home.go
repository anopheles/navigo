package app

import (
	"fmt"
	"maps"

	"github.com/anopheles/navigo/internal/library"
)

// Home's album lists come from the server each time one opens, so
// Recently played is current; the last answer stays for when the server
// can't be reached. Kind is the server's list type, such as "newest".
type (
	LoadAlbumList   struct{ Kind string }
	AlbumListLoaded struct {
		Kind        string
		Albums      []library.Album
		Err         error
		Unreachable bool
	}
)

// FetchAlbumList asks the server for one of Home's lists.
type FetchAlbumList struct{ Kind string }

// AlbumList is one of Home's lists as last fetched.
type AlbumList struct {
	Albums  []library.Album
	Loaded  bool // fetched at least once
	Loading bool
}

func (LoadAlbumList) isEvent()   {}
func (AlbumListLoaded) isEvent() {}
func (FetchAlbumList) isEffect() {}

func (s State) applyHome(e Event) (State, []Effect) {
	switch e := e.(type) {
	case LoadAlbumList:
		if s.Offline {
			return s, nil // the last answer, if any, stays
		}
		l := s.Home[e.Kind]
		l.Loading = true
		s.Home = withList(s.Home, e.Kind, l)
		return s, []Effect{FetchAlbumList(e)}
	case AlbumListLoaded:
		l := s.Home[e.Kind]
		l.Loading = false
		if e.Err != nil {
			s.Home = withList(s.Home, e.Kind, l)
			var out []Effect
			if e.Unreachable {
				s, out = s.applyOffline(Unreachable{Err: e.Err})
			}
			return s, append(
				out,
				Notice{Text: fmt.Sprintf("couldn't load the list: %v", e.Err), Err: true},
			)
		}
		l.Albums, l.Loaded = e.Albums, true
		s.Home = withList(s.Home, e.Kind, l)
	}
	return s, nil
}

// withList returns a copy of home with one list replaced, so a State's
// map is never changed under it.
func withList(home map[string]AlbumList, kind string, l AlbumList) map[string]AlbumList {
	n := maps.Clone(home)
	if n == nil {
		n = map[string]AlbumList{}
	}
	n[kind] = l
	return n
}
