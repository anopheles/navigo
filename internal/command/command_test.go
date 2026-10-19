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
