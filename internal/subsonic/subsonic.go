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
	"log/slog"
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

// Timeout is how long a request may take unless the caller's context
// sets its own deadline (sync pages do).
const Timeout = 10 * time.Second

// ErrUnreachable wraps every failure to reach the server: network errors,
// timeouts, TLS, HTTP 5xx, and replies that aren't from a Subsonic server.
// A Subsonic error from the server (*Error) is not one: the server is there.
var ErrUnreachable = errors.New("can't reach the server")

// IsUnreachable reports whether err means the server couldn't be reached.
func IsUnreachable(err error) bool { return errors.Is(err, ErrUnreachable) }

type unreachable struct{ err error }

func (u unreachable) Error() string   { return u.err.Error() }
func (u unreachable) Unwrap() []error { return []error{ErrUnreachable, u.err} }

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
	if e.Code == CodeTokenAuthDisabled {
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
		http:  &http.Client{},
	}
}

// Ping checks that the server answers and accepts the credentials.
func (c *Client) Ping(ctx context.Context) error { return c.call(ctx, "ping", nil, nil) }

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
		if IsUnreachable(err) {
			return err
		}
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
	SortName      string `json:"sortName"`
	PlayCount     int    `json:"playCount"`
	Songs         []Song `json:"song"`
}

// ArtistRef names one artist credited on a song (OpenSubsonic).
type ArtistRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Artist is an artist as search3 and getStarred2 list them.
type Artist struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	SortName   string `json:"sortName"`
	AlbumCount int    `json:"albumCount"`
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
	PlayCount   int    `json:"playCount"`
	// Played is when the user last played the song; zero if never.
	Played     time.Time `json:"played"`
	UserRating int       `json:"userRating"` // 1–5 stars, 0 if unrated
	// Artists is every artist credited on the song, as the server splits
	// them (OpenSubsonic); empty on plain Subsonic servers.
	Artists []ArtistRef `json:"artists"`
}

// ListType orders an album list.
type ListType string

// The album list orders navigo uses.
const (
	ByName   ListType = "alphabeticalByName"
	Newest   ListType = "newest"
	Frequent ListType = "frequent"
	Recent   ListType = "recent"
	Random   ListType = "random"
	Starred2 ListType = "starred"
)

// HomeSize is how many albums each of Home's lists shows.
const HomeSize = 50

// AlbumList returns the first size albums of a list, such as the newest.
func (c *Client) AlbumList(ctx context.Context, list ListType, size int) ([]Album, error) {
	var r struct {
		AlbumList2 struct {
			Album []Album `json:"album"`
		} `json:"albumList2"`
	}
	err := c.call(
		ctx,
		"getAlbumList2",
		url.Values{"type": {string(list)}, "size": {strconv.Itoa(size)}},
		&r,
	)
	return r.AlbumList2.Album, err
}

// Albums returns every album in the given order, fetching page by page.
func (c *Client) Albums(ctx context.Context, order ListType) ([]Album, error) {
	var all []Album
	for offset := 0; ; offset += pageSize {
		var r struct {
			AlbumList2 struct {
				Album []Album `json:"album"`
			} `json:"albumList2"`
		}
		params := url.Values{
			"type":   {string(order)},
			"size":   {strconv.Itoa(pageSize)},
			"offset": {strconv.Itoa(offset)},
		}
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

// SearchCounts says how many of each kind search3 returns, from where.
// A count of 0 asks for none of that kind.
type SearchCounts struct {
	ArtistCount, ArtistOffset int
	AlbumCount, AlbumOffset   int
	SongCount, SongOffset     int
}

// SearchResult is one page of search3.
type SearchResult struct {
	Artists []Artist `json:"artist"`
	Albums  []Album  `json:"album"`
	Songs   []Song   `json:"song"`
}

// Search3 runs search3. On Navidrome an empty query lists the whole
// library, which is how navigo syncs.
func (c *Client) Search3(ctx context.Context, query string, n SearchCounts) (SearchResult, error) {
	var r struct {
		SearchResult3 SearchResult `json:"searchResult3"`
	}
	params := url.Values{
		"query": {query},
		"artistCount": {
			strconv.Itoa(n.ArtistCount),
		},
		"artistOffset": {strconv.Itoa(n.ArtistOffset)},
		"albumCount":   {strconv.Itoa(n.AlbumCount)},
		"albumOffset":  {strconv.Itoa(n.AlbumOffset)},
		"songCount":    {strconv.Itoa(n.SongCount)},
		"songOffset":   {strconv.Itoa(n.SongOffset)},
	}
	err := c.call(ctx, "search3", params, &r)
	return r.SearchResult3, err
}

// Playlist is a playlist; Songs is filled only by Playlist.
type Playlist struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Owner     string `json:"owner"`
	SongCount int    `json:"songCount"`
	Songs     []Song `json:"entry"`
}

// Playlists lists the playlists the user can see.
func (c *Client) Playlists(ctx context.Context) ([]Playlist, error) {
	var r struct {
		Playlists struct {
			Playlist []Playlist `json:"playlist"`
		} `json:"playlists"`
	}
	err := c.call(ctx, "getPlaylists", nil, &r)
	return r.Playlists.Playlist, err
}

// Playlist returns one playlist with its songs, in order.
func (c *Client) Playlist(ctx context.Context, id string) (Playlist, error) {
	var r struct {
		Playlist Playlist `json:"playlist"`
	}
	err := c.call(ctx, "getPlaylist", url.Values{"id": {id}}, &r)
	return r.Playlist, err
}

// CreatePlaylist makes a playlist of songs, in order, and returns its ID.
// With id set it replaces that playlist's songs instead, which is how
// navigo reorders one.
func (c *Client) CreatePlaylist(
	ctx context.Context,
	name, id string,
	songIDs []string,
) (string, error) {
	q := url.Values{"songId": songIDs}
	if id != "" {
		q.Set("playlistId", id)
	} else {
		q.Set("name", name)
	}
	var r struct {
		Playlist Playlist `json:"playlist"`
	}
	err := c.call(ctx, "createPlaylist", q, &r)
	if id != "" {
		return id, err
	}
	return r.Playlist.ID, err
}

// UpdatePlaylist adds songs to the end of a playlist and removes the
// songs at the given indexes, counted before the change.
func (c *Client) UpdatePlaylist(ctx context.Context, id string, add []string, remove []int) error {
	q := url.Values{"playlistId": {id}}
	for _, s := range add {
		q.Add("songIdToAdd", s)
	}
	for _, i := range remove {
		q.Add("songIndexToRemove", strconv.Itoa(i))
	}
	return c.call(ctx, "updatePlaylist", q, nil)
}

// RenamePlaylist gives a playlist a new name.
func (c *Client) RenamePlaylist(ctx context.Context, id, name string) error {
	return c.call(ctx, "updatePlaylist", url.Values{"playlistId": {id}, "name": {name}}, nil)
}

// DeletePlaylist deletes a playlist.
func (c *Client) DeletePlaylist(ctx context.Context, id string) error {
	return c.call(ctx, "deletePlaylist", url.Values{"id": {id}}, nil)
}

// Star stars artists, albums and songs by ID; with on false it unstars
// them.
func (c *Client) Star(ctx context.Context, on bool, artistIDs, albumIDs, songIDs []string) error {
	q := url.Values{}
	for _, id := range artistIDs {
		q.Add("artistId", id)
	}
	for _, id := range albumIDs {
		q.Add("albumId", id)
	}
	for _, id := range songIDs {
		q.Add("id", id)
	}
	endpoint := "star"
	if !on {
		endpoint = "unstar"
	}
	return c.call(ctx, endpoint, q, nil)
}

// SetRating rates a song 1–5 stars, or clears its rating with 0.
func (c *Client) SetRating(ctx context.Context, id string, rating int) error {
	return c.call(ctx, "setRating", url.Values{"id": {id}, "rating": {strconv.Itoa(rating)}}, nil)
}

// Starred is what the user has starred.
type Starred struct {
	Artists []Artist `json:"artist"`
	Albums  []Album  `json:"album"`
	Songs   []Song   `json:"song"`
}

// SimilarArtists returns up to count artists the server finds similar to
// one, closest first (getArtistInfo2). Only artists in the library come
// back, since includeNotPresent isn't asked for; a server with no source
// for them returns none.
func (c *Client) SimilarArtists(ctx context.Context, id string, count int) ([]Artist, error) {
	var r struct {
		ArtistInfo2 struct {
			SimilarArtist []Artist `json:"similarArtist"`
		} `json:"artistInfo2"`
	}
	err := c.call(ctx, "getArtistInfo2", url.Values{"id": {id}, "count": {strconv.Itoa(count)}}, &r)
	return r.ArtistInfo2.SimilarArtist, err
}

// Starred returns the user's starred artists, albums and songs.
func (c *Client) Starred(ctx context.Context) (Starred, error) {
	var r struct {
		Starred2 Starred `json:"starred2"`
	}
	err := c.call(ctx, "getStarred2", nil, &r)
	return r.Starred2, err
}

// ScanStatus is the server's library scan state.
type ScanStatus struct {
	Scanning bool      `json:"scanning"`
	Count    int       `json:"count"`
	LastScan time.Time `json:"lastScan"`
}

// ScanStatus returns when the server last scanned its library and how many
// tracks it found.
func (c *Client) ScanStatus(ctx context.Context) (ScanStatus, error) {
	var r struct {
		ScanStatus ScanStatus `json:"scanStatus"`
	}
	err := c.call(ctx, "getScanStatus", nil, &r)
	return r.ScanStatus, err
}

// Scrobble reports a song as started (submission false) or as played
// (submission true).
func (c *Client) Scrobble(ctx context.Context, id string, submission bool) error {
	return c.call(
		ctx,
		"scrobble",
		url.Values{"id": {id}, "submission": {strconv.FormatBool(submission)}},
		nil,
	)
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
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, Timeout)
		defer cancel()
	}
	slog.Debug("server request", "endpoint", endpoint) // never the parameters: they hold the token
	q := c.auth()
	for k, vs := range params {
		q[k] = vs
	}
	req, err := c.newRequest(ctx, endpoint, q)
	if err != nil {
		return fmt.Errorf("%s: %w", endpoint, err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() == context.Canceled {
			return fmt.Errorf("%s: %w", endpoint, ctx.Err())
		}
		return fmt.Errorf("%s: %w", endpoint, unreachable{redact(err)})
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		err := fmt.Errorf("HTTP %s", resp.Status)
		if resp.StatusCode >= 500 {
			err = unreachable{err}
		}
		return fmt.Errorf("%s: %w", endpoint, err)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%s: %w", endpoint, unreachable{redact(err)})
	}
	if err := decode(body, v); err != nil {
		var se *Error
		if !errors.As(err, &se) { // not a Subsonic reply, such as a proxy's error page
			err = unreachable{err}
		}
		return fmt.Errorf("%s: %w", endpoint, err)
	}
	return nil
}

// newRequest is a request for endpoint with the signed parameters q: a
// POST form once the server offers formPost, which keeps the token out of
// the URL, and a GET otherwise.
func (c *Client) newRequest(
	ctx context.Context, endpoint string, q url.Values,
) (*http.Request, error) {
	var req *http.Request
	var err error
	if c.formPost.Load() {
		req, err = http.NewRequestWithContext(
			ctx,
			http.MethodPost,
			c.base+endpoint,
			strings.NewReader(q.Encode()),
		)
		if req != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
	} else {
		req, err = http.NewRequestWithContext(
			ctx,
			http.MethodGet,
			c.base+endpoint+"?"+q.Encode(),
			nil,
		)
	}
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", clientName)
	return req, nil
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
