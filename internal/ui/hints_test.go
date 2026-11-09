package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// The hint lines name the keys people missed: remove in the queue, P on
// tracks, and :q to quit; the key menu starts with what works here and
// lists gp, gc, g+, g- and / (#149).
func TestHintsNameTheMissingKeys(t *testing.T) {
	h := queueOfTwoAlbums(t)
	h.contains("x remove", ":q quit")
	h.keys("<Tab>3j<CR>") // an album's tracks
	h.contains("<CR> play from here", "P play this now")
}

func TestKeyMenuStartsWithWhatWorksHere(t *testing.T) {
	h := queueOfTwoAlbums(t)
	h.keys("?")
	menu := ansi.Strip(strings.Join(h.m.popup.lines, "\n"))
	here, play := strings.Index(menu, "\nHere\n"), strings.Index(menu, "\nPlay\n")
	if here < 0 || here > play || !strings.Contains(menu, "x        remove") {
		t.Errorf("the queue's key menu doesn't start with its own keys:\n%s", menu)
	}
	for _, want := range []string{
		"gp       playing", "gc       cover", "g+       more like this",
		"g-       less like this",
	} {
		if !strings.Contains(menu, want) {
			t.Errorf("the key menu lacks %q", want)
		}
	}
	if strings.Contains(menu, "/        find") {
		t.Error("the key menu lists / as find, but / filters it")
	}
	h.keys("q<Tab>?")
	h.contains("Here", "<CR>     play from here", "P        play this now")
}
