package ui

import (
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/anopheles/navigo/internal/app"
	"github.com/anopheles/navigo/internal/config"
	"github.com/anopheles/navigo/internal/library"
	"github.com/anopheles/navigo/internal/listenbrainz"
	"github.com/anopheles/navigo/internal/listenbrainz/listenbrainztest"
	"github.com/anopheles/navigo/internal/subsonic/subsonictest"
)

// lbHarness starts navigo with Recommendations on, against a fake
// ListenBrainz.
func lbHarness(t *testing.T, user string) (*harness, *listenbrainztest.Server) {
	t.Helper()
	lb := listenbrainztest.New(t)
	h := newHarness(t, 120, 40, func(c *config.Config, _ *subsonictest.Server) {
		c.ListenBrainz = config.ListenBrainz{User: user, URL: lb.URL}
	})
	return h, lb
}

// logged reports whether :messages holds a message containing text.
func (h *harness) logged(text string) bool {
	return slices.ContainsFunc(
		h.m.history,
		func(m message) bool { return strings.Contains(m.text, text) },
	)
}

func (h *harness) recommended(title string) bool {
	h.t.Helper()
	i := slices.IndexFunc(
		h.m.state.Library.Tracks,
		func(t library.Track) bool { return t.Title == title },
	)
	if i < 0 {
		h.t.Fatalf("no track %q in the fixture library", title)
	}
	return h.m.state.Recommended(h.m.state.Library.Tracks[i].ID)
}

func (h *harness) command(line string) {
	h.t.Helper()
	h.keys(":")
	h.typeText(line)
	h.keys("<CR>")
}

func TestRecommendationsFetchedAtStartup(t *testing.T) {
	h, lb := lbHarness(t, listenbrainztest.User)
	if n := len(lb.Requests()); n != 2 {
		t.Fatalf(
			"ListenBrainz requests at first start = %v, want the list and its names",
			lb.Requests(),
		)
	}
	if !h.recommended("Undertow") || h.recommended("Quiet Room") {
		t.Error("Undertow isn't the one Recommendation in the library")
	}
	// Only :messages says it.
	if !h.logged("1 of 3 Recommendations are in the library") {
		t.Errorf("messages = %v", h.m.history)
	}
	h.lacks("Recommendations are in the library")
	path := listenbrainz.Path(h.cache, h.srv.URL, subsonictest.Username)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("no stored list: %v", err)
	}

	// A second start within a day keeps the stored list without asking.
	h2 := h.restart()
	if n := len(lb.Requests()); n != 2 {
		t.Errorf("second start asked ListenBrainz: %v", lb.Requests())
	}
	if !h2.recommended("Undertow") {
		t.Error("the stored list wasn't used on the second start")
	}

	// :sync asks again; the list is unchanged, so one request does.
	h2.command("sync")
	if n := len(lb.Requests()); n != 3 {
		t.Errorf(":sync: requests %v, want one more", lb.Requests())
	}

	// ListenBrainz failing keeps the list and isn't offline.
	lb.FailNext(1)
	h2.command("sync")
	if !h2.logged("couldn't reach ListenBrainz · keeping the Recommendations from") ||
		h2.m.state.Offline || !h2.recommended("Undertow") {
		t.Errorf("after a failure: offline %v, messages %v", h2.m.state.Offline, h2.m.history)
	}

	// :logout deletes the stored list.
	h2.command("logout")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("stored list after :logout: %v", err)
	}
}

func TestRecommendationsForAnUnknownUser(t *testing.T) {
	h, _ := lbHarness(t, "nobody")
	h.contains(`ListenBrainz has no user "nobody" · check listenbrainz.user in config.yaml`)
}

func TestReloadConfigTurnsRecommendationsOnAndOff(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	lb := listenbrainztest.New(t)
	cfg := "server:\n  url: " + h.srv.URL + "\n  user: " + subsonictest.Username +
		"\nlistenbrainz:\n  user: ann\n  url: " + lb.URL + "\n"
	if err := os.MkdirAll(strings.TrimSuffix(h.path, "config.yaml"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(h.path, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	h.command("reload-config")
	if !h.recommended("Undertow") {
		t.Fatalf("turning them on didn't fetch: requests %v", lb.Requests())
	}
	if err := os.WriteFile(
		h.path,
		[]byte(strings.Split(cfg, "listenbrainz:")[0]),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	h.command("reload-config")
	if h.recommended("Undertow") {
		t.Error("turning them off kept the Recommendations")
	}
	if _, err := os.Stat(
		listenbrainz.Path(h.cache, h.srv.URL, subsonictest.Username),
	); !os.IsNotExist(
		err,
	) {
		t.Errorf("stored list after turning them off: %v", err)
	}
}

func TestInfoPopupNamesARecommendation(t *testing.T) {
	h, _ := lbHarness(t, listenbrainztest.User)
	h.keys("'s")
	h.typeText("undertow")
	h.keys("<CR>")
	if !strings.Contains(h.cursorRow(), "Undertow") {
		t.Fatalf("cursor on %q, want Undertow", h.cursorRow())
	}
	h.keys("i")
	h.contains("recommended by ListenBrainz")
	h.keys("q'b")
	h.keys("i") // an album: no such line
	h.contains("Tracks")
	h.lacks("recommended by ListenBrainz")
}

// homeRow puts the cursor on Home's row titled title.
func (h *harness) homeRow(title string) {
	h.t.Helper()
	h.keys("'h")
	for i, r := range h.m.homeRows() {
		if r.title == title {
			h.m.top().cur = i
			return
		}
	}
	h.t.Fatalf("Home has no %q row: %v", title, h.m.homeRows())
}

func TestRecommendedListOnHome(t *testing.T) {
	h, _ := lbHarness(t, listenbrainztest.User)
	h.homeRow("Recommended")
	h.keys("<CR>")
	h.contains("Recommended (1)", "Undertow")
	// <CR> plays the list from that row, as a playlist does.
	h.keys("<CR>")
	if q := h.m.state.Queue; len(q) != 1 || q[0].Track.Title != "Undertow" {
		t.Errorf("queue = %v, want the list", q)
	}
	h.keys("i")
	h.contains("recommended by ListenBrainz")
	h.keys("q")
	h.command("sort plays desc")
	h.contains("Recommended (1) (plays ↓)")
}

func TestRecommendedListOnlyWhenOn(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("'h")
	h.lacks("Recommended")
}

func TestRecommendedListWhenEmpty(t *testing.T) {
	h, _ := lbHarness(t, listenbrainztest.NewUser)
	h.homeRow("Recommended")
	h.keys("<CR>")
	h.contains("Recommended (0)", "ListenBrainz has no Recommendations for new yet")

	h2, _ := lbHarness(t, listenbrainztest.User)
	// A list that names nothing in the library.
	h2.m.state, _ = app.Apply(h2.m.state, app.RecsFetched{
		User:    listenbrainztest.User,
		API:     h2.m.deps.Config.ListenBrainz.API(),
		Outcome: app.FetchNew,
		Stored: listenbrainz.Stored{
			URL: h2.m.deps.Config.ListenBrainz.API(), User: listenbrainztest.User,
			Recs: []library.Recommendation{{Title: "Nothing", Artists: []string{"Nobody"}}},
		},
	})
	h2.homeRow("Recommended")
	h2.keys("<CR>")
	h2.contains("your one Recommendation isn't in the library")
}

func TestRecommendedListKeepsTheCursorOnANewList(t *testing.T) {
	h, _ := lbHarness(t, listenbrainztest.User)
	api := h.m.deps.Config.ListenBrainz.API()
	two := listenbrainz.Stored{
		URL:  api,
		User: listenbrainztest.User,
		Recs: []library.Recommendation{
			{Title: "Quiet Room", Artists: []string{"The Quiet Ones"}},
			{Title: "Undertow", Artists: []string{"The Quiet Ones"}},
		},
	}
	h.send(
		eventMsg{
			app.RecsFetched{
				User:    listenbrainztest.User,
				API:     api,
				Outcome: app.FetchNew,
				Stored:  two,
			},
		},
	)
	h.homeRow("Recommended")
	h.keys("<CR>j")
	if !strings.Contains(h.cursorRow(), "Undertow") {
		t.Fatalf("cursor on %q", h.cursorRow())
	}
	// Undertow moves to the top: the cursor goes with it.
	two.Recs[0], two.Recs[1] = two.Recs[1], two.Recs[0]
	two.LastUpdated = two.LastUpdated.Add(time.Hour)
	h.send(
		eventMsg{
			app.RecsFetched{
				User:    listenbrainztest.User,
				API:     api,
				Outcome: app.FetchNew,
				Stored:  two,
			},
		},
	)
	if !strings.Contains(h.cursorRow(), "Undertow") || h.m.top().cur != 0 {
		t.Errorf("after a new list the cursor is on row %d, %q", h.m.top().cur, h.cursorRow())
	}
}
