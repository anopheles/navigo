package player

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func drain(f *Fake) []Event {
	var out []Event
	for {
		select {
		case e := <-f.Events():
			out = append(out, e)
		default:
			return out
		}
	}
}

func TestFakeFollowsTheWindow(t *testing.T) {
	f := NewFake()
	ctx := context.Background()
	_ = f.Play(ctx, Span{Current: entry(1, "a"), Next: entry(2, "b")})
	f.Tick(time.Second)
	f.Finish()
	_ = f.SetNeighbours(ctx, entry(1, "a"), entry(3, "c"))
	_ = f.Pause(ctx)
	_ = f.SeekBy(ctx, 10*time.Second)

	want := []Event{
		TrackStarted{Entry: 1},
		Position{Entry: 1, Position: time.Second},
		TrackEnded{Entry: 1, Reason: Finished},
		TrackStarted{Entry: 2},
		Paused{Paused: true},
		Position{Entry: 2, Position: 10 * time.Second},
	}
	if got := drain(f); !reflect.DeepEqual(got, want) {
		t.Errorf("events = %#v\nwant %#v", got, want)
	}
	if w := f.Span(); w.Prev.ID != 1 || w.Current.ID != 2 || w.Next.ID != 3 {
		t.Errorf("span = %+v, want 1 2 3", w)
	}
	wantCalls := []string{"Play(0 1 2)", "SetNeighbours(1 3)", "Pause", "SeekBy(10s)"}
	if got := f.Calls(); !reflect.DeepEqual(got, wantCalls) {
		t.Errorf("calls = %q, want %q", got, wantCalls)
	}
}

func TestFakeEndOfQueueAndErrors(t *testing.T) {
	f := NewFake()
	ctx := context.Background()
	_ = f.Play(ctx, Span{Current: entry(1, "a")})
	f.Finish()
	if playing, _, _, _ := f.State(); playing {
		t.Error("still playing after the last entry finished")
	}
	f.Err = errors.New("boom")
	if err := f.Resume(ctx); err == nil {
		t.Error("Resume ignored Err")
	}
	f.Die(errors.New("gone"))
	evs := drain(f)
	if _, ok := evs[len(evs)-1].(Died); !ok {
		t.Errorf("last event = %#v, want Died", evs[len(evs)-1])
	}
}
