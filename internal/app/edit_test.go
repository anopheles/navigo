package app

import (
	"testing"

	"github.com/anopheles/navigo/internal/player"
)

// ids lists the queue's entry IDs.
func ids(s State) []player.EntryID {
	out := make([]player.EntryID, len(s.Queue))
	for i, q := range s.Queue {
		out[i] = q.ID
	}
	return out
}

func sameIDs(a []player.EntryID, b ...player.EntryID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// playing4 is a queue of four entries with the second one playing.
func playing4(t *testing.T) State {
	t.Helper()
	s, _ := run(
		New(),
		PlayNow{Tracks: tracks(4), Start: 1},
		FromPlayer{player.TrackStarted{Entry: 2}},
	)
	return s
}

func TestRemoveAndPaste(t *testing.T) {
	s := playing4(t)
	// Removing the next one: the player's next follows.
	s, out := run(s, Remove{From: 2, To: 3})
	if !sameIDs(ids(s), 1, 2, 4) || s.Current != 1 {
		t.Fatalf("queue %v current %d", ids(s), s.Current)
	}
	if sn := only[SetNeighbours](
		out,
	); len(sn) != 1 ||
		sn[0] != (SetNeighbours{Prev: entry(1, "t1"), Next: entry(4, "t4")}) {
		t.Errorf("neighbours = %#v", sn)
	}
	if n := only[Notice](
		out,
	); n[0].Text != "1 track removed (3 in queue)" ||
		n[0].Command != "undo" {
		t.Errorf("notice = %#v", n)
	}
	if len(s.Register) != 1 || s.Register[0].ID != "t3" {
		t.Errorf("register = %v", s.Register)
	}

	// Pasting twice before the first row: new entries, current follows.
	s, out = run(s, Paste{At: 0, Times: 2})
	if !sameIDs(ids(s), 5, 6, 1, 2, 4) || s.Current != 3 {
		t.Errorf("after paste: queue %v current %d", ids(s), s.Current)
	}
	if n := only[Notice](out); n[0].Text != "2 tracks pasted (5 in queue)" {
		t.Errorf("notice = %q", n[0].Text)
	}

	// Every edit undoes.
	s, _ = run(s, Undo{}, Undo{})
	if !sameIDs(ids(s), 1, 2, 3, 4) || s.Current != 1 {
		t.Errorf("after undo: queue %v current %d", ids(s), s.Current)
	}
}

func TestRemoveThePlayingEntry(t *testing.T) {
	s := playing4(t)
	s, out := run(s, Remove{From: 1, To: 3})
	if !sameIDs(ids(s), 1, 4) || s.Current != 0 {
		t.Fatalf("queue %v current %d", ids(s), s.Current)
	}
	// It plays on, then the queue goes on with what followed it.
	if sn := only[SetNeighbours](
		out,
	); len(sn) != 1 ||
		sn[0] != (SetNeighbours{Prev: entry(1, "t1"), Next: entry(4, "t4")}) {
		t.Errorf("neighbours = %#v", sn)
	}
	s, _ = run(s, FromPlayer{player.TrackStarted{Entry: 4}})
	if s.Current != 1 {
		t.Errorf("after moving on: current %d", s.Current)
	}

	// Removing the rest leaves it to finish alone.
	s = playing4(t)
	if _, out = run(s, Remove{From: 1, To: 4}); only[SetNeighbours](out)[0] != (SetNeighbours{}) {
		t.Errorf("neighbours = %#v", only[SetNeighbours](out))
	}
}

func TestYankAndPasteEmpty(t *testing.T) {
	s, out := run(New(), Paste{At: 0})
	if n := only[Notice](out); len(n) != 1 || len(s.Queue) != 0 || s.CanUndo() {
		t.Errorf("paste with an empty register: %#v", n)
	}
	s, out = run(s, Yank{Tracks: tracks(3)})
	if n := only[Notice](
		out,
	); n[0].Text != "3 tracks yanked" || n[0].Command != "paste-after" ||
		s.CanUndo() {
		t.Errorf("yank: %#v", n)
	}
	s, _ = run(s, Paste{At: 0})
	if len(s.Queue) != 3 || s.Current != -1 {
		t.Errorf("paste into an empty queue: %v current %d", ids(s), s.Current)
	}
}

func TestMove(t *testing.T) {
	s := playing4(t)
	s, out := run(s, Move{From: 1, To: 3, By: 1})
	if !sameIDs(ids(s), 1, 4, 2, 3) || s.Current != 2 {
		t.Fatalf("queue %v current %d", ids(s), s.Current)
	}
	if sn := only[SetNeighbours](
		out,
	); sn[0] != (SetNeighbours{Prev: entry(4, "t4"), Next: entry(3, "t3")}) {
		t.Errorf("neighbours = %#v", sn)
	}
	if n := only[Notice](out); n[0].Text != "moved 2 tracks down" {
		t.Errorf("notice = %q", n[0].Text)
	}
	// Clamped at the ends; nothing to do is no undo step.
	s, _ = run(s, Move{From: 0, To: 1, By: -3})
	if !sameIDs(ids(s), 1, 4, 2, 3) {
		t.Errorf("moving the top up: %v", ids(s))
	}
	s, _ = run(s, Move{From: 3, To: 4, By: -9}, Undo{})
	if !sameIDs(ids(s), 1, 4, 2, 3) {
		t.Errorf("after undoing a clamped move: %v", ids(s))
	}
}

func TestClearQueue(t *testing.T) {
	s, out := run(playing4(t), ClearQueue{})
	if len(s.Queue) != 0 || s.Current != -1 {
		t.Fatalf("queue %v current %d", ids(s), s.Current)
	}
	if n := only[Notice](out); n[0].Text != "queue cleared (4 tracks)" || n[0].Command != "undo" {
		t.Errorf("notice = %#v", n)
	}
	if sn := only[SetNeighbours](out); sn[0] != (SetNeighbours{}) {
		t.Errorf("neighbours = %#v", sn)
	}
	if s, _ = run(s, Undo{}); !sameIDs(ids(s), 1, 2, 3, 4) || s.Current != 1 {
		t.Errorf("after undo: %v current %d", ids(s), s.Current)
	}
}
