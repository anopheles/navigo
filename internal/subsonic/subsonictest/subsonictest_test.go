package subsonictest

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

var saltSeq atomic.Int64

// signed returns params plus auth for user and password, with a new salt.
func signed(user, password string, params url.Values) url.Values {
	salt := fmt.Sprintf("salt%d", saltSeq.Add(1))
	sum := md5.Sum([]byte(password + salt))
	q := url.Values{"u": {user}, "t": {hex.EncodeToString(sum[:])}, "s": {salt}, "v": {"1.16.1"}, "c": {"test"}, "f": {"json"}}
	for k, v := range params {
		q[k] = v
	}
	return q
}

type response struct {
	Status string
	Error  struct{ Code int }
	Album  struct {
		Name string
		Song []struct{ ID string }
	}
	Artists struct {
		Index []struct {
			Artist []struct{ Name string }
		}
	}
}

func get(t *testing.T, s *Server, endpoint string, q url.Values) response {
	t.Helper()
	resp, err := http.Get(s.URL + "/rest/" + endpoint + "?" + q.Encode())
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	return decode(t, resp)
}

func decode(t *testing.T, resp *http.Response) response {
	t.Helper()
	var env struct {
		R response `json:"subsonic-response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	return env.R
}

func TestServesRecordedFixtures(t *testing.T) {
	s := NewServer(t)

	r := get(t, s, "getArtists.view", signed(Username, Password, nil))
	if r.Status != "ok" || len(r.Artists.Index) == 0 {
		t.Fatalf("getArtists = %+v, want ok with artists", r)
	}

	// The long album has more than a page of tracks.
	long := ""
	entries, _ := fs.Glob(fixtures, "testdata/getAlbum.id=*.json")
	for _, e := range entries {
		id := strings.TrimSuffix(strings.TrimPrefix(e, "testdata/getAlbum.id="), ".json")
		if id == NotFoundID {
			continue
		}
		if a := get(t, s, "getAlbum", signed(Username, Password, url.Values{"id": {id}})); len(a.Album.Song) > 500 {
			long = a.Album.Name
		}
	}
	if long != "The Long Haul" {
		t.Errorf("album with more than 500 tracks = %q, want The Long Haul", long)
	}

	if r := get(t, s, "getAlbum", signed(Username, Password, url.Values{"id": {NotFoundID}})); r.Status != "failed" || r.Error.Code != 70 {
		t.Errorf("getAlbum %s = %+v, want error 70", NotFoundID, r)
	}
}

func TestFormPost(t *testing.T) {
	s := NewServer(t)
	resp, err := http.PostForm(s.URL+"/rest/scrobble", signed(Username, Password, url.Values{"id": {"x"}, "submission": {"false"}}))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if r := decode(t, resp); r.Status != "ok" {
		t.Fatalf("scrobble = %+v, want ok", r)
	}
	reqs := s.Requests()
	if len(reqs) != 1 || reqs[0].Endpoint != "scrobble" || reqs[0].Params.Get("submission") != "false" || reqs[0].Params.Has("t") {
		t.Errorf("Requests() = %+v, want one scrobble with submission=false and no auth params", reqs)
	}
}

func TestWrongCredentials(t *testing.T) {
	s := NewServer(t)
	for _, q := range []url.Values{signed(Username, "wrong", nil), signed("someone", Password, nil)} {
		if r := get(t, s, "ping", q); r.Status != "failed" || r.Error.Code != 40 {
			t.Errorf("ping as %s = %+v, want error 40", q.Get("u"), r)
		}
	}
}

// recorder captures test failures the server reports.
type recorder struct {
	testing.TB
	errs []string
}

func (r *recorder) Errorf(format string, args ...any) {
	r.errs = append(r.errs, fmt.Sprintf(format, args...))
}

func TestMisuseFailsTheTest(t *testing.T) {
	q := signed(Username, Password, nil)
	plain := url.Values{"u": {Username}, "p": {Password}, "f": {"json"}}
	tests := []struct {
		name string
		reqs []url.Values
		want string
	}{
		{"reused salt", []url.Values{q, q}, "salt"},
		{"plain password", []url.Values{plain}, "plain password"},
		{"missing fixture", []url.Values{signed(Username, Password, url.Values{"id": {"nope"}})}, "no fixture"},
		{"xml", []url.Values{signed(Username, Password, url.Values{"f": {"xml"}})}, "want json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &recorder{TB: t}
			s := NewServer(rec)
			for _, q := range tt.reqs {
				endpoint := "ping"
				if q.Has("id") {
					endpoint = "getAlbum"
				}
				get(t, s, endpoint, q)
			}
			if len(rec.errs) == 0 || !strings.Contains(strings.Join(rec.errs, "\n"), tt.want) {
				t.Errorf("errors = %q, want one containing %q", rec.errs, tt.want)
			}
		})
	}
}

func TestEveryFixtureIsAResponse(t *testing.T) {
	entries, err := fs.Glob(fixtures, "testdata/*.json")
	if err != nil || len(entries) == 0 {
		t.Fatalf("no fixtures: %v", err)
	}
	for _, e := range entries {
		b, _ := fixtures.ReadFile(e)
		var env struct {
			R *struct{ Status string } `json:"subsonic-response"`
		}
		if err := json.Unmarshal(b, &env); err != nil || env.R == nil || env.R.Status == "" {
			t.Errorf("%s is not a subsonic-response: %v", e, err)
		}
	}
}
