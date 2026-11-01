package ui

import "testing"

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
	h.contains("mix-add: give library, or nothing to mix around the row")
}

func TestMixAddInTheKeyMenu(t *testing.T) {
	h := homeHarness(t, 160, 40, nil)
	h.keys("?")
	h.contains("add mix from the whole library", "add mix next from the whole")
}
