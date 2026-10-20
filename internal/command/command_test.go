package command

import (
	"errors"
	"reflect"
	"testing"
)

type target struct{ got []Call }

func registry() *Registry[*target] {
	r := NewRegistry[*target]()
	record := func(t *target, c Call) error { t.got = append(t.got, c); return nil }
	r.Add(Command[*target]{Name: "quit", Aliases: []string{"q", "qa"}, Label: "quit", Run: record})
	r.Add(Command[*target]{Name: "seek", Label: "seek", Run: record})
	return r
}

func TestRun(t *testing.T) {
	r := registry()
	tg := &target{}
	for _, line := range []string{":q", "seek +10", "  :seek   -5 "} {
		if err := r.Run(tg, line, 3); err != nil {
			t.Fatalf("Run(%q): %v", line, err)
		}
	}
	want := []Call{
		{Name: "quit", Args: []string{}, Count: 3},
		{Name: "seek", Args: []string{"+10"}, Count: 3},
		{Name: "seek", Args: []string{"-5"}, Count: 3},
	}
	if !reflect.DeepEqual(tg.got, want) {
		t.Errorf("calls = %+v, want %+v", tg.got, want)
	}
}

func TestRunErrors(t *testing.T) {
	r := registry()
	if err := r.Run(&target{}, "nope", 0); !errors.Is(err, ErrUnknown) {
		t.Errorf("unknown command: %v", err)
	}
	if err := r.Run(&target{}, " : ", 0); err == nil {
		t.Error("empty line ran")
	}
}

func TestN(t *testing.T) {
	if (Call{}).N() != 1 || (Call{Count: 4}).N() != 4 {
		t.Error("N should default to 1")
	}
}

func TestDuplicatePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("duplicate alias did not panic")
		}
	}()
	r := registry()
	r.Add(Command[*target]{Name: "q"})
}

func TestMatch(t *testing.T) {
	r := NewRegistry[*target]()
	for _, c := range []Command[*target]{
		{Name: "play-now", Desc: "Replace the queue with this list and play from the track under the cursor."},
		{Name: "play-pause", Desc: "Pause or resume playback."},
		{Name: "add-end", Desc: "Add the track under the cursor to the end of the queue."},
		{Name: "queue", Desc: "Move to the queue window."},
		{Name: "quit", Aliases: []string{"q"}, Desc: "Quit navigo."},
		{Name: "next-track", Desc: "Play the next track in the queue."},
	} {
		r.Add(c)
	}
	names := func(cs []*Command[*target]) []string {
		var out []string
		for _, c := range cs {
			out = append(out, c.Name)
		}
		return out
	}
	nextHere := func(c *Command[*target]) bool { return c.Name == "next-track" }
	for _, c := range []struct {
		query string
		here  func(*Command[*target]) bool
		want  []string
	}{
		// Name prefixes keep registry order, then whole words: "Play the
		// next track".
		{"pla", nil, []string{"play-now", "play-pause", "next-track"}},
		// What applies here comes first, whatever its match.
		{"pla", nextHere, []string{"next-track", "play-now", "play-pause"}},
		// Whole words of names and descriptions, then scattered letters.
		{"queue", nil, []string{"queue", "play-now", "add-end", "next-track"}},
		{"end of", nil, []string{"add-end"}},
		{"ae", nil, []string{"play-pause", "add-end"}},
		{"nw", nil, []string{"play-now"}},
		{"Q", nil, []string{"queue", "quit", "play-now", "add-end", "next-track"}},
		{"zzz", nil, nil},
		{"  ", nil, nil},
	} {
		if got := names(r.Match(c.query, c.here)); !reflect.DeepEqual(got, c.want) {
			t.Errorf("Match(%q) = %q, want %q", c.query, got, c.want)
		}
	}
}
