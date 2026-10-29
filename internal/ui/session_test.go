package ui

import (
	"strings"
	"testing"

	"github.com/anopheles/navigo/internal/creds"
	"github.com/anopheles/navigo/internal/subsonic/subsonictest"
)

func TestSortAlbums(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("3j") // on Blue Hour
	h.keys(":sort year<CR>")
	h.contains("Albums (year ↑)", "sorted by year")
	years := []string{"2015", "2017", "2018", "2019", "2020", "2021", "2022", "2023"}
	if got := h.order(years...); strings.Join(got, ",") != strings.Join(years, ",") {
		t.Errorf("years in order %q", got)
	}
	h.cursorOn("Blue Hour") // the cursor stays on its album
	h.keys(":sort year desc<CR>")
	h.contains("Albums (year ↓)")
	h.keys("gg")
	h.cursorOn("Night Drive Vol. 1")
	h.keys("<CR>") // opens what is shown
	h.contains("Albums (year ↓) › Night Drive Vol. 1")
	h.keys("h:sort<CR>")
	h.contains("back in the usual order")
	h.keys("gg")
	h.cursorOn("Ångström Days")

	h.keys(":sort bogus<CR>")
	h.contains(
		`sort: "bogus" isn't a field here; try name, artist, year, plays, tracks, length, genre`,
	)
}

func TestFilterAlbums(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys(":filter genre dream<CR>")
	h.contains("Albums (genre dream)", "2 shown")
	h.lacks("Static Bloom", "Ångström Days")
	h.keys(":filter year 2015-2019<CR>")
	h.contains("Albums (genre dream · years 2015–2019)", "1 shown")
	h.keys(":filter<CR>")
	h.contains("filter cleared", "Static Bloom", "Ångström Days")
	h.keys(":filter year 2017<CR>")
	h.contains("1 shown", "夜明け")
	h.keys(":filter year soon<CR>")
	h.contains(`filter: "soon" isn't a year or a range like 1990-1999`)
	h.keys(":filter mood calm<CR>")
	h.contains(`filter: "mood" isn't a filter; use genre or year`)
}

func TestSortTracksAndArtists(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("3j<CR>:sort length desc<CR>") // Blue Hour by length
	h.cursorOn("Dusk Signal")             // the cursor stays on its track
	h.keys("gg")
	h.cursorOn("Streetlamp") // the longest, 3:58
	h.keys("<CR>")           // plays in the order shown
	h.queueIs("Streetlamp, Low Tide, Dusk Signal, Afterglow, Wintering, Blue Hour")

	h.keys("'a:sort albums desc<CR>")
	h.keys("gg")
	h.cursorOn("Aurora Lane")
	h.keys(":filter genre rock<CR>")
	h.contains("artists have no genre or year")

	// Not every list sorts.
	h.keys("<Tab>:sort title<CR>")
	h.contains(
		"only Albums, Artists, an artist's albums, Playlists, Home's lists and an album's tracks can be sorted",
	)
}

func TestHintsCommand(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys(":hints off<CR>")
	h.lacks("<Tab> other window")
	h.contains("hint line off for this session")
	h.keys(":hints on<CR>")
	h.contains("<Tab> other window")
	h.keys(":hints<CR>")
	h.lacks("<Tab> other window")
	h.keys(":hints maybe<CR>")
	h.contains("hints: give on or off")
}

func TestLogout(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	// The password in the keyring, not the config.
	srv := h.m.deps.Config.Server
	h.m.deps.Config.Server.Password = ""
	h.kr[creds.Service+"/"+creds.Key(srv.URL, srv.User)] = subsonictest.Password
	h.keys("3j<CR><CR>")
	h.keys(":logout<CR>")
	h.contains(
		"Log in to your Navidrome or Subsonic server",
		"logged out · the password is forgotten",
	)
	if _, ok := h.kr[creds.Service+"/"+creds.Key(srv.URL, srv.User)]; ok {
		t.Error("the keyring still has the password")
	}
	if c := h.lastCall(); c != "Stop" {
		t.Errorf("player calls end with %q, want Stop", c)
	}
	// Logging in again starts a fresh session.
	h.typeText(subsonictest.Password)
	h.keys("<CR>")
	h.contains("Recently added", "The queue is empty.")
}

func TestLogoutWithPasswordInConfig(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys(":logout<CR>")
	h.contains("the password stays in")
}
