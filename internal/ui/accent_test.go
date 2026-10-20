package ui

import (
	"image/color"
	"testing"

	"github.com/anopheles/navigo/internal/config"
	"github.com/anopheles/navigo/internal/cover"
	"github.com/anopheles/navigo/internal/subsonic/subsonictest"
)

// accentOf is the colour the accent has now.
func accentOf(t *testing.T) string {
	t.Helper()
	if len(accentStops) == 0 {
		return "none"
	}
	r, g, b, _ := accentStops[0].RGBA()
	return string([]byte{byte(r >> 8), byte(g >> 8), byte(b >> 8)})
}

func rgbOf(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return string([]byte{byte(r >> 8), byte(g >> 8), byte(b >> 8)})
}

// coverAccentHarness plays Low Tide on a true-colour terminal with cover art.
func coverAccentHarness(t *testing.T, colors config.Colors) *harness {
	t.Helper()
	h := setupHarness(t, 120, 40, withColors(colors), func(hs *harness) {
		hs.m.deps.TrueColor, hs.m.deps.CoverArtOK = true, true
		hs.m.deps.CoverCache = cover.OpenCache(t.TempDir(), 1<<20)
		applyColors(hs.m.deps.Config.RoleColors(), true) // as New does on such a terminal
	})
	h.toAlbums()
	return h
}

func TestAccentTakesTheCoversColour(t *testing.T) {
	h := coverAccentHarness(t, config.Colors{})
	def := accentOf(t)
	h.keys("3j<CR>j<CR>")
	id, _ := h.m.playingCover()
	art, err := cover.Decode(subsonictest.CoverPNG(id))
	if err != nil {
		t.Fatal(err)
	}
	want, ok := art.Accent()
	if !ok {
		t.Fatalf("the fake cover for %s has no accent; pick another album", id)
	}
	if got := accentOf(t); got != rgbOf(want) || got == def {
		t.Errorf("accent = %x, want the cover's %x (default %x)", got, rgbOf(want), def)
	}
	if rgbOf(playingColor) != rgbOf(want) {
		t.Error("the progress bar and playing row use the playing colour, which did not follow")
	}
	h.keys("X") // stopping puts the configured colour back
	if got := accentOf(t); got != def {
		t.Errorf("after stop the accent = %x, want %x", got, def)
	}
}

func TestAccentStaysWhileTheNextCoverLoads(t *testing.T) {
	h := coverAccentHarness(t, config.Colors{})
	h.keys("3j<CR>j<CR>")
	first := accentOf(t)
	id, _ := h.m.playingCover()
	h.m.covers.entries[id].state = coverLoading // as when the next album's cover is on its way
	h.m.syncAccent()
	if accentOf(t) != first {
		t.Error("the accent fell back to the default while a cover loaded")
	}
}

func TestAccentFollowsTheConfig(t *testing.T) {
	for name, c := range map[string]struct {
		colors config.Colors
		cover  bool
	}{
		"nothing set":             {config.Colors{}, true},
		"playing: cover":          {config.Colors{Playing: config.CoverColor}, true},
		"playing: a hex colour":   {config.Colors{Playing: "#ff9e64"}, false},
		"playing: an ANSI colour": {config.Colors{Playing: "2"}, false},
	} {
		h := coverAccentHarness(t, c.colors)
		before := accentOf(t)
		h.keys("3j<CR>j<CR>")
		if changed := accentOf(t) != before; changed != c.cover {
			t.Errorf("%s: the accent changed = %v, want %v", name, changed, c.cover)
		}
		if h.requests("getCoverArt") == 0 && c.cover {
			t.Errorf("%s: the cover was not asked for", name)
		}
	}
}

func TestNoCoverAccentWithoutTrueColourOrCovers(t *testing.T) {
	for name, set := range map[string]func(*harness){
		"not true colour": func(h *harness) { h.m.deps.TrueColor = false },
		"cover art off":   func(h *harness) { h.m.deps.CoverArtOK = false },
	} {
		h := setupHarness(t, 120, 40, nil, func(hs *harness) {
			hs.m.deps.TrueColor, hs.m.deps.CoverArtOK = true, true
			set(hs)
		})
		h.toAlbums()
		h.keys("3j<CR>j<CR>")
		if h.m.covers.accentFor != "" {
			t.Errorf("%s: the accent is the cover %q's", name, h.m.covers.accentFor)
		}
		if name == "cover art off" && h.requests("getCoverArt") != 0 {
			t.Errorf("%s: asked for %d covers", name, h.requests("getCoverArt"))
		}
	}
}

func TestReloadPutsTheCoverAccentBack(t *testing.T) {
	h := coverAccentHarness(t, config.Colors{})
	h.keys("3j<CR>j<CR>")
	want := accentOf(t)
	h.keys(":reload-config<CR>")
	if got := accentOf(t); got != want {
		t.Errorf("after :reload-config the accent = %x, want the cover's %x", got, want)
	}
}
