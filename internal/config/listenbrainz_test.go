package config

import (
	"strings"
	"testing"
)

func TestListenBrainz(t *testing.T) {
	for _, tc := range []struct {
		in      string
		on      bool
		api     string
		problem string
	}{
		{in: "hints: true\n", api: defaultListenBrainz},
		{in: "listenbrainz:\n  user: ann\n", on: true, api: defaultListenBrainz},
		{
			in: "listenbrainz:\n  user: ann\n  url: https://lb.example.org/\n",
			on: true, api: "https://lb.example.org",
		},
		{in: "listenbrainz:\n  user: ''\n", api: defaultListenBrainz, problem: "3:9: listenbrainz.user"},
		{
			in: "listenbrainz:\n  user: ann\n  url: lb.example.org\n", api: defaultListenBrainz,
			problem: "4:8: listenbrainz.url: give an http or https URL",
		},
		{in: "listenbrainz: ann\n", api: defaultListenBrainz, problem: "2:15: listenbrainz: give a block"},
	} {
		c, err := Parse([]byte("\n" + tc.in)) // a blank first line, to check positions
		if err != nil {
			t.Fatalf("Parse(%q): %v", tc.in, err)
		}
		l := c.ListenBrainz
		if l.On() != tc.on || l.API() != tc.api {
			t.Errorf("%q: on %v api %q, want %v %q", tc.in, l.On(), l.API(), tc.on, tc.api)
		}
		ps := c.Problems()
		switch {
		case tc.problem == "" && len(ps) > 0:
			t.Errorf("%q: problems %v, want none", tc.in, ps)
		case tc.problem != "" && (len(ps) != 1 || !strings.Contains(ps[0].Format("config.yaml"), tc.problem)):
			t.Errorf("%q: problems %v, want one containing %q", tc.in, ps, tc.problem)
		}
	}
}
