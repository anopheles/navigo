// Command recordfixtures seeds a throwaway Navidrome serving the synthetic
// fixture library (playlists and stars), then records the responses the
// fake Subsonic server in internal/subsonic/subsonictest replays. Run it
// through tools/fixtures.sh.
//
// It replaces every fixture in -out, so the testdata always matches one
// recording. Never point it at a real server: the fixtures are committed.
package main

import (
	"bytes"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"github.com/anopheles/navigo/internal/subsonic/subsonictest"
	"github.com/anopheles/navigo/tools/internal/fixturelib"
)

// pageSize matches Navidrome's cap on list endpoints.
const pageSize = 500

// homeSize is how many albums each of Home's lists asks for; navigo's
// subsonic.HomeSize.
const homeSize = 50

func main() {
	base := flag.String("url", "http://127.0.0.1:14533", "Navidrome base URL")
	out := flag.String("out", "internal/subsonic/subsonictest/testdata", "fixture directory")
	seedOnly := flag.Bool(
		"seed-only",
		false,
		"wait for the scan and seed playlists and stars, but record nothing (for e2e runs)",
	)
	flag.Parse()
	c := &client{base: *base, user: subsonictest.Username, password: subsonictest.Password}
	if *seedOnly {
		*out = ""
	}
	if err := run(c, *out); err != nil {
		log.Fatalf("recordfixtures: %v", err)
	}
}

func run(c *client, out string) error {
	if err := c.waitForScan(3 * time.Minute); err != nil {
		return err
	}
	albums, err := c.albums()
	if err != nil {
		return err
	}
	if err := c.seed(albums); err != nil {
		return err
	}
	if out == "" {
		fmt.Println("server scanned and seeded")
		return nil
	}

	rec := &recorder{c: c, files: map[string][]byte{}}
	rec.get("ping", nil)
	rec.get("getOpenSubsonicExtensions", nil)
	rec.get("getScanStatus", nil)
	rec.get("getIndexes", nil)
	var artists struct {
		Artists struct {
			Index []struct {
				Artist []struct{ ID string }
			}
		}
	}
	rec.decode(rec.get("getArtists", nil), &artists)
	for _, ix := range artists.Artists.Index {
		for _, a := range ix.Artist {
			rec.get("getArtist", url.Values{"id": {a.ID}})
		}
	}
	for _, typ := range []string{"alphabeticalByName", "newest"} {
		rec.get(
			"getAlbumList2",
			url.Values{"type": {typ}, "size": {strconv.Itoa(pageSize)}, "offset": {"0"}},
		)
	}
	// Home's lists, one page each.
	for _, typ := range []string{"newest", "frequent", "recent", "random", "starred"} {
		rec.get("getAlbumList2", url.Values{"type": {typ}, "size": {strconv.Itoa(homeSize)}})
	}
	for _, a := range albums {
		rec.get("getAlbum", url.Values{"id": {a.ID}})
	}
	// Similar artists of everyone credited, as a Mix around a seed asks
	// (external services are off here, so the lists are empty).
	credited := map[string]bool{}
	for _, ix := range artists.Artists.Index {
		for _, a := range ix.Artist {
			credited[a.ID] = true
		}
	}
	for _, a := range albums {
		for _, s := range a.Song {
			credited[s.ArtistID] = true
			for _, sa := range s.Artists {
				credited[sa.ID] = true
			}
		}
	}
	for _, id := range slices.Sorted(maps.Keys(credited)) {
		if id != "" {
			rec.get("getArtistInfo2", url.Values{"id": {id}, "count": {"20"}})
		}
	}
	rec.files["error-70.json"] = rec.get("getAlbum", url.Values{"id": {subsonictest.NotFoundID}})
	var playlists struct {
		Playlists struct{ Playlist []struct{ ID string } }
	}
	rec.decode(rec.get("getPlaylists", nil), &playlists)
	for _, p := range playlists.Playlists.Playlist {
		rec.get("getPlaylist", url.Values{"id": {p.ID}})
	}
	rec.get("getStarred2", nil)

	// The full-sync path: an empty search3 query lists the whole library,
	// one page of songs at a time; artists and albums fit in the first.
	for offset := 0; offset < fixturelib.TrackCount(); offset += pageSize {
		count := strconv.Itoa(pageSize)
		if offset > 0 {
			count = "0"
		}
		rec.get("search3", url.Values{
			"query":       {""},
			"artistCount": {count}, "artistOffset": {"0"},
			"albumCount": {count}, "albumOffset": {"0"},
			"songCount": {strconv.Itoa(pageSize)}, "songOffset": {strconv.Itoa(offset)},
		})
	}
	rec.get("search3", url.Values{"query": {"blue"}})

	// Writes last, so they don't show in the reads above. scrobble with
	// submission=false doesn't count a play.
	first := albums[0].Song[0].ID
	rec.get("scrobble", url.Values{"id": {first}, "submission": {"false"}})
	rec.get("star", url.Values{"id": {first}})
	rec.get("unstar", url.Values{"id": {first}})
	// Rated and unrated again, so the library keeps its ratings.
	rec.get("setRating", url.Values{"id": {first}, "rating": {"4"}})
	rec.get("setRating", url.Values{"id": {first}, "rating": {"0"}})
	// A playlist made, changed and deleted again, so the seeded ones stay
	// as they are.
	var made struct {
		Playlist struct{ ID string }
	}
	rec.decode(
		rec.get("createPlaylist", url.Values{"name": {"navigo-recorded"}, "songId": {first}}),
		&made,
	)
	rec.get(
		"updatePlaylist",
		url.Values{
			"playlistId":        {made.Playlist.ID},
			"songIdToAdd":       {first},
			"songIndexToRemove": {"0"},
		},
	)
	rec.get("deletePlaylist", url.Values{"id": {made.Playlist.ID}})

	bad := *c
	bad.password = "wrong"
	rec.files["error-40.json"] = must(bad.raw("ping", nil))
	if rec.err != nil {
		return rec.err
	}
	return write(out, rec.files)
}

type album struct {
	ID     string
	Name   string
	Artist string
	Song   []struct {
		ID         string
		Track      int
		DiscNumber int
		ArtistID   string
		Artists    []struct{ ID string }
	}
}

// albums returns every album with its songs.
func (c *client) albums() ([]album, error) {
	var list struct {
		AlbumList2 struct{ Album []struct{ ID string } }
	}
	if err := c.call(
		"getAlbumList2",
		url.Values{"type": {"alphabeticalByName"}, "size": {strconv.Itoa(pageSize)}},
		&list,
	); err != nil {
		return nil, err
	}
	var albums []album
	for _, a := range list.AlbumList2.Album {
		var r struct{ Album album }
		if err := c.call("getAlbum", url.Values{"id": {a.ID}}, &r); err != nil {
			return nil, err
		}
		albums = append(albums, r.Album)
	}
	if len(albums) != len(fixturelib.Albums) {
		return nil, fmt.Errorf(
			"server has %d albums, fixture library has %d",
			len(albums),
			len(fixturelib.Albums),
		)
	}
	return albums, nil
}

// seed creates the fixture playlists and stars, unless an earlier run did.
func (c *client) seed(albums []album) error {
	find := func(ref fixturelib.TrackRef) (albumID, songID string, err error) {
		for _, a := range albums {
			if a.Name != ref.Album || a.Artist != ref.AlbumArtist {
				continue
			}
			if ref.Number == 0 {
				return a.ID, "", nil
			}
			for _, s := range a.Song {
				if s.DiscNumber == ref.Disc && s.Track == ref.Number {
					return a.ID, s.ID, nil
				}
			}
		}
		return "", "", fmt.Errorf("no track %+v on the server", ref)
	}

	var existing struct {
		Playlists struct{ Playlist []struct{ Name string } }
	}
	if err := c.call("getPlaylists", nil, &existing); err != nil {
		return err
	}
	if len(existing.Playlists.Playlist) == 0 { // a fresh server: play the tracks once
		for _, ref := range fixturelib.Played {
			_, id, err := find(ref)
			if err != nil {
				return err
			}
			if err := c.call(
				"scrobble",
				url.Values{"id": {id}, "submission": {"true"}},
				nil,
			); err != nil {
				return err
			}
		}
	}
	for _, p := range fixturelib.Playlists {
		if slices.ContainsFunc(
			existing.Playlists.Playlist,
			func(e struct{ Name string }) bool { return e.Name == p.Name },
		) {
			continue
		}
		params := url.Values{"name": {p.Name}}
		for _, ref := range p.Tracks {
			_, id, err := find(ref)
			if err != nil {
				return err
			}
			params.Add("songId", id)
		}
		if err := c.call("createPlaylist", params, nil); err != nil {
			return err
		}
	}

	var artists struct {
		Artists struct {
			Index []struct {
				Artist []struct{ ID, Name string }
			}
		}
	}
	if err := c.call("getArtists", nil, &artists); err != nil {
		return err
	}
	star := url.Values{}
	for _, ix := range artists.Artists.Index {
		for _, a := range ix.Artist {
			if slices.Contains(fixturelib.StarredArtists, a.Name) {
				star.Add("artistId", a.ID)
			}
		}
	}
	for _, ref := range fixturelib.Starred {
		albumID, songID, err := find(ref)
		if err != nil {
			return err
		}
		if songID == "" {
			star.Add("albumId", albumID)
		} else {
			star.Add("id", songID)
		}
	}
	return c.call("star", star, nil)
}

type recorder struct {
	c     *client
	files map[string][]byte
	err   error
}

// get records one request and returns its body.
func (r *recorder) get(endpoint string, params url.Values) []byte {
	if r.err != nil {
		return nil
	}
	body, err := r.c.raw(endpoint, params)
	if err != nil {
		r.err = err
		return nil
	}
	r.files[subsonictest.FixtureName(endpoint, params)] = body
	return body
}

func (r *recorder) decode(body []byte, v any) {
	if r.err == nil {
		r.err = unwrap(body, v)
	}
}

// write replaces every fixture in dir with files, indented for review.
func write(dir string, files map[string][]byte) error {
	old, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return err
	}
	for _, f := range old {
		if err := os.Remove(f); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for name, body := range files {
		var buf bytes.Buffer
		if err := json.Indent(&buf, body, "", "  "); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		buf.WriteByte('\n')
		if err := os.WriteFile(filepath.Join(dir, name), buf.Bytes(), 0o644); err != nil {
			return err
		}
	}
	fmt.Printf("recorded %d fixtures into %s\n", len(files), dir)
	return nil
}

// client is a minimal Subsonic client for seeding and recording; navigo's
// own client lives in internal/subsonic.
type client struct {
	base, user, password string
}

func (c *client) waitForScan(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	want := fixturelib.TrackCount()
	for {
		var r struct {
			ScanStatus struct {
				Scanning bool
				Count    int
			}
		}
		err := c.call("getScanStatus", nil, &r)
		if err == nil && !r.ScanStatus.Scanning && r.ScanStatus.Count == want {
			return nil
		}
		if time.Now().After(deadline) {
			if err == nil {
				err = fmt.Errorf("scanned %d tracks, want %d", r.ScanStatus.Count, want)
			}
			return fmt.Errorf("server not ready after %v: %w", timeout, err)
		}
		time.Sleep(time.Second)
	}
}

// call sends a request and decodes its subsonic-response into v.
func (c *client) call(endpoint string, params url.Values, v any) error {
	body, err := c.raw(endpoint, params)
	if err != nil {
		return err
	}
	if err := unwrap(body, v); err != nil {
		return fmt.Errorf("%s: %w", endpoint, err)
	}
	return nil
}

// raw sends a token-signed request and returns the body unparsed.
func (c *client) raw(endpoint string, params url.Values) ([]byte, error) {
	salt := make([]byte, 8)
	_, _ = rand.Read(salt)
	s := hex.EncodeToString(salt)
	sum := md5.Sum([]byte(c.password + s))
	q := url.Values{
		"u": {c.user},
		"t": {hex.EncodeToString(sum[:])},
		"s": {s},
		"v": {"1.16.1"},
		"c": {"navigo-recordfixtures"},
		"f": {"json"},
	}
	for k, v := range params {
		q[k] = v
	}
	resp, err := http.Get(c.base + "/rest/" + endpoint + "?" + q.Encode())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %s", endpoint, resp.Status)
	}
	return body, nil
}

// unwrap decodes the payload of an "ok" subsonic-response into v, which
// may be nil.
func unwrap(body []byte, v any) error {
	var env struct {
		Response json.RawMessage `json:"subsonic-response"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return err
	}
	var status struct {
		Status string
		Error  struct {
			Code    int
			Message string
		}
	}
	if err := json.Unmarshal(env.Response, &status); err != nil {
		return err
	}
	if status.Status != "ok" {
		return errors.New(status.Error.Message)
	}
	if v == nil {
		return nil
	}
	return json.Unmarshal(env.Response, v)
}

func must(b []byte, err error) []byte {
	if err != nil {
		log.Fatalf("recordfixtures: %v", err)
	}
	return b
}
