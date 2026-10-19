package subsonic

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/anopheles/navigo/internal/subsonic/subsonictest"
)

func newClient(t *testing.T) (*Client, *subsonictest.Server) {
	t.Helper()
	srv := subsonictest.NewServer(t)
	c := New(Credentials{URL: srv.URL + "/", Username: subsonictest.Username, Password: subsonictest.Password})
	if err := c.Login(context.Background()); err != nil {
		t.Fatalf("Login: %v", err)
	}
	return c, srv
}

func TestLoginUsesFormPost(t *testing.T) {
	c, srv := newClient(t)
	if _, err := c.Albums(context.Background(), ByName); err != nil {
		t.Fatal(err)
	}
	var methods []string
	for _, r := range srv.Requests() {
		methods = append(methods, r.Endpoint+":"+r.Method)
	}
	want := "ping:GET getOpenSubsonicExtensions:GET getAlbumList2:POST"
	if got := strings.Join(methods, " "); got != want {
		t.Errorf("requests = %s, want %s", got, want)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	srv := subsonictest.NewServer(t)
	c := New(Credentials{URL: srv.URL, Username: subsonictest.Username, Password: "wrong"})
	err := c.Login(context.Background())
	if !IsCode(err, CodeWrongCredentials) {
		t.Fatalf("Login = %v, want error %d", err, CodeWrongCredentials)
	}
}

func TestAlbums(t *testing.T) {
	c, _ := newClient(t)
	albums, err := c.Albums(context.Background(), ByName)
	if err != nil {
		t.Fatal(err)
	}
	if len(albums) != 8 {
		t.Fatalf("got %d albums, want 8", len(albums))
	}
	var blues []string
	for _, a := range albums {
		if a.Name == "Blue" {
			blues = append(blues, a.Artist)
		}
		if a.ID == "" || a.SongCount == 0 || len(a.Songs) != 0 {
			t.Errorf("album %+v: want an ID, a song count and no songs", a)
		}
	}
	if len(blues) != 2 {
		t.Errorf("albums named Blue by %v, want two artists", blues)
	}
}

func TestAlbum(t *testing.T) {
	c, _ := newClient(t)
	ctx := context.Background()
	albums, err := c.Albums(ctx, ByName)
	if err != nil {
		t.Fatal(err)
	}
	var long Album
	for _, a := range albums {
		if a.Name == "The Long Haul" {
			long, err = c.Album(ctx, a.ID)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if len(long.Songs) != 510 {
		t.Fatalf("The Long Haul has %d songs, want 510", len(long.Songs))
	}
	last := long.Songs[509]
	if last.Title != "Mile 510" || last.DiscNumber != 2 || last.Track != 255 || last.Duration != 20 || last.AlbumID != long.ID {
		t.Errorf("last song = %+v", last)
	}
}

func TestAlbumNotFound(t *testing.T) {
	c, _ := newClient(t)
	_, err := c.Album(context.Background(), subsonictest.NotFoundID)
	var e *Error
	if !errors.As(err, &e) || e.Code != CodeNotFound {
		t.Fatalf("Album = %v, want error %d", err, CodeNotFound)
	}
}

func TestScrobble(t *testing.T) {
	c, srv := newClient(t)
	if err := c.Scrobble(context.Background(), "song1", false); err != nil {
		t.Fatal(err)
	}
	reqs := srv.Requests()
	last := reqs[len(reqs)-1]
	if last.Endpoint != "scrobble" || last.Params.Get("id") != "song1" || last.Params.Get("submission") != "false" {
		t.Errorf("request = %+v, want scrobble id=song1 submission=false", last)
	}
}

func TestStreamURL(t *testing.T) {
	c := New(Credentials{URL: "https://music.example/sub/", Username: "ann", Password: "secret"})
	a, b := c.StreamURL("tr1"), c.StreamURL("tr1")
	ua, err := url.Parse(a)
	if err != nil {
		t.Fatal(err)
	}
	ub, _ := url.Parse(b)
	if ua.Path != "/sub/rest/stream" {
		t.Errorf("path = %s", ua.Path)
	}
	q := ua.Query()
	sum := md5.Sum([]byte("secret" + q.Get("s")))
	if q.Get("id") != "tr1" || q.Get("u") != "ann" || q.Get("t") != hex.EncodeToString(sum[:]) || q.Has("p") {
		t.Errorf("query = %v, want id, user and a valid token, and no password", q)
	}
	if strings.Contains(a, "secret") {
		t.Error("stream URL contains the password")
	}
	if q.Get("s") == ub.Query().Get("s") {
		t.Error("two stream URLs share a salt")
	}
}

func TestTransportErrorsHideTheToken(t *testing.T) {
	c := New(Credentials{URL: "http://127.0.0.1:1", Username: "ann", Password: "secret"})
	err := c.Login(context.Background())
	if err == nil {
		t.Fatal("Login succeeded against a closed port")
	}
	if strings.Contains(err.Error(), "t=") || strings.Contains(err.Error(), "s=") {
		t.Errorf("error leaks auth params: %v", err)
	}
}

func TestHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)
	c := New(Credentials{URL: srv.URL, Username: "ann", Password: "secret"})
	if err := c.Login(context.Background()); err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("Login = %v, want an HTTP 502 error", err)
	}
}

func TestSearch3FullLibrary(t *testing.T) {
	c, _ := newClient(t)
	ctx := context.Background()
	first, err := c.Search3(ctx, "", SearchCounts{ArtistCount: 500, AlbumCount: 500, SongCount: 500})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Artists) != 7 || len(first.Albums) != 8 || len(first.Songs) != 500 {
		t.Fatalf("first page: %d artists, %d albums, %d songs", len(first.Artists), len(first.Albums), len(first.Songs))
	}
	second, err := c.Search3(ctx, "", SearchCounts{SongCount: 500, SongOffset: 500})
	if err != nil || len(second.Songs) != 47 || len(second.Artists) != 0 {
		t.Fatalf("second page: %d songs, %d artists, %v", len(second.Songs), len(second.Artists), err)
	}
	var guest bool
	for _, s := range append(first.Songs, second.Songs...) {
		for _, a := range s.Artists {
			guest = guest || a.Name == "Pell Mell"
		}
	}
	if !guest {
		t.Error("no song credits Pell Mell in its artists list")
	}
}

func TestPlaylistsStarredScanStatus(t *testing.T) {
	c, _ := newClient(t)
	ctx := context.Background()
	pls, err := c.Playlists(ctx)
	if err != nil || len(pls) != 2 {
		t.Fatalf("playlists = %v, %v", pls, err)
	}
	for _, p := range pls {
		full, err := c.Playlist(ctx, p.ID)
		if err != nil {
			t.Fatal(err)
		}
		if p.Name == "Morning Mix" && (len(full.Songs) != 5 || full.Songs[0].ID != full.Songs[4].ID) {
			t.Errorf("Morning Mix = %d songs, want 5 with the first one twice", len(full.Songs))
		}
	}
	st, err := c.Starred(ctx)
	if err != nil || len(st.Artists) != 1 || len(st.Albums) != 1 || len(st.Songs) != 2 {
		t.Errorf("starred = %+v, %v", st, err)
	}
	scan, err := c.ScanStatus(ctx)
	if err != nil || scan.Count != 547 || scan.LastScan.IsZero() {
		t.Errorf("scan status = %+v, %v", scan, err)
	}
}
