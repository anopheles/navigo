package library

import (
	"fmt"
	"math/rand/v2"
	"testing"
	"time"
)

func TestForgottenFavourites(t *testing.T) {
	long, recent := mixNow.AddDate(-1, 0, 0), mixNow.AddDate(0, -1, 0)
	ts := []Track{
		{ID: "starred-long", AlbumID: "x", ArtistID: "a1", Played: long},
		{ID: "starred-never", AlbumID: "x", ArtistID: "a2"},
		{ID: "starred-recent", AlbumID: "x", ArtistID: "a3", Played: recent},
		{ID: "five-long", AlbumID: "y", ArtistID: "a4", Rating: 5, Played: long},
		{ID: "four-never", AlbumID: "y", ArtistID: "a5", Rating: 4},
		{ID: "three-long", AlbumID: "y", ArtistID: "a6", Rating: 3, Played: long},
		{ID: "plain", AlbumID: "z", ArtistID: "a7"},
	}
	s := Build(
		"s",
		"u",
		mixNow,
		nil,
		nil,
		ts,
		nil,
		stars("starred-long", "starred-never", "starred-recent"),
	)
	got := map[TrackID]bool{}
	for _, tr := range s.ForgottenFavourites(50, mixNow, rand.New(rand.NewPCG(1, 1))) {
		got[tr.ID] = true
	}
	for _, id := range []TrackID{"starred-long", "starred-never", "five-long", "four-never"} {
		if !got[id] {
			t.Errorf("%s left out", id)
		}
	}
	for _, id := range []TrackID{"starred-recent", "three-long", "plain"} {
		if got[id] {
			t.Errorf("%s picked", id)
		}
	}
}

func TestForgottenFavouritesKeepVariety(t *testing.T) {
	var ts []Track
	var ids []string
	for i := range 40 {
		id := fmt.Sprintf("t%d", i)
		artist := ArtistID(fmt.Sprint("artist", i%2))
		ts = append(ts, Track{ID: TrackID(id), ArtistID: artist, AlbumID: AlbumID(fmt.Sprint(i))})
		ids = append(ids, id)
	}
	s := Build("s", "u", mixNow, nil, nil, ts, nil, stars(ids...))
	// Two artists, 20 favourites each: the per-artist limit holds.
	if got := s.ForgottenFavourites(
		50,
		mixNow,
		rand.New(rand.NewPCG(1, 1)),
	); len(
		got,
	) != 2*mixPerArtist {
		t.Errorf("%d tracks of two artists, want %d", len(got), 2*mixPerArtist)
	}
}

func TestNeverPlayed(t *testing.T) {
	var albums []Album
	var ts []Track
	add := func(id, artist string, played bool) {
		albums = append(
			albums,
			Album{ID: AlbumID(id), Name: id, ArtistID: ArtistID(artist), Artist: artist},
		)
		tr := Track{ID: TrackID(id + "-1"), AlbumID: AlbumID(id), ArtistID: ArtistID(artist)}
		ts = append(
			ts,
			tr,
			Track{ID: TrackID(id + "-2"), AlbumID: AlbumID(id), ArtistID: ArtistID(artist)},
		)
		if played {
			ts[len(ts)-2].PlayCount, ts[len(ts)-2].Played = 1, time.Now()
		}
	}
	add("fresh", "A", false)
	add("half", "B", true)
	for i := range 5 {
		add(fmt.Sprintf("box%d", i), "Box", false)
	}
	s := Build("s", "u", mixNow, nil, albums, ts, nil, nil)
	got := s.NeverPlayed(50, rand.New(rand.NewPCG(1, 1)))
	n := map[ArtistID]int{}
	for _, a := range got {
		n[a.ArtistID]++
		if a.ID == "half" {
			t.Error("an album with a played track counted")
		}
	}
	if n["A"] != 1 || n["Box"] != neverPlayedPerArtist {
		t.Errorf("per artist %v", n)
	}
}
