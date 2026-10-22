package listenbrainz_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/anopheles/navigo/internal/library"
	"github.com/anopheles/navigo/internal/listenbrainz"
	"github.com/anopheles/navigo/internal/listenbrainz/listenbrainztest"
)

var ctx = context.Background()

func TestFetchNamesTheList(t *testing.T) {
	srv := listenbrainztest.New(t)
	c := listenbrainz.New(srv.URL + "/")
	got, changed, err := c.Fetch(ctx, listenbrainztest.User, nil)
	if err != nil || !changed {
		t.Fatalf("Fetch = changed %v, %v; want a new list", changed, err)
	}
	if got.URL != srv.URL || got.User != listenbrainztest.User {
		t.Errorf("fetched for %q %q, want %q %q", got.URL, got.User, srv.URL, listenbrainztest.User)
	}
	if want := time.Unix(listenbrainztest.LastUpdated, 0); !got.LastUpdated.Equal(want) {
		t.Errorf("LastUpdated = %v, want %v", got.LastUpdated, want)
	}
	// The fourth recording is missing from the metadata, so it's dropped.
	want := []library.Recommendation{
		{
			MBID: "00000000-0000-0000-0000-000000000001", Title: "Hyperballad",
			Artists: []string{"Björk", "Björk"},
		},
		{
			MBID:  "00000000-0000-0000-0000-000000000002",
			Title: "Get Lucky (Radio Edit)",
			Artists: []string{
				"Daft Punk feat. Pharrell Williams",
				"Daft Punk",
				"Pharrell Williams",
			},
		},
		{
			MBID: "00000000-0000-0000-0000-000000000003", Title: "Undertow (2024 Remaster)",
			Artists: []string{"The Quiet Ones"},
		},
	}
	if !slices.EqualFunc(got.Recs, want, func(a, b library.Recommendation) bool {
		return a.MBID == b.MBID && a.Title == b.Title && slices.Equal(a.Artists, b.Artists)
	}) {
		t.Errorf("Recs = %+v\nwant %+v", got.Recs, want)
	}
	if n := len(srv.Requests()); n != 2 {
		t.Errorf("%d requests, want 2: %v", n, srv.Requests())
	}
}

func TestFetchStopsWhenTheListIsUnchanged(t *testing.T) {
	srv := listenbrainztest.New(t)
	c := listenbrainz.New(srv.URL)
	old := &listenbrainz.Stored{
		URL: srv.URL, User: listenbrainztest.User,
		LastUpdated: time.Unix(listenbrainztest.LastUpdated, 0),
		Recs:        []library.Recommendation{{Title: "kept"}},
	}
	got, changed, err := c.Fetch(ctx, listenbrainztest.User, old)
	if err != nil || changed {
		t.Fatalf("Fetch = changed %v, %v; want unchanged", changed, err)
	}
	if len(got.Recs) != 1 || got.Recs[0].Title != "kept" {
		t.Errorf("Recs = %+v, want the old ones", got.Recs)
	}
	if reqs := srv.Requests(); len(reqs) != 1 {
		t.Errorf("requests = %v, want only the list", reqs)
	}

	// Another user's list with the same date is no reason to stop.
	old.User = "someone else"
	if _, changed, _ := c.Fetch(ctx, listenbrainztest.User, old); !changed {
		t.Error("a list stored for another user counted as unchanged")
	}
}

func TestFetchErrors(t *testing.T) {
	srv := listenbrainztest.New(t)
	c := listenbrainz.New(srv.URL)
	if _, _, err := c.Fetch(
		ctx,
		listenbrainztest.NewUser,
		nil,
	); !errors.Is(
		err,
		listenbrainz.ErrNotGenerated,
	) {
		t.Errorf("204: err = %v, want ErrNotGenerated", err)
	}
	if _, _, err := c.Fetch(ctx, "nobody", nil); !errors.Is(err, listenbrainz.ErrNoUser) {
		t.Errorf("404: err = %v, want ErrNoUser", err)
	}
	srv.FailNext(1)
	_, _, err := c.Fetch(ctx, listenbrainztest.User, nil)
	if err == nil || errors.Is(err, listenbrainz.ErrNoUser) ||
		errors.Is(err, listenbrainz.ErrNotGenerated) {
		t.Errorf("500: err = %v, want a plain failure", err)
	}
}

func TestFetchRetriesARateLimitOnce(t *testing.T) {
	srv := listenbrainztest.New(t)
	c := listenbrainz.New(srv.URL)
	srv.RateLimitNext(1)
	if _, changed, err := c.Fetch(ctx, listenbrainztest.User, nil); err != nil || !changed {
		t.Fatalf("one 429: changed %v, %v; want the list after one retry", changed, err)
	}
	srv.RateLimitNext(2)
	_, _, err := c.Fetch(ctx, listenbrainztest.User, nil)
	var rl listenbrainz.RateLimited
	if !errors.As(err, &rl) {
		t.Errorf("two 429s: err = %v, want RateLimited", err)
	}
}

func TestStore(t *testing.T) {
	dir := t.TempDir()
	if _, err := listenbrainz.Load(
		dir,
		"https://music",
		"ann",
	); !errors.Is(
		err,
		listenbrainz.ErrNone,
	) {
		t.Fatalf("Load with no file: %v, want ErrNone", err)
	}
	s := listenbrainz.Stored{
		URL: listenbrainz.DefaultURL, User: "ann-lb",
		LastUpdated: time.Unix(100, 0).UTC(), FetchedAt: time.Unix(200, 0).UTC(),
		Recs: []library.Recommendation{{MBID: "m", Title: "T", Artists: []string{"A"}}},
	}
	if err := listenbrainz.Save(dir, "https://music", "ann", s); err != nil {
		t.Fatal(err)
	}
	got, err := listenbrainz.Load(dir, "https://music", "ann")
	if err != nil || got.User != "ann-lb" || !got.FetchedAt.Equal(s.FetchedAt) ||
		len(got.Recs) != 1 {
		t.Errorf("Load = %+v, %v; want what was saved", got, err)
	}
	if _, err := listenbrainz.Load(
		dir,
		"https://music",
		"bob",
	); !errors.Is(
		err,
		listenbrainz.ErrNone,
	) {
		t.Errorf("another Subsonic user read ann's list: %v", err)
	}
	if err := listenbrainz.Delete(dir, "https://music", "ann"); err != nil {
		t.Fatal(err)
	}
	if err := listenbrainz.Delete(dir, "https://music", "ann"); err != nil {
		t.Errorf("deleting twice: %v", err)
	}
	if _, err := listenbrainz.Load(
		dir,
		"https://music",
		"ann",
	); !errors.Is(
		err,
		listenbrainz.ErrNone,
	) {
		t.Errorf("Load after Delete: %v, want ErrNone", err)
	}
}
