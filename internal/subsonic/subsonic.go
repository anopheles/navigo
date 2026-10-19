// Package subsonic is navigo's thin client for the Subsonic/OpenSubsonic
// API. It speaks JSON, signs every request and every stream URL with a
// fresh token+salt, and holds the password in memory only.
//
// The types here mirror the server's responses; mapping them into the
// library snapshot is the library package's job.
package subsonic

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

const (
	apiVersion = "1.16.1"
	clientName = "navigo"
)

// pageSize is Navidrome's cap on list endpoints.
const pageSize = 500

// Credentials are what navigo logs in with.
type Credentials struct {
	URL      string
	Username string
	Password string
}

// Error codes from the Subsonic spec that navigo acts on.
const (
	CodeWrongCredentials  = 40
	CodeTokenAuthDisabled = 41
	CodeNotFound          = 70
)

// Error is a "failed" response from the server.
type Error struct {
	Code    int
	Message string
}

func (e *Error) Error() string {
	switch e.Code {
	case CodeTokenAuthDisabled:
		return "the server does not accept token authentication, which navigo needs"
	}
	return fmt.Sprintf("server error %d: %s", e.Code, e.Message)
}

// IsCode reports whether err is a server Error with the given code.
func IsCode(err error, code int) bool {
	var e *Error
	return errors.As(err, &e) && e.Code == code
}

// Client talks to one server as one user. It is safe for concurrent use.
type Client struct {
	creds    Credentials
	base     string
	http     *http.Client
	formPost atomic.Bool
}

// New returns a client for creds. It makes no request; call Login to check
// the credentials and learn what the server supports.
func New(creds Credentials) *Client {
	return &Client{
		creds: creds,
		base:  strings.TrimRight(creds.URL, "/") + "/rest/",
		http:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Login checks the credentials with ping and reads the server's
// OpenSubsonic extensions. A wrong user or password returns an Error with
// CodeWrongCredentials.
func (c *Client) Login(ctx context.Context) error {
	if err := c.call(ctx, "ping", nil, nil); err != nil {
		return err
	}
	var r struct {
		OpenSubsonicExtensions []struct{ Name string } `json:"openSubsonicExtensions"`
	}
	// Servers without OpenSubsonic don't know the endpoint; they get the
	// plain Subsonic behaviour.
	if err := c.call(ctx, "getOpenSubsonicExtensions", nil, &r); err != nil {
		return nil
	}
	for _, e := range r.OpenSubsonicExtensions {
		if e.Name == "formPost" {
			c.formPost.Store(true)
		}
	}
	return nil
}

// Album is an album as the server lists it. Songs is filled only by Album.
type Album struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Artist        string `json:"artist"`
	ArtistID      string `json:"artistId"`
	Year          int    `json:"year"`
	Genre         string `json:"genre"`
	SongCount     int    `json:"songCount"`
	Duration      int    `json:"duration"`
	IsCompilation bool   `json:"isCompilation"`
	CoverArt      string `json:"coverArt"`
	Songs         []Song `json:"song"`
}

// Song is one track as the server returns it. Durations are in seconds.
type Song struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Album       string `json:"album"`
	AlbumID     string `json:"albumId"`
	Artist      string `json:"artist"`
	ArtistID    string `json:"artistId"`
	AlbumArtist string `json:"displayAlbumArtist"`
	Track       int    `json:"track"`
	DiscNumber  int    `json:"discNumber"`
	Year        int    `json:"year"`
	Genre       string `json:"genre"`
	Duration    int    `json:"duration"`
	CoverArt    string `json:"coverArt"`
}

// ListType orders an album list.
type ListType string

// The album list orders navigo uses.
const (
	ByName ListType = "alphabeticalByName"
	Newest ListType = "newest"
)

// Albums returns every album in the given order, fetching page by page.
func (c *Client) Albums(ctx context.Context, order ListType) ([]Album, error) {
	var all []Album
	for offset := 0; ; offset += pageSize {
		var r struct {
			AlbumList2 struct {
				Album []Album `json:"album"`
			} `json:"albumList2"`
		}
		params := url.Values{"type": {string(order)}, "size": {strconv.Itoa(pageSize)}, "offset": {strconv.Itoa(offset)}}
		if err := c.call(ctx, "getAlbumList2", params, &r); err != nil {
			return nil, err
		}
		all = append(all, r.AlbumList2.Album...)
		if len(r.AlbumList2.Album) < pageSize {
			return all, nil
		}
	}
}

// Album returns one album with its songs.
func (c *Client) Album(ctx context.Context, id string) (Album, error) {
	var r struct {
		Album Album `json:"album"`
	}
	err := c.call(ctx, "getAlbum", url.Values{"id": {id}}, &r)
	return r.Album, err
}

// Scrobble reports a song as started (submission false) or as played
// (submission true).
func (c *Client) Scrobble(ctx context.Context, id string, submission bool) error {
	return c.call(ctx, "scrobble", url.Values{"id": {id}, "submission": {strconv.FormatBool(submission)}}, nil)
}

// StreamURL returns a URL that streams the song, signed with a fresh salt.
// It carries a token, so hand it to mpv over IPC only and never log it.
func (c *Client) StreamURL(id string) string {
	q := c.auth()
	q.Set("id", id)
	return c.base + "stream?" + q.Encode()
}

// auth returns the auth parameters with a fresh salt.
func (c *Client) auth() url.Values {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	salt := hex.EncodeToString(b)
	sum := md5.Sum([]byte(c.creds.Password + salt))
	return url.Values{
		"u": {c.creds.Username},
		"t": {hex.EncodeToString(sum[:])},
		"s": {salt},
		"v": {apiVersion},
		"c": {clientName},
		"f": {"json"},
	}
}

// call sends one request and decodes the subsonic-response into v, which
// may be nil.
func (c *Client) call(ctx context.Context, endpoint string, params url.Values, v any) error {
	q := c.auth()
	for k, vs := range params {
		q[k] = vs
	}
	var req *http.Request
	var err error
	if c.formPost.Load() {
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, c.base+endpoint, strings.NewReader(q.Encode()))
		if req != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
	} else {
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, c.base+endpoint+"?"+q.Encode(), nil)
	}
	if err != nil {
		return fmt.Errorf("%s: %w", endpoint, err)
	}
	req.Header.Set("User-Agent", clientName)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", endpoint, redact(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return fmt.Errorf("%s: HTTP %s", endpoint, resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%s: %w", endpoint, redact(err))
	}
	if err := decode(body, v); err != nil {
		return fmt.Errorf("%s: %w", endpoint, err)
	}
	return nil
}

func decode(body []byte, v any) error {
	var env struct {
		Response *json.RawMessage `json:"subsonic-response"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("not a Subsonic response: %w", err)
	}
	if env.Response == nil {
		return errors.New("not a Subsonic response")
	}
	var status struct {
		Status string `json:"status"`
		Error  *Error `json:"error"`
	}
	if err := json.Unmarshal(*env.Response, &status); err != nil {
		return err
	}
	if status.Status != "ok" {
		if status.Error == nil {
			return fmt.Errorf("status %q without an error", status.Status)
		}
		return status.Error
	}
	if v == nil {
		return nil
	}
	return json.Unmarshal(*env.Response, v)
}

// redact strips the query, which holds the token and salt, from the URL a
// transport error names.
func redact(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		if u, perr := url.Parse(ue.URL); perr == nil {
			u.RawQuery = ""
			return &url.Error{Op: ue.Op, URL: u.String(), Err: ue.Err}
		}
	}
	return err
}
