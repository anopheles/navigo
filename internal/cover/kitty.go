package cover

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// Kitty graphics, drawn with unicode placeholders: the image is sent once
// and given a size in cells, and then shows wherever placeholder cells with
// its id are printed. They are ordinary text, so they scroll, redraw and
// survive a terminal multiplexer like text does.

// chunk is the most base64 bytes kitty accepts in one escape code.
const chunk = 4096

// placeholder is the character kitty draws image cells for.
const placeholder = '\U0010EEEE'

// Transmit is the escape code that stores the Art under id as a virtual
// placement of cols×rows cells. Inside tmux it goes through the DCS
// passthrough, which tmux only forwards with allow-passthrough on.
func Transmit(id uint32, cols, rows int, a *Art, side int, tmux bool) string {
	data := base64.StdEncoding.EncodeToString(a.PNG(side))
	var sb strings.Builder
	for i := 0; i < len(data); i += chunk {
		end := min(i+chunk, len(data))
		more := 1
		if end == len(data) {
			more = 0
		}
		if i == 0 {
			fmt.Fprintf(&sb, "\x1b_Ga=T,U=1,f=100,i=%d,c=%d,r=%d,q=2,m=%d;%s\x1b\\",
				id, cols, rows, more, data[i:end])
		} else {
			fmt.Fprintf(&sb, "\x1b_Gq=2,m=%d;%s\x1b\\", more, data[i:end])
		}
	}
	return wrap(sb.String(), tmux)
}

// Delete is the escape code that frees the image stored under id.
func Delete(id uint32, tmux bool) string {
	return wrap(fmt.Sprintf("\x1b_Ga=d,d=I,i=%d,q=2\x1b\\", id), tmux)
}

// wrap puts an escape code inside tmux's passthrough, doubling its escapes.
func wrap(s string, tmux bool) string {
	if !tmux {
		return s
	}
	return "\x1bPtmux;" + strings.ReplaceAll(s, "\x1b", "\x1b\x1b") + "\x1b\\"
}

// MaxCells is the most columns or rows a placeholder image can have: the
// size of the table of numbers kitty reads.
const MaxCells = len(diacritics)

// Placeholders are the rows of cells that show the image sent under id as
// cols×rows cells. The id travels in the foreground colour, so it must fit
// 24 bits. It returns nil if the image is larger than MaxCells.
func Placeholders(id uint32, cols, rows int) []string {
	if cols > MaxCells || rows > MaxCells || id == 0 || id >= 1<<24 {
		return nil
	}
	fg := fmt.Sprintf("\x1b[38;2;%d;%d;%dm", id>>16&255, id>>8&255, id&255)
	out := make([]string, rows)
	for r := range rows {
		var sb strings.Builder
		sb.WriteString(fg)
		for c := range cols {
			sb.WriteRune(placeholder)
			sb.WriteRune(diacritics[r])
			sb.WriteRune(diacritics[c])
		}
		sb.WriteString("\x1b[39m")
		out[r] = sb.String()
	}
	return out
}

// Supported reports whether kitty graphics should work, from the
// environment: the terminals that speak them say so in TERM, TERM_PROGRAM
// or KITTY_WINDOW_ID, and under tmux the image only arrives if tmux passes
// it through (passthrough reports that setting).
func Supported(getenv func(string) string, passthrough func() bool) bool {
	kitty := getenv("TERM") == "xterm-kitty" || getenv("KITTY_WINDOW_ID") != "" ||
		getenv("TERM_PROGRAM") == "ghostty" || getenv("TERM_PROGRAM") == "WezTerm" ||
		getenv("GHOSTTY_RESOURCES_DIR") != ""
	if !kitty {
		return false
	}
	return getenv("TMUX") == "" || passthrough()
}
