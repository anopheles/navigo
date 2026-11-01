package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/anopheles/navigo/internal/keymap"
)

func TestHelp(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("<F1>")
	h.contains(" Help ", "── Everywhere ──", "<Space>", "play-pause", "Pause or resume playback")
	golden.RequireEqual(t, h.screen())
	var headings []string
	for _, r := range h.m.top().help.all {
		if r.heading != "" {
			headings = append(headings, r.heading)
		}
	}
	for _, want := range []string{"In the queue", "On a playlist's tracks", "Visual mode", "Commands without a key: type them after :"} {
		if !strings.Contains(strings.Join(headings, "|"), want) {
			t.Errorf("headings %q lack %q", headings, want)
		}
	}
	h.cursorOn("j") // the heading is skipped

	// / filters as you type; <Esc> puts the filter back.
	h.keys("/")
	h.typeText("shuffle")
	h.contains("Shuffle the queue once")
	h.lacks("play-pause", "── In the queue ──")
	h.keys("<CR>")
	h.contains("Shuffle the queue once")
	h.keys("/")
	h.typeText("zz9")
	h.contains(`nothing in Help matches "shufflezz9"`)
	h.keys("<Esc>")
	h.contains("Shuffle the queue once")
	h.keys("/<C-u><CR>")
	h.contains("play-pause")

	// Help follows the table: a key changed there shows.
	if err := h.m.engine.Table().Bind(keymap.Normal, "global", "<C-p>", "play-pause"); err != nil {
		t.Fatal(err)
	}
	h.keys("'b'?/")
	h.typeText("play-pause")
	h.keys("<CR>")
	h.contains("<C-p>", "<Space>")
}

func TestKeyMenu(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("3j<CR>j?")
	h.contains("Keys here", "Play", "Queue", "press a key to run it")
	h.contains("A        add to queue")
	golden.RequireEqual(t, h.screen())

	// j scrolls rather than moving the cursor; a listed key runs.
	h.keys("G")
	h.contains("Select", "Go", "Help", "? ")
	h.cursorOn("Low Tide")
	if h.m.popup == nil {
		t.Fatal("G closed the menu")
	}
	h.keys("gg")
	h.keys("A")
	if h.m.popup != nil {
		t.Error("the menu is still open after A")
	}
	h.contains("1 track added to the end of the queue")

	// Two keys, ga, run too; q and <Esc> close; <F1> goes to Help.
	h.keys("?ga")
	h.onPath("Albums › Blue Hour")
	h.keys("?q")
	h.lacks("Keys here")
	h.keys("?<F1>")
	h.contains(" Help ")
}

func TestPrefixMenu(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("g")
	h.lacks("after g:")
	h.advance(400 * time.Millisecond)
	h.lacks("after g:")
	h.advance(100 * time.Millisecond)
	h.contains("after g:", "a      album", "r      artist", "g      first row")
	// The next key runs and the menu goes.
	h.keys("g")
	h.lacks("after g:")

	// After an operator: the doubled key and the motions.
	h.keys("<CR>A<Tab>d")
	h.advance(500 * time.Millisecond)
	h.contains("after d:", "d      this row", "j      down")
	h.keys("<Esc>")
	h.lacks("after d:")

	// A key typed before the delay keeps the menu away.
	h.keys("'")
	h.advance(300 * time.Millisecond)
	h.keys("b")
	h.advance(time.Second)
	h.lacks("after '")
	if !strings.Contains(h.screen(), " Albums ") {
		t.Error("'b didn't run")
	}
}

// Unlike other popups, the key menu runs a key as the selection would:
// d there removes the selected rows rather than waiting for a motion.
// ? has no Visual binding by default, so the test makes one.
func TestKeyMenuInVisualRunsVisualKeys(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys(":vmap ? key-menu<CR>")
	h.keys("3j<CR>VjjjjjA<Tab>") // six tracks in the queue, focus on it
	h.queueIs("Dusk Signal, Low Tide, Streetlamp, Blue Hour, Wintering, Afterglow")
	h.keys("Vj?")
	if h.m.popup == nil || h.m.popup.keyMenu == nil {
		t.Fatal("? didn't open the key menu")
	}
	h.keys("d")
	h.queueIs("Streetlamp, Blue Hour, Wintering, Afterglow")
	if h.m.engine.Pending() != "" {
		t.Errorf("keys pending: %q", h.m.engine.Pending())
	}
}
