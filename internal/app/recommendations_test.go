package app

import (
	"testing"
	"time"

	"github.com/anopheles/navigo/internal/library"
	"github.com/anopheles/navigo/internal/listenbrainz"
)

const lbAPI = "https://api.listenbrainz.org"

func recsOn() State { return New().WithListenBrainz("ann-lb", lbAPI) }

// lbLibrary is t1…t3, each "Track n" by "Band".
func lbLibrary() *library.Snapshot {
	ts := tracks(3)
	for i := range ts {
		ts[i].Artist = "Band"
	}
	return library.Build("https://m.example", "ann", time.Time{}, nil, nil, ts, nil, nil)
}

// lbList recommends the titles, all by "Band".
func lbList(updated int64, titles ...string) listenbrainz.Stored {
	st := listenbrainz.Stored{URL: lbAPI, User: "ann-lb", LastUpdated: time.Unix(updated, 0).UTC()}
	for _, t := range titles {
		st.Recs = append(st.Recs, library.Recommendation{Title: t, Artists: []string{"Band"}})
	}
	return st
}

func fetched(o FetchOutcome, st listenbrainz.Stored) RecsFetched {
	return RecsFetched{User: "ann-lb", API: lbAPI, Outcome: o, Stored: st}
}

// startedWith is navigo started, logged in, with the cache and the stored
// list read, and the effects of the last step.
func startedWith(stored *listenbrainz.Stored) (State, []Effect) {
	s, _ := run(recsOn(), Start{}, LoggedIn{}, RecsLoaded{Stored: stored})
	return run(s, CacheLoaded{Library: lbLibrary()})
}

func TestRecsOffDoesNothing(t *testing.T) {
	s, out := run(New(), Start{}, LoggedIn{}, CacheLoaded{Library: lbLibrary()}, SyncNow{})
	if len(only[LoadRecs](out))+len(only[FetchRecs](out)) != 0 {
		t.Errorf("Recommendations off, but %#v", out)
	}
	if _, out := run(s, LoggedOut{}); len(only[DeleteRecs](out)) != 1 {
		t.Error(":logout didn't delete a stored list left from before")
	}
}

func TestStartupFetchesWhenDue(t *testing.T) {
	s, out := run(recsOn(), Start{})
	if len(only[LoadRecs](out)) != 1 {
		t.Fatalf("Start effects = %#v, want LoadRecs", out)
	}
	// The startup sync check comes before the stored list is read: the
	// fetch waits for it.
	s, out = run(s, LoggedIn{}, CacheLoaded{Library: lbLibrary()})
	if len(only[StartSync](out)) != 1 || len(only[FetchRecs](out)) != 0 {
		t.Fatalf("effects = %#v, want a sync and no fetch yet", out)
	}
	old := lbList(100, "Track 1")
	s, out = run(s, RecsLoaded{Stored: &old})
	if f := only[FetchRecs](out); len(f) != 1 || f[0].Force || f[0].Old == nil ||
		f[0].User != "ann-lb" || f[0].API != lbAPI {
		t.Fatalf("FetchRecs = %#v, want one, not forced, with the stored list", f)
	}
	if !s.Recommended("t1") || s.Recommended("t2") {
		t.Error("the stored list isn't matched to the library")
	}
	// Coming back online runs the check again, but not while one runs.
	s, _ = run(s, Unreachable{})
	if _, out = run(s, ProbeResult{Why: ProbeRetry}); len(only[FetchRecs](out)) != 0 {
		t.Errorf("a second fetch started while one runs: %#v", out)
	}
	s, _ = run(s, fetched(FetchNotDue, listenbrainz.Stored{}), Unreachable{})
	if _, out = run(s, ProbeResult{Why: ProbeRetry}); len(only[FetchRecs](out)) != 1 {
		t.Errorf("back online, effects = %#v, want a fetch", out)
	}
}

func TestStoredListForAnotherUserIsIgnored(t *testing.T) {
	other := lbList(100, "Track 1")
	other.User = "bob-lb"
	s, out := startedWith(&other)
	if s.Recommended("t1") {
		t.Error("another user's list counted")
	}
	if f := only[FetchRecs](out); len(f) != 1 || f[0].Old != nil {
		t.Errorf("FetchRecs = %#v, want one without the old list", f)
	}
}

func TestNewList(t *testing.T) {
	s, _ := startedWith(nil)
	st := lbList(200, "Track 2", "Track 3", "Elsewhere")
	st.FetchedAt = time.Unix(300, 0)
	s, out := run(s, fetched(FetchNew, st))
	if !s.Recommended("t2") || !s.Recommended("t3") || s.Recommended("t1") {
		t.Error("the new list isn't matched")
	}
	if sv := only[SaveRecs](out); len(sv) != 1 || len(sv[0].Stored.Recs) != 3 {
		t.Errorf("SaveRecs = %#v", sv)
	}
	if n := only[Notice](out); len(n) != 1 || !n[0].Quiet ||
		n[0].Text != "2 of 3 Recommendations are in the library" {
		t.Errorf("notice = %#v", n)
	}
	// A sync that brings a new library matches it again.
	ts := tracks(1)
	ts[0].Artist = "Band"
	s, _ = run(
		s,
		SyncNow{},
		Synced{
			Library: library.Build("https://m.example", "ann", time.Time{}, nil, nil, ts, nil, nil),
		},
	)
	if s.Recommended("t2") {
		t.Error("a track gone from the library is still matched")
	}
}

func TestUnchangedListKeepsQuiet(t *testing.T) {
	old := lbList(100, "Track 1")
	s, _ := startedWith(&old)
	st := old
	st.FetchedAt = time.Unix(500, 0)
	s, out := run(s, fetched(FetchUnchanged, st))
	if len(only[Notice](out)) != 0 || len(only[SaveRecs](out)) != 1 {
		t.Errorf("effects = %#v, want a save and no message", out)
	}
	if !s.Recommended("t1") {
		t.Error("the list was lost")
	}
}

func TestFetchFailures(t *testing.T) {
	old := lbList(1792692721, "Track 1") // 22 Oct 2026, 18:12 UTC
	for _, tc := range []struct {
		name   string
		stored *listenbrainz.Stored
		o      FetchOutcome
		keep   bool
		delete bool
		want   Notice
	}{
		{
			name: "unreachable with a list", stored: &old, o: FetchFailed, keep: true,
			want: Notice{
				Text:  "couldn't reach ListenBrainz · keeping the Recommendations from " + old.LastUpdated.Local().Format("2 Jan"),
				Quiet: true,
			},
		},
		{
			name: "unreachable without one", o: FetchFailed,
			want: Notice{Text: "couldn't reach ListenBrainz · no Recommendations yet", Quiet: true},
		},
		{
			name: "no such user", stored: &old, o: FetchNoUser, delete: true,
			want: Notice{Text: `ListenBrainz has no user "ann-lb" · check listenbrainz.user in config.yaml`, Err: true},
		},
		{
			name: "not generated", stored: &old, o: FetchNotGenerated, delete: true,
			want: Notice{Text: "no Recommendations from ListenBrainz for ann-lb yet · they can take a day or two after your first listens"},
		},
	} {
		s, _ := startedWith(tc.stored)
		s, out := run(s, fetched(tc.o, listenbrainz.Stored{}))
		if s.Recommended("t1") != tc.keep {
			t.Errorf("%s: Recommended = %v, want %v", tc.name, !tc.keep, tc.keep)
		}
		if got := len(only[DeleteRecs](out)) == 1; got != tc.delete {
			t.Errorf("%s: deleted %v, want %v", tc.name, got, tc.delete)
		}
		if n := only[Notice](out); len(n) != 1 || n[0] != tc.want {
			t.Errorf("%s: notice = %#v\nwant %#v", tc.name, n, tc.want)
		}
		if s.Offline {
			t.Errorf("%s: ListenBrainz failing made navigo offline", tc.name)
		}
	}
}

func TestSyncNowForcesAFetch(t *testing.T) {
	s, _ := startedWith(nil)
	s, _ = run(s, fetched(FetchNew, lbList(1, "Track 1")))
	_, out := run(s, SyncNow{})
	if f := only[FetchRecs](out); len(f) != 1 || !f[0].Force {
		t.Errorf("FetchRecs = %#v, want a forced one", f)
	}
	// Offline too: ListenBrainz doesn't need the server.
	s, _ = run(s, Unreachable{})
	if _, out = run(s, SyncNow{}); len(only[FetchRecs](out)) != 1 {
		t.Errorf("offline :sync effects = %#v, want a fetch", out)
	}
}

func TestReloadConfig(t *testing.T) {
	s, _ := startedWith(nil)
	s, _ = run(s, fetched(FetchNew, lbList(1, "Track 1")))

	// The same settings change nothing.
	if _, out := run(s, ConfigureListenBrainz{User: "ann-lb", API: lbAPI}); len(out) != 0 {
		t.Errorf("same settings: %#v", out)
	}
	// Another user fetches at once, and a late answer for the old one is
	// dropped.
	s2, out := run(s, ConfigureListenBrainz{User: "bob-lb", API: lbAPI})
	if f := only[FetchRecs](
		out,
	); len(f) != 1 || !f[0].Force || f[0].User != "bob-lb" ||
		f[0].Old != nil {
		t.Errorf("new user: FetchRecs = %#v", f)
	}
	if s2.Recommended("t1") {
		t.Error("the old user's list still counts")
	}
	if s3, _ := run(s2, fetched(FetchNew, lbList(2, "Track 1"))); s3.Recommended("t1") {
		t.Error("an answer for the old user was taken")
	}
	// Off deletes the list.
	s, out = run(s, ConfigureListenBrainz{})
	if len(only[DeleteRecs](out)) != 1 || s.Recommended("t1") {
		t.Errorf("off: effects %#v, still recommended %v", out, s.Recommended("t1"))
	}
	// Turned on again mid-run: fetched at once, no file to wait for.
	if _, out = run(
		s,
		ConfigureListenBrainz{User: "ann-lb", API: lbAPI},
	); len(
		only[FetchRecs](out),
	) != 1 {
		t.Errorf("on again: %#v, want a fetch", out)
	}
}

func TestLogoutDeletesTheList(t *testing.T) {
	s, _ := startedWith(nil)
	s, _ = run(s, fetched(FetchNew, lbList(1, "Track 1")))
	s, out := run(s, LoggedOut{})
	if len(only[DeleteRecs](out)) != 1 || s.Recommended("t1") {
		t.Errorf(":logout: %#v, still recommended %v", out, s.Recommended("t1"))
	}
}

func TestThousands(t *testing.T) {
	for n, want := range map[int]string{0: "0", 999: "999", 1000: "1,000", 1234567: "1,234,567"} {
		if got := Thousands(n); got != want {
			t.Errorf("Thousands(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestMixLeansTowardRecommendations(t *testing.T) {
	s, _ := startedWith(nil)
	s, _ = run(s, fetched(FetchNew, lbList(1, "Track 1")))
	first := 0
	const runs = 2000
	for seed := range uint64(runs) {
		m, _ := run(s, Mix{N: 1, Seed: seed, At: time.Unix(1, 0)})
		if len(m.Queue) == 1 && m.Queue[0].Track.ID == "t1" {
			first++
		}
	}
	// Three tracks by one artist: t1 weighs 3, the others 1 each.
	if got := float64(first) / runs; got < 0.55 || got > 0.65 {
		t.Errorf("t1 was picked %.3f of the time, want about 0.6", got)
	}
}
