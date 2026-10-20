package keymap

import (
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	for seq, want := range map[string][]string{
		"gg":      {"g", "g"},
		"<C-d>":   {"<C-d>"},
		"<c-d>":   {"<C-d>"},
		"<cr>":    {"<CR>"},
		"<C-w>w":  {"<C-w>", "w"},
		"<":       {"<lt>"},
		"<lt>":    {"<lt>"},
		"<nope>":  {"<lt>", "n", "o", "p", "e", ">"},
		"'b":      {"'", "b"},
		"<Space>": {"<Space>"},
		"<F1>":    {"<F1>"},
		"<A-x>":   {"<A-x>"},
		"<M-x>":   {"<A-x>"},
		"é":       {"é"},
	} {
		got, err := Parse(seq)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("Parse(%q) = %q, %v; want %q", seq, got, err, want)
		}
	}
}

func TestFromTerminal(t *testing.T) {
	for _, c := range []struct{ s, text, want string }{
		{"j", "j", "j"},
		{"G", "G", "G"},
		{">", ">", ">"},
		{"<", "<", "<lt>"},
		{"space", " ", "<Space>"},
		{"enter", "", "<CR>"},
		{"esc", "", "<Esc>"},
		{"tab", "", "<Tab>"},
		{"shift+tab", "", "<S-Tab>"},
		{"ctrl+d", "", "<C-d>"},
		{"ctrl+w", "", "<C-w>"},
		{"alt+x", "", "<A-x>"},
		{"up", "", "<Up>"},
		{"f1", "", "<F1>"},
		{"backspace", "", "<BS>"},
	} {
		if got := FromTerminal(c.s, c.text); got != c.want {
			t.Errorf("FromTerminal(%q, %q) = %q, want %q", c.s, c.text, got, c.want)
		}
	}
}

func table(t *testing.T) *Table {
	t.Helper()
	tb := NewTable()
	for _, b := range []struct{ scope, seq, cmd string }{
		{"global", "j", "down"},
		{"global", "gg", "first"},
		{"global", "G", "last"},
		{"global", "<C-w>w", "window"},
		{"global", "'b", "albums"},
		{"global", "<CR>", "nothing"},
		{"tracks", "<CR>", "play-now"},
		{"tracks", "a", "add-next"},
		{"queue", "d", "remove-one"},
		{"queue", "dd", "remove"},
	} {
		if err := tb.Bind(Normal, b.scope, b.seq, b.cmd); err != nil {
			t.Fatal(err)
		}
	}
	return tb
}

func feed(e *Engine, scopes []string, keys ...string) Result {
	var r Result
	for _, k := range keys {
		r = e.Feed(Normal, scopes, k)
	}
	return r
}

func TestEngine(t *testing.T) {
	tracks := []string{"tracks", "global"}
	queue := []string{"queue", "global"}
	tests := []struct {
		name   string
		scopes []string
		keys   []string
		want   Result
	}{
		{"single key", tracks, []string{"j"}, Result{Kind: Run, Command: "down"}},
		{"count", tracks, []string{"1", "2", "j"}, Result{Kind: Run, Command: "down", Count: 12}},
		{"count with zero", tracks, []string{"1", "0", "G"}, Result{Kind: Run, Command: "last", Count: 10}},
		{"zero alone is no count", tracks, []string{"0"}, Result{Kind: NoMatch}},
		{"pure prefix waits", tracks, []string{"g"}, Result{Kind: Pending}},
		{"pure prefix runs", tracks, []string{"g", "g"}, Result{Kind: Run, Command: "first"}},
		{"ctrl prefix", tracks, []string{"<C-w>", "w"}, Result{Kind: Run, Command: "window"}},
		{"mark", tracks, []string{"'", "b"}, Result{Kind: Run, Command: "albums"}},
		{"unknown after prefix", tracks, []string{"g", "x"}, Result{Kind: NoMatch}},
		{"esc cancels a prefix", tracks, []string{"g", "<Esc>"}, Result{Kind: NoMatch}},
		{"narrow scope wins", tracks, []string{"<CR>"}, Result{Kind: Run, Command: "play-now"}},
		{"wide scope otherwise", queue, []string{"<CR>"}, Result{Kind: Run, Command: "nothing"}},
		{"not in this scope", queue, []string{"a"}, Result{Kind: NoMatch}},
		{"ambiguous prefix waits", queue, []string{"d"}, Result{Kind: Pending, Ambiguous: true}},
		{"ambiguous prefix completes", queue, []string{"d", "d"}, Result{Kind: Run, Command: "remove"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewEngine(table(t))
			if got := feed(e, tt.scopes, tt.keys...); got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestEngineTimeoutAndPending(t *testing.T) {
	e := NewEngine(table(t))
	queue := []string{"queue", "global"}
	feed(e, queue, "3", "d")
	if p := e.Pending(); p != "3d" {
		t.Errorf("Pending = %q, want 3d", p)
	}
	if got := e.Timeout(Normal, queue); got != (Result{Kind: Run, Command: "remove-one", Count: 3}) {
		t.Errorf("Timeout = %+v", got)
	}
	if e.Pending() != "" {
		t.Error("still pending after Timeout ran")
	}
	feed(e, queue, "g")
	if got := e.Timeout(Normal, queue); got.Kind != Pending {
		t.Errorf("Timeout on a pure prefix = %+v, want still pending", got)
	}
}

func TestKeys(t *testing.T) {
	tb := table(t)
	if err := tb.Bind(Normal, "global", "l", "nothing"); err != nil {
		t.Fatal(err)
	}
	if got := tb.Keys(Normal, []string{"tracks", "global"}, "nothing"); !reflect.DeepEqual(got, []string{"l"}) {
		t.Errorf("Keys(nothing) = %q, want only l: <CR> is shadowed by tracks", got)
	}
	if got := tb.Keys(Normal, []string{"queue", "global"}, "nothing"); !reflect.DeepEqual(got, []string{"<CR>", "l"}) {
		t.Errorf("Keys(nothing) in queue = %q", got)
	}
}

func TestKeysFor(t *testing.T) {
	tb := NewTable()
	for seq, cmd := range map[string]string{".": "seek +10", ",": "seek -10", "X": "stop"} {
		if err := tb.Bind(Normal, "global", seq, cmd); err != nil {
			t.Fatal(err)
		}
	}
	g := []string{"global"}
	for line, want := range map[string][]string{":seek  +10": {"."}, "seek -10": {","}, "seek +30": nil, "stop": {"X"}} {
		if got := tb.KeysFor(Normal, g, line); !reflect.DeepEqual(got, want) {
			t.Errorf("KeysFor(%q) = %q, want %q", line, got, want)
		}
	}
}

func TestBindReplaces(t *testing.T) {
	tb := table(t)
	if err := tb.Bind(Normal, "global", "j", "seek +10"); err != nil {
		t.Fatal(err)
	}
	e := NewEngine(tb)
	if got := feed(e, []string{"global"}, "j"); got.Command != "seek +10" {
		t.Errorf("j runs %q after rebinding", got.Command)
	}
}
