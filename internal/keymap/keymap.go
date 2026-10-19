// Package keymap matches key presses against bindings. It is pure: keys
// go in as vim notation ("j", "<C-d>", "<CR>") and a command line comes
// out, so it has no idea of the terminal or of what commands do.
//
// Lookup walks a scope chain from narrow to wide; the narrowest scope with
// a binding for the keys typed so far wins. Digits before a binding make a
// count. A pure prefix (bound only as the start of longer bindings) waits
// for the next key without a time limit; an ambiguous prefix (bound on its
// own as well) waits until Timeout.
package keymap

import (
	"fmt"
	"strings"
)

// Mode decides which bindings apply.
type Mode string

// The modes the skeleton uses. Command-line keys are handled by the line
// editor, not by bindings.
const (
	Normal  Mode = "normal"
	Visual  Mode = "visual"
	Cmdline Mode = "cmdline"
)

// Binding pairs a key sequence with a command line, within one scope.
type Binding struct {
	Keys    []string // one vim-notation token per key
	Command string
}

// Seq is the binding's keys in vim notation, such as "gg" or "<C-w>w".
func (b Binding) Seq() string { return strings.Join(b.Keys, "") }

// Name is the command's name: the first word of its command line.
func (b Binding) Name() string {
	name, _, _ := strings.Cut(strings.TrimPrefix(b.Command, ":"), " ")
	return name
}

// Table holds bindings per mode and scope, in the order they were bound.
type Table struct {
	bindings map[Mode]map[string][]Binding
}

// NewTable returns an empty table.
func NewTable() *Table {
	return &Table{bindings: map[Mode]map[string][]Binding{}}
}

// Bind adds a binding, replacing one with the same keys in that scope.
func (t *Table) Bind(mode Mode, scope, seq, command string) error {
	keys, err := Parse(seq)
	if err != nil {
		return err
	}
	if t.bindings[mode] == nil {
		t.bindings[mode] = map[string][]Binding{}
	}
	b := Binding{Keys: keys, Command: command}
	list := t.bindings[mode][scope]
	for i, old := range list {
		if old.Seq() == b.Seq() {
			list[i] = b
			return nil
		}
	}
	t.bindings[mode][scope] = append(list, b)
	return nil
}

// Keys returns the key sequences that run the named command in this scope
// chain, narrowest scope first, skipping any a narrower scope shadows.
func (t *Table) Keys(mode Mode, scopes []string, name string) []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range scopes {
		for _, b := range t.bindings[mode][s] {
			seq := b.Seq()
			if seen[seq] {
				continue
			}
			seen[seq] = true
			if b.Name() == name {
				out = append(out, seq)
			}
		}
	}
	return out
}

// lookup reports the binding for keys exactly, if any, and whether keys
// begin a longer binding, over the scope chain.
func (t *Table) lookup(mode Mode, scopes []string, keys []string) (exact *Binding, prefix bool) {
	seq := strings.Join(keys, "")
	seen := map[string]bool{}
	for _, s := range scopes {
		for i := range t.bindings[mode][s] {
			b := &t.bindings[mode][s][i]
			bs := b.Seq()
			if seen[bs] {
				continue
			}
			seen[bs] = true
			switch {
			case bs == seq:
				if exact == nil {
					exact = b
				}
			case len(b.Keys) > len(keys) && strings.HasPrefix(bs, seq) && sameStart(b.Keys, keys):
				prefix = true
			}
		}
	}
	return exact, prefix
}

func sameStart(long, short []string) bool {
	for i := range short {
		if long[i] != short[i] {
			return false
		}
	}
	return true
}

// Kind is what a key press led to.
type Kind int

const (
	// NoMatch: the keys typed so far match nothing; they were dropped.
	NoMatch Kind = iota
	// Pending: waiting for more keys. Ambiguous is set when a binding
	// would run at Timeout.
	Pending
	// Run: Command should run, Count times where it scales.
	Run
)

// Result of feeding a key.
type Result struct {
	Kind      Kind
	Command   string
	Count     int // 0 when no count was typed
	Ambiguous bool
}

// Engine accumulates keys until they match a binding.
type Engine struct {
	table *Table
	count string
	keys  []string
}

// NewEngine returns an engine over t.
func NewEngine(t *Table) *Engine { return &Engine{table: t} }

// Table returns the engine's table.
func (e *Engine) Table() *Table { return e.table }

// Pending returns what has been typed toward a binding, such as "3g", for
// the status line; empty when nothing is pending.
func (e *Engine) Pending() string { return e.count + strings.Join(e.keys, "") }

// Reset drops anything pending.
func (e *Engine) Reset() { e.count, e.keys = "", nil }

// Feed adds one key in vim notation.
func (e *Engine) Feed(mode Mode, scopes []string, key string) Result {
	if len(e.keys) == 0 && mode != Cmdline && isCountDigit(key, e.count != "") {
		e.count += key
		return Result{Kind: Pending}
	}
	keys := append(append([]string(nil), e.keys...), key)
	exact, prefix := e.table.lookup(mode, scopes, keys)
	switch {
	case exact != nil && !prefix:
		return e.run(exact)
	case prefix:
		e.keys = keys
		return Result{Kind: Pending, Ambiguous: exact != nil}
	default:
		e.Reset()
		return Result{Kind: NoMatch}
	}
}

// Timeout resolves an ambiguous prefix by running its own binding. It
// returns NoMatch when nothing ambiguous is pending.
func (e *Engine) Timeout(mode Mode, scopes []string) Result {
	if len(e.keys) == 0 {
		return Result{Kind: NoMatch}
	}
	exact, _ := e.table.lookup(mode, scopes, e.keys)
	if exact == nil {
		return Result{Kind: Pending}
	}
	return e.run(exact)
}

func (e *Engine) run(b *Binding) Result {
	n := 0
	fmt.Sscan(e.count, &n)
	e.Reset()
	return Result{Kind: Run, Command: b.Command, Count: n}
}

func isCountDigit(key string, started bool) bool {
	if len(key) != 1 || key[0] < '0' || key[0] > '9' {
		return false
	}
	return key[0] != '0' || started
}
