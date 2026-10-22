package library

import (
	"fmt"
	"math/rand/v2"
	"testing"
)

// seedLibrary has artists a0…a9, each with three albums of six tracks.
// Artists a0–a4 play "Rock" and a5–a9 "Jazz"; a9 has no genre at all.
// a1's name differs from a0's only by case.
func seedLibrary() *Snapshot {
	var ts []Track
	for a := range 10 {
		name := fmt.Sprintf("Artist %d", a)
		if a == 1 {
			name = "ARTIST 0"
		}
		genre := "Rock"
		if a >= 5 {
			genre = "Jazz"
		}
		if a == 9 {
			genre = ""
		}
		id := ArtistID(fmt.Sprintf("a%d", a))
		for b := range 3 {
			for t := range 6 {
				ts = append(ts, Track{
					ID:       TrackID(fmt.Sprintf("a%d-b%d-t%d", a, b, t)),
					Title:    fmt.Sprintf("Track %d-%d-%d", a, b, t),
					Artist:   name,
					ArtistID: id,
					Artists:  []ArtistRef{{ID: id, Name: name}},
					AlbumID:  AlbumID(fmt.Sprintf("a%d-b%d", a, b)),
					Genre:    genre,
				})
			}
		}
	}
	pl := []Playlist{{ID: "p", Name: "Mixed", Tracks: []TrackID{
		"a5-b0-t0", "a6-b0-t0", "a7-b0-t0", "a8-b0-t0", "a5-b1-t0", "a6-b1-t0",
	}}}
	return Build("s", "u", mixNow.AddDate(-1, 0, 0), nil, nil, ts, pl, nil)
}

func mixAround(s *Snapshot, seed Seed, n int, r uint64) []Track {
	return s.MixAround(seed, n, mixNow, rand.New(rand.NewPCG(r, r)))
}

// checkAround checks the rules every Mix around a seed keeps: no track
// twice, never one artist (any spelling) twice in a row, at most a third
// from the seed, and at most 3 per artist and 2 per album for the rest.
func checkAround(t *testing.T, r uint64, got []Track, n int, fromSeed func(Track) bool, seedAlbums bool) {
	t.Helper()
	seen := map[TrackID]bool{}
	byArtist, byAlbum := map[string]int{}, map[AlbumID]int{}
	seedN := 0
	key := func(tr Track) string {
		if tr.ArtistID == "a1" {
			return "a0"
		}
		return string(tr.ArtistID)
	}
	for i, tr := range got {
		if seen[tr.ID] {
			t.Fatalf("seed %d: %s twice", r, tr.ID)
		}
		seen[tr.ID] = true
		if i > 0 && key(got[i-1]) == key(tr) {
			t.Fatalf("seed %d: %s twice in a row at %d", r, key(tr), i)
		}
		if fromSeed(tr) {
			seedN++
			if !seedAlbums {
				if byAlbum[tr.AlbumID]++; byAlbum[tr.AlbumID] > mixPerAlbum {
					t.Fatalf("seed %d: too many of album %s", r, tr.AlbumID)
				}
			}
			continue
		}
		byArtist[key(tr)]++
		byAlbum[tr.AlbumID]++
		if byArtist[key(tr)] > mixPerArtist || byAlbum[tr.AlbumID] > mixPerAlbum {
			t.Fatalf("seed %d: too many of %s or %s", r, key(tr), tr.AlbumID)
		}
	}
	if seedN > max(1, n/3) {
		t.Fatalf("seed %d: %d tracks from the seed, want at most %d", r, seedN, max(1, n/3))
	}
}

func TestMixAroundAnAlbum(t *testing.T) {
	s := seedLibrary()
	seed := Seed{Kind: SeedAlbum, ID: "a2-b0", Name: "Album"}
	fromSeed := func(tr Track) bool { return tr.ArtistID == "a2" } // the album's only artist
	for r := range uint64(100) {
		got := mixAround(s, seed, 15, r)
		if len(got) == 0 || got[0].AlbumID != "a2-b0" {
			t.Fatalf("seed %d: opens with %v, want a track of the album", r, got)
		}
		checkAround(t, r, got, 15, fromSeed, true)
		for _, tr := range got {
			if tr.Genre != "Rock" {
				t.Fatalf("seed %d: %s (%q) has nothing in common with the album", r, tr.ID, tr.Genre)
			}
		}
	}
}

func TestMixAroundATrackPlaysItFirst(t *testing.T) {
	s := seedLibrary()
	// Asked for, so it opens the Mix even rated one star and just played.
	i := s.trackByID["a6-b2-t3"]
	s.Tracks[i].Rating = 1
	s.Tracks[i].Played = mixNow
	seed := Seed{Kind: SeedTrack, ID: "a6-b2-t3", Name: "Track"}
	for r := range uint64(50) {
		got := mixAround(s, seed, 20, r)
		if len(got) == 0 || got[0].ID != "a6-b2-t3" {
			t.Fatalf("seed %d: opens with %v", r, got)
		}
		checkAround(t, r, got, 20, func(tr Track) bool { return tr.ArtistID == "a6" }, false)
	}
}

func TestMixAroundAnArtistTakesBothSpellings(t *testing.T) {
	s := seedLibrary()
	seed := Seed{Kind: SeedArtist, ID: "a0", Name: "Artist 0"}
	fromSeed := func(tr Track) bool { return tr.ArtistID == "a0" || tr.ArtistID == "a1" }
	sawOther := false
	for r := range uint64(100) {
		got := mixAround(s, seed, 30, r)
		if len(got) == 0 || !fromSeed(got[0]) {
			t.Fatalf("seed %d: opens with %v, want the artist", r, got)
		}
		checkAround(t, r, got, 30, fromSeed, false)
		for _, tr := range got {
			sawOther = sawOther || tr.ArtistID == "a1"
		}
	}
	if !sawOther {
		t.Error("the other spelling never came up; it's the same artist")
	}
}

func TestMixAroundAPlaylistOfManyArtists(t *testing.T) {
	s := seedLibrary()
	seed := Seed{Kind: SeedPlaylist, ID: "p", Name: "Mixed"}
	inList := map[TrackID]bool{}
	for _, id := range s.Playlists[0].Tracks {
		inList[id] = true
	}
	// No artist makes up half of it, so only its own tracks are from it.
	for r := range uint64(100) {
		got := mixAround(s, seed, 30, r)
		if len(got) == 0 || !inList[got[0].ID] {
			t.Fatalf("seed %d: opens with %v, want a track of the playlist", r, got)
		}
		checkAround(t, r, got, 30, func(tr Track) bool { return inList[tr.ID] }, false)
	}
}

func TestMixAroundNeverPads(t *testing.T) {
	s := seedLibrary()
	// a9 has no genre and nobody shares its artist: only its own tracks
	// are close, and the third from the seed is all it can take.
	got := mixAround(s, Seed{Kind: SeedArtist, ID: "a9", Name: "Artist 9"}, 50, 1)
	if len(got) != 1 {
		t.Errorf("got %d tracks, want just the opener (it can't sit next to itself)", len(got))
	}
	for _, tr := range got {
		if tr.ArtistID != "a9" {
			t.Errorf("padded with %s", tr.ID)
		}
	}
	if got := mixAround(s, Seed{Kind: SeedAlbum, ID: "nope", Name: "Gone"}, 50, 1); len(got) != 0 {
		t.Errorf("a seed not in the library gave %d tracks", len(got))
	}
}

func TestMixAroundLeavesOutOneStar(t *testing.T) {
	s := seedLibrary()
	for i := range s.Tracks {
		if s.Tracks[i].AlbumID == "a2-b0" && s.Tracks[i].Number != 99 {
			s.Tracks[i].Rating = 1
		}
	}
	for r := range uint64(30) {
		for _, tr := range mixAround(s, Seed{Kind: SeedAlbum, ID: "a2-b0", Name: "Album"}, 15, r) {
			if tr.AlbumID == "a2-b0" {
				t.Fatalf("seed %d: picked one-star %s", r, tr.ID)
			}
		}
	}
}

func TestCloseness(t *testing.T) {
	s := seedLibrary()
	c := s.closeness(Seed{Kind: SeedAlbum, ID: "a2-b0"})
	tr := func(id TrackID) Track { t, _ := s.Track(id); return t }
	for _, tc := range []struct {
		id   TrackID
		want float64
	}{
		{"a2-b0-t0", 3 + 2 + 1}, // in the seed, its artist, its genre
		{"a2-b1-t0", 2 + 1},     // the artist's other album
		{"a3-b0-t0", 1},         // the genre alone
		{"a6-b0-t0", 0},         // nothing in common
	} {
		if got := c.of(tr(tc.id)); got != tc.want {
			t.Errorf("%s: closeness %v, want %v", tc.id, got, tc.want)
		}
	}
	// A playlist: each artist's share is a fraction of it.
	c = s.closeness(Seed{Kind: SeedPlaylist, ID: "p"})
	if got, want := c.of(tr("a8-b2-t5")), 2*0.40824829046386296+1; !near(got, want) {
		t.Errorf("a8 is 1/6 of the playlist: closeness %v, want %v", got, want)
	}
}

func near(a, b float64) bool { return a-b < 1e-9 && b-a < 1e-9 }
