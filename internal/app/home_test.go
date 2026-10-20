package app

import (
	"errors"
	"testing"

	"github.com/anopheles/navigo/internal/library"
)

func TestHomeLists(t *testing.T) {
	s, out := run(New(), LoadAlbumList{Kind: "recent"})
	if f := only[FetchAlbumList](out); len(f) != 1 || f[0].Kind != "recent" || !s.Home["recent"].Loading {
		t.Fatalf("load: %#v, %+v", out, s.Home)
	}
	albums := []library.Album{{ID: "a1", Name: "One"}}
	s, _ = run(s, AlbumListLoaded{Kind: "recent", Albums: albums})
	if l := s.Home["recent"]; l.Loading || !l.Loaded || len(l.Albums) != 1 {
		t.Errorf("loaded: %+v", l)
	}

	// Offline nothing is asked, and the last answer stays.
	off := s
	off.Offline = true
	if off, out = run(off, LoadAlbumList{Kind: "recent"}); len(out) != 0 || len(off.Home["recent"].Albums) != 1 {
		t.Errorf("offline: %#v, %+v", out, off.Home)
	}

	// A failure keeps the old list too, and an unreachable server means offline.
	s0 := s
	s, _ = run(s, LoadAlbumList{Kind: "recent"})
	s, out = run(s, AlbumListLoaded{Kind: "recent", Err: errors.New("timeout"), Unreachable: true})
	if !s.Offline || len(s.Home["recent"].Albums) != 1 || s.Home["recent"].Loading {
		t.Errorf("after a failure: offline %v, %+v", s.Offline, s.Home["recent"])
	}
	if len(only[Notice](out)) == 0 {
		t.Error("no message after a failure")
	}
	if s0.Home["recent"].Loading {
		t.Error("Apply changed a State it was given")
	}
}
