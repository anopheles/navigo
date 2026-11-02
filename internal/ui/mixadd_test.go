package ui

import (
	"testing"

	"github.com/anopheles/navigo/internal/keymap"
)

// gM adds to the end and gN after the current track; neither touches what
// plays, a count sets the size, and u takes the whole batch away.
func TestMixAddKeys(t *testing.T) {
	h := homeHarness(t, 160, 40, nil)
	h.keys("2gm")
	playing := h.m.state.Queue[h.m.state.Current].Track.ID
	before := len(h.m.state.Queue)
	second := h.m.state.Queue[1].Track.ID

	h.keys("'hj1gM")
	h.contains("added 1 track")
	if got := len(h.m.state.Queue); got != before+1 {
		t.Fatalf("after 1gM, queue %d, want %d", got, before+1)
	}
	if got := h.m.state.Queue[1].Track.ID; got != second {
		t.Errorf("gM changed what comes next: %s", got)
	}
	h.keys("u")
	if got := len(h.m.state.Queue); got != before {
		t.Fatalf("after u, queue %d, want %d", got, before)
	}

	h.keys("1gN")
	if got := len(h.m.state.Queue); got != before+1 {
		t.Fatalf("after 1gN, queue %d, want %d", got, before+1)
	}
	if got := h.m.state.Queue[2].Track.ID; got != second {
		t.Errorf("after 1gN, the old next track is %s, want %s", got, second)
	}
	if got := h.m.state.Queue[h.m.state.Current].Track.ID; got != playing {
		t.Errorf("gN changed what plays: %s", got)
	}
}

func TestMixAddDefaultsAndArguments(t *testing.T) {
	h := homeHarness(t, 160, 40, nil)
	h.keys(":mix-add-end library<CR>")
	h.contains("added 10 tracks")
	h.lacks("tracks from")
	if got := len(h.m.state.Queue); got != 10 {
		t.Errorf("queue %d, want 10", got)
	}
	h.keys(":mix-add-next everything<CR>")
	h.contains("mix-add-next: give a track count, library or both")
}

func TestMixAddInTheKeyMenu(t *testing.T) {
	h := homeHarness(t, 160, 40, nil)
	h.keys("?")
	h.contains("add mix from the whole library", "add mix next from the whole")
}

func TestMixCommandsTakeACount(t *testing.T) {
	for _, line := range []string{"mix-add-end library 2", "mix-add-end 2 library"} {
		h := homeHarness(t, 160, 40, nil)
		h.keys(":" + line + "<CR>")
		h.contains("added 2 tracks")
		h.lacks("tracks from")
	}

	h := homeHarness(t, 160, 40, nil)
	h.keys(":mix 3 library<CR>")
	h.contains("mixed 3 tracks")
	if got := len(h.m.state.Queue); got != 3 {
		t.Errorf(":mix 3 library: queue %d", got)
	}
	h.keys(":mix-add-next 1<CR>")
	h.contains("added 1 track")
	h.lacks("tip: next time")
	if got := len(h.m.state.Queue); got != 4 {
		t.Errorf(":mix-add-next 1: queue %d, want 4", got)
	}
}

func TestMixArgsRefused(t *testing.T) {
	for _, a := range [][]string{
		{"0"}, {"-3"}, {"+5"}, {"2.5"}, {"5", "5"}, {"library", "library"},
		{"library", "5", "5"}, {"everything"},
	} {
		if err := mixArgs("mix-add-end")(a); err == nil {
			t.Errorf("%q was accepted", a)
		}
	}
	for _, a := range [][]string{nil, {"5"}, {"library"}, {"library", "5"}, {"5", "library"}} {
		if err := mixArgs("mix-add-end")(a); err != nil {
			t.Errorf("%q: %v", a, err)
		}
	}
}

// A count pressed on a key beats the number its binding has; the key menu
// shows the binding's number.
func TestMixBindingWithACount(t *testing.T) {
	h := homeHarness(t, 160, 40, nil)
	err := h.m.engine.Table().Bind(keymap.Normal, "global", "<C-p>", "mix-add-end library 2")
	if err != nil {
		t.Fatal(err)
	}
	h.keys("?")
	h.contains("add mix 2 from the whole")
	h.keys("q<C-p>")
	h.contains("added 2 tracks")
	h.keys("u1<C-p>")
	h.contains("added 1 track")
	if got := len(h.m.state.Queue); got != 1 {
		t.Errorf("1<C-p>: queue %d, want 1", got)
	}
}
