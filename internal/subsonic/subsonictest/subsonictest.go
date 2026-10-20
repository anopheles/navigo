// Package subsonictest is a fake Subsonic server for tests (ADR 0003). It
// replays JSON responses recorded from a throwaway Navidrome serving the
// synthetic fixture library, so the real client is tested against real
// server output.
//
// Each request is looked up by endpoint and its parameters, without the auth
// parameters, as named by [FixtureName]. A request with no recorded fixture
// fails the test. To add fixtures, add the request to tools/recordfixtures
// and re-record with tools/fixtures.sh.
//
// The server checks token+salt auth like a real one: a wrong user or token
// gets the recorded error 40. A plain password or a reused salt fails the
// test, because navigo must sign every request with a fresh salt.
package subsonictest

import (
	"crypto/md5"
	"embed"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
)

// Credentials the fixtures were recorded with: Navidrome's auto-created
// admin, with the password set in tools/navidrome/compose.yaml.
const (
	Username = "admin"
	Password = "navigo-fixtures"
)

// NotFoundID is an album ID with a recorded "not found" (error 70) response.
const NotFoundID = "does-not-exist"

//go:embed testdata
var fixtures embed.FS

// authParams are left out of fixture names.
var authParams = []string{"u", "t", "s", "p", "v", "c", "f", "apiKey"}

// ignoresParams are write endpoints replayed the same whatever they are
// called with: a plain "ok", or for createPlaylist the playlist the
// recording made. Check their parameters with Requests.
var ignoresParams = []string{
	"scrobble",
	"star",
	"unstar",
	"createPlaylist",
	"updatePlaylist",
	"deletePlaylist",
}

// FixtureName is the testdata file name a request is replayed from:
// "getAlbum.id=xyz.json", or "getArtists.json" without parameters.
func FixtureName(endpoint string, params url.Values) string {
	if slices.Contains(ignoresParams, endpoint) {
		return endpoint + ".json"
	}
	p := url.Values{}
	for k, v := range params {
		if !slices.Contains(authParams, k) {
			p[k] = v
		}
	}
	if len(p) == 0 {
		return endpoint + ".json"
	}
	return endpoint + "." + p.Encode() + ".json"
}

// Request is one request the server received, without its auth parameters.
type Request struct {
	Method   string
	Endpoint string
	Params   url.Values
}

// Server is a running fake Subsonic server, closed when the test ends.
type Server struct {
	// URL is the server's base URL; the API lives under URL + "/rest/".
	URL string

	t     testing.TB
	mu    sync.Mutex
	salts map[string]bool
	reqs  []Request
	down  bool
}

// SetDown makes the server drop every connection without answering, as an
// unreachable server would, until it is set up again.
func (s *Server) SetDown(down bool) {
	s.mu.Lock()
	s.down = down
	s.mu.Unlock()
}

// NewServer starts a fake server for the duration of the test.
func NewServer(t testing.TB) *Server {
	t.Helper()
	s := &Server{t: t, salts: map[string]bool{}}
	srv := httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(srv.Close)
	s.URL = srv.URL
	return s
}

// Requests returns every request received so far, in order.
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.reqs)
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	down := s.down
	s.mu.Unlock()
	if down {
		if hj, ok := w.(http.Hijacker); ok {
			if conn, _, err := hj.Hijack(); err == nil {
				_ = conn.Close()
				return
			}
		}
		http.Error(w, "down", http.StatusServiceUnavailable)
		return
	}
	endpoint, ok := strings.CutPrefix(r.URL.Path, "/rest/")
	if !ok {
		s.t.Errorf("subsonictest: request outside /rest/: %s", r.URL.Path)
		http.NotFound(w, r)
		return
	}
	endpoint = strings.TrimSuffix(endpoint, ".view")
	if err := r.ParseForm(); err != nil {
		s.t.Errorf("subsonictest: %s: %v", endpoint, err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	params := r.Form
	if f := params.Get("f"); f != "json" {
		s.t.Errorf("subsonictest: %s: f=%q, want json", endpoint, f)
	}

	name := FixtureName(endpoint, params)
	if !s.authorized(endpoint, params) {
		name = "error-40.json"
	}

	clean := url.Values{}
	for k, v := range params {
		if !slices.Contains(authParams, k) {
			clean[k] = v
		}
	}
	s.mu.Lock()
	s.reqs = append(s.reqs, Request{Method: r.Method, Endpoint: endpoint, Params: clean})
	s.mu.Unlock()

	body, err := fixtures.ReadFile("testdata/" + name)
	if err != nil {
		s.t.Errorf(
			"subsonictest: no fixture %s; add the request to tools/recordfixtures and re-record",
			name,
		)
		body, _ = fixtures.ReadFile("testdata/error-70.json")
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// authorized reports whether the request carries a valid token for
// Username and Password. Misuse of the scheme fails the test.
func (s *Server) authorized(endpoint string, params url.Values) bool {
	if params.Has("p") {
		s.t.Errorf("subsonictest: %s: plain password sent; sign with token+salt", endpoint)
		return false
	}
	salt, token := params.Get("s"), params.Get("t")
	if salt == "" || token == "" {
		s.t.Errorf("subsonictest: %s: missing token or salt", endpoint)
		return false
	}
	s.mu.Lock()
	reused := s.salts[salt]
	s.salts[salt] = true
	s.mu.Unlock()
	if reused {
		s.t.Errorf(
			"subsonictest: %s: salt %q reused; every request needs a fresh salt",
			endpoint,
			salt,
		)
	}
	sum := md5.Sum([]byte(Password + salt))
	return params.Get("u") == Username && token == hex.EncodeToString(sum[:])
}
