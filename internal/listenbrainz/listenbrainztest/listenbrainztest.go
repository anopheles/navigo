// Package listenbrainztest is a fake ListenBrainz API for tests, in the
// spirit of ADR 0003: the real client runs against it over HTTP. Its
// replies are hand-written in testdata, since a synthetic ListenBrainz
// user can't be recorded the way the fixture Navidrome is.
//
// User "ann" has four Recommendations, one of which the metadata leaves
// out and one of which, Undertow by The Quiet Ones, is in the synthetic
// fixture library; user "new" has none yet (204); any other user doesn't exist (404).
package listenbrainztest

import (
	"embed"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// The fixture users, and the last_updated of ann's list.
const (
	User        = "ann"
	NewUser     = "new"
	LastUpdated = 1792692721
)

//go:embed testdata
var fixtures embed.FS

// Server is the fake API. URL is its base, for listenbrainz.New.
type Server struct {
	*httptest.Server
	t *testing.T

	mu        sync.Mutex
	requests  []string
	rateLimit int // the next this many requests get a 429
	fail      int // the next this many requests get a 500
}

// New starts a fake ListenBrainz, closed when the test ends.
func New(t *testing.T) *Server {
	t.Helper()
	s := &Server{t: t}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.Close)
	return s
}

// RateLimitNext makes the next n requests get a 429 with a reset of 0 s.
func (s *Server) RateLimitNext(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rateLimit = n
}

// FailNext makes the next n requests get a 500.
func (s *Server) FailNext(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fail = n
}

// Requests are the requests served so far, as "METHOD path".
func (s *Server) Requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.requests...)
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.requests = append(s.requests, r.Method+" "+r.URL.Path)
	limited, failed := s.rateLimit > 0, s.fail > 0 && s.rateLimit == 0
	if limited {
		s.rateLimit--
	} else if failed {
		s.fail--
	}
	s.mu.Unlock()
	switch {
	case limited:
		w.Header().Set("X-RateLimit-Reset-In", "0")
		http.Error(
			w,
			`{"code":429,"error":"You have exceeded your rate limit."}`,
			http.StatusTooManyRequests,
		)
		return
	case failed:
		http.Error(w, "boom", http.StatusInternalServerError)
		return
	}

	if user, ok := strings.CutPrefix(r.URL.Path, "/1/cf/recommendation/user/"); ok &&
		r.Method == http.MethodGet {
		switch strings.TrimSuffix(user, "/recording") {
		case User:
			if r.URL.Query().Get("count") != "1000" {
				s.t.Errorf("listenbrainztest: count=%q, want 1000", r.URL.Query().Get("count"))
			}
			s.file(w, "recommendations.json")
		case NewUser:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, `{"code":404,"error":"Cannot find user"}`, http.StatusNotFound)
		}
		return
	}
	if r.URL.Path == "/1/metadata/recording/" && r.Method == http.MethodPost {
		var body struct {
			MBIDs []string `json:"recording_mbids"`
			Inc   string   `json:"inc"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Inc != "artist" {
			s.t.Errorf(
				"listenbrainztest: metadata body %+v (%v), want recording_mbids and inc=artist",
				body,
				err,
			)
		}
		s.file(w, "metadata.json")
		return
	}
	s.t.Errorf("listenbrainztest: unexpected request %s %s", r.Method, r.URL)
	http.NotFound(w, r)
}

func (s *Server) file(w http.ResponseWriter, name string) {
	b, err := fixtures.ReadFile("testdata/" + name)
	if err != nil {
		s.t.Errorf("listenbrainztest: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
}
