package library

import (
	"fmt"
	"math/rand/v2"
	"testing"
)

// radioLib has artists a0–a11, each with 2 albums of 5 tracks; the seed
// is two of a0's tracks, and a1–a11 are a0's similar artists.
func radioLib() (*Snapshot, Seed, [][]ArtistRef) {
	s := Build("s", "u", mixNow, nil, nil, mixLibrary(12, 2, 5), nil, nil)
	seed := Seed{Kind: SeedTracks, Tracks: []TrackID{"a0-b0-t0", "a0-b0-t1"}}
	var similar []ArtistRef
	for a := 1; a < 12; a++ {
		similar = append(similar, ArtistRef{ID: ArtistID(fmt.Sprintf("a%d", a))})
	}
	return s, seed, [][]ArtistRef{similar}
}

func radio(
	s *Snapshot,
	seed Seed,
	similar [][]ArtistRef,
	steer Steer,
	after *Track,
	i uint64,
) []Track {
	return s.RadioMix(
		seed,
		similar,
		RadioBatch,
		mixNow,
		rand.New(rand.NewPCG(i, i)),
		nil,
		steer,
		after,
	)
}

func TestRadioLeavesOutExcluded(t *testing.T) {
	s, seed, similar := radioLib()
	ex := map[TrackID]bool{"a0-b0-t0": true, "a0-b0-t1": true, "a1-b0-t0": true}
	for i := range uint64(200) {
		for _, tr := range radio(s, seed, similar, Steer{Exclude: ex}, nil, i) {
			if ex[tr.ID] {
				t.Fatalf("picked excluded %s", tr.ID)
			}
		}
	}
}

func TestRadioSteersByArtist(t *testing.T) {
	s, seed, similar := radioLib()
	share := func(steer Steer, artist ArtistID) float64 {
		n, all := 0, 0
		for i := range uint64(400) {
			for _, tr := range radio(s, seed, similar, steer, nil, i) {
				all++
				if tr.ArtistID == artist {
					n++
				}
			}
		}
		return float64(n) / float64(all)
	}
	plain := share(Steer{}, "a3")
	if less := share(Steer{Less: map[ArtistID]bool{"a3": true}}, "a3"); less != 0 {
		t.Errorf("a3's share %.3f with less: want none", less)
	}
	if more := share(Steer{More: map[ArtistID]bool{"a3": true}}, "a3"); more <= plain {
		t.Errorf("a3's share %.3f with more, %.3f without: want more", more, plain)
	}
}

func TestRadioDoesntRepeatTheLastArtist(t *testing.T) {
	s, seed, similar := radioLib()
	for i := range uint64(200) {
		for _, a := range []ArtistID{"a0", "a1", "a2", "a3"} {
			prev := Track{ID: "x", ArtistID: a}
			if got := radio(
				s,
				seed,
				similar,
				Steer{},
				&prev,
				i,
			); len(got) > 0 &&
				got[0].ArtistID == a {
				t.Fatalf("first pick by %s right after %s", got[0].ArtistID, a)
			}
		}
	}
}

func TestRadioNeedsASeed(t *testing.T) {
	s, _, similar := radioLib()
	if got := radio(s, Seed{Kind: SeedTracks}, similar, Steer{}, nil, 1); len(got) != 0 {
		t.Errorf("picked %d with no seed", len(got))
	}
}

func TestRadioRepeatsTheLastArtistRatherThanStop(t *testing.T) {
	s, seed, _ := radioLib()
	prev := Track{ID: "x", ArtistID: "a0"}
	// Without similar artists only a0 is close.
	if got := radio(s, seed, nil, Steer{}, &prev, 1); len(got) == 0 {
		t.Error("picked nothing when only the last track's artist is close")
	}
}
