package ui

import (
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/anopheles/navigo/internal/config"
)

// <C-w>o in the library closes the queue window: the library takes the
// whole width, and the message says how to get the queue back (#176).
func TestCloseTheQueue(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	jumps := len(h.m.jumps)
	h.keys("<C-w>o")
	h.lacks("Queue (0)")
	h.contains("Albums   <Tab> queue", "queue closed · <Tab> opens it")
	if h.m.focus != libraryWindow {
		t.Error("the focus left the library")
	}
	if len(h.m.jumps) != jumps {
		t.Errorf("the jump list grew from %d to %d", jumps, len(h.m.jumps))
	}
	golden.RequireEqual(t, h.screen())
}

// In the queue it closes the library: the queue takes the whole width,
// and the focus stays (#187).
func TestCloseTheLibrary(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("<Tab><C-w>o")
	if h.m.focus != queueWindow {
		t.Error("the focus left the queue")
	}
	h.lacks(" Albums ")
	h.contains("Queue (0)   <Tab> library", "library closed · <Tab> opens it")
	golden.RequireEqual(t, h.screen())
}

// Pressed again in the queue it says how to reopen the library, and
// doesn't.
func TestCloseTheLibraryTwice(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("<Tab><C-w>o")
	h.keys("<Esc><C-w>o")
	h.lacks(" Albums ")
	h.contains("library closed · <Tab> opens it")
}

// Every way to the library opens it again, beside the queue.
func TestReopenTheLibrary(t *testing.T) {
	for _, keys := range []string{"<Tab>", "<C-w>w", "<C-w>h", ":albums<CR>"} {
		t.Run(keys, func(t *testing.T) {
			h := newHarness(t, 120, 40, nil)
			h.keys("<Tab><C-w>o")
			h.keys(keys)
			if h.m.focus != libraryWindow {
				t.Error("the focus didn't go to the library")
			}
			h.contains("Queue (0)", " Albums ")
			h.lacks("<Tab> library")
		})
	}
}

// A section's jump, and a search, open the library too.
func TestReopenTheLibraryByJumps(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("<Tab><C-w>o'a")
	h.contains("Queue (0)", " Artists ")
	h.lacks("<Tab> library")

	h.keys("<Tab><C-w>o")
	h.keys("s")
	h.typeText("blue")
	h.keys("<CR>")
	if h.m.focus != libraryWindow {
		t.Error("the search didn't go to the library")
	}
	h.contains("Queue (0)")
	h.lacks("<Tab> library")
}

// The queue alone heads with the card.
func TestTheQueueAloneKeepsTheCard(t *testing.T) {
	h := coverHarness(t, 120, 40, config.CoverAuto, false)
	h.play()
	h.fake.Tick(42 * time.Second)
	h.pump()
	h.keys("<Tab><C-w>o")
	h.lacks(" Albums ")
	h.contains("next: Streetlamp", "Queue (6) · 12:37   <Tab> library")
}

// <C-w>= opens the closed window again and keeps the focus where it is.
func TestShowBothWindows(t *testing.T) {
	for _, start := range []string{"", "<Tab>"} {
		h := newHarness(t, 120, 40, nil)
		h.keys(start + "<C-w>o")
		focus := h.m.focus
		h.keys("<C-w>=")
		if h.m.focus != focus {
			t.Errorf("%q: the focus moved", start)
		}
		h.contains("Queue (0)", " Albums ")
		h.lacks("<Tab> library", "<Tab> queue")
	}
}

// With both windows shown <C-w>= does nothing and says nothing.
func TestShowBothWindowsWhenShown(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	before := h.screen()
	h.keys("<C-w>=")
	if got := h.screen(); got != before {
		t.Errorf("the screen changed:\n%s", got)
	}
}

// A narrow terminal has no room for both: <C-w>= says so, and widening
// then shows both.
func TestShowBothWindowsWhenNarrow(t *testing.T) {
	h := newHarness(t, 80, 30, nil)
	h.keys("<C-w>o<C-w>=")
	h.contains("too narrow for both windows")
	h.send(tea.WindowSizeMsg{Width: 120, Height: 30})
	h.contains("Queue (0)", " Albums ")
}

// The Ctrl spellings Vim accepts do what the plain ones do.
func TestWindowKeysWithCtrl(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("<C-w><C-w>")
	if h.m.focus != queueWindow {
		t.Fatal("<C-w><C-w> didn't go to the queue")
	}
	h.keys("<C-w><C-h>")
	if h.m.focus != libraryWindow {
		t.Fatal("<C-w><C-h> didn't go to the library")
	}
	h.keys("<C-w><C-l>")
	if h.m.focus != queueWindow {
		t.Fatal("<C-w><C-l> didn't go to the queue")
	}
	h.keys("<C-w><C-o>")
	h.lacks(" Albums ")
	h.contains("library closed")
}

// Pressed again it says how to reopen the queue, and doesn't.
func TestCloseTheQueueTwice(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("<C-w>o")
	h.keys("<Esc><C-w>o")
	h.lacks("Queue (0)")
	h.contains("queue closed · <Tab> opens it")
}

// Every way to the queue opens it again, beside the library.
func TestReopenTheQueue(t *testing.T) {
	for _, keys := range []string{"<Tab>", "<C-w>w", "<C-w>l", "'q", ":queue<CR>"} {
		t.Run(keys, func(t *testing.T) {
			h := newHarness(t, 120, 40, nil)
			h.keys("<C-w>o")
			h.keys(keys)
			if h.m.focus != queueWindow {
				t.Error("the focus didn't go to the queue")
			}
			h.contains("Queue (0)", " Albums ")
			h.lacks("<Tab> queue")
		})
	}
}

// gp, and going back in the jump list to the queue, open it too.
func TestReopenTheQueueByJumps(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("3j<CR>j<CR>") // Low Tide plays
	h.keys("<C-w>o")
	h.keys("gp")
	h.contains("Queue (6)", " Blue Hour ")
	h.cursorOn("Low Tide")

	h.keys("'a<C-w>o")
	h.lacks("Queue (6)")
	h.keys("<C-o>") // back to the queue, where 'a jumped from
	if h.m.focus != queueWindow {
		t.Fatal("<C-o> didn't go back to the queue")
	}
	h.contains("Queue (6)")
}

// The search prompt's <C-q> goes to the queue, and so opens it.
func TestReopenTheQueueFromSearch(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("<C-w>o")
	h.keys("s")
	h.typeText("blue")
	h.keys("<C-q>")
	h.contains("Queue (0)")
}

// In Artists the columns take the whole width: all three show.
func TestCloseTheQueueInColumns(t *testing.T) {
	h := newHarness(t, 120, 30, nil)
	h.keys("'a")
	if got := h.depths(); !slices.Equal(got, []int{1, 2}) {
		t.Fatalf("beside the queue: columns %v", got)
	}
	h.keys("<C-w>o")
	if got := h.depths(); !slices.Equal(got, []int{1, 2, 3}) {
		t.Errorf("queue closed: columns %v", got)
	}
	h.lacks("Queue (0)")
	golden.RequireEqual(t, h.screen())
}

// On a narrow terminal it closes the window that isn't shown; widened,
// that window stays closed, and <Tab> brings it back.
func TestCloseTheQueueWhenNarrow(t *testing.T) {
	h := newHarness(t, 80, 30, nil)
	h.keys("<Tab><C-w>o")
	h.contains("Queue (0)   <Tab> library", "library closed")
	h.send(tea.WindowSizeMsg{Width: 120, Height: 30})
	h.lacks(" Albums ")
	h.keys("<Tab>")
	h.contains("Queue (0)", " Albums ")
	h.send(tea.WindowSizeMsg{Width: 80, Height: 30})
	h.contains("Albums   <Tab> queue")
}

// The card goes with the queue window and comes back with it.
func TestCloseTheQueueHidesTheCard(t *testing.T) {
	h := coverHarness(t, 120, 40, config.CoverAuto, false)
	h.play()
	h.fake.Tick(42 * time.Second)
	h.pump()
	h.contains("next: Streetlamp")
	h.keys("<C-w>o")
	h.lacks("next: Streetlamp", "Queue (6)")
	h.contains("▶  Low Tide", " 2/6 ")
	h.keys("<Tab>")
	h.contains("next: Streetlamp")
}

// Closing it is session only: a restart shows the queue again.
func TestClosedQueueIsNotKept(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("<C-w>o")
	h = h.restart()
	h.contains("Queue (0)")
}
