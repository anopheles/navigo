// PROTOTYPE: throwaway code for "What can a user change about navigo's look?"
// (anopheles/navigo#52). Never merge into main.
//
// One fake screen (library window, queue window, hint line, now-playing bar,
// status line) drawn with three theming surfaces, switchable with F2:
//
//	A  Roles    colors: overrides any of the 7 palette roles by name
//	B  Presets  theme: picks a named preset, nothing else is configurable
//	C  Accent   accent: one colour that drives playing, progress and focus
//
// t   next option in the current variant
// c   colour depth the terminal would have: 16 / 256 / truecolor
// n   NO_COLOR on/off
// b   simulated terminal background: none / light / dark
// q   quit
//
// Run: cd prototype/theming && go run .   (best at 100x30 or wider)
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ---------- colours ----------

// A colour is "" (terminal default), "ansi:N" (one of the 16) or "#rrggbb".
type sty struct {
	fg, bg                       string
	bold, faint, reverse, underl bool
}

const (
	rBase = iota
	rFaint
	rCursor // cursor row, focused window
	rCursor2
	rPlaying
	rKey
	rErr
	rSel
	rTitle
	rTitle2
	nRoles
)

var roleNames = [nRoles]string{"base", "faint", "cursor", "cursor-other", "playing", "key", "error", "selection", "title", "title-other"}

type theme [nRoles]sty

func defaultTheme() theme {
	var t theme
	t[rFaint] = sty{faint: true}
	t[rCursor] = sty{reverse: true}
	t[rCursor2] = sty{underl: true}
	t[rPlaying] = sty{fg: "ansi:5", bold: true}
	t[rKey] = sty{fg: "ansi:6", bold: true}
	t[rErr] = sty{fg: "ansi:1"}
	t[rSel] = sty{bg: "ansi:4"}
	t[rTitle] = sty{reverse: true, bold: true}
	t[rTitle2] = sty{bold: true}
	return t
}

var ansi16 = [16][3]int{
	{0, 0, 0}, {205, 0, 0}, {0, 205, 0}, {205, 205, 0}, {0, 0, 238}, {205, 0, 205}, {0, 205, 205}, {229, 229, 229},
	{127, 127, 127}, {255, 0, 0}, {0, 255, 0}, {255, 255, 0}, {92, 92, 255}, {255, 0, 255}, {0, 255, 255}, {255, 255, 255},
}

var named = map[string]string{"green": "ansi:2", "yellow": "ansi:3", "blue": "ansi:4", "red": "ansi:1", "magenta": "ansi:5", "cyan": "ansi:6", "gray": "ansi:8"}

func rgb(h string) (int, int, int) {
	v, _ := strconv.ParseUint(h[1:], 16, 32)
	return int(v >> 16), int(v >> 8 & 255), int(v & 255)
}

func dist(r, g, b int, c [3]int) int {
	return (r-c[0])*(r-c[0]) + (g-c[1])*(g-c[1]) + (b-c[2])*(b-c[2])
}

// resolve turns a colour into what a terminal of the given depth would show.
func resolve(c string, depth int) string {
	if c == "" {
		return ""
	}
	if n, ok := strings.CutPrefix(c, "ansi:"); ok {
		return n
	}
	r, g, b := rgb(c)
	switch depth {
	case 0: // 16
		best, bd := 0, 1<<30
		for i, p := range ansi16 {
			if d := dist(r, g, b, p); d < bd {
				best, bd = i, d
			}
		}
		return strconv.Itoa(best)
	case 1: // 256: nearest of the 6x6x6 cube
		q := func(v int) int { return (v*5 + 127) / 255 }
		return strconv.Itoa(16 + 36*q(r) + 6*q(g) + q(b))
	}
	return c
}

// ---------- variants ----------

type option struct {
	label, yaml, cost string
	th                theme
}

func with(f func(*theme)) theme { t := defaultTheme(); f(&t); return t }

func variantA() []option {
	return []option{
		{"nothing set", "# no colors: block", "7 roles to document, validate and keep stable", defaultTheme()},
		{"swap two", "colors:\n  playing: green\n  key: yellow", "", with(func(t *theme) {
			t[rPlaying].fg, t[rKey].fg = named["green"], named["yellow"]
		})},
		{"hex palette", "colors:\n  playing: '#ff9e64'\n  key: '#7dcfff'\n  error: '#f7768e'\n  selection: '#3b4261'", "", with(func(t *theme) {
			t[rPlaying].fg, t[rKey].fg, t[rErr].fg, t[rSel].bg = "#ff9e64", "#7dcfff", "#f7768e", "#3b4261"
		})},
		{"selection gray", "colors:\n  selection: gray", "a user can make the selection invisible", with(func(t *theme) {
			t[rSel].bg = named["gray"]
		})},
	}
}

func preset(play, key, err, sel string) theme {
	return with(func(t *theme) {
		t[rPlaying].fg, t[rKey].fg, t[rErr].fg, t[rSel].bg = play, key, err, sel
	})
}

func variantB() []option {
	mono := with(func(t *theme) {
		t[rPlaying] = sty{bold: true}
		t[rKey] = sty{bold: true, underl: true}
		t[rErr] = sty{bold: true, reverse: true}
		t[rSel] = sty{reverse: true}
	})
	return []option{
		{"default", "theme: default", "1 key; presets must be curated and tested on light and dark", defaultTheme()},
		{"gruvbox", "theme: gruvbox", "", preset("#d3869b", "#8ec07c", "#fb4934", "#504945")},
		{"nord", "theme: nord", "", preset("#b48ead", "#88c0d0", "#bf616a", "#434c5e")},
		{"solarized", "theme: solarized", "", preset("#d33682", "#2aa198", "#dc322f", "#073642")},
		{"mono", "theme: mono", "attributes only; same as NO_COLOR", mono},
	}
}

func variantC() []option {
	acc := func(c string) theme {
		return with(func(t *theme) {
			t[rPlaying].fg = c
			t[rTitle] = sty{fg: c, bold: true, reverse: true}
		})
	}
	return []option{
		{"magenta (default)", "# no accent: key", "1 key, but playing and focus now share a colour", defaultTheme()},
		{"green", "accent: green", "", acc(named["green"])},
		{"orange", "accent: '#ff9e64'", "", acc("#ff9e64")},
		{"blue", "accent: '#7aa2f7'", "", acc("#7aa2f7")},
		{"red", "accent: red", "red now means both 'playing' and 'error'", acc(named["red"])},
	}
}

// ---------- model ----------

type model struct {
	v, o    [3]int // variant; option per variant
	variant int
	depth   int // 0=16 1=256 2=truecolor
	noColor bool
	bg      int // 0 terminal, 1 light, 2 dark
	opts    [3][]option
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch k.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "f2", "tab":
		m.variant = (m.variant + 1) % 3
	case "t":
		m.o[m.variant] = (m.o[m.variant] + 1) % len(m.opts[m.variant])
	case "c":
		m.depth = (m.depth + 1) % 3
	case "n":
		m.noColor = !m.noColor
	case "b":
		m.bg = (m.bg + 1) % 3
	}
	return m, nil
}

const width = 100

func (m model) style(s sty) lipgloss.Style {
	st := lipgloss.NewStyle()
	base := [3]string{"", "#fdf6e3", "#1e1e1e"}[m.bg]
	basefg := [3]string{"", "#333333", "#d0d0d0"}[m.bg]
	fg, bg := s.fg, s.bg
	if m.noColor {
		fg, bg = "", ""
	}
	if bg == "" {
		bg = base
	}
	if fg == "" {
		fg = basefg
	}
	if m.bg != 0 { // simulated backgrounds are always truecolor
		if bg == base {
			st = st.Background(lipgloss.Color(base))
		}
	}
	if c := resolve(fg, m.depth); c != "" {
		st = st.Foreground(lipgloss.Color(c))
	}
	if bg != "" && bg != base {
		st = st.Background(lipgloss.Color(resolve(bg, m.depth)))
	} else if bg == base && base != "" {
		st = st.Background(lipgloss.Color(base))
	}
	if s.reverse {
		st = st.Reverse(true)
	}
	return st.Bold(s.bold).Faint(s.faint).Underline(s.underl)
}

type seg struct {
	s    string
	role int
}

func (m model) line(th theme, segs ...seg) string {
	var b strings.Builder
	n := 0
	for _, sg := range segs {
		b.WriteString(m.style(th[sg.role]).Render(sg.s))
		n += lipgloss.Width(sg.s)
	}
	if n < width {
		b.WriteString(m.style(th[rBase]).Render(strings.Repeat(" ", width-n)))
	}
	return b.String()
}

func pad(s string, n int) string {
	if w := lipgloss.Width(s); w < n {
		return s + strings.Repeat(" ", n-w)
	}
	return s
}

func (m model) View() tea.View {
	op := m.opts[m.variant][m.o[m.variant]]
	th := op.th
	if m.noColor {
		// NO_COLOR keeps attributes; the selection needs a non-colour cue.
		th[rSel].reverse = true
	}
	albums := []string{"Night Drive", "Blue Hour", "Low Tide", "Paper Lanterns", "Salt & Static", "Afterimage"}
	queue := []string{"Blue Hour", "Low Tide", "Night Drive", "Afterimage"}
	const lw, rw = 58, 40
	var out []string
	out = append(out, m.line(th, seg{" prototype ", rFaint}, seg{fmt.Sprintf("variant %s · %s", string("ABC"[m.variant]), []string{"Roles", "Presets", "Accent"}[m.variant]), rBase}))
	out = append(out, m.line(th, seg{pad(" Albums ", lw), rTitle}, seg{pad(" Queue ", rw), rTitle2}))
	for i := 0; i < 6; i++ {
		left := []seg{{pad("  "+albums[i], lw), rBase}}
		switch i {
		case 2:
			left = []seg{{pad("▎ "+albums[i], lw), rSel}}
		case 3:
			left = []seg{{pad("▎ "+albums[i], lw), rCursor}}
		}
		right := seg{pad("  "+queue[min(i, 3)], rw), rBase}
		if i > 3 {
			right = seg{pad("", rw), rBase}
		}
		switch i {
		case 0:
			right = seg{pad("▶ "+queue[0], rw), rPlaying}
		case 1:
			right = seg{pad("  "+queue[1], rw), rCursor2}
		}
		out = append(out, m.line(th, append(left, right)...))
	}
	out = append(out, m.line(th, seg{"  ", rBase}, seg{"<CR>", rKey}, seg{" play  ", rFaint}, seg{"A", rKey}, seg{" add to queue  ", rFaint}, seg{"a", rKey}, seg{" play next  ", rFaint}, seg{"V", rKey}, seg{" select  ", rFaint}, seg{"ga", rKey}, seg{" album  ", rFaint}, seg{"'?", rKey}, seg{" help", rFaint}))
	out = append(out, m.line(th, seg{strings.Repeat("─", width), rFaint}))
	out = append(out, m.line(th, seg{" ▶ Blue Hour", rPlaying}, seg{"  Ann Lee · Night Drive   1/4  ", rBase}, seg{"RG track", rFaint}))
	out = append(out, m.line(th, seg{" " + strings.Repeat("━", 3), rPlaying}, seg{strings.Repeat("━", 19), rFaint}, seg{"  1:12 / 4:03   ", rBase}, seg{"repeat all", rFaint}))
	out = append(out, m.line(th, seg{" can't reach https://music.example.com · retrying", rErr}))
	out = append(out, m.line(th))
	out = append(out, m.line(th, seg{" config.yaml", rFaint}))
	for _, l := range strings.Split(op.yaml, "\n") {
		out = append(out, m.line(th, seg{"   " + l, rKey}))
	}
	out = append(out, m.line(th))
	if op.cost != "" {
		out = append(out, m.line(th, seg{" cost: " + op.cost, rBase}))
	}
	depth := [3]string{"16", "256", "truecolor"}[m.depth]
	bg := [3]string{"terminal", "light", "dark"}[m.bg]
	out = append(out, m.line(th, seg{fmt.Sprintf(" option %d/%d %q · depth %s · background %s · NO_COLOR %v", m.o[m.variant]+1, len(m.opts[m.variant]), op.label, depth, bg, m.noColor), rFaint}))
	out = append(out, m.line(th, seg{" F2 variant · t option · c depth · b background · n NO_COLOR · q quit", rFaint}))
	return tea.NewView(strings.Join(out, "\n"))
}

func main() {
	m := model{opts: [3][]option{variantA(), variantB(), variantC()}, depth: 2}
	if _, err := tea.NewProgram(m).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
