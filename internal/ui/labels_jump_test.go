package ui

import (
	"slices"
	"strings"
	"testing"

	"github.com/anopheles/navigo/internal/keymap"
)

func TestLabelJump(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.cursorOn("Ångström Days")
	h.keys("f")
	h.contains("type a label to jump", "<CR> nearest")
	if h.m.mode != keymap.Search {
		t.Errorf("mode %v, want Search", h.m.mode)
	}

	// Every match on screen but the cursor's row gets a label, nearest
	// first, in its first column; the matches are lit as / lights them.
	h.typeText("blue")
	h.contains("3 rows")
	screen := h.m.render()
	for label, row := range map[string]string{"a": "Aurora La", "s": "The Quiet", "d": "Blue Hour"} {
		if l := lineWith(
			t,
			screen,
			row,
		); !strings.HasPrefix(
			l,
			"│"+h.m.labelStyle().Render(label),
		) {
			t.Errorf("row %s lacks label %s: %q", row, label, l)
		}
	}
	if !strings.Contains(screen, findStyle.Render("Blue")) {
		t.Error("the matches aren't lit")
	}

	// A label jumps, into the jump list.
	h.keys("s")
	h.cursorOn("The Quiet Ones")
	if h.m.mode != keymap.Normal {
		t.Errorf("mode %v after the jump", h.m.mode)
	}
	h.keys("<C-o>")
	h.cursorOn("Ångström Days")

	// <CR> goes to the nearest; <Esc> stays; a count changes nothing.
	h.keys("3f")
	h.typeText("blue")
	h.keys("<CR>")
	h.cursorOn("Aurora Lane")
	h.keys("f")
	h.typeText("hour")
	h.keys("<Esc>")
	h.cursorOn("Aurora Lane")

	// No match keeps the prompt open, and <CR> does nothing there.
	h.keys("f")
	h.typeText("zzz")
	h.contains("no match")
	h.keys("<CR>")
	h.contains("no match")
	h.keys("<Esc>")
}

func TestLabelsSkipLettersThatContinue(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("f")
	h.typeText("b")
	for _, l := range h.m.labeling.labels {
		if l == 'l' {
			t.Fatal("l continues Blue and Bloom, so it can't be a label")
		}
	}
	h.typeText("l") // narrows instead of jumping
	if h.m.line.Value() != "bl" || !h.m.labeling.prompting {
		t.Errorf("typing on: %q, prompting %v", h.m.line.Value(), h.m.labeling.prompting)
	}
}

func TestLabelJumpIsAMotion(t *testing.T) {
	h := newHarness(t, 120, 40, nil)

	// In Visual mode it extends the selection.
	h.keys("Vf")
	h.typeText("hour")
	h.keys("a")
	if h.m.mode != keymap.Visual {
		t.Fatalf("mode %v, want Visual", h.m.mode)
	}
	if from, to := h.m.selection(); from != 0 || to != 4 {
		t.Errorf("selection %d-%d, want 0-4", from, to)
	}
	h.keys("<Esc>")

	// After an operator it takes the rows to the label, both ends in.
	h.keys("ggj<CR>VGA<Tab>") // the eight tracks of Blue in the queue
	h.contains("Queue (8)")
	h.keys("df")
	h.typeText("slow") // the fourth track
	h.keys("a")
	h.contains("Queue (4)")
	h.cursorOn("Harbour Lights")
	h.keys("<C-o>") // a jump after an operator isn't recorded
	h.lacks("at the oldest place")
}

func TestLabelJumpFromTheKeyMenu(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("?")
	if !slices.ContainsFunc(h.m.popup.menu.rows, func(r keyMenuRow) bool { return r.key == "f" }) {
		t.Error("the key menu doesn't list f")
	}
	h.keys("f")
	if h.m.popup != nil {
		t.Error("the key menu stayed open")
	}
	h.contains("type a label to jump")
}
