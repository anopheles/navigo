package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"
)

func TestStartsOnHome(t *testing.T) {
	h := homeHarness(t, 120, 40, nil)
	h.contains(" Home ", "Recently added", "Most played", "Recently played", "Random", "Starred", "<CR> open")
	if got := h.order("Recently added", "Most played", "Recently played", "Random", "Starred"); strings.Join(got, ",") != "Recently added,Most played,Recently played,Random,Starred" {
		t.Errorf("Home's lists in order %q", got)
	}
	golden.RequireEqual(t, h.screen())
}

func TestHomeLists(t *testing.T) {
	h := homeHarness(t, 120, 40, nil)
	for _, c := range []struct {
		keys, title, kind string
		want              []string
	}{
		{"<CR>", "Recently added", "newest", []string{"Night Drive Vol. 1", "The Long Haul", "夜明け"}},
		{"j<CR>", "Most played", "frequent", []string{"Blue Hour", "Static Bloom"}},
		{"jj<CR>", "Recently played", "recent", []string{"Static Bloom", "Blue Hour"}},
		{"3j<CR>", "Random", "random", []string{"Static Bloom", "Blue Hour", "The Long Haul"}},
		{"4j<CR>", "Starred", "starred", []string{"★ Blue Hour"}},
	} {
		h.keys("'h'hgg" + c.keys)
		h.contains("Home › " + c.title)
		if got := h.order(c.want...); strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("%s shows %q, want %q in that order", c.title, got, c.want)
		}
		if n := h.requests("getAlbumList2"); n == 0 {
			t.Errorf("%s: no getAlbumList2 request", c.title)
		}
	}
	var kinds []string
	for _, r := range h.srv.Requests() {
		if r.Endpoint == "getAlbumList2" && r.Params.Get("size") == "50" {
			kinds = append(kinds, r.Params.Get("type"))
		}
	}
	if strings.Join(kinds, ",") != "newest,frequent,recent,random,starred" {
		t.Errorf("requests for %q", kinds)
	}

	// An album opens as anywhere else; ga and the rest work on its rows.
	h.keys("'hggj<CR><CR>")
	h.contains("Home › Most played › Blue Hour", "Dusk Signal")
}

func TestHomeOffline(t *testing.T) {
	h := homeHarness(t, 120, 40, nil)
	h.keys("j<CR>") // Most played, while online
	h.contains("Blue Hour")
	h.srv.SetDown(true)
	h.keys(":sync<CR>")
	h.contains("offline")
	// A list never fetched says why it's empty; the last answer stays.
	h.keys("hgg<CR>")
	h.contains("can't reach the server · Home's lists need it")
	h.keys("hj<CR>")
	h.contains("Home › Most played", "Blue Hour")
}
