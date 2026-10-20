package ui

import (
	"fmt"
	"strings"
	"testing"
)

// screenLine is the screen line that shows Mile n, or -1.
func (h *harness) mileLine(n int) int { return h.lineIndex(fmt.Sprintf("Mile %03d", n)) }

func (h *harness) onMile(n int) {
	h.t.Helper()
	h.cursorOn(fmt.Sprintf("Mile %03d", n))
}

// The Long Haul has 510 tracks; 34 rows show at 120×41.
func TestPageAndScreenMotions(t *testing.T) {
	h := newHarness(t, 120, 41, nil)
	h.keys("4j<CR>")
	if rows := h.m.bodyHeight(); rows != 34 {
		t.Fatalf("%d rows on screen", rows)
	}
	h.keys("<C-f>")
	h.onMile(35)
	if l := h.mileLine(35); l != 1 {
		t.Errorf("after <C-f> Mile 035 is on line %d, want the top", l)
	}
	h.keys("2<C-f>")
	h.onMile(103)
	h.keys("<C-b>")
	h.onMile(69)
	if l := h.mileLine(69); l != 34 {
		t.Errorf("after <C-b> Mile 069 is on line %d, want the bottom", l)
	}

	// <C-e>/<C-y> move the view; the cursor stays while it can.
	h.keys("<C-e>")
	h.onMile(69)
	if l := h.mileLine(69); l != 33 {
		t.Errorf("after <C-e> Mile 069 is on line %d", l)
	}
	h.keys("40<C-e>") // the cursor would leave the screen: it goes along
	h.onMile(77)
	h.keys("3<C-y>")
	h.onMile(77)

	// H, M, L, with counts from the top and the bottom.
	h.keys("H")
	h.onMile(74)
	h.keys("3H")
	h.onMile(76)
	h.keys("L")
	h.onMile(107)
	h.keys("2L")
	h.onMile(106)
	h.keys("M")
	h.onMile(90)

	// zt, zz, zb scroll the cursor row to the top, middle, bottom.
	h.keys("zt")
	if l := h.mileLine(90); l != 1 {
		t.Errorf("zt: line %d", l)
	}
	h.keys("zb")
	if l := h.mileLine(90); l != 34 {
		t.Errorf("zb: line %d", l)
	}
	h.keys("zz")
	if l := h.mileLine(90); l != 18 {
		t.Errorf("zz: line %d", l)
	}

	// [count]% goes that far down.
	h.keys("50%")
	h.onMile(255)
	h.keys("100%")
	h.onMile(510)
	h.keys("%")
	h.contains("give a percentage before %")

	// Motions work after an operator: y with H covers the rows to the top.
	h.keys("50%yH")
	h.contains("tracks yanked")
}

func TestGroupMotionsAcrossGroups(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys(":search blue<CR>") // Albums, then Tracks
	h.cursorOn("Blue")
	h.keys("}")
	h.cursorOn("3:58") // the track Blue, first of Tracks
	h.keys("2{")
	h.cursorOn("2019") // back at the first album
}

func TestFindWrapping(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("/")
	h.contains("<CR> find")
	h.typeText("blue")
	h.keys("<CR>")
	h.cursorOn("Aurora Lane") // Blue, the first match after the cursor
	if !strings.Contains(h.m.render(), findStyle.Render("Blue")) {
		t.Error("matches aren't highlighted")
	}
	h.keys("n")
	h.cursorOn("The Quiet Ones")
	h.keys("2n") // Blue Hour, then round to the first Blue
	h.cursorOn("Aurora Lane")
	h.cursorOn("2019")
	h.contains("went on from the top")
	h.keys("N") // up from the top goes round to Blue Hour
	h.cursorOn("Blue Hour")
	h.contains("went on from the bottom")

	// :noh and <Esc> clear the highlights; n still works.
	h.keys(":noh<CR>")
	if strings.Contains(h.m.render(), findStyle.Render("Blue")) {
		t.Error(":noh left highlights")
	}
	h.keys("n")
	h.cursorOn("2019")
	h.keys("<Esc>")
	if strings.Contains(h.m.render(), findStyle.Render("Blue")) {
		t.Error("<Esc> left highlights")
	}

	// :find-back searches up.
	h.keys("G:find-back<CR>")
	h.contains("?")
	h.typeText("blue")
	h.keys("<CR>")
	h.cursorOn("Blue Hour")

	// Smart case: a capital letter matches case.
	h.keys("gg/")
	h.typeText("Blue")
	h.keys("<CR>")
	h.cursorOn("2019")
	h.keys("/")
	h.typeText("BLUE")
	h.keys("<CR>")
	h.contains(`no match for "BLUE" here · <CR> searches the whole library`)
	// The offer: <CR> searches the library with the same text.
	h.keys("<CR>")
	h.contains("Search “BLUE”", "── Albums")
}

func TestFindOfferGoesAway(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("/")
	h.typeText("zzz")
	h.keys("<CR>j<CR>")
	h.lacks("Search “zzz”")
	h.contains("Albums › Blue") // j moved, <CR> opened the album
}

func TestFindInTheQueue(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("3j<CR>VGA<Tab>/")
	h.typeText("low")
	h.keys("<CR>")
	h.cursorOn("Low Tide")
}

func TestJumpList(t *testing.T) {
	h := newHarness(t, 120, 40, nil) // went to Albums with 'b: a jump
	h.keys("'a3j'p")                 // j and k aren't jumps; marks are
	h.contains(" Playlists ")
	h.keys("<C-o>")
	h.contains(" Artists ")
	h.cursorOn("The Quiet Ones")
	h.keys("<C-o>")
	h.contains(" Albums ")
	h.keys("<C-o>")
	h.contains(" Home ")
	h.keys("<C-o>")
	h.contains("at the oldest place in the jump list")

	h.keys("<C-i>")
	h.contains(" Albums ")
	h.keys(":forward<CR>")
	h.contains(" Artists ")
	h.keys(":forward<CR>:forward<CR>")
	h.contains(" Playlists ", "at the newest place in the jump list")

	// gg and G are jumps; a new jump drops what was ahead.
	h.keys("'bjjG<C-o>")
	h.cursorOn("The Quiet Ones") // the second Blue, where G jumped from
	h.keys("<C-o>")
	h.contains(" Playlists ")

	// ga, gr and gp are jumps, across windows.
	h.keys("'bgg3j<CR>j<CR>'a")
	h.keys("gp")
	if h.m.focus != queueWindow || h.m.queueCur != 1 {
		t.Errorf("gp: focus %d, row %d", h.m.focus, h.m.queueCur)
	}
	h.keys("<C-o>")
	if h.m.focus != libraryWindow {
		t.Error("<C-o> didn't go back to the library window")
	}
	h.contains(" Artists ")
	h.keys("<C-o>")
	h.contains("Albums › Blue Hour")
	h.cursorOn("Low Tide")
}

func TestGoPlayingWithNothing(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("gp")
	h.contains("nothing in the queue plays")
}
