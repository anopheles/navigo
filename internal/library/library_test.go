package library

import (
	"context"
	"errors"
	"os"
	"slices"
	"testing"

	"github.com/anopheles/navigo/internal/subsonic"
	"github.com/anopheles/navigo/internal/subsonic/subsonictest"
)

func client(t *testing.T) (*subsonic.Client, string) {
	t.Helper()
	srv := subsonictest.NewServer(t)
	c := subsonic.New(
		subsonic.Credentials{
			URL:      srv.URL,
			Username: subsonictest.Username,
			Password: subsonictest.Password,
		},
	)
	if err := c.Login(context.Background()); err != nil {
		t.Fatal(err)
	}
	return c, srv.URL
}

func synced(t *testing.T) (*Snapshot, *subsonic.Client, string) {
	t.Helper()
	c, url := client(t)
	ctx := context.Background()
	due, st, err := Due(ctx, c, nil, url, subsonictest.Username)
	if err != nil || !due {
		t.Fatalf("Due with no snapshot = %v, %v", due, err)
	}
	var calls [][2]int
	s, err := Sync(
		ctx,
		c,
		url,
		subsonictest.Username,
		st,
		func(done, total int) { calls = append(calls, [2]int{done, total}) },
	)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(calls, [][2]int{{500, 547}, {547, 547}}) {
		t.Errorf("progress = %v", calls)
	}
	return s, c, url
}

func artist(t *testing.T, s *Snapshot, name string) Artist {
	t.Helper()
	for _, a := range s.Artists {
		if a.Name == name {
			return a
		}
	}
	t.Fatalf("no artist %q", name)
	return Artist{}
}

func TestSync(t *testing.T) {
	s, _, _ := synced(t)
	if len(s.Tracks) != 547 || len(s.Albums) != 8 || len(s.Artists) != 7 {
		t.Fatalf("%d tracks, %d albums, %d artists", len(s.Tracks), len(s.Albums), len(s.Artists))
	}
	// Artists: by sort name, track-only artists included, own album counts.
	var names []string
	for _, a := range s.Artists {
		names = append(names, a.Name)
	}
	if want := []string{
		"Aurora Lane",
		"Long Haul Orchestra",
		"Pell Mell",
		"The Quiet Ones",
		"Various Artists",
		"Zoë Brandt",
		"森の音",
	}; !slices.Equal(
		names,
		want,
	) {
		t.Errorf("artists = %q, want %q", names, want)
	}
	for name, want := range map[string]int{"Aurora Lane": 2, "Pell Mell": 0, "Various Artists": 1, "The Quiet Ones": 2} {
		if got := artist(t, s, name).AlbumCount; got != want {
			t.Errorf("%s has %d own albums, want %d", name, got, want)
		}
	}
	// Albums by sort name; an album's tracks in disc and track order.
	var long Album
	for _, a := range s.Albums {
		if a.Name == "The Long Haul" {
			long = a
		}
	}
	ts := s.AlbumTracks(long.ID)
	if len(ts) != 510 || ts[0].Title != "Mile 001" || ts[255].Disc != 2 || ts[255].Number != 1 ||
		ts[509].Title != "Mile 510" {
		t.Errorf(
			"The Long Haul: %d tracks, first %q, 256th %d-%d, last %q",
			len(ts),
			ts[0].Title,
			ts[255].Disc,
			ts[255].Number,
			ts[509].Title,
		)
	}
	// Each credited artist is kept on the track.
	pell := artist(t, s, "Pell Mell")
	var credited int
	for _, tr := range s.Tracks {
		for _, a := range tr.Artists {
			if a.ID == pell.ID {
				credited++
			}
		}
	}
	if credited != 2 {
		t.Errorf("Pell Mell credited on %d tracks, want 2", credited)
	}
}

func TestArtistAlbums(t *testing.T) {
	s, _, _ := synced(t)
	names := func(as []Album) []string {
		var out []string
		for _, a := range as {
			out = append(out, a.Name)
		}
		return out
	}
	for _, c := range []struct {
		artist         string
		own, appearsOn []string
	}{
		{"Aurora Lane", []string{"Blue", "Blue Hour"}, []string{"Night Drive Vol. 1"}},
		{"Pell Mell", nil, []string{"Night Drive Vol. 1"}},
		{"Various Artists", []string{"Night Drive Vol. 1"}, nil},
		{"Zoë Brandt", []string{"Ångström Days"}, []string{"Night Drive Vol. 1"}}, // a guest on one track
	} {
		a := artist(t, s, c.artist)
		own, appears := s.ArtistAlbums(a.ID)
		if !slices.Equal(names(own), c.own) || !slices.Equal(names(appears), c.appearsOn) {
			t.Errorf(
				"%s: own %q, appears on %q; want %q, %q",
				c.artist,
				names(own),
				names(appears),
				c.own,
				c.appearsOn,
			)
		}
		if s.AppearsOn(a.ID) != len(c.appearsOn) {
			t.Errorf("%s appears on %d, want %d", c.artist, s.AppearsOn(a.ID), len(c.appearsOn))
		}
	}
}

func TestPlaylistsAndStars(t *testing.T) {
	s, _, _ := synced(t)
	var mix Playlist
	for _, p := range s.Playlists {
		if p.Name == "Morning Mix" {
			mix = p
		}
	}
	ts := s.PlaylistTracks(mix.ID)
	if len(ts) != 5 || ts[0].ID != ts[4].ID {
		t.Errorf("Morning Mix = %d tracks, want 5 with the first one twice", len(ts))
	}
	if !s.IsStarred(string(artist(t, s, "The Quiet Ones").ID)) ||
		s.IsStarred(string(artist(t, s, "Pell Mell").ID)) {
		t.Error("stars wrong for artists")
	}
	if len(s.Starred) != 4 {
		t.Errorf("%d starred, want 4", len(s.Starred))
	}
}

func TestDueAndRefresh(t *testing.T) {
	s, c, url := synced(t)
	ctx := context.Background()
	if due, _, err := Due(ctx, c, s, url, subsonictest.Username); err != nil || due {
		t.Errorf("Due right after a sync = %v, %v", due, err)
	}
	if due, _, _ := Due(ctx, c, s, url, "someone-else"); !due {
		t.Error("a snapshot for another user isn't due")
	}
	r, err := Refresh(ctx, c, s)
	if err != nil || len(r.Playlists) != 2 || len(r.Tracks) != len(s.Tracks) || r == s {
		t.Errorf(
			"Refresh = %d playlists, %d tracks, same=%v, %v",
			len(r.Playlists),
			len(r.Tracks),
			r == s,
			err,
		)
	}
}

func TestAlbumByAlbumFallback(t *testing.T) {
	c, _ := client(t)
	artists, albums, tracks, err := albumByAlbum(context.Background(), c, 547, func(int, int) {})
	if err != nil {
		t.Fatal(err)
	}
	if len(albums) != 8 || len(tracks) != 547 || len(artists) != 6 {
		t.Errorf("%d albums, %d tracks, %d artists", len(albums), len(tracks), len(artists))
	}
}

func TestCache(t *testing.T) {
	s, _, url := synced(t)
	dir := t.TempDir()
	if _, err := Load(dir, url, subsonictest.Username); !errors.Is(err, ErrNoCache) {
		t.Errorf("Load with no file = %v", err)
	}
	if err := Save(dir, s); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir, url, subsonictest.Username)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Tracks) != 547 || !got.LastScan.Equal(s.LastScan) ||
		len(got.AlbumTracks(s.Albums[0].ID)) == 0 ||
		!got.IsStarred(s.Starred[0]) {
		t.Error("the loaded snapshot differs or has no indexes")
	}
	if _, err := Load(dir, url, "someone-else"); !errors.Is(err, ErrNoCache) {
		t.Errorf("another user's cache = %v", err)
	}
	path := CachePath(dir, url, subsonictest.Username)
	if err := os.WriteFile(path, []byte("not gob"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir, url, subsonictest.Username); !errors.Is(err, ErrNoCache) {
		t.Errorf("damaged cache = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("damaged cache file was kept")
	}
}

func TestNext(t *testing.T) {
	n := next(
		subsonic.SearchCounts{ArtistCount: 500, AlbumCount: 500, SongCount: 500},
		500,
		30,
		500,
	)
	want := subsonic.SearchCounts{
		ArtistCount:  500,
		ArtistOffset: 500,
		SongCount:    500,
		SongOffset:   500,
	}
	if n != want {
		t.Errorf("next = %+v, want %+v", n, want)
	}
}
