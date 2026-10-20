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
	"slices"
	"strings"
)

// Mode decides which bindings apply.
type Mode string

// The modes navigo uses. Command-line and Search keys are handled by the
// line editor, not by bindings.
const (
	Normal  Mode = "normal"
	Visual  Mode = "visual"
	Cmdline Mode = "cmdline"
	Search  Mode = "search"
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

// KeysFor is like Keys, but for one exact command line, arguments and
// all, so "seek +30" doesn't find the key bound to "seek +10".
func (t *Table) KeysFor(mode Mode, scopes []string, line string) []string {
	want := strings.Join(strings.Fields(strings.TrimPrefix(strings.TrimSpace(line), ":")), " ")
	var out []string
	seen := map[string]bool{}
	for _, s := range scopes {
		for _, b := range t.bindings[mode][s] {
			seq := b.Seq()
			if seen[seq] {
				continue
			}
			seen[seq] = true
			if strings.Join(strings.Fields(strings.TrimPrefix(b.Command, ":")), " ") == want {
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
	// Rejected: the keys are understood but not supported; Text says why.
	Rejected
)

// Result of feeding a key.
type Result struct {
	Kind      Kind
	Command   string
	Count     int // 0 when no count was typed
	Ambiguous bool
	// For an operator: the motion command line it applies to, or Double
	// when it was typed twice (dd, yy) for whole rows.
	Motion string
	Double bool
	Text   string
}

// Kinds tells the engine which commands are operators, which wait for a
// motion, and which are motions; the table only knows command lines.
type Kinds struct {
	Operator func(name string) bool
	Motion   func(name string) bool
}

// Engine accumulates keys until they match a binding. In Normal mode an
// operator waits for a motion, its own keys again, or a count before
// either: [count] operator [count] motion | [count] operator operator.
type Engine struct {
	table *Table
	kinds Kinds
	count string
	keys  []string

	op      *Binding // the operator waiting for its motion
	opCount string   // the count typed before the operator
	reg     bool     // " was typed: a register name comes next
}

// NewEngine returns an engine over t.
func NewEngine(t *Table) *Engine { return &Engine{table: t} }

// SetKinds tells the engine about operators and motions.
func (e *Engine) SetKinds(k Kinds) { e.kinds = k }

// Table returns the engine's table.
func (e *Engine) Table() *Table { return e.table }

// Pending returns what has been typed toward a binding, such as "3g" or
// "2d3", for the status line; empty when nothing is pending.
func (e *Engine) Pending() string {
	p := e.count + strings.Join(e.keys, "")
	if e.op != nil {
		p = e.opCount + e.op.Seq() + p
	}
	if e.reg {
		p = `"` + p
	}
	return p
}

// Reset drops anything pending.
func (e *Engine) Reset() { e.count, e.keys, e.op, e.opCount, e.reg = "", nil, nil, "", false }

// Feed adds one key in vim notation.
func (e *Engine) Feed(mode Mode, scopes []string, key string) Result {
	if e.reg {
		e.Reset()
		return Result{Kind: Rejected, Text: `navigo has one register: drop the "` + key + ` and the yank or paste uses it`}
	}
	if len(e.keys) == 0 && mode != Cmdline && isCountDigit(key, e.count != "") {
		e.count += key
		return Result{Kind: Pending}
	}
	keys := append(append([]string(nil), e.keys...), key)
	if e.op != nil && slices.Equal(keys, e.op.Keys) {
		return e.finish(Result{Double: true})
	}
	exact, prefix := e.lookup(mode, scopes, keys)
	switch {
	case exact != nil && !prefix:
		return e.resolve(mode, exact)
	case prefix:
		e.keys = keys
		return Result{Kind: Pending, Ambiguous: exact != nil}
	case key == `"` && len(e.keys) == 0 && e.op == nil && mode != Cmdline:
		e.reg = true
		return Result{Kind: Pending}
	default:
		e.Reset()
		return Result{Kind: NoMatch}
	}
}

// Timeout resolves an ambiguous prefix by running its own binding. It
// returns NoMatch when nothing ambiguous is pending.
func (e *Engine) Timeout(mode Mode, scopes []string) Result {
	if len(e.keys) == 0 {
		if e.op != nil || e.reg {
			return Result{Kind: Pending}
		}
		return Result{Kind: NoMatch}
	}
	exact, _ := e.lookup(mode, scopes, e.keys)
	if exact == nil {
		return Result{Kind: Pending}
	}
	return e.resolve(mode, exact)
}

// lookup is Table.lookup, but Visual mode also has Normal mode's motions,
// and after an operator only motions count.
func (e *Engine) lookup(mode Mode, scopes []string, keys []string) (*Binding, bool) {
	exact, prefix := e.table.lookup(mode, scopes, keys)
	if mode == Visual && exact == nil {
		ne, np := e.table.lookup(Normal, scopes, keys)
		if ne != nil && e.isMotion(ne) {
			exact = ne
		}
		prefix = prefix || np
	}
	if e.op != nil && exact != nil && !e.isMotion(exact) {
		exact = nil
	}
	return exact, prefix
}

func (e *Engine) resolve(mode Mode, b *Binding) Result {
	switch {
	case e.op != nil:
		return e.finish(Result{Motion: b.Command})
	case mode == Normal && e.kinds.Operator != nil && e.kinds.Operator(b.Name()):
		e.op, e.opCount, e.count, e.keys = b, e.count, "", nil
		return Result{Kind: Pending}
	}
	n := 0
	fmt.Sscan(e.count, &n)
	e.Reset()
	return Result{Kind: Run, Command: b.Command, Count: n}
}

// finish runs the pending operator; counts before and after it multiply.
func (e *Engine) finish(r Result) Result {
	var a, b int
	fmt.Sscan(e.opCount, &a)
	fmt.Sscan(e.count, &b)
	r.Kind, r.Command = Run, e.op.Command
	if a > 0 || b > 0 {
		r.Count = max(a, 1) * max(b, 1)
	}
	e.Reset()
	return r
}

func (e *Engine) isMotion(b *Binding) bool {
	return e.kinds.Motion != nil && e.kinds.Motion(b.Name())
}

func isCountDigit(key string, started bool) bool {
	if len(key) != 1 || key[0] < '0' || key[0] > '9' {
		return false
	}
	return key[0] != '0' || started
}
