package ui

import (
	"github.com/anopheles/navigo/internal/app"
	"github.com/anopheles/navigo/internal/library"
)

// homeLists are Home's rows, in the spec's order, with the server's
// album list type for each.
var homeLists = []struct{ title, kind, empty string }{
	{"Recently added", "newest", "nothing here yet"},
	{"Most played", "frequent", "nothing played yet"},
	{"Recently played", "recent", "nothing played yet"},
	{"Random", "random", "nothing here yet"},
	{"Starred", "starred", "nothing starred yet · * stars an album"},
}

// openHomeList opens one of Home's lists and asks the server for it
// again; until it answers, the last answer shows.
func (m *Model) openHomeList(i int) {
	l := homeLists[i]
	m.lib = append(m.lib, frame{kind: homeAlbums, homeKind: l.kind, title: l.title})
	m.event(app.LoadAlbumList{Kind: l.kind})
}

// homeAlbums is the albums one of Home's lists shows. An album the
// library has is taken from it, so stars and play counts match.
func (m *Model) homeAlbums(kind string) []library.Album {
	l := m.state.Home[kind]
	lib := m.state.Library
	if lib == nil {
		return l.Albums
	}
	out := make([]library.Album, len(l.Albums))
	for i, a := range l.Albums {
		if known, ok := lib.Album(a.ID); ok {
			a = known
		}
		out[i] = a
	}
	return out
}

// emptyHome says why a list of Home's has no albums.
func (m *Model) emptyHome(kind string) string {
	l := m.state.Home[kind]
	switch {
	case l.Loaded && !l.Loading:
		for _, h := range homeLists {
			if h.kind == kind {
				return " " + h.empty
			}
		}
	case m.state.Offline:
		return " can't reach the server · Home's lists need it; the library works from the cache"
	}
	return " loading…"
}
