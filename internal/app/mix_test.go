package app

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/anopheles/navigo/internal/library"
)

// mixable is n tracks, each by its own artist, so a Mix can take them all.
func mixable(n int) *library.Snapshot {
	ts := tracks(n)
	for i := range ts {
		ts[i].ArtistID = library.ArtistID(fmt.Sprintf("a%d", i))
		ts[i].AlbumID = library.AlbumID(fmt.Sprintf("b%d", i))
	}
	return library.Build("s", "u", time.Time{}, nil, nil, ts, nil, nil)
}

func TestMix(t *testing.T) {
	at := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	s, out := run(New(), CacheLoaded{Library: mixable(80)}, Mix{N: 50, Seed: 7, At: at})
	if len(s.Queue) != 50 || s.Current != 0 {
		t.Fatalf("queue %d current %d, want 50 playing from the first", len(s.Queue), s.Current)
	}
	if p := only[Play](out); len(p) != 1 {
		t.Errorf("plays = %#v", p)
	}
	if n := only[Notice](out); len(n) != 1 || n[0].Text != "mixed 50 tracks" || n[0].Command != "" {
		t.Errorf("notices = %#v", n)
	}
	first := s.Queue[0].Track.ID

	// The same seed and time give the same mix; a second one replaces
	// the queue and undo brings the first back.
	again, _ := run(New(), CacheLoaded{Library: mixable(80)}, Mix{N: 50, Seed: 7, At: at})
	if again.Queue[0].Track.ID != first {
		t.Error("the same seed made another mix")
	}
	s, out = run(s, Mix{N: 50, Seed: 8, At: at})
	if n := only[Notice](out); n[0].Command != "undo" {
		t.Errorf("second mix notice = %#v, want an undo hint", n[0])
	}
	if s, _ = run(s, Undo{}); s.Queue[0].Track.ID != first {
		t.Error("undo didn't bring the first mix back")
	}
}

func TestMixShortAndEmpty(t *testing.T) {
	s, out := run(New(), CacheLoaded{Library: mixable(5)}, Mix{N: 50, Seed: 1})
	if n := only[Notice](out); len(s.Queue) != 5 || n[0].Text != "mixed 5 of 50 tracks" {
		t.Errorf("queue %d, notices %#v", len(s.Queue), n)
	}

	s, out = run(New(), Mix{N: 50, Seed: 1})
	if n := only[Notice](out); len(s.Queue) != 0 || !n[0].Err {
		t.Errorf("no library: queue %d, notices %#v", len(s.Queue), n)
	}
	lib := mixable(2)
	for i := range lib.Tracks {
		lib.Tracks[i].Rating = 1
	}
	s, out = run(New(), CacheLoaded{Library: lib}, Mix{N: 50, Seed: 1})
	if n := only[Notice](
		out,
	); len(s.Queue) != 0 ||
		n[0].Text != "nothing to mix · every track is rated one star" {
		t.Errorf("all one star: queue %d, notices %#v", len(s.Queue), n)
	}
}

// syncing is a first run: no cache, logged in, the first sync running.
func syncing() State {
	s, _ := run(New(), CacheLoaded{}, LoggedIn{})
	return s
}

func TestMixWaitsForTheFirstSync(t *testing.T) {
	s, out := run(syncing(), Mix{N: 50, Seed: 7})
	if n := only[Notice](out); len(n) != 1 || n[0].Err ||
		n[0].Text != "the library is syncing · the mix starts once it's done" {
		t.Errorf("notices = %#v", n)
	}
	if !s.MixWaiting() || len(s.Queue) != 0 {
		t.Fatalf("waiting %v, queue %d", s.MixWaiting(), len(s.Queue))
	}
	s, out = run(s, Synced{Library: mixable(80)})
	if len(s.Queue) != 50 || len(only[Play](out)) != 1 || s.MixWaiting() {
		t.Errorf("after the sync: queue %d, plays %d, waiting %v",
			len(s.Queue), len(only[Play](out)), s.MixWaiting())
	}
	if n := only[Notice](out); len(n) != 1 || n[0].Text != "mixed 50 tracks" {
		t.Errorf("notices after the sync = %#v", n)
	}
}

func TestWaitingMixGivesWayToWhatPlays(t *testing.T) {
	s, _ := run(syncing(), Mix{N: 50, Seed: 7}, PlayNow{Tracks: tracks(3)})
	s, _ = run(s, Synced{Library: mixable(80)})
	if len(s.Queue) != 3 || s.MixWaiting() {
		t.Errorf("queue %d, waiting %v; the mix replaced what was played since",
			len(s.Queue), s.MixWaiting())
	}
}

func TestWaitingMixAndAFailedSync(t *testing.T) {
	s, _ := run(syncing(), Mix{N: 50, Seed: 7}, Synced{Err: errDown})
	if s.MixWaiting() || !s.Sync.Failed {
		t.Fatalf("waiting %v, failed %v", s.MixWaiting(), s.Sync.Failed)
	}
	_, out := run(s, Mix{N: 50, Seed: 7})
	want := Notice{
		Text: "nothing to mix · the library couldn't sync", Command: "sync",
		Hint: "tries again", Err: true,
	}
	if n := only[Notice](out); len(n) != 1 || n[0] != want {
		t.Errorf("notices = %#v", n)
	}
}

func TestWaitingMixIsDroppedOffline(t *testing.T) {
	s, _ := run(syncing(), Mix{N: 50, Seed: 7}, Unreachable{Err: errDown})
	if s.MixWaiting() {
		t.Error("a mix still waits for a sync that was abandoned")
	}
	_, out := run(s, Mix{N: 50, Seed: 7})
	if n := only[Notice](out); len(n) != 1 ||
		n[0].Text != "nothing to mix · offline, and the library hasn't synced yet" {
		t.Errorf("notices = %#v", n)
	}
}

func TestEditsBeforeTheFirstSyncSayWhy(t *testing.T) {
	for _, e := range []Event{
		SetStars{On: true, Tracks: []string{"t1"}, Name: "Track 1"},
		SetRating{Tracks: []library.TrackID{"t1"}, Rating: 4, Name: "Track 1"},
		PlaylistCreate{Name: "new"},
	} {
		_, out := run(syncing(), e)
		if n := only[Notice](out); len(n) != 1 || !n[0].Err ||
			!strings.HasSuffix(n[0].Text, " · the library is still syncing") {
			t.Errorf("%T: notices = %#v", e, n)
		}
	}
}

// genreLib is n artists with two albums of five tracks each, every track
// in one genre, so a Mix around any of them has company.
func genreLib(n int) *library.Snapshot {
	var ts []library.Track
	for a := range n {
		for b := range 2 {
			for i := range 5 {
				ts = append(ts, library.Track{
					ID:       library.TrackID(fmt.Sprintf("a%d-b%d-t%d", a, b, i)),
					Title:    fmt.Sprintf("Song %d-%d-%d", a, b, i),
					ArtistID: library.ArtistID(fmt.Sprintf("a%d", a)),
					AlbumID:  library.AlbumID(fmt.Sprintf("a%d-b%d", a, b)),
					Genre:    "Rock",
				})
			}
		}
	}
	return library.Build("s", "u", time.Time{}, nil, nil, ts, nil, nil)
}

func TestMixAroundASeed(t *testing.T) {
	seed := &library.Seed{Kind: library.SeedTrack, ID: "a3-b1-t2", Name: "Song 3-1-2"}
	s, out := answered(
		run(New(), CacheLoaded{Library: genreLib(30)}, Mix{N: 50, Seed: 7, Around: seed}),
	)
	if len(s.Queue) != 50 || s.Queue[0].Track.ID != "a3-b1-t2" {
		t.Fatalf(
			"queue %d, opening with %v; want 50 from the seed track",
			len(s.Queue),
			s.Queue[0].Track.ID,
		)
	}
	if n := only[Notice](out); len(n) != 1 || n[0].Text != "mixed 50 tracks from Song 3-1-2" {
		t.Errorf("notices = %#v", n)
	}

	small := &library.Seed{Kind: library.SeedTrack, ID: "a1-b0-t0", Name: "Song 1-0-0"}
	_, out = answered(
		run(New(), CacheLoaded{Library: genreLib(3)}, Mix{N: 50, Seed: 7, Around: small}),
	)
	if n := only[Notice](out); len(n) != 1 || !strings.HasPrefix(n[0].Text, "mixed ") ||
		!strings.HasSuffix(n[0].Text, " of 50 tracks from Song 1-0-0") {
		t.Errorf("too few: notices = %#v", n)
	}

	gone := &library.Seed{Kind: library.SeedAlbum, ID: "nope", Name: "Gone"}
	s, out = run(
		New(),
		CacheLoaded{Library: genreLib(3)},
		PlayNow{Tracks: tracks(2)},
		Mix{N: 50, Seed: 7, Around: gone},
	)
	if n := only[Notice](
		out,
	); len(s.Queue) != 2 || !n[len(n)-1].Err ||
		n[len(n)-1].Text != "nothing to mix from Gone" {
		t.Errorf("nothing close: queue %d, notices %#v", len(s.Queue), n)
	}
}

func TestMixAroundASeedWaitsForTheFirstSync(t *testing.T) {
	seed := &library.Seed{Kind: library.SeedTrack, ID: "a3-b1-t2", Name: "Song 3-1-2"}
	s, out := run(syncing(), Mix{N: 50, Seed: 7, Around: seed})
	if n := only[Notice](
		out,
	); len(n) != 1 ||
		n[0].Text != "mixing from Song 3-1-2 once the library has synced" {
		t.Errorf("notices = %#v", n)
	}
	s, out = answered(run(s, Synced{Library: genreLib(30)}))
	if len(s.Queue) != 50 || s.Queue[0].Track.ID != "a3-b1-t2" {
		t.Errorf("after the sync: queue %d", len(s.Queue))
	}
	if n := only[Notice](out); len(n) != 1 || n[0].Text != "mixed 50 tracks from Song 3-1-2" {
		t.Errorf("notices after the sync = %#v", n)
	}
	_, out = run(syncing(), Synced{Err: errDown}, Mix{N: 50, Seed: 7, Around: seed})
	if n := only[Notice](
		out,
	); n[len(n)-1].Text != "nothing to mix from Song 3-1-2 · the library couldn't sync" {
		t.Errorf("failed sync: notices = %#v", n)
	}
}

// answered answers a Mix's similar-artist lookup, if one was asked for,
// with empty lists, and returns what follows with the effects before it
// minus the lookup and its "mixing from" notice.
func answered(s State, out []Effect) (State, []Effect) {
	f := only[FetchSimilar](out)
	if len(f) == 0 {
		return s, out
	}
	s, more := run(
		s,
		SimilarLoaded{Seq: f[0].Seq, Lists: make([][]library.ArtistRef, len(f[0].Artists))},
	)
	return s, more
}

func TestMixAroundASeedAsksForSimilarArtists(t *testing.T) {
	lib := genreLib(30)
	seed := &library.Seed{Kind: library.SeedAlbum, ID: "a3-b1", Name: "Album 3"}
	s, out := run(
		New(),
		CacheLoaded{Library: lib},
		PlayNow{Tracks: tracks(2)},
		Mix{N: 30, Seed: 7, Around: seed},
	)
	f := only[FetchSimilar](out)
	if len(f) != 1 || len(f[0].Artists) != 1 || f[0].Artists[0] != "a3" {
		t.Fatalf("lookups = %#v, want one for a3", f)
	}
	if n := only[Notice](out); n[len(n)-1].Text != "mixing from Album 3…" {
		t.Errorf("notices = %#v", n)
	}
	if len(s.Queue) != 2 {
		t.Fatalf("the queue changed to %d before the lookup came back", len(s.Queue))
	}

	// Similar artists come in: a Mix with them and without the genre alone.
	s, out = run(
		s,
		SimilarLoaded{Seq: f[0].Seq, Lists: [][]library.ArtistRef{{{ID: "a9", Name: "a9"}}}},
	)
	if n := only[Notice](out); len(n) == 0 || !strings.HasSuffix(n[0].Text, "tracks from Album 3") {
		t.Errorf("notices = %#v", n)
	}
	for _, q := range s.Queue {
		if a := q.Track.ArtistID; a != "a3" && a != "a9" {
			t.Fatalf("%s by %s in a Mix around a3 with similar a9 only", q.Track.ID, a)
		}
	}

	// A late answer to an old lookup changes nothing.
	before := len(s.Queue)
	if s, _ = run(s, SimilarLoaded{Seq: f[0].Seq}); len(s.Queue) != before {
		t.Error("an old lookup's answer made another Mix")
	}
}

func TestPendingMixGivesWay(t *testing.T) {
	seed := &library.Seed{Kind: library.SeedAlbum, ID: "a3-b1", Name: "Album 3"}
	start := func() (State, int) {
		s, out := run(New(), CacheLoaded{Library: genreLib(30)}, Mix{N: 30, Seed: 7, Around: seed})
		return s, only[FetchSimilar](out)[0].Seq
	}

	// The queue changed meanwhile: the Mix is dropped.
	s, seq := start()
	s, _ = run(s, PlayNow{Tracks: tracks(3)}, SimilarLoaded{Seq: seq})
	if len(s.Queue) != 3 {
		t.Errorf("queue %d; the Mix replaced what was played since", len(s.Queue))
	}

	// A second gm replaces the first.
	s, seq = start()
	other := &library.Seed{Kind: library.SeedAlbum, ID: "a5-b0", Name: "Album 5"}
	s, out := run(s, Mix{N: 30, Seed: 8, Around: other})
	seq2 := only[FetchSimilar](out)[0].Seq
	s, _ = run(s, SimilarLoaded{Seq: seq})
	if len(s.Queue) != 0 {
		t.Fatal("the replaced Mix ran")
	}
	s, out = run(s, SimilarLoaded{Seq: seq2})
	if n := only[Notice](out); len(n) == 0 || !strings.HasSuffix(n[0].Text, "from Album 5") {
		t.Errorf("notices = %#v", n)
	}
}

func TestMixAroundASeedWithoutTheServer(t *testing.T) {
	seed := &library.Seed{Kind: library.SeedAlbum, ID: "a3-b1", Name: "Album 3"}
	// Offline: no lookup, and the Mix is made at once.
	s, _ := run(New(), CacheLoaded{Library: genreLib(30)}, Unreachable{Err: errDown})
	s, out := run(s, Mix{N: 30, Seed: 7, Around: seed})
	if len(only[FetchSimilar](out)) != 0 || len(s.Queue) == 0 {
		t.Errorf("offline: lookups %v, queue %d", only[FetchSimilar](out), len(s.Queue))
	}

	// A lookup that couldn't reach the server: offline, and the Mix is
	// still made from the cache, with no extra message about it.
	s, out = run(New(), CacheLoaded{Library: genreLib(30)}, Mix{N: 30, Seed: 7, Around: seed})
	s, out = run(s, SimilarLoaded{Seq: only[FetchSimilar](out)[0].Seq, Unreachable: true})
	if !s.Offline || len(s.Queue) == 0 {
		t.Errorf("unreachable: offline %v, queue %d", s.Offline, len(s.Queue))
	}
	n := only[Notice](out)
	if !strings.HasSuffix(n[0].Text, "tracks from Album 3") {
		t.Errorf("notices = %#v; the Mix's own message comes first", n)
	}
}
