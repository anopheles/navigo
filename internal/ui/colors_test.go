package ui

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"
)

var sgrRE = regexp.MustCompile(`\x1b\[([0-9;]*)m`)

// sgrs lists the SGR parameter lists in s, such as "7" or "1;35".
func sgrs(s string) []string {
	var out []string
	for _, m := range sgrRE.FindAllStringSubmatch(s, -1) {
		out = append(out, m[1])
	}
	return out
}

// has reports whether one of the SGRs in s sets attribute a.
func has(s string, a int) bool {
	for _, p := range sgrs(s) {
		if slices.Contains(strings.Split(p, ";"), strconv.Itoa(a)) {
			return true
		}
	}
	return false
}

// playingScreen plays Low Tide with the cursors elsewhere: on Blue Hour
// in the library, on the first row of the focused queue.
func playingScreen(t *testing.T) *harness {
	h := newHarness(t, 120, 40, nil)
	h.keys("3j<CR>j<CR>jj<Tab>")
	return h
}

// Every colour is one of the terminal's 16: no 256-colour or true-colour
// values.
func TestOnlySixteenColours(t *testing.T) {
	h := playingScreen(t)
	for _, screen := range []string{h.m.render(), func() string { h.keys("?"); return h.m.render() }()} {
		for _, p := range sgrs(screen) {
			if strings.Contains(p, "38;") || strings.Contains(p, "48;") ||
				strings.Contains(p, "58;") {
				t.Fatalf("extended colour in SGR %q", p)
			}
		}
	}
}

// With colours off the screen still shows which window has the focus,
// where each cursor is, and what plays.
func TestColoursOff(t *testing.T) {
	h := playingScreen(t)
	var b strings.Builder
	w := colorprofile.NewWriter(&b, nil)
	w.Profile = colorprofile.ASCII
	if _, err := w.WriteString(h.m.render()); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(b.String(), "\n")
	for _, l := range lines {
		for _, p := range sgrs(l) {
			for _, a := range strings.Split(p, ";") {
				if n, _ := strconv.Atoi(a); n >= 30 && n <= 49 || n >= 90 && n <= 107 {
					t.Fatalf("a colour (%s) with colours off: %q", p, l)
				}
			}
		}
	}
	find := func(text string) string {
		for _, l := range lines {
			if strings.Contains(l, text) {
				return l
			}
		}
		t.Fatalf("no line with %q", text)
		return ""
	}
	// The focused window's title is reverse, the other's only bold: the
	// first SGR is the library title's, the last before "Queue" the queue's.
	title := lines[0]
	before := sgrs(title[:strings.Index(title, "Queue (6)")])
	if len(before) < 2 || strings.Contains(";"+before[0]+";", ";7;") ||
		!strings.Contains(";"+before[len(before)-1]+";", ";7;") {
		t.Errorf("titles don't show the focus: %q", title)
	}
	// The cursor is reverse in the focused queue (on its first row),
	// underlined in the library (on Blue Hour).
	q := strings.SplitN(find("Dusk Signal"), "│", 2)
	if len(q) != 2 || !has(q[1], 7) {
		t.Errorf("the queue's cursor row isn't reverse: %q", find("Dusk Signal"))
	}
	lib := strings.SplitN(find("4 Blue Hour"), "│", 2)[0]
	if !has(lib, 4) {
		t.Errorf("the library's cursor row isn't underlined: %q", lib)
	}
	// What plays is bold, with ▶.
	playing := strings.SplitN(find("▶ "), "│", 2)
	if len(playing) != 2 || !has(playing[1], 1) || !strings.Contains(playing[1], "▶") {
		t.Errorf("the playing row isn't bold with ▶: %q", playing)
	}
}
