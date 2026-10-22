package library

import (
	"fmt"
	"math/rand/v2"
	"testing"
	"time"
)

var mixNow = time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

// mixLibrary has artists × albums × tracks, each album by its own artist.
func mixLibrary(artists, albums, tracks int) []Track {
	var out []Track
	for a := range artists {
		for b := range albums {
			for t := range tracks {
				out = append(out, Track{
					ID:       TrackID(fmt.Sprintf("a%d-b%d-t%d", a, b, t)),
					ArtistID: ArtistID(fmt.Sprintf("a%d", a)),
					AlbumID:  AlbumID(fmt.Sprintf("a%d-b%d", a, b)),
				})
			}
		}
	}
	return out
}

func mix(tracks []Track, starred []string, n int, seed uint64) []Track {
	s := Build("s", "u", time.Time{}, nil, nil, tracks, nil, starred)
	return s.Mix(n, mixNow, rand.New(rand.NewPCG(seed, seed)))
}

func TestMixVarietyRules(t *testing.T) {
	for seed := range uint64(200) {
		got := mix(mixLibrary(8, 4, 6), nil, MixSize, seed)
		// Near the end, the last artist with tracks left may have nothing
		// to sit between, so a couple can be left out.
		if len(got) < 8*mixPerArtist-2 {
			t.Fatalf("seed %d: %d tracks, want about every artist's 3", seed, len(got))
		}
		seen, byArtist, byAlbum := map[TrackID]bool{}, map[ArtistID]int{}, map[AlbumID]int{}
		for i, tr := range got {
			if seen[tr.ID] {
				t.Fatalf("seed %d: %s twice", seed, tr.ID)
			}
			seen[tr.ID] = true
			byArtist[tr.ArtistID]++
			byAlbum[tr.AlbumID]++
			if byArtist[tr.ArtistID] > mixPerArtist || byAlbum[tr.AlbumID] > mixPerAlbum {
				t.Fatalf("seed %d: too many of %s or %s", seed, tr.ArtistID, tr.AlbumID)
			}
			if i > 0 && got[i-1].ArtistID == tr.ArtistID {
				t.Fatalf("seed %d: %s twice in a row at %d", seed, tr.ArtistID, i)
			}
		}
	}
}

// Artists whose names differ only by case count as one for the variety
// rules, though Browse lists them apart.
func TestMixVarietyFoldsCase(t *testing.T) {
	tracks := mixLibrary(10, 2, 3)
	for i := range tracks {
		switch tracks[i].ArtistID {
		case "a0":
			tracks[i].Artists = []ArtistRef{{ID: "a0", Name: "ELETTRA STORM"}}
		case "a1":
			tracks[i].Artists = []ArtistRef{{ID: "a1", Name: "Elettra Storm"}}
		}
	}
	same := func(id ArtistID) bool { return id == "a0" || id == "a1" }
	for seed := range uint64(200) {
		got := mix(tracks, nil, MixSize, seed)
		n := 0
		for i, tr := range got {
			if !same(tr.ArtistID) {
				continue
			}
			n++
			if i > 0 && same(got[i-1].ArtistID) {
				t.Fatalf("seed %d: the two spellings in a row at %d", seed, i)
			}
		}
		if n > mixPerArtist {
			t.Fatalf("seed %d: %d tracks by the two spellings, want at most %d", seed, n, mixPerArtist)
		}
	}
}

func TestMixSizeAndShortfall(t *testing.T) {
	if got := mix(mixLibrary(30, 2, 2), nil, 10, 1); len(got) != 10 {
		t.Errorf("asked for 10, got %d", len(got))
	}
	// One artist: three tracks, which can't sit next to each other, so
	// only the first is left.
	if got := mix(mixLibrary(1, 5, 5), nil, MixSize, 1); len(got) != 1 {
		t.Errorf("one artist gave %d tracks, want 1", len(got))
	}
	if got := mix(nil, nil, MixSize, 1); len(got) != 0 {
		t.Errorf("an empty library gave %d tracks", len(got))
	}
}

func TestMixLeavesOutOneStar(t *testing.T) {
	tracks := mixLibrary(4, 1, 1)
	tracks[2].Rating = 1
	for seed := range uint64(50) {
		for _, tr := range mix(tracks, nil, MixSize, seed) {
			if tr.ID == tracks[2].ID {
				t.Fatalf("seed %d: picked a one-star track", seed)
			}
		}
	}
}

// TestMixWeights checks how often the first pick is track 0, out of two
// tracks by different artists, against its share of the two weights.
func TestMixWeights(t *testing.T) {
	cases := []struct {
		name    string
		change  func(*Track)
		starred bool
		share   float64
	}{
		{"starred", func(*Track) {}, true, 3.0 / 4},
		{"five stars", func(t *Track) { t.Rating = 5 }, false, 2.0 / 3},
		{"two stars", func(t *Track) { t.Rating = 2 }, false, 0.5 / 1.5},
		{
			"played yesterday",
			func(t *Track) { t.Played = mixNow.AddDate(0, 0, -1) },
			false,
			0.1 / 1.1,
		},
		{"played 90 days ago", func(t *Track) { t.Played = mixNow.AddDate(0, 0, -90) }, false, 0.5},
		{"played often", func(t *Track) { t.PlayCount = 500 }, false, 0.5},
	}
	const runs = 4000
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tracks := mixLibrary(2, 1, 1)
			c.change(&tracks[0])
			var starred []string
			if c.starred {
				starred = []string{string(tracks[0].ID)}
			}
			first := 0
			for seed := range uint64(runs) {
				if mix(tracks, starred, 1, seed)[0].ID == tracks[0].ID {
					first++
				}
			}
			if got := float64(first) / runs; got < c.share-0.03 || got > c.share+0.03 {
				t.Errorf("picked first %.3f of the time, want %.3f", got, c.share)
			}
		})
	}
}

func TestStaleness(t *testing.T) {
	day := func(d int) time.Time { return mixNow.AddDate(0, 0, -d) }
	for _, c := range []struct {
		played time.Time
		want   float64
	}{
		{time.Time{}, 1},
		{day(0), 0.1},
		{day(7), 0.1},
		{day(90), 1},
		{day(400), 1},
	} {
		if got := staleness(c.played, mixNow); got != c.want {
			t.Errorf("played %v: %v, want %v", c.played, got, c.want)
		}
	}
	if mid := staleness(day(48), mixNow); mid < 0.5 || mid > 0.6 {
		t.Errorf("48 days: %v, want about halfway", mid)
	}
}
