package ui

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"syscall"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/anopheles/navigo/internal/config"
	"github.com/anopheles/navigo/internal/subsonic"
	"github.com/anopheles/navigo/internal/subsonic/subsonictest"
)

// A long error and the keyring question wrap inside the box, whose
// borders stay on screen, and the answers to the question show (#145).
func TestLoginFormWrapsAtEveryWidth(t *testing.T) {
	for _, w := range []int{40, 60, 80} {
		t.Run(fmt.Sprint(w), func(t *testing.T) {
			h := newHarness(t, w, 30, func(c *config.Config, srv *subsonictest.Server) {
				c.Server = config.Server{URL: srv.URL, User: subsonictest.Username}
			})
			h.m.deps.Keyring = longErrKeyring{}
			h.typeText(subsonictest.Password)
			h.keys("<CR>")
			h.contains("y save it there", "n only for", "instead?")
			h.lacks("<CR> log in")
			for _, l := range strings.Split(h.screen(), "\n") {
				if lipgloss.Width(l) > w {
					t.Errorf("line %d wide at %d columns: %q", lipgloss.Width(l), w, l)
				}
				if strings.Contains(l, "│") && strings.Count(l, "│") != 2 {
					t.Errorf("box line without both borders: %q", l)
				}
			}
		})
	}
}

type longErrKeyring struct{ brokenKeyring }

func (longErrKeyring) Set(string, string, string) error {
	return errors.New("dial unix /run/user/1000/bus: connect: no such file or directory, " +
		"and the secret service is not available on this machine at all")
}

// Esc goes back from :login, so its footer says back, not quit.
func TestLoginFooterSaysBackMidSession(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys(":login<CR>")
	h.contains("<Esc> back")
	h.lacks("<Esc> quit")
}

func TestLoginErrorsInWords(t *testing.T) {
	refused := &url.Error{
		Op: "Get", URL: "http://127.0.0.1:14599/rest/ping",
		Err: &net.OpError{Op: "dial", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)},
	}
	for _, c := range []struct {
		err  error
		want string
	}{
		{refused, "nothing answers at 127.0.0.1:14599 · is the server running?"},
		{
			&url.Error{Op: "Get", URL: "http://music.example/rest/ping", Err: &net.DNSError{
				Err: "no such host", Name: "music.example", IsNotFound: true,
			}},
			"can't find the host music.example · check the URL",
		},
		{
			fmt.Errorf("ping: %w", context.DeadlineExceeded),
			"the server didn't answer in time · check the URL and that it's running",
		},
		{
			fmt.Errorf("%w: invalid character '<'", subsonic.ErrNotSubsonic),
			"that URL isn't a Subsonic server · check its path",
		},
		{errors.New("something else"), "something else"},
	} {
		if got := loginErr(c.err); got != c.want {
			t.Errorf("loginErr(%v) = %q, want %q", c.err, got, c.want)
		}
	}
}
