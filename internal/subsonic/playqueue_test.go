package subsonic

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/anopheles/navigo/internal/subsonic/subsonictest"
)

func TestPlayQueueRoundTrip(t *testing.T) {
	c, srv := newClient(t)
	ctx := context.Background()
	if q, err := c.PlayQueue(ctx); err != nil || len(q.IDs) != 0 || q.Current != -1 {
		t.Fatalf("before any save: %+v, %v", q, err)
	}
	if err := c.SavePlayQueue(
		ctx,
		[]string{"a", "b", "c"},
		1,
		134500*time.Millisecond,
	); err != nil {
		t.Fatal(err)
	}
	q, err := c.PlayQueue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(q.IDs, []string{"a", "b", "c"}) || q.Current != 1 ||
		q.Position != 134500*time.Millisecond {
		t.Errorf("queue = %+v", q)
	}
	if held, _ := srv.PlayQueue(subsonictest.Username); held.ChangedBy != "navigo" {
		t.Errorf("changed by %q, want navigo", held.ChangedBy)
	}
	if err := c.SavePlayQueue(ctx, nil, -1, 0); err != nil {
		t.Fatal(err)
	}
	if _, ok := srv.PlayQueue(subsonictest.Username); ok {
		t.Error("saving no ids left a queue on the server")
	}
}

func TestPlayQueueErrors(t *testing.T) {
	c, srv := newClient(t)
	srv.SetPlayQueueFault(subsonictest.PlayQueueMissing)
	_, err := c.PlayQueue(context.Background())
	if err == nil || IsUnreachable(err) {
		t.Errorf("a server without the call: %v, want an error that isn't unreachable", err)
	}
	srv.SetPlayQueueFault(subsonictest.PlayQueueBroken)
	if err := c.SavePlayQueue(context.Background(), []string{"a"}, -1, 0); !IsUnreachable(err) {
		t.Errorf("HTTP 500: %v, want unreachable", err)
	}
}

func TestLongPlayQueueGoesOutAsPost(t *testing.T) {
	srv := subsonictest.NewServer(t)
	// No Login, so the client doesn't know the server offers formPost.
	c := New(Credentials{
		URL: srv.URL, Username: subsonictest.Username, Password: subsonictest.Password,
	})
	ids := make([]string, 600)
	for i := range ids {
		ids[i] = strings.Repeat("x", 20) + strings.Repeat("y", i%5)
	}
	ctx := context.Background()
	if err := c.SavePlayQueue(ctx, ids[:5], 0, 0); err != nil {
		t.Fatal(err)
	}
	if err := c.SavePlayQueue(ctx, ids, 0, 0); err != nil {
		t.Fatal(err)
	}
	var methods []string
	for _, r := range srv.Requests() {
		methods = append(methods, r.Method)
	}
	if got := strings.Join(methods, " "); got != "GET POST" {
		t.Errorf("methods = %s, want GET POST", got)
	}
	if q, _ := srv.PlayQueue(subsonictest.Username); len(q.IDs) != 600 {
		t.Errorf("the server kept %d of 600 ids", len(q.IDs))
	}
}

func TestPlayQueueKeepsWhichCopyIsCurrentWhenTheServerKeepsAnIndex(t *testing.T) {
	c, srv := newClient(t) // logs in, so it learns the server offers indexBasedQueue
	ctx := context.Background()
	ids := []string{"a", "b", "a"}
	if err := c.SavePlayQueue(ctx, ids, 2, 9*time.Second); err != nil {
		t.Fatal(err)
	}
	q, err := c.PlayQueue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(q.IDs, ids) || q.Current != 2 || q.Position != 9*time.Second {
		t.Errorf("queue = %+v, want the second copy of a current", q)
	}
	seen := map[string]bool{}
	for _, r := range srv.Requests() {
		seen[r.Endpoint] = true
	}
	if !seen["savePlayQueueByIndex"] || !seen["getPlayQueueByIndex"] || seen["savePlayQueue"] {
		t.Errorf("endpoints used: %v", seen)
	}
}

func TestPlayQueueWithoutIndexSupportFallsBackToTheFirstCopy(t *testing.T) {
	srv := subsonictest.NewServer(t)
	// No Login, so the client doesn't know the server offers indexBasedQueue.
	c := New(Credentials{
		URL: srv.URL, Username: subsonictest.Username, Password: subsonictest.Password,
	})
	ctx := context.Background()
	if err := c.SavePlayQueue(ctx, []string{"a", "b", "a"}, 2, 9*time.Second); err != nil {
		t.Fatal(err)
	}
	q, err := c.PlayQueue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if q.Current != 0 {
		t.Errorf("current = %d, want 0: the ID-based calls can't tell the copies apart", q.Current)
	}
	for _, r := range srv.Requests() {
		if strings.HasSuffix(r.Endpoint, "ByIndex") {
			t.Errorf("used %s on a server not known to offer it", r.Endpoint)
		}
	}
}
