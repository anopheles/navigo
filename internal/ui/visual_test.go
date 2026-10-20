package ui

import (
	"strings"
	"testing"

	"github.com/anopheles/navigo/internal/keymap"
)

// queueTitles is the queue as track titles.
func (h *harness) queueTitles() string {
	var out []string
	for _, q := range h.m.state.Queue {
		out = append(out, q.Track.Title)
	}
	return strings.Join(out, ", ")
}

func (h *harness) queueIs(want string) {
	h.t.Helper()
	if got := h.queueTitles(); got != want {
		h.t.Errorf("queue = %s\n   want %s", got, want)
	}
}

func TestVisualAddFromAlbum(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("3j<CR>Vjj")
	if h.m.mode != keymap.Visual {
		t.Fatalf("mode %s after V", h.m.mode)
	}
	h.contains("A add 3 tracks to queue", "a play 3 tracks next", "y yank 3 tracks")
	h.keys("A")
	h.contains("3 tracks added to the end of the queue (3 in queue)")
	h.queueIs("Dusk Signal, Low Tide, Streetlamp")
	if h.m.mode != keymap.Normal || h.m.visual != nil {
		t.Error("still in Visual mode after A")
	}

	// : applies a command to the selection.
	h.keys("GVk:")
	h.typeText("add-next")
	h.keys("<CR>")
	h.contains("2 tracks will play next")
	h.queueIs("Wintering, Afterglow, Dusk Signal, Low Tide, Streetlamp")

	// <Esc> ends it; so does V again.
	h.keys("V<Esc>")
	if h.m.visual != nil || h.m.mode != keymap.Normal {
		t.Error("<Esc> didn't end Visual mode")
	}
	h.keys("VV")
	if h.m.visual != nil {
		t.Error("V didn't end Visual mode")
	}
}

func TestQueueEditsWhilePlaying(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("3j<CR><CR><Tab>") // Blue Hour plays from Dusk Signal (entry 1)
	h.queueIs("Dusk Signal, Low Tide, Streetlamp, Blue Hour, Wintering, Afterglow")

	// dd on the next track: the player's next follows.
	h.keys("jdd")
	h.queueIs("Dusk Signal, Streetlamp, Blue Hour, Wintering, Afterglow")
	h.contains("1 track removed (5 in queue) · u undoes")
	if c := h.lastCall(); c != "SetNeighbours(0 3)" {
		t.Errorf("after dd: %s", c)
	}
	h.cursorOn("Streetlamp")

	// p pastes it back after the cursor, K moves it up to play next.
	h.keys("p")
	h.queueIs("Dusk Signal, Streetlamp, Low Tide, Blue Hour, Wintering, Afterglow")
	h.cursorOn("Low Tide")
	h.keys("K")
	h.queueIs("Dusk Signal, Low Tide, Streetlamp, Blue Hour, Wintering, Afterglow")
	if c := h.lastCall(); c != "SetNeighbours(0 7)" {
		t.Errorf("after K: %s", c)
	}

	// A selection moves together and is removed together.
	h.keys("Vj")
	h.contains("d remove 2 tracks", "J move down")
	h.keys("J")
	h.queueIs("Dusk Signal, Blue Hour, Low Tide, Streetlamp, Wintering, Afterglow")
	if c := h.lastCall(); c != "SetNeighbours(0 4)" {
		t.Errorf("after J: %s", c)
	}
	h.keys("d")
	h.queueIs("Dusk Signal, Blue Hour, Wintering, Afterglow")
	h.contains("2 tracks removed (4 in queue)")

	// Every edit undoes, one at a time, and the neighbours follow.
	h.keys("u")
	h.queueIs("Dusk Signal, Blue Hour, Low Tide, Streetlamp, Wintering, Afterglow")
	h.keys("uuuu")
	h.queueIs("Dusk Signal, Low Tide, Streetlamp, Blue Hour, Wintering, Afterglow")
	if c := h.lastCall(); c != "SetNeighbours(0 2)" {
		t.Errorf("after undoing: %s", c)
	}
	h.keys("<C-r>")
	h.queueIs("Dusk Signal, Streetlamp, Blue Hour, Wintering, Afterglow")
}

func TestOperatorsInTheQueue(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("3j<CR>Vjjjjj")
	h.keys("A<Tab>") // six tracks, nothing playing
	h.keys("2x")
	h.queueIs("Streetlamp, Blue Hour, Wintering, Afterglow")
	h.keys("d2j")
	h.queueIs("Afterglow")
	h.keys("P")
	h.queueIs("Streetlamp, Blue Hour, Wintering, Afterglow")
	h.keys("ggyGGp")
	h.queueIs("Streetlamp, Blue Hour, Wintering, Afterglow, Streetlamp, Blue Hour, Wintering, Afterglow")
	h.contains("4 tracks pasted (8 in queue)")
	h.keys("gg3dd")
	h.queueIs("Afterglow, Streetlamp, Blue Hour, Wintering, Afterglow")
	h.keys("dG")
	h.queueIs("")
	h.contains("The queue is empty.")
}

// The register is shared: yank in the library, paste in the queue.
func TestYankInLibraryPasteInQueue(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("3j<CR>jyj")
	h.contains("2 tracks yanked · p in the queue pastes them")
	h.cursorOn("Low Tide")
	h.keys("<Tab>p")
	h.queueIs("Low Tide, Streetlamp")
	h.contains("2 tracks pasted (2 in queue)")

	// An album row yanks the album; d does nothing outside the queue.
	h.keys("<Tab>hyy")
	h.contains("6 tracks yanked")
	h.keys("Vd")
	h.contains("only the queue and playlists can be edited")
	h.keys(`"a`)
	h.contains("navigo has one register")
}

// In Search, a selection spans the groups; artist rows are left out.
func TestVisualInSearch(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys(":search pell<CR>")
	h.cursorOn("Pell Mell")
	h.keys("VG")
	h.contains("A add 2 tracks to queue")
	h.keys("A")
	h.contains("2 tracks added to the end of the queue (2 in queue) · u undoes · 1 artist skipped")
	h.queueIs("Blue Highway, Night Drive")

	// Only artists: nothing to add.
	h.keys("ggVA")
	h.contains("an artist can't be added")
}
