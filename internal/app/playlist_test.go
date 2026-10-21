package app

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/anopheles/navigo/internal/library"
)

// withPlaylist is a state whose library has tracks t1..t4 and the
// playlist "Rock" of t1, t2, t3.
func withPlaylist() State {
	ts := tracks(4)
	lib := library.Build(
		"https://m.example",
		"ann",
		time.Time{},
		nil,
		nil,
		ts,
		[]library.Playlist{
			{ID: "pl", Name: "Rock", Tracks: []library.TrackID{"t1", "t2", "t3"}},
		},
		nil,
	)
	s := New()
	s.Library = lib
	return s
}

// written answers a write as the server would.
func written(s State, out []Effect, newID library.PlaylistID) (State, []Effect) {
	w := only[WritePlaylist](out)
	if len(w) != 1 {
		panic("no WritePlaylist")
	}
	return run(s, PlaylistWritten{Write: w[0], NewID: newID})
}

func rock(s State) []library.TrackID {
	p, _ := s.Library.Playlist("pl")
	return p.Tracks
}

func TestPlaylistRemove(t *testing.T) {
	s, out := run(withPlaylist(), PlaylistRemove{ID: "pl", From: 0, To: 2})
	w := only[WritePlaylist](out)[0]
	if w.Kind != UpdatePlaylist || !reflect.DeepEqual(w.Remove, []int{0, 1}) || w.ID != "pl" {
		t.Fatalf("write = %+v", w)
	}
	if len(rock(s)) != 3 {
		t.Error("the library changed before the server agreed")
	}
	if len(s.Register) != 2 || s.Register[0].ID != "t1" {
		t.Errorf("register = %v", s.Register)
	}
	s, out = written(s, out, "")
	if got := rock(s); !reflect.DeepEqual(got, []library.TrackID{"t3"}) {
		t.Errorf("after: %v", got)
	}
	if n := only[Notice](
		out,
	); n[0].Text != "2 tracks removed from Rock" ||
		n[0].Command != "paste-before" {
		t.Errorf("notice = %#v", n)
	}
	if len(only[SaveCache](out)) != 1 {
		t.Error("the cache wasn't saved")
	}
}

func TestPlaylistInsertMoveCreateDelete(t *testing.T) {
	s, out := run(withPlaylist(), PlaylistInsert{ID: "pl", At: -1, Tracks: tracks(4)[3:]})
	if w := only[WritePlaylist](out)[0]; w.Kind != UpdatePlaylist ||
		!reflect.DeepEqual(w.Add, []library.TrackID{"t4"}) {
		t.Errorf("append = %+v", w)
	}
	s, _ = written(s, out, "")

	// In the middle, or moving: the whole list is sent.
	s, out = run(s, PlaylistInsert{ID: "pl", At: 1, Tracks: tracks(1)})
	if w := only[WritePlaylist](out)[0]; w.Kind != ReplacePlaylist ||
		!reflect.DeepEqual(w.Songs, []library.TrackID{"t1", "t1", "t2", "t3", "t4"}) {
		t.Errorf("insert = %+v", w)
	}
	s, out = written(s, out, "")
	if n := only[Notice](out); n[0].Text != "1 track added to Rock (5 in it)" {
		t.Errorf("notice = %q", n[0].Text)
	}
	s, out = run(s, PlaylistMove{ID: "pl", From: 4, To: 5, By: -9})
	s, _ = written(s, out, "")
	if got := rock(s); !reflect.DeepEqual(got, []library.TrackID{"t4", "t1", "t1", "t2", "t3"}) {
		t.Errorf("after the move: %v", got)
	}

	s, out = run(s, PlaylistCreate{Name: "Jazz", Tracks: tracks(2)})
	if w := only[WritePlaylist](out)[0]; w.Kind != CreatePlaylist || w.Name != "Jazz" {
		t.Errorf("create = %+v", w)
	}
	s, _ = written(s, out, "new")
	if p, ok := s.Library.Playlist("new"); !ok || p.Name != "Jazz" || len(p.Tracks) != 2 {
		t.Errorf("created: %+v %v", p, ok)
	}

	s, out = run(s, PlaylistDelete{ID: "pl"})
	s, out = written(s, out, "")
	if _, ok := s.Library.Playlist("pl"); ok {
		t.Error("still there after deleting")
	}
	if n := only[Notice](out); n[0].Text != "playlist Rock deleted" {
		t.Errorf("notice = %q", n[0].Text)
	}
}

func TestPlaylistEditsOfflineAndFailing(t *testing.T) {
	s := withPlaylist()
	s.Offline = true
	if _, out := run(
		s,
		PlaylistRemove{ID: "pl", From: 0, To: 1},
	); len(only[WritePlaylist](out)) != 0 ||
		!only[Notice](out)[0].Err {
		t.Errorf("offline: %#v", out)
	}

	s, out := run(withPlaylist(), PlaylistRemove{ID: "pl", From: 0, To: 1})
	w := only[WritePlaylist](out)[0]
	s, out = run(s, PlaylistWritten{Write: w, Err: errors.New("timeout"), Unreachable: true})
	if !s.Offline || len(rock(s)) != 3 {
		t.Errorf("after an unreachable server: offline %v, playlist %v", s.Offline, rock(s))
	}
	if n := only[Notice](out); n[len(n)-1].Text != "couldn't change Rock: timeout" {
		t.Errorf("notice = %#v", n)
	}
}

func TestPlaylistRename(t *testing.T) {
	s, out := run(withPlaylist(), PlaylistRename{ID: "pl", Name: "Jazz"})
	w := only[WritePlaylist](out)[0]
	if w.Kind != RenamePlaylist || w.ID != "pl" || w.After.Name != "Jazz" {
		t.Fatalf("write = %+v", w)
	}
	if p, _ := s.Library.Playlist("pl"); p.Name != "Rock" {
		t.Error("renamed before the server agreed")
	}
	s, out = written(s, out, "")
	if p, _ := s.Library.Playlist("pl"); p.Name != "Jazz" || len(p.Tracks) != 3 {
		t.Errorf("after: %+v", p)
	}
	if n := only[Notice](out); n[0].Text != "playlist Rock renamed to Jazz" {
		t.Errorf("notice = %q", n[0].Text)
	}
}
