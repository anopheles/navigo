package app

import (
	"fmt"
	"slices"

	"github.com/anopheles/navigo/internal/library"
)

// Events that change playlists on the server. Rows are indexes in the
// playlist's tracks; To is exclusive. The UI asks for confirmation before
// removals and deletions: these events are the confirmed request.
type (
	PlaylistRemove struct {
		ID       library.PlaylistID
		From, To int
	}
	// PlaylistInsert adds tracks before row At; -1 adds them at the end.
	PlaylistInsert struct {
		ID     library.PlaylistID
		At     int
		Tracks []library.Track
	}
	PlaylistMove struct {
		ID           library.PlaylistID
		From, To, By int
	}
	PlaylistCreate struct {
		Name   string
		Tracks []library.Track
	}
	PlaylistDelete struct{ ID library.PlaylistID }
	// PlaylistWritten is the server's answer to a WritePlaylist.
	PlaylistWritten struct {
		Write WritePlaylist
		// NewID is the ID the server gave a created playlist.
		NewID       library.PlaylistID
		Err         error
		Unreachable bool
	}
)

// WriteKind is the server call a WritePlaylist needs.
type WriteKind int

const (
	// UpdatePlaylist adds Add to the end and removes rows Remove.
	UpdatePlaylist WriteKind = iota
	// ReplacePlaylist sets the playlist's tracks to Songs.
	ReplacePlaylist
	// CreatePlaylist makes a playlist Name of Songs.
	CreatePlaylist
	DeletePlaylist
)

// WritePlaylist changes a playlist on the server. After is the playlist as
// it will be, put into the library once the server agrees; Done is the
// message then.
type WritePlaylist struct {
	Kind   WriteKind
	ID     library.PlaylistID
	Name   string
	Add    []library.TrackID
	Remove []int
	Songs  []library.TrackID
	After  library.Playlist
	Done   Notice
}

func (PlaylistRemove) isEvent()  {}
func (PlaylistInsert) isEvent()  {}
func (PlaylistMove) isEvent()    {}
func (PlaylistCreate) isEvent()  {}
func (PlaylistDelete) isEvent()  {}
func (PlaylistWritten) isEvent() {}
func (WritePlaylist) isEffect()  {}

var offlineEdit = Notice{Text: "offline · playlists can be changed once the server is back", Err: true}

func (s State) applyPlaylist(e Event) (State, []Effect) {
	if w, ok := e.(PlaylistWritten); ok {
		return s.playlistWritten(w)
	}
	if s.Offline {
		return s, []Effect{offlineEdit}
	}
	if s.Library == nil {
		return s, nil
	}
	if c, ok := e.(PlaylistCreate); ok {
		ids := trackIDs(c.Tracks)
		return s, []Effect{WritePlaylist{
			Kind: CreatePlaylist, Name: c.Name, Songs: ids,
			After: library.Playlist{Name: c.Name, Owner: s.Library.User, Tracks: ids},
			Done:  Notice{Text: fmt.Sprintf("playlist %s made with %s", c.Name, count(len(ids)))},
		}}
	}

	var id library.PlaylistID
	switch e := e.(type) {
	case PlaylistRemove:
		id = e.ID
	case PlaylistInsert:
		id = e.ID
	case PlaylistMove:
		id = e.ID
	case PlaylistDelete:
		id = e.ID
	}
	p, ok := s.Library.Playlist(id)
	if !ok {
		return s, []Effect{Notice{Text: "that playlist is gone", Err: true}}
	}
	after := p
	w := WritePlaylist{ID: p.ID}

	switch e := e.(type) {
	case PlaylistRemove:
		from, to := max(e.From, 0), min(e.To, len(p.Tracks))
		if from >= to {
			return s, nil
		}
		s.Register = slices.Clone(s.Library.PlaylistTracks(p.ID)[from:to])
		after.Tracks = slices.Concat(p.Tracks[:from], p.Tracks[to:])
		w.Kind = UpdatePlaylist
		for i := from; i < to; i++ {
			w.Remove = append(w.Remove, i)
		}
		w.Done = Notice{Text: fmt.Sprintf("%s removed from %s", count(to-from), p.Name), Command: "paste-before", Hint: "puts them back"}
	case PlaylistInsert:
		if len(e.Tracks) == 0 {
			return s, nil
		}
		ids := trackIDs(e.Tracks)
		at := e.At
		if at < 0 || at > len(p.Tracks) {
			at = len(p.Tracks)
		}
		after.Tracks = slices.Insert(slices.Clone(p.Tracks), at, ids...)
		if at == len(p.Tracks) {
			w.Kind, w.Add = UpdatePlaylist, ids
		} else {
			w.Kind, w.Songs = ReplacePlaylist, after.Tracks
		}
		w.Done = Notice{Text: fmt.Sprintf("%s added to %s (%d in it)", count(len(ids)), p.Name, len(after.Tracks))}
	case PlaylistMove:
		from, to := max(e.From, 0), min(e.To, len(p.Tracks))
		by := min(max(e.By, -from), len(p.Tracks)-to)
		if from >= to || by == 0 {
			return s, nil
		}
		block := slices.Clone(p.Tracks[from:to])
		after.Tracks = slices.Insert(slices.Concat(p.Tracks[:from], p.Tracks[to:]), from+by, block...)
		w.Kind, w.Songs = ReplacePlaylist, after.Tracks
		dir := "down"
		if by < 0 {
			dir = "up"
		}
		w.Done = Notice{Text: fmt.Sprintf("moved %s %s in %s", count(to-from), dir, p.Name)}
	case PlaylistDelete:
		w.Kind = DeletePlaylist
		w.Done = Notice{Text: "playlist " + p.Name + " deleted"}
	}
	w.Name, w.After = p.Name, after
	return s, []Effect{w}
}

func (s State) playlistWritten(w PlaylistWritten) (State, []Effect) {
	if w.Err != nil {
		var out []Effect
		if w.Unreachable {
			s, out = s.applyOffline(Unreachable{Err: w.Err})
		}
		return s, append(out, Notice{Text: fmt.Sprintf("couldn't change %s: %v", w.Write.Name, w.Err), Err: true})
	}
	if s.Library == nil {
		return s, nil
	}
	if w.Write.Kind == DeletePlaylist {
		s.Library = s.Library.WithoutPlaylist(w.Write.ID)
	} else {
		after := w.Write.After
		if w.NewID != "" {
			after.ID = w.NewID
		}
		s.Library = s.Library.WithPlaylist(after)
	}
	return s, []Effect{SaveCache{Library: s.Library}, w.Write.Done}
}

func trackIDs(ts []library.Track) []library.TrackID {
	out := make([]library.TrackID, len(ts))
	for i, t := range ts {
		out[i] = t.ID
	}
	return out
}
