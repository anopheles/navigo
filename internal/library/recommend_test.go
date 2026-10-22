package library

import (
	"maps"
	"slices"
	"testing"
	"time"
)

func TestRecommended(t *testing.T) {
	track := func(id, title, artist string, credited ...string) Track {
		tr := Track{ID: TrackID(id), Title: title, Artist: artist}
		for _, c := range credited {
			tr.Artists = append(tr.Artists, ArtistRef{Name: c})
		}
		return tr
	}
	s := Build("s", "u", time.Time{}, nil, nil, []Track{
		track("plain", "Hyperballad", "Björk"),
		track("copy", "Hyperballad", "Björk"), // the same song on a compilation
		track("remaster", "Heroes (2017 Remaster)", "David Bowie"),
		track("live", "Wonderwall - Live at Knebworth", "Oasis"),
		track("credited", "Get Lucky", "Daft Punk feat. Pharrell Williams",
			"Daft Punk", "Pharrell Williams"),
		track("and", "Up & Up", "Coldplay"),
		track("cover", "Hurt", "Johnny Cash"),
		track("brackets", "(Intro)", "Someone"),
	}, nil, nil)

	got, firsts := s.Recommended([]Recommendation{
		{Title: "hyperballad", Artists: []string{"BJORK"}},
		{Title: "Heroes", Artists: []string{"David Bowie"}},
		{Title: "Wonderwall (Remastered)", Artists: []string{"Oasis"}},
		{Title: "Get Lucky", Artists: []string{"Daft Punk & Pharrell", "Pharrell Williams"}},
		{Title: "Up and Up", Artists: []string{"Coldplay"}},
		{Title: "Hurt", Artists: []string{"Nine Inch Nails"}}, // a title alone never matches
		// All-bracket titles aren't emptied to match each other.
		{Title: "(Outro)", Artists: []string{"Someone"}},
		{Title: "Not in the library", Artists: []string{"Nobody"}},
	})
	want := []TrackID{"and", "copy", "credited", "live", "plain", "remaster"}
	if keys := slices.Sorted(maps.Keys(got)); !slices.Equal(keys, want) {
		t.Errorf("matched tracks = %v, want %v", keys, want)
	}
	// One per Recommendation, in their order, the first copy in the library.
	var ids []TrackID
	for _, t := range firsts {
		ids = append(ids, t.ID)
	}
	if want := []TrackID{"plain", "remaster", "live", "credited", "and"}; !slices.Equal(ids, want) {
		t.Errorf("firsts = %v, want %v", ids, want)
	}
}

func TestRecommendedNone(t *testing.T) {
	s := Build(
		"s",
		"u",
		time.Time{},
		nil,
		nil,
		[]Track{{ID: "a", Title: "A", Artist: "B"}},
		nil,
		nil,
	)
	if got, firsts := s.Recommended(nil); len(got) != 0 || len(firsts) != 0 {
		t.Errorf("Recommended(nil) = %v, %v; want nothing", got, firsts)
	}
}
