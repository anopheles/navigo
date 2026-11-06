// PROTOTYPE — throwaway, lives on branch prototype/bar-contrast. Do not merge.
//
// Question: the now-playing bar (the "status line") is hard to read. Which
// way of colouring it stays readable for every colour it can be given: a
// theme, a colors: entry, one of the 16 terminal colours, or any album
// cover's colour?
//
// Three variants plus the current bar, switchable with ←/→ in
// `go run ./cmd/prototype-barcontrast`, or in the real app with
// NAVIGO_PROTOTYPE_BAR=A|B|C. The matrix shows each input painted on a
// simulated terminal (its background, foreground and palette), with the
// WCAG contrast ratio of each part of the bar.
package ui

import (
	"fmt"
	"image/color"
	"math"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/lucasb-eyer/go-colorful"

	"github.com/anopheles/navigo/internal/config"
)

// barInk is how the bar is coloured for one role colour. nil fg is the
// terminal's own text colour, nil bg its own background. rev draws the
// mode and time segments as the role in reverse video: the terminal's
// background as text on the role.
type barInk struct {
	modeFg, modeBg color.Color
	midFg, midBg   color.Color
	title          color.Color
	rev            bool
}

type protoVariant struct {
	Key, Name, About string
	ink              func(role color.Color, termBg colorful.Color) barInk
}

// ProtoPaletteKnown simulates navigo having asked the terminal for its 16
// colours (OSC 4, as it already asks for the background with OSC 11), so a
// role like "magenta" has an exact value. Toggled with p.
var ProtoPaletteKnown bool

// ProtoVariantCount is how many variants the switcher cycles through.
var ProtoVariantCount = len(protoVariants)

var protoVariants = []protoVariant{
	{
		"0", "Current",
		"black text on the role; the role on bright black; the title in the role",
		func(role color.Color, _ colorful.Color) barInk {
			return barInk{ansi.Black, role, role, ansi.BrightBlack, role, false}
		},
	},
	{
		"A", "Pick the ink",
		"keep every colour as given; pick black or white text by contrast, and lift/lower the role's lightness (OKLCH, hue kept) where it is text",
		func(role color.Color, bg colorful.Color) barInk {
			if n, ok := role.(ansi.BasicColor); ok { // no exact value: guess from xterm's palette
				ink := color.Color(ansi.BrightWhite)
				if strings.Contains(" 2 3 6 7 9 10 11 13 14 15 ", fmt.Sprintf(" %d ", n)) {
					ink = ansi.Black
				}
				return barInk{ink, role, role, nil, role, false}
			}
			rc, _ := colorful.MakeColor(role)
			ink := colorful.Color{R: 1, G: 1, B: 1}
			if contrast(colorful.Color{}, rc) > contrast(ink, rc) {
				ink = colorful.Color{}
			}
			surface := shiftL(bg, 0.12)
			return barInk{ink, role, ensure(role, surface, 4.5), surface, ensure(role, bg, 4.5), false}
		},
	},
	{
		"B", "Fit the colour",
		"the text is the terminal's own background; the role's lightness moves until that reads on it (pastel on dark terminals, deep on light); the middle is a tint",
		func(role color.Color, bg colorful.Color) barInk {
			if _, ok := role.(ansi.BasicColor); ok { // reverse video: the pairing the theme was designed for
				return barInk{nil, nil, role, nil, role, true}
			}
			fill := ensure(role, bg, 4.5)
			fc, _ := colorful.MakeColor(fill)
			return barInk{bg, fill, nil, bg.BlendRgb(fc, 0.22), fill, false}
		},
	},
	{
		"C", "No fills",
		"drop the coloured blocks: the mode and title are bold text in the role, made to read on the terminal's background; nothing else is coloured",
		func(role color.Color, bg colorful.Color) barInk {
			if _, ok := role.(ansi.BasicColor); ok {
				return barInk{role, nil, nil, nil, role, false}
			}
			c := ensure(role, bg, 4.5)
			return barInk{c, nil, nil, nil, c, false}
		},
	},
}

// protoBar is the variant the real app draws, from NAVIGO_PROTOTYPE_BAR.
var protoBar = func() int {
	v := strings.ToUpper(os.Getenv("NAVIGO_PROTOTYPE_BAR"))
	for i, p := range protoVariants {
		if p.Key == v {
			return i
		}
	}
	return 0
}()

// inkFor is the bar's colours in the real app, for the variant chosen.
func (m *Model) inkFor(role color.Color) barInk {
	bg := colorful.Color{}
	if m.covers.bg != nil {
		bg, _ = colorful.MakeColor(m.covers.bg)
	}
	return protoVariants[protoBar].ink(role, bg)
}

// --- colour maths ---

func luminance(c colorful.Color) float64 {
	r, g, b := c.Clamped().LinearRgb()
	return 0.2126*r + 0.7152*g + 0.0722*b
}

// contrast is the WCAG 2 ratio, 1 to 21.
func contrast(a, b colorful.Color) float64 {
	la, lb := luminance(a), luminance(b)
	return (math.Max(la, lb) + 0.05) / (math.Min(la, lb) + 0.05)
}

// shiftL moves c's OKLCH lightness by d away from the nearer end.
func shiftL(c colorful.Color, d float64) colorful.Color {
	l, ch, h := c.OkLch()
	if l < 0.6 {
		l += d
	} else {
		l -= d
	}
	return colorful.OkLch(l, ch, h).Clamped()
}

// ensure moves c's OKLCH lightness, keeping its hue and as much chroma as
// fits in sRGB, until it has min contrast against bg.
func ensure(c color.Color, bg colorful.Color, min float64) color.Color {
	cf, ok := colorful.MakeColor(c)
	if !ok || contrast(cf, bg) >= min {
		return c
	}
	white, black := colorful.Color{R: 1, G: 1, B: 1}, colorful.Color{}
	up := contrast(white, bg) >= contrast(black, bg)
	l, ch, h := cf.OkLch()
	for range 120 {
		if up {
			l = math.Min(1, l+0.01)
		} else {
			l = math.Max(0, l-0.01)
		}
		cand := colorful.OkLch(l, ch, h)
		for k := 0; !cand.IsValid() && k < 80; k++ {
			ch *= 0.94
			cand = colorful.OkLch(l, ch, h)
		}
		if cand = cand.Clamped(); contrast(cand, bg) >= min {
			return cand
		}
	}
	if up {
		return white
	}
	return black
}

// --- the simulated terminals ---

// simTerm is a terminal's background, text and 16 colours. Missing
// palette entries are xterm's.
type simTerm struct {
	name   string
	bg, fg string
	pal    map[int]string
}

var xterm = [16]string{
	"#000000", "#cd0000", "#00cd00", "#cdcd00", "#0000ee", "#cd00cd", "#00cdcd", "#e5e5e5",
	"#7f7f7f", "#ff0000", "#00ff00", "#ffff00", "#5c5cff", "#ff00ff", "#00ffff", "#ffffff",
}

func hx(s string) colorful.Color { c, _ := colorful.Hex(s); return c }

func (t simTerm) resolve(c color.Color, def string) colorful.Color {
	switch v := c.(type) {
	case nil:
		return hx(def)
	case ansi.BasicColor:
		if s, ok := t.pal[int(v)]; ok {
			return hx(s)
		}
		return hx(xterm[v])
	}
	cf, _ := colorful.MakeColor(c)
	return cf
}

func pal16(s string) map[int]string {
	m := map[int]string{}
	for i, f := range strings.Fields(s) {
		m[i] = f
	}
	return m
}

var (
	xtermDark  = simTerm{"xterm dark", "#000000", "#e5e5e5", nil}
	xtermLight = simTerm{"xterm light", "#ffffff", "#000000", nil}
	solDark    = simTerm{"solarized dark", "#002b36", "#839496", pal16(
		"#073642 #dc322f #859900 #b58900 #268bd2 #d33682 #2aa198 #eee8d5 " +
			"#002b36 #cb4b16 #586e75 #657b83 #839496 #6c71c4 #93a1a1 #fdf6e3")}
	solLight  = simTerm{"solarized light", "#fdf6e3", "#657b83", solDark.pal}
	gruvLight = simTerm{"gruvbox light", "#fbf1c7", "#3c3836", pal16(
		"#fbf1c7 #cc241d #98971a #d79921 #458588 #b16286 #689d6a #7c6f64 " +
			"#928374 #9d0006 #79740e #b57614 #076678 #8f3f71 #427b58 #3c3836")}
	latte = simTerm{"catppuccin latte", "#eff1f5", "#4c4f69", pal16(
		"#5c5f77 #d20f39 #40a02b #df8e1d #1e66f5 #ea76cb #179299 #acb0be " +
			"#6c6f85 #d20f39 #40a02b #df8e1d #1e66f5 #ea76cb #179299 #bcc0cc")}
	mocha = simTerm{"catppuccin mocha", "#1e1e2e", "#cdd6f4", pal16(
		"#45475a #f38ba8 #a6e3a1 #f9e2af #89b4fa #f5c2e7 #94e2d5 #bac2de " +
			"#585b70 #f38ba8 #a6e3a1 #f9e2af #89b4fa #f5c2e7 #94e2d5 #a6adc8")}
)

// themeTerms are the terminals each theme assumes: their background, text,
// black and bright black (the two the current bar uses). Approximate.
var themeTerms = map[string]simTerm{
	"catppuccin-latte":     latte,
	"catppuccin-frappe":    {"", "#303446", "#c6d0f5", map[int]string{0: "#51576d", 8: "#626880"}},
	"catppuccin-macchiato": {"", "#24273a", "#cad3f5", map[int]string{0: "#494d64", 8: "#5b6078"}},
	"catppuccin-mocha":     mocha,
	"gruvbox-dark":         {"", "#282828", "#ebdbb2", map[int]string{0: "#282828", 8: "#928374"}},
	"gruvbox-light":        gruvLight,
	"nord":                 {"", "#2e3440", "#d8dee9", map[int]string{0: "#3b4252", 8: "#4c566a"}},
	"dracula":              {"", "#282a36", "#f8f8f2", map[int]string{0: "#21222c", 8: "#6272a4"}},
	"solarized-dark":       solDark,
	"solarized-light":      solLight,
	"tokyo-night":          {"", "#1a1b26", "#c0caf5", map[int]string{0: "#15161e", 8: "#414868"}},
	"tokyo-night-storm":    {"", "#24283b", "#c0caf5", map[int]string{0: "#1d202f", 8: "#414868"}},
	"tokyo-night-day":      {"", "#e1e2e7", "#3760bf", map[int]string{0: "#e9e9ed", 8: "#a1a6c5"}},
	"one-dark":             {"", "#282c34", "#abb2bf", map[int]string{0: "#282c34", 8: "#5a6374"}},
	"one-light":            {"", "#fafafa", "#383a42", map[int]string{0: "#383a42", 8: "#4f525e"}},
	"rose-pine":            {"", "#191724", "#e0def4", map[int]string{0: "#26233a", 8: "#6e6a86"}},
	"rose-pine-moon":       {"", "#232136", "#e0def4", map[int]string{0: "#393552", 8: "#6e6a86"}},
	"rose-pine-dawn":       {"", "#faf4ed", "#575279", map[int]string{0: "#f2e9e1", 8: "#9893a5"}},
	"everforest-dark":      {"", "#2d353b", "#d3c6aa", map[int]string{0: "#475258", 8: "#859289"}},
	"kanagawa":             {"", "#1f1f28", "#dcd7ba", map[int]string{0: "#16161d", 8: "#727169"}},
	"monokai":              {"", "#272822", "#f8f8f2", map[int]string{0: "#272822", 8: "#75715e"}},
}

// protoCase is one input: a role colour in one mode on one terminal.
type protoCase struct {
	section, label, mode string
	role                 color.Color
	term                 simTerm
	playing              color.Color // the title's colour; nil means role
}

func protoCases() []protoCase {
	var cs []protoCase
	for _, name := range config.ThemeNames() {
		t, ok := themeTerms[name]
		if !ok {
			t = mocha
		}
		r := config.Config{Theme: config.Theme{Name: name}}.RoleColors()
		for _, mr := range [][2]string{{"NORMAL", r.Playing}, {"COMMAND", r.Key}, {"VISUAL", r.Selection}} {
			cs = append(cs, protoCase{"themes", name, mr[0], lipgloss.Color(mr[1]), t, lipgloss.Color(r.Playing)})
		}
	}
	for _, t := range []simTerm{xtermDark, xtermLight} {
		for _, mr := range []struct {
			mode string
			c    color.Color
		}{{"NORMAL", lipgloss.Color(defaultPlaying)}, {"COMMAND", ansi.Cyan}, {"VISUAL", ansi.Blue}} {
			cs = append(cs, protoCase{"no theme", "default, " + t.name, mr.mode, mr.c, t, lipgloss.Color(defaultPlaying)})
		}
	}
	for _, t := range []simTerm{xtermDark, xtermLight, solDark, solLight, gruvLight, latte, mocha} {
		for _, mr := range []struct {
			mode, name string
			c          ansi.BasicColor
		}{
			{"NORMAL", "magenta", ansi.Magenta}, {"COMMAND", "cyan", ansi.Cyan},
			{"VISUAL", "blue", ansi.Blue}, {"NORMAL", "yellow", ansi.Yellow},
			{"NORMAL", "bright-black", ansi.BrightBlack}, {"NORMAL", "white", ansi.White},
		} {
			cs = append(cs, protoCase{"16 colours", mr.name + ", " + t.name, mr.mode, mr.c, t, nil})
		}
	}
	for _, t := range []simTerm{mocha, latte} {
		for _, g := range []string{"#000000", "#333333", "#777777", "#bbbbbb", "#ffffff"} {
			cs = append(cs, protoCase{"covers", g + ", " + t.name, "NORMAL", lipgloss.Color(g), t, nil})
		}
		for _, l := range []float64{0.25, 0.45, 0.65, 0.85} {
			for h := 0.0; h < 360; h += 45 {
				c := colorful.OkLch(l, 0.15, h).Clamped()
				cs = append(cs, protoCase{"covers", c.Hex() + ", " + t.name, "NORMAL", c, t, nil})
			}
		}
	}
	return cs
}

// --- rendering one bar on a simulated terminal ---

type protoRatios struct{ mode, mid, title float64 }

func (r protoRatios) min() float64 { return math.Min(r.mode, math.Min(r.mid, r.title)) }

// protoBarLine draws c's bar with variant v, w cells wide, on c's terminal.
func protoBarLine(c protoCase, v int, w int) (string, protoRatios) {
	t := c.term
	bg := hx(t.bg)
	role := c.role
	if ProtoPaletteKnown { // the terminal answered OSC 4: its 16 colours are exact
		if _, ok := role.(ansi.BasicColor); ok {
			role = t.resolve(role, t.fg)
			c.role = role
		}
	}
	ink := protoVariants[v].ink(role, bg)
	if c.playing != nil {
		ink.title = protoVariants[v].ink(c.playing, bg).title
	}
	type seg struct {
		text   string
		fg, bg colorful.Color
		bold   bool
	}
	block := func(text string, bold bool) seg {
		if ink.rev {
			return seg{text, bg, t.resolve(c.role, t.bg), bold}
		}
		return seg{text, t.resolve(ink.modeFg, t.fg), t.resolve(ink.modeBg, t.bg), bold}
	}
	mid := func(text string) seg {
		return seg{text, t.resolve(ink.midFg, t.fg), t.resolve(ink.midBg, t.bg), false}
	}
	plain := func(text string, fg colorful.Color, bold bool) seg { return seg{text, fg, bg, bold} }

	mode := block(" "+c.mode+" ", true)
	state := mid(" ▶ ")
	pos := mid(" 3/12 ")
	times := block(" 1:23 / 4:56 ", true)
	title := plain(" Low Tide", t.resolve(ink.title, t.fg), true)
	rest := plain(" — Artist · Album", hx(t.fg), false)
	used := 0
	for _, s := range []seg{mode, state, pos, times, title, rest} {
		used += lipgloss.Width(s.text)
	}
	gap := plain(strings.Repeat(" ", max(w-used, 0)), hx(t.fg), false)

	var b strings.Builder
	for _, s := range []seg{mode, state, title, rest, gap, pos, times} {
		st := lipgloss.NewStyle().Foreground(s.fg).Background(s.bg).Bold(s.bold)
		if s == rest {
			st = st.Faint(true)
		}
		b.WriteString(st.Render(s.text))
	}
	return b.String(), protoRatios{
		mode:  contrast(mode.fg, mode.bg),
		mid:   contrast(state.fg, state.bg),
		title: contrast(title.fg, title.bg),
	}
}

func ratioCell(r float64) string {
	col := lipgloss.Color("#e5484d")
	switch {
	case r >= 4.5:
		col = lipgloss.Color("#46a758")
	case r >= 3:
		col = lipgloss.Color("#f5a524")
	}
	return lipgloss.NewStyle().Foreground(col).Render(fmt.Sprintf("%5.1f", r))
}

// ProtoMatrix is the prototype's screen: a summary of every variant, then
// the rows from off on, h lines in all.
func ProtoMatrix(v int, offp *int, w, h int) (string, int) {
	off := *offp
	cs := protoCases()
	var out []string
	bold := lipgloss.NewStyle().Bold(true)
	faint := lipgloss.NewStyle().Faint(true)
	out = append(out, bold.Render(" PROTOTYPE · status line contrast")+faint.Render(
		fmt.Sprintf("  — %d inputs, WCAG ratio (≥4.5 green, ≥3 amber, else red) · 16-colour palette: %s",
			len(cs), map[bool]string{false: "unknown (today)", true: "asked with OSC 4"}[ProtoPaletteKnown])))
	for i, p := range protoVariants {
		fails, worst, worstAt := 0, 99.0, ""
		for _, c := range cs {
			_, r := protoBarLine(c, i, 60)
			if r.min() < 4.5 {
				fails++
			}
			if r.min() < worst {
				worst, worstAt = r.min(), c.label+" "+c.mode
			}
		}
		mark := "  "
		if i == v {
			mark = "▸ "
		}
		line := fmt.Sprintf("%s%s %-15s %3d/%d below 4.5   worst %s  %s",
			mark, p.Key, p.Name, fails, len(cs), ratioCell(worst), faint.Render(worstAt))
		if i == v {
			line = bold.Render(line)
		}
		out = append(out, line)
	}
	out = append(out, faint.Render(" "+protoVariants[v].About), "")
	head := fmt.Sprintf(" %-12s %-34s %-60s %s", "section", "input", "the bar, on that terminal", " mode   mid title")
	out = append(out, faint.Render(fit(head, w)))

	barW := 60
	rows := h - len(out) - 2
	off = max(min(off, len(cs)-rows), 0)
	*offp = off
	for _, c := range cs[off:min(off+rows, len(cs))] {
		bar, r := protoBarLine(c, v, barW)
		out = append(out, fmt.Sprintf(" %-12s %-34s %s %s %s %s",
			c.section, fit(c.label, 34), bar, ratioCell(r.mode), ratioCell(r.mid), ratioCell(r.title)))
	}
	for len(out) < h-1 {
		out = append(out, "")
	}
	p := protoVariants[v]
	pill := lipgloss.NewStyle().Reverse(true).Bold(true).Padding(0, 2).
		Render(fmt.Sprintf("‹  %s (%s)  ›", p.Key, p.Name))
	keys := faint.Render(fmt.Sprintf("   ←/→ variant · p palette known · j/k scroll · rows %d–%d of %d · q quit",
		off+1, min(off+rows, len(cs)), len(cs)))
	out = append(out, lipgloss.PlaceHorizontal(w, lipgloss.Center, pill+keys))
	return strings.Join(out[:h], "\n"), off
}
