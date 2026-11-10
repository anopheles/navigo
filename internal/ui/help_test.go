package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/anopheles/navigo/internal/keymap"
)

func TestHelp(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("<F1>")
	h.contains(" Help ", "── Everywhere ──", "label-jump", "Type part of a row on screen")
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
	h.contains("label-jump")

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

// / in the key menu filters it as you type, by key, label, command or
// description, rather than starting a find.
func TestKeyMenuFilter(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("3j<CR>j?")
	h.contains("/ filter")
	h.keys("/")
	h.contains("filter keys: ", "PROMPT")
	h.typeText("seek")
	h.contains("Keys here /seek", "seek +10", "seek -10")
	menu := ansi.Strip(strings.Join(h.m.popup.lines, "\n"))
	for _, gone := range []string{"<C-x> Ctrl+x", "Queue", "add to queue"} {
		if strings.Contains(menu, gone) { // nor the legend, nor empty groups
			t.Errorf("filtered by seek, the key menu still has %q:\n%s", gone, menu)
		}
	}

	// <Esc> at the prompt puts the old filter back; <CR> keeps the new one.
	h.keys("<Esc>")
	h.contains("add to queue")
	h.lacks("Keys here /")
	h.keys("/")
	h.typeText("shuffle")
	h.keys("<CR>")
	h.contains("Keys here /shuffle", "S        shuffle")
	h.lacks("seek +10")

	// A listed key still runs; q clears the filter before closing.
	h.keys("q")
	h.contains("Keys here", "add to queue")
	h.lacks("Keys here /")
	h.keys("/")
	h.typeText("add to")
	h.keys("<CR>A")
	h.lacks("Keys here")
	h.contains("1 track added to the end of the queue")

	// The filter starts empty each time.
	h.keys("?")
	h.lacks("Keys here /")

	// Nothing here matches: <F1> takes the filter to Help.
	h.keys("/")
	h.typeText("queue-clear")
	h.keys("<CR>")
	h.contains(`nothing here matches "queue-clear" · <F1> searches all keys`)
	h.keys("<F1>")
	h.contains("Help /queue-clear", ":queue-clear")
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

// q, h and <Esc> close Help back to where it was opened, a filter going
// first, and i shows a row's whole description (#146).
func TestHelpClosesBackToWhereItOpened(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("3j<CR>jj")
	h.onPath("Albums › Blue Hour")
	for _, k := range []string{"q", "h", "<Esc>"} {
		h.keys("<F1>")
		h.contains(" Help ")
		h.keys(k)
		h.onPath("Albums › Blue Hour")
		h.cursorOn("Streetlamp")
	}

	h.keys("<F1>/")
	h.typeText("mix-add-next")
	h.keys("<CR>")
	h.contains("Help /mix-add-next")
	h.keys("<Esc>") // the filter goes first
	h.contains(" Help ")
	h.lacks("Help /mix-add-next")
	h.keys("/")
	h.typeText("mix-add-next")
	h.keys("<CR>i")
	h.contains("(library: always the library); u undoes it.")
	h.keys("q<Esc><Esc>") // the popup, the filter, then Help
	h.onPath("Albums › Blue Hour")

	// Help isn't walked back into with <C-o>.
	h.keys("<F1>'p<C-o>")
	h.lacks(" Help ")
}

func TestHelpShowsWholeCommandNames(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("<F1>/")
	h.typeText("queue-clear")
	h.keys("<CR>")
	h.contains(":queue-clear  ")
	h.lacks("queue-cl…")
}

// The key menu grows with a large terminal.
func TestKeyMenuGrows(t *testing.T) {
	h := newHarness(t, 200, 60, nil)
	h.keys("3j<CR>j?")
	if w := h.m.popupWidth(); w != 160 {
		t.Errorf("popup %d wide on a 200-column screen, want 160", w)
	}
	if r := h.m.popupRows(); r != 40 {
		t.Errorf("key menu %d rows on a 60-row screen, want 40", r)
	}
}

// Help and the key menu say how keys are written and what the marks mean,
// and the descriptions say what keys do before naming the vim term (#155).
func TestHelpExplainsItsNotation(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("<F1>")
	h.contains("<CR> Enter · <C-x> Ctrl+x", "• in the queue · ▶ playing · ★ starred")
	h.cursorOn("j") // the legend is skipped
	h.keys("/")
	h.typeText("yank")
	h.keys("<CR>")
	h.contains("Copy rows (yank): yy copies this row")
	h.lacks("register", "<C-x> Ctrl+x") // the legend goes while filtered
	h.keys("q?")
	h.contains("<CR> Enter · <C-x> Ctrl+x")
	h.keys("qqn")
	h.contains("nothing to find again · / finds a row in this list")
}
