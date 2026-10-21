package subsonic

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/anopheles/navigo/internal/subsonic/subsonictest"
)

func newClient(t *testing.T) (*Client, *subsonictest.Server) {
	t.Helper()
	srv := subsonictest.NewServer(t)
	c := New(
		Credentials{
			URL:      srv.URL + "/",
			Username: subsonictest.Username,
			Password: subsonictest.Password,
		},
	)
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
	if last.Title != "Mile 510" || last.DiscNumber != 2 || last.Track != 255 ||
		last.Duration != 20 ||
		last.AlbumID != long.ID {
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
	if last.Endpoint != "scrobble" || last.Params.Get("id") != "song1" ||
		last.Params.Get("submission") != "false" {
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
	if q.Get("id") != "tr1" || q.Get("u") != "ann" || q.Get("t") != hex.EncodeToString(sum[:]) ||
		q.Has("p") {
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
	first, err := c.Search3(
		ctx,
		"",
		SearchCounts{ArtistCount: 500, AlbumCount: 500, SongCount: 500},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Artists) != 7 || len(first.Albums) != 8 || len(first.Songs) != 500 {
		t.Fatalf(
			"first page: %d artists, %d albums, %d songs",
			len(first.Artists),
			len(first.Albums),
			len(first.Songs),
		)
	}
	second, err := c.Search3(ctx, "", SearchCounts{SongCount: 500, SongOffset: 500})
	if err != nil || len(second.Songs) != 47 || len(second.Artists) != 0 {
		t.Fatalf(
			"second page: %d songs, %d artists, %v",
			len(second.Songs),
			len(second.Artists),
			err,
		)
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
		if p.Name == "Morning Mix" &&
			(len(full.Songs) != 5 || full.Songs[0].ID != full.Songs[4].ID) {
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

func TestUnreachable(t *testing.T) {
	c, srv := newClient(t)
	ctx := context.Background()
	srv.SetDown(true)
	if err := c.Ping(ctx); !IsUnreachable(err) {
		t.Errorf("dropped connection: %v, want unreachable", err)
	}
	srv.SetDown(false)
	if err := c.Ping(ctx); err != nil {
		t.Errorf("back up: %v", err)
	}
	// A Subsonic error means the server is there.
	if _, err := c.Album(
		ctx,
		subsonictest.NotFoundID,
	); IsUnreachable(err) ||
		!IsCode(err, CodeNotFound) {
		t.Errorf("not found = %v", err)
	}
	// 5xx and replies that aren't from a Subsonic server are unreachable.
	for name, h := range map[string]http.HandlerFunc{
		"502":        func(w http.ResponseWriter, r *http.Request) { http.Error(w, "bad gateway", http.StatusBadGateway) },
		"proxy page": func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("<html>captive portal</html>")) },
	} {
		hs := httptest.NewServer(h)
		err := New(Credentials{URL: hs.URL, Username: "a", Password: "b"}).Ping(ctx)
		hs.Close()
		if !IsUnreachable(err) {
			t.Errorf("%s: %v, want unreachable", name, err)
		}
	}
	// No server at all.
	if err := New(
		Credentials{URL: "http://127.0.0.1:1", Username: "a", Password: "b"},
	).Ping(ctx); !IsUnreachable(
		err,
	) {
		t.Errorf("refused: %v", err)
	}
}

func TestDefaultTimeout(t *testing.T) {
	hs := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }),
	)
	defer hs.Close()
	c := New(Credentials{URL: hs.URL, Username: "a", Password: "b"})
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := c.Ping(ctx); !IsUnreachable(err) || time.Since(start) > 2*time.Second {
		t.Errorf("Ping with a 200ms deadline: %v after %v", err, time.Since(start))
	}
}

func TestPlaylistWrites(t *testing.T) {
	c, srv := newClient(t)
	ctx := context.Background()
	id, err := c.CreatePlaylist(ctx, "Rock", "", []string{"a", "b"})
	if err != nil || id == "" {
		t.Fatalf("CreatePlaylist = %q, %v", id, err)
	}
	if id, err := c.CreatePlaylist(ctx, "", "pl1", []string{"b", "a"}); err != nil || id != "pl1" {
		t.Errorf("replacing = %q, %v", id, err)
	}
	if err := c.UpdatePlaylist(ctx, "pl1", []string{"c"}, []int{0, 2}); err != nil {
		t.Fatal(err)
	}
	if err := c.RenamePlaylist(ctx, "pl1", "Jazz"); err != nil {
		t.Fatal(err)
	}
	if err := c.DeletePlaylist(ctx, "pl1"); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range srv.Requests() {
		if r.Endpoint != "ping" && r.Endpoint != "getOpenSubsonicExtensions" {
			got = append(got, r.Endpoint+" "+r.Params.Encode())
		}
	}
	want := []string{
		"createPlaylist name=Rock&songId=a&songId=b",
		"createPlaylist playlistId=pl1&songId=b&songId=a",
		"updatePlaylist playlistId=pl1&songIdToAdd=c&songIndexToRemove=0&songIndexToRemove=2",
		"updatePlaylist name=Jazz&playlistId=pl1",
		"deletePlaylist id=pl1",
	}
	if !slices.Equal(got, want) {
		t.Errorf("requests =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestStar(t *testing.T) {
	c, srv := newClient(t)
	ctx := context.Background()
	if err := c.Star(ctx, true, []string{"ar"}, []string{"al"}, []string{"s1", "s2"}); err != nil {
		t.Fatal(err)
	}
	if err := c.Star(ctx, false, nil, nil, []string{"s1"}); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range srv.Requests() {
		if r.Endpoint == "star" || r.Endpoint == "unstar" {
			got = append(got, r.Endpoint+" "+r.Params.Encode())
		}
	}
	want := []string{"star albumId=al&artistId=ar&id=s1&id=s2", "unstar id=s1"}
	if !slices.Equal(got, want) {
		t.Errorf("requests = %q, want %q", got, want)
	}
}

func TestSetRating(t *testing.T) {
	c, srv := newClient(t)
	ctx := context.Background()
	if err := c.SetRating(ctx, "s1", 4); err != nil {
		t.Fatal(err)
	}
	if err := c.SetRating(ctx, "s1", 0); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range srv.Requests() {
		if r.Endpoint == "setRating" {
			got = append(got, r.Params.Encode())
		}
	}
	if want := []string{"id=s1&rating=4", "id=s1&rating=0"}; !slices.Equal(got, want) {
		t.Errorf("requests = %q, want %q", got, want)
	}
}

func TestCoverArt(t *testing.T) {
	c, srv := newClient(t)
	got, err := c.CoverArt(context.Background(), "al-1")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, subsonictest.CoverPNG("al-1")) {
		t.Errorf("got %d bytes, not the cover for al-1", len(got))
	}
	var req subsonictest.Request
	for _, r := range srv.Requests() {
		if r.Endpoint == "getCoverArt" {
			req = r
		}
	}
	if req.Method != http.MethodPost || req.Params.Get("id") != "al-1" ||
		req.Params.Get("size") == "" || req.Params.Has("f") {
		t.Errorf("request = %+v, want a form POST for al-1 with a size and no f", req)
	}
}

func TestCoverArtNotFound(t *testing.T) {
	c, _ := newClient(t)
	if _, err := c.CoverArt(context.Background(), subsonictest.NotFoundID); !errors.Is(
		err, ErrNoCover) {
		t.Errorf("err = %v, want ErrNoCover", err)
	}
}

func TestCoverArtErrors(t *testing.T) {
	for name, h := range map[string]http.HandlerFunc{
		"a Subsonic not-found": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"subsonic-response":{"status":"failed","version":"1.16.1",` +
				`"error":{"code":70,"message":"not found"}}}`))
		},
		"a server error": func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		},
		"an HTML page": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html>login</html>"))
		},
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(h)
			defer srv.Close()
			c := New(Credentials{URL: srv.URL + "/", Username: "u", Password: "p"})
			_, err := c.CoverArt(context.Background(), "x")
			switch {
			case err == nil:
				t.Error("no error")
			case name == "a Subsonic not-found" && !errors.Is(err, ErrNoCover):
				t.Errorf("err = %v, want ErrNoCover", err)
			case name != "a Subsonic not-found" && errors.Is(err, ErrNoCover):
				t.Errorf("err = %v, which says there is no cover", err)
			}
		})
	}
}
