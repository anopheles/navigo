package config

import (
	"fmt"
	"strconv"

	"go.yaml.in/yaml/v3"
)

// Keys is the keys: block as written: its options, and its bindings in
// file order with their positions. The UI checks the bindings against
// its commands and scopes; Problems are what was wrong with the block's
// shape, found while reading it. Neither stops navigo.
type Keys struct {
	Leader    string
	Timeout   int // ms; 0 when not set
	Hints     *bool
	HintDelay int // ms; 0 when not set
	Defaults  *bool
	Bindings  []Binding
	Problems  []Problem
}

// Binding is one key sequence → command line in the keys: block.
type Binding struct {
	Mode    string // normal, visual, cmdline or search
	Scope   string // empty for cmdline, which has none
	Keys    string
	Command string
	Delete  bool // ~: remove the binding, falling through to a wider scope
	Pos
}

// Pos is a place in config.yaml.
type Pos struct{ Line, Col int }

// Problem is something wrong in config.yaml, at a place in it.
type Problem struct {
	Pos
	Msg string
}

// Format is the problem as "config.yaml:LINE:COL: message".
func (p Problem) Format(path string) string {
	return fmt.Sprintf("%s:%d:%d: %s", path, p.Line, p.Col, p.Msg)
}

// DefaultsOn reports whether the default bindings apply.
func (k Keys) DefaultsOn() bool { return k.Defaults == nil || *k.Defaults }

// HintsOn reports whether prefix menus show.
func (k Keys) HintsOn() bool { return k.Hints == nil || *k.Hints }

// LeaderKey is the leader, '\' unless set.
func (k Keys) LeaderKey() string {
	if k.Leader == "" {
		return `\`
	}
	return k.Leader
}

func pos(n *yaml.Node) Pos { return Pos{n.Line, n.Column} }

// UnmarshalYAML reads the block node by node, so every binding keeps its
// line and column and a wrong shape is a Problem rather than an error.
func (k *Keys) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		k.problem(n, "keys: must be a mapping of options and modes")
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		key, val := n.Content[i], n.Content[i+1]
		switch key.Value {
		case "leader":
			if val.Kind != yaml.ScalarNode || val.Value == "" {
				k.problem(val, "leader: give a key, such as '\\' or '<Space>'")
				continue
			}
			k.Leader = val.Value
		case "timeout":
			k.Timeout = k.millis(val, "timeout")
		case "hint-delay":
			k.HintDelay = k.millis(val, "hint-delay")
		case "hints":
			k.Hints = k.boolean(val, "hints")
		case "defaults":
			k.Defaults = k.boolean(val, "defaults")
		case "normal", "visual":
			if val.Kind != yaml.MappingNode {
				k.problem(val, key.Value+": must map scopes, such as global, to bindings")
				continue
			}
			for j := 0; j+1 < len(val.Content); j += 2 {
				scope, bindings := val.Content[j], val.Content[j+1]
				k.bindings(key.Value, scope.Value, bindings)
			}
		case "cmdline", "search":
			k.bindings(key.Value, "", val)
		default:
			k.problem(
				key,
				fmt.Sprintf(
					"unknown option or mode %q; modes are normal, visual, cmdline and search",
					key.Value,
				),
			)
		}
	}
	return nil
}

// bindings reads one scope's mapping of key sequence → command line.
func (k *Keys) bindings(mode, scope string, n *yaml.Node) {
	if n.Kind != yaml.MappingNode {
		k.problem(n, "must map key sequences to commands")
		return
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		key, val := n.Content[i], n.Content[i+1]
		b := Binding{Mode: mode, Scope: scope, Keys: key.Value, Pos: pos(key)}
		switch {
		case val.Kind != yaml.ScalarNode:
			k.problem(
				val,
				fmt.Sprintf(
					"%s: give a command line, nop for a dead key, or ~ to delete the binding",
					key.Value,
				),
			)
			continue
		case val.Tag == "!!null":
			b.Delete = true
		case val.Value == "":
			k.problem(
				val,
				fmt.Sprintf(
					"%s: empty command; use nop for a dead key or ~ to delete the binding",
					key.Value,
				),
			)
			continue
		default:
			b.Command = val.Value
		}
		k.Bindings = append(k.Bindings, b)
	}
}

func (k *Keys) millis(n *yaml.Node, name string) int {
	v, err := strconv.Atoi(n.Value)
	if n.Kind != yaml.ScalarNode || err != nil || v < 0 {
		k.problem(n, name+": give a number of milliseconds")
		return 0
	}
	return v
}

func (k *Keys) boolean(n *yaml.Node, name string) *bool {
	var v bool
	if n.Kind != yaml.ScalarNode || n.Decode(&v) != nil {
		k.problem(n, name+": give true or false")
		return nil
	}
	return &v
}

func (k *Keys) problem(n *yaml.Node, msg string) {
	k.Problems = append(k.Problems, Problem{pos(n), msg})
}
