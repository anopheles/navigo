// PROTOTYPE: throwaway code for "How far can navigo's look go with reusable
// components, without costing UX?" Never merge into main.
//
// One fake home screen (library, queue, now-playing bar, status) drawn three
// structurally different ways. Rows, keys and hints are identical in all
// three; only the chrome changes.
//
//	A  Glow   same layout; truecolor gradients, tinted cursor row, rounded panes
//	B  Art    now-playing becomes a card: cover art (x/mosaic) + metadata + wave
//	C  Pulse  live: spring-eased progress (harmonica), spectrum (ntcharts), marquee
//
// left/right or v/V  next/previous variant      j/k  move       space  pause
// tab  other window  q  quit
//
// Run: cd prototype/visuals && go run .   (best at 120x34 or wider, truecolor)
package main

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/NimbleMarkets/ntcharts/v2/sparkline"
	"github.com/charmbracelet/harmonica"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/mosaic"
)

type track struct {
	title, artist, album string
	secs                 int
}

var tracks = []track{
	{"Dusk Signal", "Aurora Lane", "Blue Hour", 142},
	{"Low Tide", "Aurora Lane", "Blue Hour", 185},
	{"Streetlamp", "Aurora Lane", "Blue Hour", 238},
	{"Blue Hour", "Aurora Lane", "Blue Hour", 31},
	{"Wintering", "Aurora Lane", "Blue Hour", 64},
	{"Afterglow", "Aurora Lane", "Blue Hour", 97},
	{"Paper Lanterns", "Kestrel & Moth", "Small Hours", 201},
	{"Salt Flats", "Kestrel & Moth", "Small Hours", 176},
}

const playing = 1

var variants = []string{"A Glow", "B Art", "C Pulse"}

type tick time.Time

type model struct {
	w, h     int
	variant  int
	focus    int // 0 library, 1 queue
	cur      [2]int
	paused   bool
	pos      float64 // seconds
	shown    float64 // spring-eased progress for variant C
	vel      float64
	spring   harmonica.Spring
	wave     sparkline.Model
	frame    int
	cover    string
	coverFor [2]int
}

func newModel() *model {
	m := &model{pos: 42, shown: 42, spring: harmonica.NewSpring(harmonica.FPS(30), 6, 0.9)}
	m.wave = sparkline.New(40, 2)
	return m
}

func (m *model) Init() tea.Cmd { return doTick() }

func doTick() tea.Cmd {
	return tea.Tick(33*time.Millisecond, func(t time.Time) tea.Msg { return tick(t) })
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tick:
		m.frame++
		if !m.paused {
			m.pos += 1.0 / 30
			if m.pos > float64(tracks[playing].secs) {
				m.pos = 0
			}
		}
		// the shown position lags and settles: harmonica spring
		m.shown, m.vel = m.spring.Update(m.shown, m.vel, m.pos)
		m.wave.Push(fakeLevel(m.frame, m.paused))
		return m, doTick()
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "right", "v":
			m.variant = (m.variant + 1) % len(variants)
		case "left", "V":
			m.variant = (m.variant + len(variants) - 1) % len(variants)
		case "tab":
			m.focus = 1 - m.focus
		case "j", "down":
			m.cur[m.focus] = min(m.cur[m.focus]+1, len(tracks)-1)
		case "k", "up":
			m.cur[m.focus] = max(m.cur[m.focus]-1, 0)
		case "space", " ":
			m.paused = !m.paused
		}
	}
	return m, nil
}

func fakeLevel(frame int, paused bool) float64 {
	if paused {
		return 0.05
	}
	t := float64(frame)
	return 0.5 + 0.28*math.Sin(t/3.1) + 0.17*math.Sin(t/1.3+1) + 0.05*math.Sin(t*2.7)
}

// ---------- palette ----------

var (
	cMagenta = lipgloss.Color("#ff5fd2")
	cViolet  = lipgloss.Color("#8a63ff")
	cBlue    = lipgloss.Color("#3fa7ff")
	cCyan    = lipgloss.Color("#3be8e0")
	cInk     = lipgloss.Color("#16131f")
	cTint    = lipgloss.Color("#2a2547")
	cTint2   = lipgloss.Color("#1f1c33")
	cFaint   = lipgloss.Color("#6c6788")
)

func fg(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }

var (
	dim  = lipgloss.NewStyle().Faint(true)
	key  = lipgloss.NewStyle().Foreground(ansi.Cyan).Bold(true)
	bold = lipgloss.NewStyle().Bold(true)
)

func gradText(s string, stops ...color.Color) string {
	rs := []rune(s)
	if len(rs) == 0 {
		return ""
	}
	cs := lipgloss.Blend1D(len(rs), stops...)
	var b strings.Builder
	for i, r := range rs {
		b.WriteString(lipgloss.NewStyle().Foreground(cs[i]).Bold(true).Render(string(r)))
	}
	return b.String()
}

func clock(s float64) string { return fmt.Sprintf("%d:%02d", int(s)/60, int(s)%60) }

func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = ansi.Truncate(s, w, "…")
	if d := w - ansi.StringWidth(s); d > 0 {
		s += strings.Repeat(" ", d)
	}
	return s
}

// ---------- rows (identical content in every variant) ----------

func (m *model) row(i, w int, win int, tint bool) string {
	t := tracks[i]
	mark := "  "
	if win == 1 && i == playing {
		mark = "▶ "
		if m.paused {
			mark = "⏸ "
		}
	}
	d := clock(float64(t.secs))
	left := fmt.Sprintf("%s%3d %s", mark, i+1, t.title)
	mid := t.artist
	aw := min(max(w/4, 12), 24)
	tw := w - aw - len(d) - 3
	line := fit(left, tw) + " " + fit(mid, aw) + " " + fmt.Sprintf("%*s", len(d), d)
	line = fit(line, w)
	cursor := m.cur[win] == i
	focused := m.focus == win
	current := win == 1 && i == playing
	switch m.variant {
	case 0, 1, 2:
		switch {
		case cursor && focused && m.variant == 0:
			return lipgloss.NewStyle().Background(cTint).Foreground(lipgloss.White).Bold(true).Render(line)
		case cursor && focused:
			return lipgloss.NewStyle().Reverse(true).Render(line)
		case cursor:
			return lipgloss.NewStyle().Underline(true).Render(line)
		case current && m.variant == 0:
			return gradText(line, cMagenta, cViolet)
		case current:
			return lipgloss.NewStyle().Foreground(ansi.Magenta).Bold(true).Render(line)
		}
	}
	return line
}

func (m *model) window(win, w, h int) []string {
	var out []string
	for i := 0; i < len(tracks) && len(out) < h; i++ {
		out = append(out, m.row(i, w, win, false))
	}
	for len(out) < h {
		out = append(out, strings.Repeat(" ", w))
	}
	return out
}

func (m *model) hint() string {
	h := func(k, d string) string { return key.Render(k) + " " + dim.Render(d) }
	return " " + h("<CR>", "play") + "  " + h("V", "select") + "  " + h("<Space>", "play/pause") +
		"  " + h("u", "undo") + "  " + h("<Tab>", "other window") + "  " + h("?", "keys")
}

// ---------- variants ----------

func (m *model) View() tea.View {
	var s string
	if m.w < 80 || m.h < 24 {
		s = "terminal too small for the prototype (need 80x24)"
	} else {
		switch m.variant {
		case 0:
			s = m.viewGlow()
		case 1:
			s = m.viewArt()
		default:
			s = m.viewPulse()
		}
	}
	v := tea.NewView(s + "\n" + m.switcher())
	v.AltScreen = true
	return v
}

func (m *model) switcher() string {
	pill := lipgloss.NewStyle().Background(lipgloss.White).Foreground(lipgloss.Black).Bold(true)
	bar := pill.Render(" ◀ " + variants[m.variant] + " ▶ ")
	pad := max((m.w-lipgloss.Width(bar))/2, 0)
	return strings.Repeat(" ", pad) + bar
}

func (m *model) bodyH(footer int) int { return m.h - 1 /*switcher*/ - footer - 2 /*titles+hint*/ }

// A: same shape as today. Gradient title bar, tinted cursor, gradient playing
// row, gradient progress bar. Rounded panes frame each window.
func (m *model) viewGlow() string {
	foot := 4
	h := m.bodyH(foot)
	lw := m.w * 3 / 5
	rw := m.w - lw
	pane := func(title string, win, w int) []string {
		focused := m.focus == win
		bc := fg(cFaint)
		if focused {
			bc = fg(cViolet)
		}
		inner := w - 2
		t := " " + title + " "
		var top string
		if focused {
			top = bc.Render("╭") + gradText(t, cMagenta, cViolet, cBlue) +
				bc.Render(strings.Repeat("─", max(inner-lipgloss.Width(t), 0))+"╮")
		} else {
			top = bc.Render("╭") + dim.Render(t) +
				bc.Render(strings.Repeat("─", max(inner-lipgloss.Width(t), 0))+"╮")
		}
		lines := []string{top}
		for _, r := range m.window(win, inner, h-1) {
			lines = append(lines, bc.Render("│")+r+bc.Render("│"))
		}
		lines = append(lines, bc.Render("╰"+strings.Repeat("─", inner)+"╯"))
		return lines
	}
	l, r := pane("Albums › Blue Hour", 0, lw), pane("Queue (8)", 1, rw)
	var lines []string
	for i := range l {
		lines = append(lines, l[i]+r[i])
	}
	lines = append(lines, m.hint())
	t := tracks[playing]
	lines = append(lines,
		" "+gradText("▶ "+t.title, cMagenta, cViolet)+dim.Render(" — "+t.artist+" · "+t.album),
		m.gradBar(m.w-2, m.pos, float64(t.secs), 1),
		dim.Render(" playing · 8 tracks in the queue"))
	return strings.Join(lines, "\n")
}

func (m *model) gradBar(w int, pos, total float64, indent int) string {
	label := " " + clock(pos) + " / " + clock(total)
	bw := w - len(label)
	if bw < 4 {
		bw = 4
	}
	done := int(math.Round(float64(bw) * pos / total))
	cs := lipgloss.Blend1D(max(done, 1), cMagenta, cViolet, cBlue)
	var b strings.Builder
	b.WriteString(strings.Repeat(" ", indent))
	for i := 0; i < done; i++ {
		b.WriteString(fg(cs[i]).Render("━"))
	}
	if done < bw {
		b.WriteString(fg(cCyan).Render("●"))
		b.WriteString(fg(cFaint).Render(strings.Repeat("─", bw-done-1)))
	}
	return b.String() + dim.Render(label)
}

// B: unframed windows (as today) but the bottom bar grows into a card with
// cover art, so the screen has one visual anchor.
func (m *model) viewArt() string {
	const artH = 6
	foot := artH + 2
	h := m.bodyH(foot)
	lw := m.w * 3 / 5
	rw := m.w - lw - 1
	title := func(s string, w int, f bool) string {
		if f {
			return lipgloss.NewStyle().Reverse(true).Bold(true).Render(fit(s, w))
		}
		return bold.Render(fit(s, w))
	}
	lines := []string{title(" Albums › Blue Hour", lw, m.focus == 0) + " " + title(" Queue (8)", rw, m.focus == 1)}
	left, right := m.window(0, lw, h), m.window(1, rw, h)
	for i := range h {
		lines = append(lines, left[i]+dim.Render("│")+right[i])
	}
	lines = append(lines, m.hint(), dim.Render(strings.Repeat("─", m.w)))
	t := tracks[playing]
	art := strings.Split(m.coverArt(artH*2, artH), "\n")
	meta := []string{
		bold.Render(t.title),
		fg(cMagenta).Render(t.artist) + dim.Render(" · "+t.album),
		dim.Render("FLAC 44.1 kHz · RG track · 2/8 in queue"),
		"",
		m.gradBar(m.w-artH*2-6, m.pos, float64(t.secs), 0),
		m.waveLine(m.w - artH*2 - 6),
	}
	for i := range artH {
		lines = append(lines, " "+art[i]+"  "+meta[i])
	}
	lines = append(lines, dim.Render(" playing · 8 tracks in the queue"))
	return strings.Join(lines, "\n")
}

func (m *model) waveLine(w int) string {
	m.wave.Resize(w, 1)
	m.wave.DrawBraille()
	return fg(cViolet).Render(strings.TrimRight(m.wave.View(), "\n"))
}

// coverArt paints a synthetic cover (no assets in the prototype) and renders
// it with x/mosaic: half/quarter blocks, truecolor.
func (m *model) coverArt(w, h int) string {
	if m.cover != "" && m.coverFor == [2]int{w, h} {
		return m.cover
	}
	img := image.NewRGBA(image.Rect(0, 0, 160, 160))
	for y := range 160 {
		for x := range 160 {
			fx, fy := float64(x)/160, float64(y)/160
			d := math.Hypot(fx-0.5, fy-0.62)
			sun := math.Max(0, 1-d*3.2)
			r := 20 + 200*fy*fy + 235*sun
			g := 15 + 60*fy + 150*sun
			b := 70 + 150*(1-fy) + 40*sun
			if fy > 0.62 && math.Abs(math.Sin(fx*30+fy*8)) > 0.93 {
				r, g, b = r*1.4, g*1.2, b*1.1
			}
			img.Set(x, y, color.RGBA{uint8(math.Min(r, 255)), uint8(math.Min(g, 255)), uint8(math.Min(b, 255)), 255})
		}
	}
	mo := mosaic.New().Width(w * 2).Height(h * 2) // pixels: a cell is 2x2
	m.cover = strings.TrimRight(mo.Render(img), "\n")
	m.coverFor = [2]int{w, h}
	return m.cover
}

// C: Windows get a calmer flat treatment; the bottom is a live deck: spring-
// eased progress, spectrum bars, scrolling title when the title overflows.
func (m *model) viewPulse() string {
	const deckH = 5
	h := m.bodyH(deckH + 1)
	lw := m.w * 3 / 5
	rw := m.w - lw - 1
	title := func(s string, w int, f bool) string {
		st := lipgloss.NewStyle().Bold(true)
		if f {
			st = st.Foreground(lipgloss.Black).Background(cCyan)
		} else {
			st = st.Foreground(cFaint)
		}
		return st.Render(fit(s, w))
	}
	lines := []string{title(" Albums › Blue Hour", lw, m.focus == 0) + " " + title(" Queue (8)", rw, m.focus == 1)}
	left, right := m.window(0, lw, h), m.window(1, rw, h)
	for i := range h {
		lines = append(lines, left[i]+fg(cFaint).Render("│")+right[i])
	}
	lines = append(lines, m.hint())
	t := tracks[playing]
	bw := m.w - 30
	// spectrum: 24 bars scaled by phase-shifted fake levels
	spec := m.spectrum(24, deckH-1)
	info := []string{
		bold.Render(m.marquee(t.title+" — "+t.artist+" · "+t.album, 26)),
		fg(cFaint).Render(clock(m.shown) + " / " + clock(float64(t.secs))),
		m.pulseBar(bw),
		dim.Render("RG track · 2/8 in queue"),
	}
	lines = append(lines, fg(cFaint).Render(strings.Repeat("─", m.w)))
	for i := range deckH - 1 {
		right := ""
		if i < len(info) {
			right = info[i]
		}
		lines = append(lines, " "+spec[i]+"  "+right)
	}
	lines = append(lines, dim.Render(" playing · 8 tracks in the queue"))
	return strings.Join(lines, "\n")
}

func (m *model) marquee(s string, w int) string {
	if ansi.StringWidth(s) <= w {
		return s
	}
	s += "   ·   "
	rs := []rune(s + s)
	off := (m.frame / 6) % (len([]rune(s)))
	return string(rs[off : off+w])
}

func (m *model) pulseBar(w int) string {
	total := float64(tracks[playing].secs)
	n := int(math.Round(float64(w) * m.shown / total))
	n = max(0, min(n, w))
	glow := 0.5 + 0.5*math.Sin(float64(m.frame)/5)
	head := lipgloss.Blend1D(10, cMagenta, cCyan)[int(glow*9)]
	var b strings.Builder
	b.WriteString(fg(cViolet).Render(strings.Repeat("█", max(n-1, 0))))
	if n > 0 {
		b.WriteString(fg(head).Bold(true).Render("█"))
	}
	b.WriteString(fg(cTint).Render(strings.Repeat("█", w-n)))
	return b.String()
}

// spectrum draws vertical bars with eighth-blocks; levels are fake (mpv does
// not expose samples), shaped from the same signal as the waveform.
func (m *model) spectrum(n, h int) []string {
	blocks := []rune(" ▁▂▃▄▅▆▇█")
	cols := lipgloss.Blend1D(h, cMagenta, cViolet, cBlue)
	out := make([]string, h)
	for row := range h {
		var b strings.Builder
		for c := range n {
			lvl := fakeLevel(m.frame-c*2, m.paused)
			lvl *= 0.55 + 0.45*math.Sin(float64(c)/3.0+1)
			v := lvl * float64(h)
			cell := v - float64(h-1-row)
			idx := int(math.Max(0, math.Min(8, cell*8)))
			b.WriteString(fg(cols[row]).Render(string(blocks[idx])) + " ")
		}
		out[row] = b.String()
	}
	return out
}

func main() {
	if _, err := tea.NewProgram(newModel()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
