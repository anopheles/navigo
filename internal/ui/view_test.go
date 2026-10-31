package ui

import "testing"

func TestWindowTitle(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	if got := h.m.View().WindowTitle; got != "navigo" {
		t.Errorf("window title %q, want navigo", got)
	}
}
