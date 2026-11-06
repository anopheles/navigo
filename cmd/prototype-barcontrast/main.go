// PROTOTYPE — throwaway, branch prototype/bar-contrast. Do not merge.
// The status-line contrast matrix: ←/→ switch variant, j/k or PgUp/PgDn
// scroll, q quits. Needs a true-colour terminal.
package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/anopheles/navigo/internal/ui"
)

type model struct{ v, off, w, h int }

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "right", "l":
			m.v = (m.v + 1) % ui.ProtoVariantCount
		case "left", "h":
			m.v = (m.v + ui.ProtoVariantCount - 1) % ui.ProtoVariantCount
		case "j", "down":
			m.off++
		case "k", "up":
			m.off--
		case "pgdown", "space", "ctrl+d":
			m.off += m.h / 2
		case "pgup", "ctrl+u":
			m.off -= m.h / 2
		case "p":
			ui.ProtoPaletteKnown = !ui.ProtoPaletteKnown
		case "g":
			m.off = 0
		case "G":
			m.off = 1 << 20
		}
	}
	return m, nil
}

func (m model) View() tea.View {
	s := ""
	if m.h > 0 {
		s, _ = ui.ProtoMatrix(m.v, &m.off, m.w, m.h)
	}
	v := tea.NewView(s)
	v.AltScreen = true
	return v
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "-dump" { // the matrix as text, for a quick look
		off := 0
		ui.ProtoPaletteKnown = len(os.Args) > 2
		for v := range ui.ProtoVariantCount {
			s, _ := ui.ProtoMatrix(v, &off, 180, 200)
			fmt.Println(s)
		}
		return
	}
	if _, err := tea.NewProgram(model{}).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
