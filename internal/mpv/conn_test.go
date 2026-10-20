package mpv

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeMPV is the far end of a Conn. Each request goes to handle, which
// writes whatever lines it likes; a nil handle echoes the first argument.
type fakeMPV struct {
	t    *testing.T
	nc   net.Conn
	wmu  sync.Mutex
	mu   sync.Mutex
	seen []map[string]any
}

func newFake(t *testing.T, handle func(f *fakeMPV, id int64, cmd []any)) (*Conn, *fakeMPV) {
	t.Helper()
	a, b := net.Pipe()
	f := &fakeMPV{t: t, nc: b}
	if handle == nil {
		handle = func(f *fakeMPV, id int64, cmd []any) { f.reply(id, "success", cmd[1]) }
	}
	go func() {
		r := bufio.NewReader(b)
		for {
			line, err := r.ReadBytes('\n')
			if err != nil {
				return
			}
			var msg map[string]any
			if err := json.Unmarshal(line, &msg); err != nil {
				t.Errorf("mpv got a line that isn't JSON: %q", line)
				return
			}
			f.mu.Lock()
			f.seen = append(f.seen, msg)
			f.mu.Unlock()
			cmd, _ := msg["command"].([]any)
			go handle(f, int64(msg["request_id"].(float64)), cmd)
		}
	}()
	c := NewConn(a)
	t.Cleanup(func() { c.Close(); b.Close() })
	return c, f
}

func (f *fakeMPV) send(line string) {
	f.wmu.Lock()
	defer f.wmu.Unlock()
	_, _ = f.nc.Write([]byte(line + "\n"))
}

func (f *fakeMPV) reply(id int64, status string, data any) {
	b, _ := json.Marshal(map[string]any{"request_id": id, "error": status, "data": data})
	f.send(string(b))
}

func TestRepliesMatchRequests(t *testing.T) {
	c, _ := newFake(t, nil)
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			data, err := c.Command(context.Background(), "echo", i)
			if err != nil || string(data) != fmt.Sprint(i) {
				t.Errorf("echo %d = %s, %v", i, data, err)
			}
		}()
	}
	wg.Wait()
}

func TestEventsInterleaveWithReplies(t *testing.T) {
	c, _ := newFake(t, func(f *fakeMPV, id int64, cmd []any) {
		f.send(`{"event":"start-file","playlist_entry_id":7}`)
		f.send(`not json at all`)
		f.send(
			"{\"event\":\"property-change\",\"id\":3,\"name\":\"media-title\",\"data\":\"bad \xff utf8\"}",
		)
		f.send(`{"event":"property-change","id":2,"name":"duration"}`)
		f.reply(id, "success", 1)
	})
	if _, err := c.Command(context.Background(), "x"); err != nil {
		t.Fatal(err)
	}
	want := []Event{
		{Name: "start-file", PlaylistEntryID: 7},
		{
			Name:     "property-change",
			ID:       3,
			Property: "media-title",
			Data:     json.RawMessage("\"bad \xff utf8\""),
		},
		{Name: "property-change", ID: 2, Property: "duration"},
	}
	for _, w := range want {
		got := <-c.Events()
		if got.Name != w.Name || got.ID != w.ID || got.PlaylistEntryID != w.PlaylistEntryID ||
			string(got.Data) != string(w.Data) {
			t.Errorf("event = %+v, want %+v", got, w)
		}
	}
}

func TestEventsDontBlockReplies(t *testing.T) {
	c, _ := newFake(t, func(f *fakeMPV, id int64, cmd []any) {
		for range 1000 {
			f.send(`{"event":"seek"}`)
		}
		f.reply(id, "success", nil)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := c.Command(ctx, "x"); err != nil {
		t.Fatalf("reply stuck behind unread events: %v", err)
	}
}

func TestLargeLine(t *testing.T) {
	big := strings.Repeat("a", 1<<20)
	c, _ := newFake(t, func(f *fakeMPV, id int64, cmd []any) { f.reply(id, "success", big) })
	var got string
	data, err := c.Command(context.Background(), "x")
	if err == nil {
		err = json.Unmarshal(data, &got)
	}
	if err != nil || got != big {
		t.Fatalf("1 MiB reply: len %d, %v", len(got), err)
	}
}

func TestMPVError(t *testing.T) {
	c, _ := newFake(
		t,
		func(f *fakeMPV, id int64, cmd []any) { f.reply(id, "property unavailable", nil) },
	)
	var v float64
	if err := c.Get(
		context.Background(),
		"duration",
		&v,
	); err == nil ||
		!strings.Contains(err.Error(), "property unavailable") {
		t.Fatalf("Get = %v, want mpv's error", err)
	}
}

func TestNamedCommand(t *testing.T) {
	c, f := newFake(
		t,
		func(f *fakeMPV, id int64, cmd []any) { f.reply(id, "success", map[string]int{"playlist_entry_id": 4}) },
	)
	data, err := c.Named(
		context.Background(),
		"loadfile",
		map[string]any{"url": "http://x", "flags": "append"},
	)
	if err != nil || string(data) != `{"playlist_entry_id":4}` {
		t.Fatalf("Named = %s, %v", data, err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	cmd := f.seen[0]["command"].(map[string]any)
	if cmd["name"] != "loadfile" || cmd["url"] != "http://x" || cmd["flags"] != "append" {
		t.Errorf("sent %v", cmd)
	}
}

func TestMPVDiesFailsPendingRequests(t *testing.T) {
	c, f := newFake(t, func(f *fakeMPV, id int64, cmd []any) {
		f.send(`{"event":"shutdown"}`)
		f.nc.Close()
	})
	_, err := c.Command(context.Background(), "x")
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("Command = %v, want ErrClosed", err)
	}
	<-c.Done()
	if ev := <-c.Events(); ev.Name != "shutdown" {
		t.Errorf("queued event = %+v, want shutdown", ev)
	}
	if _, ok := <-c.Events(); ok {
		t.Error("Events still open after the connection ended")
	}
	if _, err := c.Command(context.Background(), "x"); !errors.Is(err, ErrClosed) {
		t.Errorf("Command after end = %v, want ErrClosed", err)
	}
	_ = f
}

func TestContextCancel(t *testing.T) {
	c, _ := newFake(t, func(f *fakeMPV, id int64, cmd []any) {})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := c.Command(ctx, "x"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Command = %v, want deadline exceeded", err)
	}
}
