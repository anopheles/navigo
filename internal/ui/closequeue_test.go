package ui

import (
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/anopheles/navigo/internal/config"
)

// <C-w>o closes the queue window: the library takes the whole width, and
// the message says how to get the queue back (#176).
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

// From the queue window it still closes the queue, not the library, and
// the focus goes to the library.
func TestCloseTheQueueFromIt(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("<Tab><C-w>o")
	if h.m.focus != libraryWindow {
		t.Error("the focus stayed on the closed queue")
	}
	h.lacks("Queue (0)")
	h.contains("Albums   <Tab> queue")
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

// On a narrow terminal it moves to the library; widened, the queue stays
// closed, and <Tab> brings it back.
func TestCloseTheQueueWhenNarrow(t *testing.T) {
	h := newHarness(t, 80, 30, nil)
	h.keys("<Tab><C-w>o")
	h.contains("Albums   <Tab> queue")
	h.lacks("Queue (0)")
	h.send(tea.WindowSizeMsg{Width: 120, Height: 30})
	h.lacks("Queue (0)")
	h.keys("<Tab>")
	h.contains("Queue (0)", " Albums ")
	h.send(tea.WindowSizeMsg{Width: 80, Height: 30})
	h.contains("Queue (0)   <Tab> library")
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
