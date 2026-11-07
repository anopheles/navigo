package ui

import "testing"

// A key that runs nothing here says so, and where it would run (#144).
func TestDeadKeysSaySo(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("q")
	h.contains("q does nothing here · :q quits")
	h.keys("x")
	h.contains("x is remove in the queue ('q)")
	h.keys("Y")
	h.contains("Y does nothing here · ? lists the keys here")
	h.keys("<Esc>g<Esc>") // cancelled on purpose: nothing to say
	h.lacks("does nothing here")
}

// A command's error goes once the next command works.
func TestCommandErrorClearsOnNextCommand(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys(":nope<CR>")
	h.contains("not a command: nope")
	h.keys("j")
	h.lacks("not a command: nope")
}

func TestSyncSaysWhenDone(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys(":sync<CR>")
	h.contains("library synced ·")
}
