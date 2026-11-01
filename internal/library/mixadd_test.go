package library

import (
	"math/rand/v2"
	"testing"
)

func mixAdd(
	s *Snapshot,
	seed *Seed,
	exclude map[TrackID]bool,
	after, before *Track,
	i uint64,
) []Track {
	r := rand.New(rand.NewPCG(i, i))
	return s.MixAdd(seed, nil, MixAddSize, mixNow, r, nil, exclude, after, before)
}

func TestMixAddLeavesOutExcluded(t *testing.T) {
	s, seed, similar := radioLib()
	ex := map[TrackID]bool{"a1-b0-t0": true, "a2-b0-t0": true}
	for i := range uint64(100) {
		r := rand.New(rand.NewPCG(i, i))
		for _, tr := range s.MixAdd(&seed, similar, MixAddSize, mixNow, r, nil, ex, nil, nil) {
			if ex[tr.ID] {
				t.Fatalf("around a seed, picked excluded %s", tr.ID)
			}
		}
		for _, tr := range mixAdd(s, nil, ex, nil, nil, i) {
			if ex[tr.ID] {
				t.Fatalf("from the library, picked excluded %s", tr.ID)
			}
		}
	}
}

func TestMixAddMindsBothJoins(t *testing.T) {
	s, _, _ := radioLib()
	after, before := s.Tracks[0], s.Tracks[len(s.Tracks)-1]
	for i := range uint64(200) {
		out := mixAdd(s, nil, nil, &after, &before, i)
		if len(out) != MixAddSize {
			t.Fatalf("got %d tracks, want %d", len(out), MixAddSize)
		}
		if s.mixArtist(out[0]) == s.mixArtist(after) {
			t.Fatalf("seed %d: starts on the artist before it", i)
		}
		if s.mixArtist(out[len(out)-1]) == s.mixArtist(before) {
			t.Fatalf("seed %d: ends on the artist after it", i)
		}
		for j := 1; j < len(out); j++ {
			if s.mixArtist(out[j]) == s.mixArtist(out[j-1]) {
				t.Fatalf("seed %d: one artist twice in a row at %d", i, j)
			}
		}
	}
}

// The batch keeps its size when the last join can't be kept.
func TestMixAddGivesWayAtTheEnd(t *testing.T) {
	s, _, _ := radioLib()
	var only []Track
	for _, tr := range s.Tracks {
		if s.mixArtist(tr) == s.mixArtist(s.Tracks[0]) {
			only = append(only, tr)
		}
	}
	if got := s.endBefore(only[:1], nil, &only[1]); len(got) != 1 {
		t.Errorf("got %d tracks, want 1", len(got))
	}
}
