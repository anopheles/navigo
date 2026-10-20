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
	{"global", "=", "volume +5"},
	{"global", "+", "volume +5"},
	{"global", "-", "volume -5"},
	{"global", "S", "shuffle"},
	{"global", "R", "repeat"},
	{"global", "i", "info"},
	{"global", "*", "star"},
	{"global", "u", "undo"},
	{"global", "<C-r>", "redo"},
	{"global", "<Tab>", "window"},
	{"global", "<C-w>w", "window"},
	{"global", "'a", "artists"},
	{"global", "'b", "albums"},
	{"global", "'p", "playlists"},
	{"global", "'s", "search"},
	{"global", "'q", "queue"},
	{"global", ":", "cmdline"},
	{"global", "V", "visual"},
	{"global", "v", "visual"},
	{"global", "y", "yank"},
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
	{"queue", "d", "delete"},
	{"queue", "x", "remove"},
	{"queue", "p", "paste-after"},
	{"queue", "P", "paste-before"},
	{"queue", "J", "move-down"},
	{"queue", "K", "move-up"},
	{"playlist-tracks", "d", "delete"},
	{"playlist-tracks", "x", "remove"},
	{"playlist-tracks", "p", "paste-after"},
	{"playlist-tracks", "P", "paste-before"},
	{"playlist-tracks", "J", "move-down"},
	{"playlist-tracks", "K", "move-up"},
	{"playlist-tracks", "u", "playlist-undo"},
	{"playlist-tracks", "<C-r>", "playlist-undo"},
	{"playlists", "dd", "playlist-delete"},
	{"search", "a", "add-next"},
	{"search", "A", "add-end"},
	{"popup", "<CR>", "choose"},
	{"popup", "q", "close"},
	{"popup", "<Esc>", "close"},
}

// visualDefaults are the Visual mode bindings; Normal mode's motions
// apply there too. Operators act on the selection at once.
var visualDefaults = []struct{ scope, keys, command string }{
	{"global", "A", "add-end"},
	{"global", "a", "add-next"},
	{"global", "y", "yank"},
	{"global", "d", "delete"},
	{"global", "x", "delete"},
	{"global", "J", "move-down"},
	{"global", "K", "move-up"},
	{"global", "*", "star"},
	{"global", ":", "cmdline"},
	{"global", "V", "visual"},
	{"global", "v", "visual"},
	{"global", "<Esc>", "cancel"},
	{"global", "<C-c>", "cancel"},
}

// defaultTable returns the default bindings.
func defaultTable() *keymap.Table {
	t := keymap.NewTable()
	for mode, list := range map[keymap.Mode][]struct{ scope, keys, command string }{keymap.Normal: defaults, keymap.Visual: visualDefaults} {
		for _, b := range list {
			if err := t.Bind(mode, b.scope, b.keys, b.command); err != nil {
				panic("ui: default binding " + b.keys + ": " + err.Error())
			}
		}
	}
	return t
}
