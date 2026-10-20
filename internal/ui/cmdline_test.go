package ui

import (
	"strings"
	"testing"
)

// lineIndex is the screen line holding s, or -1.
func (h *harness) lineIndex(s string) int {
	for i, l := range strings.Split(h.screen(), "\n") {
		if strings.Contains(l, s) {
			return i
		}
	}
	return -1
}

func TestCmdlineMatches(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys(":")
	h.typeText("que")
	if l := h.lineIndex(
		"Move to the queue window.",
	); l < 0 || !strings.Contains(strings.Split(h.screen(), "\n")[l], " queue ") ||
		!strings.Contains(strings.Split(h.screen(), "\n")[l], "'q") {
		t.Errorf("no queue row with its key:\n%s", h.screen())
	}

	// On a track, the commands for tracks come first, then the rest.
	h.keys("<Esc><CR>")
	h.keys(":")
	h.typeText("queue")
	addEnd, queue := h.lineIndex("add-end"), h.lineIndex("Move to the queue window.")
	if addEnd < 0 || queue < 0 || addEnd > queue {
		t.Errorf(
			"add-end on line %d, queue on %d: want what applies here first:\n%s",
			addEnd,
			queue,
			h.screen(),
		)
	}
	if !strings.HasSuffix(strings.Split(h.screen(), "\n")[addEnd], " A ") {
		t.Errorf("add-end's key A not listed:\n%s", h.screen())
	}

	// Arguments are no longer names: the list goes.
	h.typeText(" x")
	h.lacks("Move to the queue window.")
}

func TestCmdlineComplete(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys(":")
	h.typeText("alb")
	h.keys("<Tab>")
	if v := h.m.line.Value(); v != "albums" {
		t.Errorf("<Tab> completed %q", v)
	}
	h.keys("<Esc>:")
	h.typeText("se")
	h.keys("<Tab>")
	first := h.m.line.Value()
	h.keys("<Tab>")
	second := h.m.line.Value()
	h.keys("<S-Tab>")
	if first == second || h.m.line.Value() != first || !strings.HasPrefix(first, "se") ||
		!strings.HasPrefix(second, "se") {
		t.Errorf("<Tab> gave %q then %q, <S-Tab> back to %q", first, second, h.m.line.Value())
	}
	h.keys("<Esc>")

	// Arguments: :search completes from the search history, newest first.
	h.keys(":search blue<CR>:search quiet<CR>:")
	h.typeText("search ")
	h.keys("<Tab>")
	if v := h.m.line.Value(); v != "search quiet" {
		t.Errorf("first argument completion %q", v)
	}
	h.keys("<Tab>")
	if v := h.m.line.Value(); v != "search blue" {
		t.Errorf("second argument completion %q", v)
	}
	h.keys("<BS><BS><BS><BS>q<Tab>")
	if v := h.m.line.Value(); v != "search quiet" {
		t.Errorf("completion of \"q\" = %q", v)
	}
}

func TestCmdlineTips(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys(":albums<CR>")
	h.contains("tip: next time press 'b")

	// After what the command says itself.
	h.keys("<CR>:add-end<CR>")
	h.contains(
		"1 track added to the end of the queue (1 in queue) · u undoes · tip: next time press A",
	)
	h.keys(":messages<CR>")
	h.contains("u undoes · tip: next time press A")
	h.keys("q<Esc>")

	// No tip when no key does exactly that, or when the command failed.
	h.keys(":seek +30<CR>")
	h.lacks("tip:")
	h.keys(":seek x<CR>")
	h.lacks("tip:")
}

func TestCmdlineHistory(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys(":albums<CR>:queue<CR>:")
	h.typeText("dra")
	h.keys("<Up>")
	if v := h.m.line.Value(); v != "queue" {
		t.Errorf("<Up> = %q", v)
	}
	h.keys("<Up><Up>")
	if v := h.m.line.Value(); v != "albums" {
		t.Errorf("<Up><Up><Up> = %q, want the oldest", v)
	}
	h.keys("<Down><Down>")
	if v := h.m.line.Value(); v != "dra" {
		t.Errorf("<Down> back to %q, want the draft", v)
	}
	h.keys("<Esc>")

	n := h.restart()
	n.keys(":<Up>")
	if v := n.m.line.Value(); v != "queue" {
		t.Errorf("after a restart, <Up> = %q", v)
	}
}
