// Package command parses command lines, as typed at ":" or bound to keys,
// and runs them from a registry. Every key runs exactly one command, so
// the registry is also what the hint line, the key menu and help describe.
package command

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"
)

// Call is one parsed command line plus the count typed before its key.
// An operator's call also has its motion, or Double for whole rows.
type Call struct {
	Name   string
	Args   []string
	Count  int // 0 when no count was typed
	Motion string
	Double bool
}

// N is the count, or 1 when none was typed.
func (c Call) N() int { return max(c.Count, 1) }

// Parse splits a command line such as ":seek +10" into name and arguments.
func Parse(line string) (Call, error) {
	fields := strings.Fields(strings.TrimPrefix(strings.TrimSpace(line), ":"))
	if len(fields) == 0 {
		return Call{}, errors.New("empty command")
	}
	return Call{Name: fields[0], Args: fields[1:]}, nil
}

// Command is one entry in the registry.
type Command[T any] struct {
	Name    string
	Aliases []string // also accepted at ":", such as q for quit
	// Label is the short form for the hint line ("play"); Desc is one
	// sentence for the key menu and help.
	Label string
	Desc  string
	// Motion marks commands that move the cursor and take a count.
	Motion bool
	// Operator marks commands that act on the rows a motion covers.
	Operator bool
	// Jump marks commands whose move goes on the jump list, such as gg.
	Jump bool
	Run  func(target T, c Call) error
	// Complete, if set, lists the argument texts <Tab> offers, such as
	// playlist names.
	Complete func(target T) []string
	// Args checks the arguments of a command line before it is bound to a
	// key. Nil means the command takes none.
	Args func(args []string) error
}

// Registry maps names to commands, in the order they were added.
type Registry[T any] struct {
	byName map[string]*Command[T]
	all    []*Command[T]
}

// NewRegistry returns an empty registry.
func NewRegistry[T any]() *Registry[T] {
	return &Registry[T]{byName: map[string]*Command[T]{}}
}

// Add registers c. A name or alias registered twice is a programming
// error, so it panics.
func (r *Registry[T]) Add(c Command[T]) {
	cp := c
	for _, n := range append([]string{c.Name}, c.Aliases...) {
		if _, dup := r.byName[n]; dup {
			panic("command: " + n + " registered twice")
		}
		r.byName[n] = &cp
	}
	r.all = append(r.all, &cp)
}

// Get returns the command for a name or alias.
func (r *Registry[T]) Get(name string) (*Command[T], bool) {
	c, ok := r.byName[name]
	return c, ok
}

// All returns every command in the order added.
func (r *Registry[T]) All() []*Command[T] { return r.all }

// ErrUnknown is returned for a name that isn't registered.
var ErrUnknown = errors.New("not a command")

// Run parses line and runs it on target with count.
func (r *Registry[T]) Run(target T, line string, count int) error {
	return r.RunCall(target, line, Call{Count: count})
}

// RunCall parses line and runs it on target with the count and motion
// of with.
func (r *Registry[T]) RunCall(target T, line string, with Call) error {
	call, err := Parse(line)
	if err != nil {
		return err
	}
	c, ok := r.byName[call.Name]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknown, call.Name)
	}
	call.Name = c.Name
	call.Count, call.Motion, call.Double = with.Count, with.Motion, with.Double
	return c.Run(target, call)
}

// How a command matches what was typed, best first.
const (
	namePrefix = iota // the name or an alias starts with it
	wholeWords        // each word starts a word of the name or description
	scattered         // its letters appear in the name, in order
)

// Match finds the commands a partly typed name could mean. Commands that
// apply here (by here) come first, then by how they match: name prefix,
// whole words in the name or description, then scattered letters in the
// name. Ties keep registry order. Case is ignored.
func (r *Registry[T]) Match(query string, here func(*Command[T]) bool) []*Command[T] {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	type hit struct {
		c          *Command[T]
		here, tier int
		order      int
	}
	var hits []hit
	for i, c := range r.all {
		tier, ok := matchTier(q, c)
		if !ok {
			continue
		}
		h := 1
		if here != nil && here(c) {
			h = 0
		}
		hits = append(hits, hit{c, h, tier, i})
	}
	slices.SortFunc(hits, func(a, b hit) int {
		return cmp.Or(
			cmp.Compare(a.here, b.here),
			cmp.Compare(a.tier, b.tier),
			cmp.Compare(a.order, b.order),
		)
	})
	out := make([]*Command[T], len(hits))
	for i, h := range hits {
		out[i] = h.c
	}
	return out
}

func matchTier[T any](q string, c *Command[T]) (int, bool) {
	for _, n := range append([]string{c.Name}, c.Aliases...) {
		if strings.HasPrefix(n, q) {
			return namePrefix, true
		}
	}
	words := append(splitWords(c.Name), splitWords(c.Desc)...)
	all := true
	for _, w := range splitWords(q) {
		if !slices.ContainsFunc(words, func(x string) bool { return strings.HasPrefix(x, w) }) {
			all = false
			break
		}
	}
	if all {
		return wholeWords, true
	}
	if subsequence(q, c.Name) {
		return scattered, true
	}
	return 0, false
}

func splitWords(s string) []string {
	return strings.FieldsFunc(
		strings.ToLower(s),
		func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) },
	)
}

// subsequence reports whether q's letters appear in s in order.
func subsequence(q, s string) bool {
	q = strings.ReplaceAll(q, " ", "")
	for _, r := range s {
		if q == "" {
			break
		}
		if r == rune(q[0]) {
			q = q[1:]
		}
	}
	return q == ""
}
