// Package listenbrainz reads the user's Recommendations from ListenBrainz
// (ADR 0008): the recommendation list, then the names of its recordings.
// It also keeps the last list in a small file beside the library cache.
// ListenBrainz calls the endpoint experimental, so replies are decoded
// leniently: only the recording MBIDs are required.
package listenbrainz

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/anopheles/navigo/internal/library"
)

// DefaultURL is the ListenBrainz API navigo asks unless config names another.
const DefaultURL = "https://api.listenbrainz.org"

// Timeout is how long one request may take, as for the Subsonic API.
const Timeout = 10 * time.Second

// maxWait caps how long a 429 waits before its one retry.
const maxWait = 30 * time.Second

var (
	// ErrNoUser: ListenBrainz has no such user (404).
	ErrNoUser = errors.New("ListenBrainz has no such user")
	// ErrNotGenerated: ListenBrainz hasn't made Recommendations for the
	// user yet (204).
	ErrNotGenerated = errors.New("ListenBrainz has no Recommendations for the user yet")
)

// RateLimited is a 429; ResetIn is when ListenBrainz says to try again.
type RateLimited struct{ ResetIn time.Duration }

func (e RateLimited) Error() string {
	return fmt.Sprintf("ListenBrainz rate limit: try again in %s", e.ResetIn)
}

// Client talks to one ListenBrainz API, without a token.
type Client struct {
	base string
	http *http.Client
}

// New returns a client for the API at base, such as DefaultURL.
func New(base string) *Client {
	return &Client{
		base: strings.TrimRight(base, "/"),
		http: &http.Client{Timeout: Timeout},
	}
}

// List is the user's recommendation list as ListenBrainz sends it: when it
// was made, and the recordings, best first.
type List struct {
	LastUpdated time.Time
	MBIDs       []string
}

// Recommendations fetches the user's list, all 1,000 at once.
func (c *Client) Recommendations(ctx context.Context, user string) (List, error) {
	var body struct {
		Payload struct {
			LastUpdated int64 `json:"last_updated"`
			MBIDs       []struct {
				MBID string `json:"recording_mbid"`
			} `json:"mbids"`
		} `json:"payload"`
	}
	path := "/1/cf/recommendation/user/" + url.PathEscape(user) + "/recording?count=1000"
	if err := c.do(ctx, http.MethodGet, path, nil, &body); err != nil {
		return List{}, err
	}
	l := List{LastUpdated: time.Unix(body.Payload.LastUpdated, 0).UTC()}
	for _, m := range body.Payload.MBIDs {
		if m.MBID != "" {
			l.MBIDs = append(l.MBIDs, m.MBID)
		}
	}
	return l, nil
}

// Name looks up the title and artists of each recording in one request.
// Recordings ListenBrainz doesn't know, or sends without a title or
// artist, are left out.
func (c *Client) Name(ctx context.Context, mbids []string) ([]library.Recommendation, error) {
	if len(mbids) == 0 {
		return nil, nil
	}
	req, err := json.Marshal(struct {
		MBIDs []string `json:"recording_mbids"`
		Inc   string   `json:"inc"`
	}{mbids, "artist"})
	if err != nil {
		return nil, err
	}
	var body map[string]struct {
		Recording struct {
			Name string `json:"name"`
		} `json:"recording"`
		Artist struct {
			Name    string `json:"name"`
			Artists []struct {
				Name string `json:"name"`
			} `json:"artists"`
		} `json:"artist"`
	}
	if err := c.do(ctx, http.MethodPost, "/1/metadata/recording/", req, &body); err != nil {
		return nil, err
	}
	var out []library.Recommendation
	for _, id := range mbids {
		m, ok := body[id]
		if !ok || m.Recording.Name == "" {
			continue
		}
		r := library.Recommendation{MBID: id, Title: m.Recording.Name}
		if m.Artist.Name != "" {
			r.Artists = append(r.Artists, m.Artist.Name)
		}
		for _, a := range m.Artist.Artists {
			if a.Name != "" {
				r.Artists = append(r.Artists, a.Name)
			}
		}
		if len(r.Artists) > 0 {
			out = append(out, r)
		}
	}
	return out, nil
}

// Fetch returns the user's Recommendations, named. When the list's
// last_updated is old's, it stops after one request and returns old's
// Recommendations with changed false. A 429 waits as told and tries once
// more.
func (c *Client) Fetch(
	ctx context.Context,
	user string,
	old *Stored,
) (s Stored, changed bool, err error) {
	var l List
	err = c.retry(ctx, func() (err error) { l, err = c.Recommendations(ctx, user); return })
	if err != nil {
		return Stored{}, false, err
	}
	s = Stored{URL: c.base, User: user, LastUpdated: l.LastUpdated}
	if old != nil && old.URL == c.base && old.User == user && old.LastUpdated.Equal(l.LastUpdated) {
		s.Recs = old.Recs
		return s, false, nil
	}
	err = c.retry(ctx, func() (err error) { s.Recs, err = c.Name(ctx, l.MBIDs); return })
	if err != nil {
		return Stored{}, false, err
	}
	return s, true, nil
}

// retry runs f, and once more after a 429's wait.
func (c *Client) retry(ctx context.Context, f func() error) error {
	err := f()
	var rl RateLimited
	if !errors.As(err, &rl) {
		return err
	}
	if err := sleep(ctx, min(rl.ResetIn, maxWait)); err != nil {
		return err
	}
	return f()
}

func (c *Client) do(ctx context.Context, method, path string, body []byte, v any) error {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, r)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "navigo")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNoContent:
		return ErrNotGenerated
	case resp.StatusCode == http.StatusNotFound:
		return ErrNoUser
	case resp.StatusCode == http.StatusTooManyRequests:
		secs, err := strconv.Atoi(resp.Header.Get("X-RateLimit-Reset-In"))
		if err != nil || secs < 0 {
			secs = 10 // ListenBrainz's window
		}
		return RateLimited{ResetIn: time.Duration(secs) * time.Second}
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("ListenBrainz answered %s", resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("ListenBrainz sent a reply navigo can't read: %w", err)
	}
	return nil
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
