// Package command parses command lines, as typed at ":" or bound to keys,
// and runs them from a registry. Every key runs exactly one command, so
// the registry is also what the hint line, the key menu and help describe.
package command

import (
	"errors"
	"fmt"
	"strings"
)

// Call is one parsed command line plus the count typed before its key.
type Call struct {
	Name  string
	Args  []string
	Count int // 0 when no count was typed
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
	Run    func(target T, c Call) error
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
	call, err := Parse(line)
	if err != nil {
		return err
	}
	c, ok := r.byName[call.Name]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknown, call.Name)
	}
	call.Name = c.Name
	call.Count = count
	return c.Run(target, call)
}
