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
	{"global", "<C-f>", "page-down"},
	{"global", "<C-b>", "page-up"},
	{"global", "<C-e>", "scroll-down"},
	{"global", "<C-y>", "scroll-up"},
	{"global", "H", "screen-top"},
	{"global", "M", "screen-middle"},
	{"global", "L", "screen-bottom"},
	{"global", "zz", "scroll-center"},
	{"global", "zt", "scroll-top"},
	{"global", "zb", "scroll-bottom"},
	{"global", "%", "percent"},
	{"global", "/", "find"},
	{"global", "n", "find-next"},
	{"global", "N", "find-prev"},
	{"global", "<C-o>", "jump-back"},
	{"global", "<C-i>", "forward"},
	{"global", "gp", "go-playing"},
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
	{"global", "gm", "mix"},
	{"global", "R", "repeat"},
	{"global", "i", "info"},
	{"global", "gc", "cover"},
	{"global", "gl", "playlist-add"},
	{"global", "gR", "radio"},
	{"global", "g+", "radio-more"},
	{"global", "g-", "radio-less"},
	{"global", "*", "star"},
	{"global", "u", "undo"},
	{"global", "<C-r>", "redo"},
	{"global", "<Tab>", "window"},
	{"global", "<C-w>w", "window"},
	{"global", "'h", "home"},
	{"global", "'a", "artists"},
	{"global", "'b", "albums"},
	{"global", "'p", "playlists"},
	{"global", "'s", "search"},
	{"global", "'?", "help"},
	{"global", "<F1>", "help"},
	{"global", "?", "key-menu"},
	{"global", "'q", "queue"},
	{"global", ":", "cmdline"},
	{"global", "V", "visual"},
	{"global", "v", "visual"},
	{"global", "y", "yank"},
	{"global", "<Esc>", "cancel"},
	{"global", "<C-c>", "quit"},
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
	{"help", "/", "help-filter"},
	{"search", "a", "add-next"},
	{"search", "A", "add-end"},
	{"popup", "<CR>", "choose"},
	{"popup", "/", "popup-filter"},
	{"popup", "q", "close"},
	{"popup", "<Esc>", "close"},
}

// visualDefaults are the Visual mode bindings; Normal mode's motions
// apply there too. Operators act on the selection at once.
var visualDefaults = []struct{ scope, keys, command string }{
	{"global", "A", "add-end"},
	{"global", "a", "add-next"},
	{"global", "gl", "playlist-add"},
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

// lineDefaults edit the ":" line, the search prompt and Help's filter.
var lineDefaults = []struct{ scope, keys, command string }{
	{"global", "<CR>", "accept"},
	{"global", "<Esc>", "cancel"},
	{"global", "<C-c>", "cancel"},
	{"global", "<Tab>", "complete"},
	{"global", "<S-Tab>", "complete-back"},
	{"global", "<Up>", "history-back"},
	{"global", "<Down>", "history-forward"},
	{"global", "<C-p>", "history-back"},
	{"global", "<C-n>", "history-forward"},
	{"global", "<C-a>", "line-start"},
	{"global", "<C-e>", "line-end"},
	{"global", "<C-w>", "kill-word"},
	{"global", "<C-u>", "kill-to-start"},
	{"global", "<C-k>", "kill-to-end"},
}

// defaultTable returns the default bindings.
func defaultTable() *keymap.Table {
	t := keymap.NewTable()
	for mode, list := range map[keymap.Mode][]struct{ scope, keys, command string }{keymap.Normal: defaults, keymap.Visual: visualDefaults, keymap.Cmdline: lineDefaults} {
		for _, b := range list {
			if err := t.Bind(mode, b.scope, b.keys, b.command); err != nil {
				panic("ui: default binding " + b.keys + ": " + err.Error())
			}
		}
	}
	return t
}
