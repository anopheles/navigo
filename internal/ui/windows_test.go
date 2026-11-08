package ui

import "testing"

// <C-w>h goes to the library window and <C-w>l to the queue window,
// whichever has the focus: at the edge they do nothing, and neither
// enters the jump list (#175).
func TestGoToAWindow(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	jumps := len(h.m.jumps)
	h.keys("<C-w>l")
	if h.m.focus != queueWindow {
		t.Fatal("<C-w>l didn't go to the queue window")
	}
	h.keys("<C-w>l")
	if h.m.focus != queueWindow {
		t.Error("<C-w>l in the queue left it")
	}
	h.keys("<C-w>h")
	if h.m.focus != libraryWindow {
		t.Fatal("<C-w>h didn't go to the library window")
	}
	h.keys("<C-w>h")
	if h.m.focus != libraryWindow {
		t.Error("<C-w>h in the library left it")
	}
	h.lacks("does nothing here")
	if len(h.m.jumps) != jumps {
		t.Errorf("the jump list grew from %d to %d", jumps, len(h.m.jumps))
	}
}

// As <Tab> does, <C-w>l takes the queue's cursor to a new playing track.
func TestGoToTheQueueFindsWhatPlays(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("3j<CR>jj<CR>") // Streetlamp plays
	h.keys("<C-w>l")
	if h.m.focus != queueWindow {
		t.Fatal("<C-w>l didn't go to the queue window")
	}
	h.cursorOn("Streetlamp")
}

// On a narrow terminal, where one window shows, they swap it.
func TestGoToAWindowWhenNarrow(t *testing.T) {
	h := newHarness(t, 80, 30, nil)
	h.keys("<C-w>l")
	h.contains(" Queue (")
	h.lacks(" Albums ")
	h.keys("<C-w>h")
	h.contains(" Albums ")
	h.lacks(" Queue (")
}

// <C-h> and <C-l> can be bound to them, as a tmux user would.
func TestGoToAWindowRebound(t *testing.T) {
	h := keysHarness(t, `keys:
  normal:
    global:
      '<C-h>': window-left
      '<C-l>': window-right
`)
	h.keys("<C-l>")
	if h.m.focus != queueWindow {
		t.Fatal("<C-l> didn't go to the queue window")
	}
	h.keys("<C-h>")
	if h.m.focus != libraryWindow {
		t.Error("<C-h> didn't go to the library window")
	}
}
