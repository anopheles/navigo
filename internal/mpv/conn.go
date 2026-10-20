// Package mpv drives an mpv process over its JSON IPC: a unix socket on
// Linux and macOS, a named pipe on Windows. It knows mpv's protocol, not
// navigo's domain; the player package builds playback on top of it.
package mpv

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
)

// ErrClosed is returned for requests on a connection that has ended.
var ErrClosed = errors.New("mpv: connection closed")

// Event is one asynchronous message from mpv, such as start-file or
// property-change. Fields an event doesn't carry are zero.
type Event struct {
	Name string `json:"event"`

	// property-change: the observer ID and property name. Data is nil when
	// the property is unavailable, such as duration between files.
	ID       int64           `json:"id"`
	Property string          `json:"name"`
	Data     json.RawMessage `json:"data"`

	// start-file and end-file.
	PlaylistEntryID int64  `json:"playlist_entry_id"`
	Reason          string `json:"reason"`
	FileError       string `json:"file_error"`
}

// Conn is one IPC connection. Requests may be sent from any goroutine.
// Events queue without limit, so a slow reader of Events never holds up
// replies.
type Conn struct {
	nc net.Conn

	wmu sync.Mutex // one writer, one Write per message

	mu      sync.Mutex
	cond    *sync.Cond
	nextID  int64
	pending map[int64]chan reply
	queue   []Event
	ended   bool
	err     error

	events chan Event
	done   chan struct{}
	stop   chan struct{}
	once   sync.Once
}

type reply struct {
	data json.RawMessage
	err  error
}

// NewConn takes over nc, which must be connected to mpv's IPC server.
func NewConn(nc net.Conn) *Conn {
	c := &Conn{
		nc:      nc,
		pending: map[int64]chan reply{},
		events:  make(chan Event),
		done:    make(chan struct{}),
		stop:    make(chan struct{}),
	}
	c.cond = sync.NewCond(&c.mu)
	go c.read()
	go c.deliver()
	return c
}

// Events returns the events in the order mpv sent them. It is closed after
// the connection ends and every queued event has been received.
func (c *Conn) Events() <-chan Event { return c.events }

// Done is closed when the connection has ended.
func (c *Conn) Done() <-chan struct{} { return c.done }

// Err is why the connection ended: nil while it is open, ErrClosed after
// Close, otherwise the read error (io.EOF when mpv went away).
func (c *Conn) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

// Close ends the connection. Pending requests fail with ErrClosed.
func (c *Conn) Close() error {
	c.once.Do(func() {
		c.mu.Lock()
		if c.err == nil {
			c.err = ErrClosed
		}
		c.mu.Unlock()
		close(c.stop)
	})
	return c.nc.Close()
}

// Command runs a positional command such as ("seek", 10, "relative") and
// returns its data.
func (c *Conn) Command(ctx context.Context, args ...any) (json.RawMessage, error) {
	return c.send(ctx, args)
}

// Named runs a command in the named-argument form, whose meaning doesn't
// depend on argument positions that change between mpv versions.
func (c *Conn) Named(
	ctx context.Context,
	name string,
	args map[string]any,
) (json.RawMessage, error) {
	cmd := map[string]any{"name": name}
	for k, v := range args {
		cmd[k] = v
	}
	return c.send(ctx, cmd)
}

// Get reads a property into v.
func (c *Conn) Get(ctx context.Context, property string, v any) error {
	data, err := c.Command(ctx, "get_property", property)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// Set sets a property.
func (c *Conn) Set(ctx context.Context, property string, value any) error {
	_, err := c.Command(ctx, "set_property", property, value)
	return err
}

// Observe asks mpv to send a property-change event with id whenever the
// property changes, starting with its current value.
func (c *Conn) Observe(ctx context.Context, id int64, property string) error {
	_, err := c.Command(ctx, "observe_property", id, property)
	return err
}

func (c *Conn) send(ctx context.Context, command any) (json.RawMessage, error) {
	ch := make(chan reply, 1)
	c.mu.Lock()
	if c.ended || c.err != nil {
		err := c.err
		c.mu.Unlock()
		return nil, closedErr(err)
	}
	c.nextID++
	id := c.nextID
	c.pending[id] = ch
	c.mu.Unlock()

	msg, err := json.Marshal(struct {
		Command   any   `json:"command"`
		RequestID int64 `json:"request_id"`
	}{command, id})
	if err != nil {
		c.forget(id)
		return nil, err
	}
	c.wmu.Lock()
	_, err = c.nc.Write(append(msg, '\n'))
	c.wmu.Unlock()
	if err != nil {
		c.forget(id)
		return nil, fmt.Errorf("mpv: write: %w", err)
	}

	select {
	case r := <-ch:
		return r.data, r.err
	case <-ctx.Done():
		c.forget(id)
		return nil, ctx.Err()
	}
}

func (c *Conn) forget(id int64) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

// read parses mpv's lines until the connection ends. Lines that aren't
// JSON objects are skipped.
func (c *Conn) read() {
	r := bufio.NewReader(c.nc)
	var err error
	for {
		var line []byte
		line, err = r.ReadBytes('\n')
		if len(line) > 0 {
			c.handle(line)
		}
		if err != nil {
			break
		}
	}

	c.mu.Lock()
	if c.err == nil {
		if errors.Is(err, net.ErrClosed) || errors.Is(err, io.ErrClosedPipe) {
			err = io.EOF
		}
		c.err = err
	}
	c.ended = true
	for id, ch := range c.pending {
		ch <- reply{err: closedErr(c.err)}
		delete(c.pending, id)
	}
	c.cond.Broadcast()
	c.mu.Unlock()
	close(c.done)
}

func (c *Conn) handle(line []byte) {
	var msg struct {
		Event     string          `json:"event"`
		RequestID *int64          `json:"request_id"`
		Error     string          `json:"error"`
		Data      json.RawMessage `json:"data"`
	}
	if json.Unmarshal(line, &msg) != nil {
		return
	}
	if msg.Event != "" {
		var ev Event
		if json.Unmarshal(line, &ev) != nil {
			return
		}
		c.mu.Lock()
		c.queue = append(c.queue, ev)
		c.cond.Signal()
		c.mu.Unlock()
		return
	}
	if msg.RequestID == nil {
		return
	}
	c.mu.Lock()
	ch, ok := c.pending[*msg.RequestID]
	delete(c.pending, *msg.RequestID)
	c.mu.Unlock()
	if !ok {
		return
	}
	if msg.Error != "success" {
		ch <- reply{err: fmt.Errorf("mpv: %s", msg.Error)}
		return
	}
	ch <- reply{data: msg.Data}
}

// deliver hands queued events to Events until the connection has ended and
// the queue is empty, or Close is called.
func (c *Conn) deliver() {
	defer close(c.events)
	quit := make(chan struct{})
	defer close(quit)
	go func() {
		select {
		case <-c.stop:
		case <-quit:
			return
		}
		c.mu.Lock()
		c.cond.Broadcast()
		c.mu.Unlock()
	}()
	for {
		c.mu.Lock()
		for len(c.queue) == 0 && !c.ended && !c.stopped() {
			c.cond.Wait()
		}
		if len(c.queue) == 0 || c.stopped() {
			c.mu.Unlock()
			return
		}
		ev := c.queue[0]
		c.queue = c.queue[1:]
		c.mu.Unlock()
		select {
		case c.events <- ev:
		case <-c.stop:
			return
		}
	}
}

func (c *Conn) stopped() bool {
	select {
	case <-c.stop:
		return true
	default:
		return false
	}
}

func closedErr(err error) error {
	if err == nil || errors.Is(err, ErrClosed) {
		return ErrClosed
	}
	return fmt.Errorf("%w: %w", ErrClosed, err)
}
