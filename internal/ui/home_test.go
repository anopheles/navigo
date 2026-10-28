package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"
)

func TestStartsOnHome(t *testing.T) {
	h := homeHarness(t, 120, 40, nil)
	h.contains(
		" Home ",
		"Mix",
		"Recently added",
		"Most played",
		"Recently played",
		"Played this session",
		"Random albums",
		"Starred albums",
		"Starred tracks",
		"<CR> open",
	)
	if got := h.order(
		"Mix",
		"Recently added",
		"Most played",
		"Recently played",
		"Played this session",
		"Random albums",
		"Starred albums",
		"Starred tracks",
	); strings.Join(
		got,
		",",
	) != "Mix,Recently added,Most played,Recently played,Played this session,Random albums,Starred albums,Starred tracks" {
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
		{"j<CR>", "Recently added", "newest", []string{"Night Drive Vol. 1", "The Long Haul", "夜明け"}},
		{"2j<CR>", "Most played", "frequent", []string{"Blue Hour", "Static Bloom"}},
		{"3j<CR>", "Recently played", "recent", []string{"Static Bloom", "Blue Hour"}},
		{"5j<CR>", "Random albums", "random", []string{"Static Bloom", "Blue Hour", "The Long Haul"}},
		{"6j<CR>", "Starred albums", "starred", []string{"★ Blue Hour"}},
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
	h.keys("'hgg2j<CR><CR>")
	h.contains("Home › Most played › Blue Hour", "Dusk Signal")
}

func TestHomeOffline(t *testing.T) {
	h := homeHarness(t, 120, 40, nil)
	h.keys("2j<CR>") // Most played, while online
	h.contains("Blue Hour")
	h.srv.SetDown(true)
	h.keys(":sync<CR>")
	h.contains("offline")
	// A list never fetched says why it's empty; the last answer stays.
	h.keys("hggj<CR>")
	h.contains("can't reach the server · Home's lists need it")
	h.keys("hgg2j<CR>")
	h.contains("Home › Most played", "Blue Hour")
}

func TestHomeMix(t *testing.T) {
	h := homeHarness(t, 120, 40, nil)
	h.contains("<CR> mix")
	h.keys("j")
	h.contains("<CR> open")
	h.keys("k<CR>") // Mix, the first row
	h.contains(" Home ", "mixed ")
	if h.m.top().kind != homeList {
		t.Error("Mix left Home")
	}
	if n := len(h.m.state.Queue); n == 0 || n > 50 {
		t.Errorf("queue has %d entries after a mix", n)
	}
	if n := h.requests("getAlbumList2"); n != 0 {
		t.Errorf("Mix asked the server for %d album lists", n)
	}

	// A count sets the size, and u undoes it.
	first := h.m.state.Queue
	h.keys("'a'hgg3<CR>")
	h.contains("mixed 3 tracks · u brings the old queue back")
	if len(h.m.state.Queue) != 3 {
		t.Errorf("3gm queued %d", len(h.m.state.Queue))
	}
	h.keys("u")
	if len(h.m.state.Queue) != len(first) {
		t.Errorf("undo left %d entries, want %d", len(h.m.state.Queue), len(first))
	}

	// The pick comes from the cache, so it works offline too.
	h.srv.SetDown(true)
	h.keys(":sync<CR>")
	h.contains("offline")
	h.keys(":mix<CR>")
	h.contains("mixed ")
}
