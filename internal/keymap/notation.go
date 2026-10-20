package keymap

import (
	"fmt"
	"regexp"
	"strings"
)

// names are the bracketed key names, keyed by their lower-case spelling.
var names = map[string]string{
	"cr": "<CR>", "enter": "<CR>", "return": "<CR>",
	"esc": "<Esc>", "space": "<Space>", "tab": "<Tab>", "s-tab": "<S-Tab>",
	"bs": "<BS>", "backspace": "<BS>", "del": "<Del>", "delete": "<Del>", "lt": "<lt>", "bar": "<Bar>", "bslash": "<Bslash>",
	"up": "<Up>", "down": "<Down>", "left": "<Left>", "right": "<Right>",
	"home": "<Home>", "end": "<End>", "pageup": "<PageUp>", "pagedown": "<PageDown>", "pgup": "<PageUp>", "pgdown": "<PageDown>",
	"leader": "<leader>",
}

func init() {
	for i := 1; i <= 12; i++ {
		names[fmt.Sprintf("f%d", i)] = fmt.Sprintf("<F%d>", i)
	}
}

// Parse splits a key sequence in vim notation into one token per key, in
// canonical spelling: "gg" → g g, "<c-d>" → <C-d>, "<" → <lt>. A "<" that
// doesn't open a known name is the < key.
func Parse(seq string) ([]string, error) {
	if seq == "" {
		return nil, fmt.Errorf("empty key sequence")
	}
	var keys []string
	for rest := seq; rest != ""; {
		if rest[0] == '<' {
			if end := strings.IndexByte(rest, '>'); end > 1 {
				if tok, ok := canonical(rest[1:end]); ok {
					keys = append(keys, tok)
					rest = rest[end+1:]
					continue
				}
			}
			keys = append(keys, "<lt>")
			rest = rest[1:]
			continue
		}
		r := []rune(rest)[0]
		keys = append(keys, string(r))
		rest = rest[len(string(r)):]
	}
	return keys, nil
}

// canonical spells a bracketed name without its brackets, such as "c-d".
func canonical(name string) (string, bool) {
	lower := strings.ToLower(name)
	if tok, ok := names[lower]; ok {
		return tok, true
	}
	mod, key, ok := strings.Cut(lower, "-")
	if !ok || key == "" {
		return "", false
	}
	switch mod {
	case "c":
		mod = "C"
	case "a", "m":
		mod = "A"
	default:
		return "", false
	}
	// Keep the key's own case for letters (<C-d>); name others (<C-Space>).
	keyPart := name[len(name)-len(key):]
	if tok, ok := names[key]; ok {
		keyPart = strings.Trim(tok, "<>")
	} else if len([]rune(keyPart)) != 1 {
		return "", false
	}
	return "<" + mod + "-" + keyPart + ">", true
}

// FromTerminal turns a key as the terminal library spells it ("ctrl+d",
// "enter", "G", "shift+tab") into one vim-notation token. text is the
// printable text the key produced, if any.
func FromTerminal(s, text string) string {
	if text != "" && !strings.Contains(s, "ctrl+") && !strings.Contains(s, "alt+") {
		switch text {
		case " ":
			return "<Space>"
		case "<":
			return "<lt>"
		}
		return text
	}
	if s == "shift+tab" {
		return "<S-Tab>"
	}
	parts := strings.Split(s, "+")
	key, mods := parts[len(parts)-1], parts[:len(parts)-1]
	if key == "" { // "ctrl++"
		key = "+"
	}
	inner := key
	if tok, ok := names[key]; ok {
		inner = strings.Trim(tok, "<>")
	}
	if len(mods) == 0 {
		if len([]rune(key)) == 1 {
			return key
		}
		return "<" + inner + ">"
	}
	var prefix string
	for _, m := range mods {
		switch m {
		case "ctrl":
			prefix += "C-"
		case "alt":
			prefix += "A-"
		case "shift":
			prefix += "S-"
		}
	}
	return "<" + prefix + inner + ">"
}

// bracketed finds a bracketed key name, such as <C-d> or <nope>.
var bracketed = regexp.MustCompile(`<[A-Za-z0-9-]+>`)

// Check reports what Parse lets through but is most likely a mistake: a
// bracketed name it doesn't know, which it reads as the < key and the
// letters after it. <lt> writes a < key.
func Check(seq string) error {
	if seq == "" {
		return fmt.Errorf("empty key sequence")
	}
	for _, name := range bracketed.FindAllString(seq, -1) {
		if _, ok := canonical(name[1 : len(name)-1]); !ok {
			return fmt.Errorf("unknown key name %s; write <lt> for the < key", name)
		}
	}
	return nil
}
