package ui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/anopheles/navigo/internal/app"
)

// lineHistorySize is how many lines each history keeps.
const lineHistorySize = 100

// lineHistory is what was entered at the ":" line or the search prompt,
// newest last, kept in a file in the state directory.
type lineHistory struct {
	file  string // the file's name in the state directory
	what  string // for errors: "search history"
	items []string
	// While <Up>/<Down> walk it: an index in items, or len(items) for
	// what was typed before walking (draft).
	idx   int
	draft string
}

func loadHistory(dir, file, what string) lineHistory {
	h := lineHistory{file: file, what: what}
	if dir == "" {
		return h
	}
	b, err := os.ReadFile(filepath.Join(dir, file))
	if err != nil {
		return h
	}
	h.items = slices.DeleteFunc(strings.Split(string(b), "\n"), func(s string) bool { return strings.TrimSpace(s) == "" })
	h.idx = len(h.items)
	return h
}

// last is the newest line, or "".
func (h *lineHistory) last() string {
	if len(h.items) == 0 {
		return ""
	}
	return h.items[len(h.items)-1]
}

// begin starts a walk; onNewest when the prompt already shows the newest
// line, so <Up> goes to the one before.
func (h *lineHistory) begin(onNewest bool) {
	h.idx, h.draft = len(h.items), ""
	if onNewest && h.idx > 0 {
		h.idx--
	}
}

// step moves to an older (or newer) line and returns it; current is what
// the prompt shows, kept as the draft when the walk leaves it.
func (h *lineHistory) step(older bool, current string) string {
	if h.idx == len(h.items) {
		h.draft = current
	}
	if older {
		h.idx = max(h.idx-1, 0)
	} else {
		h.idx = min(h.idx+1, len(h.items))
	}
	if h.idx < len(h.items) {
		return h.items[h.idx]
	}
	return h.draft
}

// remember adds a line to the end of a history and saves it.
func (m *Model) remember(h *lineHistory, line string) tea.Cmd {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	items := slices.DeleteFunc(h.items, func(s string) bool { return s == line })
	items = append(items, line)
	if over := len(items) - lineHistorySize; over > 0 {
		items = items[over:]
	}
	h.items, h.idx = items, len(items)
	dir, file, what, lines := m.deps.StateDir, h.file, h.what, slices.Clone(items)
	if dir == "" {
		return nil
	}
	return func() tea.Msg {
		if err := saveHistory(dir, file, lines); err != nil {
			return eventMsg{app.Failed{What: "couldn't save the " + what, Err: err}}
		}
		return nil
	}
}

// saveHistory writes a history, newest last, replacing the file in one
// step.
func saveHistory(dir, file string, lines []string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, file+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(strings.Join(lines, "\n") + "\n"); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, file))
}
