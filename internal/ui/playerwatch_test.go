package ui

import (
	"testing"
	"time"
)

func now() <-chan time.Time {
	c := make(chan time.Time, 1)
	c <- time.Time{}
	return c
}

func never() <-chan time.Time { return nil }

func TestPlayerWatch(t *testing.T) {
	// Answered in time: never reported.
	w := newPlayerWatch()
	w.start()
	if w.answered() || w.wait(never) {
		t.Error("a prompt call was reported stuck")
	}

	// Slow: reported stuck, then answered, which must be said.
	w = newPlayerWatch()
	w.start()
	if !w.wait(now) {
		t.Fatal("a slow call wasn't reported")
	}
	if !w.answered() {
		t.Error("the answer after a stuck report wasn't passed on")
	}

	// Answered just as time ran out: both ready, never left stuck.
	for range 100 {
		w = newPlayerWatch()
		w.start()
		answeredLate := w.answered()
		if w.wait(now) || answeredLate {
			t.Fatal("a call that answered was left stuck")
		}
	}
}
