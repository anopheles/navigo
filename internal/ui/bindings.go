package ui

import "github.com/anopheles/navigo/internal/keymap"

// defaults is the walking skeleton's slice of the default key table, in
// the order help and hints list keys: the first key bound to a command is
// the one hints show.
var defaults = []struct{ scope, keys, command string }{
	{"global", "j", "down"},
	{"global", "<Down>", "down"},
	{"global", "k", "up"},
	{"global", "<Up>", "up"},
	{"global", "gg", "first"},
	{"global", "G", "last"},
	{"global", "<C-d>", "half-page-down"},
	{"global", "<C-u>", "half-page-up"},
	{"global", "}", "next-group"},
	{"global", "{", "prev-group"},
	{"global", "ga", "go-album"},
	{"global", "gr", "go-artist"},
	{"global", "h", "back"},
	{"global", "<Space>", "play-pause"},
	{"global", ">", "next-track"},
	{"global", "<lt>", "prev-track"},
	{"global", ".", "seek +10"},
	{"global", ",", "seek -10"},
	{"global", "X", "stop"},
	{"global", "u", "undo"},
	{"global", "<C-r>", "redo"},
	{"global", "<Tab>", "window"},
	{"global", "<C-w>w", "window"},
	{"global", "'a", "artists"},
	{"global", "'b", "albums"},
	{"global", "'s", "search"},
	{"global", "'q", "queue"},
	{"global", ":", "cmdline"},
	{"global", "<Esc>", "cancel"},
	{"global", "<C-c>", "interrupt"},
	{"global", "ZZ", "quit"},
	{"global", "ZQ", "quit"},
	{"containers", "<CR>", "open"},
	{"containers", "l", "open"},
	{"tracks", "<CR>", "play-now"},
	{"tracks", "A", "add-end"},
	{"tracks", "a", "add-next"},
	{"queue", "<CR>", "play-row"},
	{"search", "a", "add-next"},
	{"search", "A", "add-end"},
	{"popup", "<CR>", "choose"},
	{"popup", "q", "close"},
	{"popup", "<Esc>", "close"},
}

// defaultTable returns the default bindings in Normal mode.
func defaultTable() *keymap.Table {
	t := keymap.NewTable()
	for _, b := range defaults {
		if err := t.Bind(keymap.Normal, b.scope, b.keys, b.command); err != nil {
			panic("ui: default binding " + b.keys + ": " + err.Error())
		}
	}
	return t
}
