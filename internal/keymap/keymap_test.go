package keymap

import (
	"reflect"
	"strings"
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

// opTable has the operators d and y, motions, and Visual bindings.
func opTable(t *testing.T) *Engine {
	t.Helper()
	tb := NewTable()
	for _, b := range []struct {
		mode            Mode
		scope, seq, cmd string
	}{
		{Normal, "global", "j", "down"},
		{Normal, "global", "gg", "first"},
		{Normal, "global", "G", "last"},
		{Normal, "global", "ga", "go-album"},
		{Normal, "global", "y", "yank"},
		{Normal, "global", "x", "remove"},
		{Normal, "queue", "d", "delete"},
		{Visual, "global", "d", "delete"},
		{Visual, "global", "<Esc>", "cancel"},
	} {
		if err := tb.Bind(b.mode, b.scope, b.seq, b.cmd); err != nil {
			t.Fatal(err)
		}
	}
	e := NewEngine(tb)
	e.SetKinds(Kinds{
		Operator: func(n string) bool { return n == "delete" || n == "yank" },
		Motion:   func(n string) bool { return n == "down" || n == "first" || n == "last" },
	})
	return e
}

func TestOperators(t *testing.T) {
	q := []string{"queue", "global"}
	for _, c := range []struct {
		name string
		mode Mode
		keys []string
		want Result
	}{
		{"operator waits", Normal, []string{"d"}, Result{Kind: Pending}},
		{"with a motion", Normal, []string{"d", "j"}, Result{Kind: Run, Command: "delete", Motion: "down"}},
		{"doubled", Normal, []string{"y", "y"}, Result{Kind: Run, Command: "yank", Double: true}},
		{"count before", Normal, []string{"3", "d", "d"}, Result{Kind: Run, Command: "delete", Double: true, Count: 3}},
		{"counts multiply", Normal, []string{"2", "d", "3", "j"}, Result{Kind: Run, Command: "delete", Motion: "down", Count: 6}},
		{"count after", Normal, []string{"y", "4", "j"}, Result{Kind: Run, Command: "yank", Motion: "down", Count: 4}},
		{"prefixed motion", Normal, []string{"d", "g", "g"}, Result{Kind: Run, Command: "delete", Motion: "first"}},
		{"not a motion", Normal, []string{"d", "g", "a"}, Result{Kind: NoMatch}},
		{"another command", Normal, []string{"d", "x"}, Result{Kind: NoMatch}},
		{"Esc cancels", Normal, []string{"d", "<Esc>"}, Result{Kind: NoMatch}},
		{"operators run at once in Visual", Visual, []string{"d"}, Result{Kind: Run, Command: "delete"}},
		{"Visual has Normal's motions", Visual, []string{"g", "g"}, Result{Kind: Run, Command: "first"}},
		{"but not its other commands", Visual, []string{"x"}, Result{Kind: NoMatch}},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := opTable(t)
			var got Result
			for _, k := range c.keys {
				got = e.Feed(c.mode, q, k)
			}
			if got != c.want {
				t.Errorf("got %+v, want %+v", got, c.want)
			}
		})
	}
}

func TestOperatorPending(t *testing.T) {
	e := opTable(t)
	q := []string{"queue", "global"}
	for _, k := range []string{"2", "d", "3"} {
		e.Feed(Normal, q, k)
	}
	if p := e.Pending(); p != "2d3" {
		t.Errorf("Pending = %q, want 2d3", p)
	}
	if r := e.Timeout(Normal, q); r.Kind != Pending {
		t.Errorf("Timeout while an operator waits = %+v", r)
	}
	// A register name is understood, and refused.
	e.Reset()
	e.Feed(Normal, q, `"`)
	if r := e.Feed(Normal, q, "a"); r.Kind != Rejected || !strings.Contains(r.Text, "one register") {
		t.Errorf(`"a = %+v, want Rejected`, r)
	}
	if r := e.Feed(Normal, q, "j"); r != (Result{Kind: Run, Command: "down"}) {
		t.Errorf("after the rejection, j = %+v", r)
	}

	// d is an operator in the queue only.
	e.Reset()
	if r := e.Feed(Normal, []string{"tracks", "global"}, "d"); r.Kind != NoMatch {
		t.Errorf("d outside the queue = %+v", r)
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

func TestEffectiveAndContinuations(t *testing.T) {
	tb := table(t)
	seqs := func(bs []Binding) []string {
		var out []string
		for _, b := range bs {
			out = append(out, b.Seq()+"="+b.Command)
		}
		return out
	}
	// <CR> in tracks shadows the global one.
	if got := seqs(tb.Effective(Normal, []string{"tracks", "global"})); !reflect.DeepEqual(got, []string{
		"<CR>=play-now", "a=add-next", "j=down", "gg=first", "G=last", "<C-w>w=window", "'b=albums",
	}) {
		t.Errorf("Effective = %q", got)
	}
	if got := seqs(tb.Continuations(Normal, []string{"queue", "global"}, []string{"d"})); !reflect.DeepEqual(got, []string{"dd=remove"}) {
		t.Errorf("Continuations(d) = %q", got)
	}
	if got := seqs(tb.Continuations(Normal, []string{"global"}, []string{"g"})); !reflect.DeepEqual(got, []string{"gg=first"}) {
		t.Errorf("Continuations(g) = %q", got)
	}
	if got := tb.Scopes(Normal); !reflect.DeepEqual(got, []string{"global", "queue", "tracks"}) {
		t.Errorf("Scopes = %q", got)
	}
}

func TestUnbindCloneCheck(t *testing.T) {
	tb := table(t)
	c := tb.Clone()
	if ok, err := c.Unbind(Normal, "global", "gg"); !ok || err != nil {
		t.Fatalf("Unbind(gg) = %v, %v", ok, err)
	}
	if ok, _ := c.Unbind(Normal, "global", "gg"); ok {
		t.Error("unbound gg twice")
	}
	if len(c.Keys(Normal, []string{"global"}, "first")) != 0 || len(tb.Keys(Normal, []string{"global"}, "first")) != 1 {
		t.Error("Unbind on the clone changed the original, or didn't unbind")
	}
	for seq, ok := range map[string]bool{"gg": true, "<C-d>": true, "<leader>x": true, "<lt>b": true, "<F12>": true, "<nope>": false, "": false} {
		if err := Check(seq); (err == nil) != ok {
			t.Errorf("Check(%q) = %v", seq, err)
		}
	}
}
