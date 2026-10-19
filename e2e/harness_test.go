//go:build e2e

package e2e

import (
	"bufio"
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
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/vt"
	"github.com/charmbracelet/x/xpty"
)

const (
	width  = 120
	height = 40
)

var binary string // built once in TestMain

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "navigo-e2e-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	binary = filepath.Join(dir, "navigo")
	build := exec.Command("go", "build", "-o", binary, "../cmd/navigo")
	build.Stdout, build.Stderr = os.Stderr, os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "building navigo:", err)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// server is the Navidrome the tests run against.
type server struct{ url, user, password string }

// target returns the server from the environment or .env.e2e, skipping the
// test when there is none.
func target(t *testing.T) server {
	t.Helper()
	s := server{os.Getenv("NAVIGO_E2E_URL"), os.Getenv("NAVIGO_E2E_USER"), os.Getenv("NAVIGO_E2E_PASSWORD")}
	if s.url == "" {
		if f, err := os.Open("../.env.e2e"); err == nil {
			defer f.Close()
			sc := bufio.NewScanner(f)
			for sc.Scan() {
				k, v, _ := strings.Cut(strings.TrimSpace(sc.Text()), "=")
				v = strings.Trim(v, `"'`)
				switch k {
				case "NAVIGO_E2E_URL":
					s.url = v
				case "NAVIGO_E2E_USER":
					s.user = v
				case "NAVIGO_E2E_PASSWORD":
					s.password = v
				}
			}
		}
	}
	if s.url == "" || s.user == "" || s.password == "" {
		t.Skip("no e2e server: set NAVIGO_E2E_URL/USER/PASSWORD or write .env.e2e")
	}
	if _, err := exec.LookPath("mpv"); err != nil {
		t.Fatal("e2e needs mpv on PATH")
	}
	return s
}

// session is one navigo run in a pseudo-terminal.
type session struct {
	t    *testing.T
	pty  xpty.Pty
	term *vt.SafeEmulator
	cmd  *exec.Cmd
	home string
	done chan struct{}
}

// start runs navigo with its own HOME and XDG dirs and a config.yaml that
// logs in to s and keeps mpv silent.
func start(t *testing.T, s server) *session {
	t.Helper()
	home := t.TempDir()
	env := []string{"HOME=" + home, "TERM=xterm-256color", "PATH=" + os.Getenv("PATH")}
	for _, d := range []string{"XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_RUNTIME_DIR", "XDG_DATA_HOME", "XDG_STATE_HOME"} {
		p := filepath.Join(home, strings.ToLower(strings.TrimPrefix(d, "XDG_")))
		if err := os.MkdirAll(p, 0o700); err != nil {
			t.Fatal(err)
		}
		env = append(env, d+"="+p)
	}
	cfgDir := filepath.Join(home, "config_home", "navigo")
	cfg := fmt.Sprintf("server:\n  url: %q\n  user: %q\n  password: %q\nplayer:\n  mpv_args: [--ao=null]\n", s.url, s.user, s.password)
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	p, err := xpty.NewPty(width, height)
	if err != nil {
		t.Fatal(err)
	}
	term := vt.NewSafeEmulator(width, height)
	cmd := exec.Command(binary, "--debug")
	cmd.Env = env
	if err := p.Start(cmd); err != nil {
		t.Fatal(err)
	}
	ss := &session{t: t, pty: p, term: term, cmd: cmd, home: home, done: make(chan struct{})}
	go func() { _, _ = io.Copy(term, p) }() // navigo's output onto the screen
	go func() { _, _ = io.Copy(p, term) }() // the terminal's replies to navigo's queries
	go func() { _ = xpty.WaitProcess(context.Background(), cmd); close(ss.done) }()
	t.Cleanup(ss.stop)
	return ss
}

// screen is the visible text.
func (s *session) screen() string { return s.term.String() }

// keys writes raw key bytes, such as "j", "\r" for <CR> or ":q\r".
func (s *session) keys(b string) {
	s.t.Helper()
	if _, err := s.pty.Write([]byte(b)); err != nil {
		s.t.Fatal(err)
	}
}

// waitFor polls the screen until ok returns true, failing after timeout.
func (s *session) waitFor(what string, timeout time.Duration, ok func(screen string) bool) string {
	s.t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		scr := s.screen()
		if ok(scr) {
			return scr
		}
		if time.Now().After(deadline) {
			s.t.Fatalf("timed out after %v waiting for %s", timeout, what)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (s *session) waitText(text string, timeout time.Duration) string {
	s.t.Helper()
	return s.waitFor(fmt.Sprintf("%q on screen", text), timeout, func(scr string) bool { return strings.Contains(scr, text) })
}

// stop quits navigo with :q, kills it if that doesn't work, and on a
// failed test writes out the screen and navigo's debug log.
func (s *session) stop() {
	if s.t.Failed() {
		s.t.Logf("screen:\n%s", s.screen())
		if log, err := os.ReadFile(filepath.Join(s.home, "cache_home", "navigo", "navigo.log")); err == nil {
			s.t.Logf("navigo.log:\n%s", log)
		}
	}
	// <Esc> first leaves any pending keys or the : line; a pause after it,
	// or the terminal reads <Esc>: as Alt-:.
	_, _ = s.pty.Write([]byte("\x1b"))
	time.Sleep(200 * time.Millisecond)
	_, _ = s.pty.Write([]byte(":q\r"))
	select {
	case <-s.done:
	case <-time.After(5 * time.Second):
		_ = s.cmd.Process.Kill()
		<-s.done
		s.t.Error("navigo didn't quit on :q")
	}
	_ = s.pty.Close()
}

// nowPlaying asks the server what is playing for this user.
func nowPlaying(t *testing.T, s server) []string {
	t.Helper()
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	salt := hex.EncodeToString(b)
	sum := md5.Sum([]byte(s.password + salt))
	q := url.Values{"u": {s.user}, "t": {hex.EncodeToString(sum[:])}, "s": {salt}, "v": {"1.16.1"}, "c": {"navigo-e2e"}, "f": {"json"}}
	resp, err := http.Get(strings.TrimRight(s.url, "/") + "/rest/getNowPlaying?" + q.Encode())
	if err != nil {
		t.Fatal(errors.Unwrap(err)) // the URL holds a token
	}
	defer resp.Body.Close()
	var r struct {
		Response struct {
			Status     string `json:"status"`
			NowPlaying struct {
				Entry []struct {
					Title    string `json:"title"`
					Username string `json:"username"`
				} `json:"entry"`
			} `json:"nowPlaying"`
		} `json:"subsonic-response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil || r.Response.Status != "ok" {
		t.Fatalf("getNowPlaying: %v, status %q", err, r.Response.Status)
	}
	var titles []string
	for _, e := range r.Response.NowPlaying.Entry {
		if e.Username == s.user {
			titles = append(titles, e.Title)
		}
	}
	return titles
}
