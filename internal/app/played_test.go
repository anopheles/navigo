package app

import (
	"fmt"
	"slices"
	"testing"

	"github.com/anopheles/navigo/internal/library"
	"github.com/anopheles/navigo/internal/player"
)

func playedIDs(s State) []library.TrackID {
	var out []library.TrackID
	for _, t := range s.Played {
		out = append(out, t.ID)
	}
	return out
}

func TestPlayedNewestFirstOncePerTrack(t *testing.T) {
	s, _ := run(online(), PlayNow{Tracks: tracks(3)})
	if len(s.Played) != 0 {
		t.Fatalf("played %v before anything started", playedIDs(s))
	}
	for _, e := range []player.EntryID{1, 2, 1} {
		s, _ = run(s, FromPlayer{player.TrackStarted{Entry: e}})
	}
	if got, want := playedIDs(s), []library.TrackID{"t1", "t2"}; !slices.Equal(got, want) {
		t.Errorf("played %v, want %v", got, want)
	}
}

func TestPlayedOfflineToo(t *testing.T) {
	s, _ := run(online(), PlayNow{Tracks: tracks(2)})
	s.Offline = true
	s, _ = run(s, FromPlayer{player.TrackStarted{Entry: 2}})
	if got := playedIDs(s); !slices.Equal(got, []library.TrackID{"t2"}) {
		t.Errorf("played %v offline, want t2", got)
	}
}

func TestPlayedIsCapped(t *testing.T) {
	var list []library.Track
	for i := range maxPlayed + 10 {
		list = played(list, library.Track{ID: library.TrackID(fmt.Sprint(i))})
	}
	if len(list) != maxPlayed || list[0].ID != library.TrackID(fmt.Sprint(maxPlayed+9)) {
		t.Errorf("%d played, first %s", len(list), list[0].ID)
	}
}
