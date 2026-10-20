package config

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Colors is the colors: block: a colour for each role the user can
// recolour. A role left out, or given a value that can't be read (a
// Problem), keeps the terminal-palette default. Values are as ParseColor
// returns them.
type Colors struct {
	Playing, Key, Error, Selection string
	Problems                       []Problem
}

// colorNames are the 16 colours by name: the 8 basic ones, "gray" for
// bright black, and "bright-" before a basic name.
var colorNames = func() map[string]int {
	m := map[string]int{"gray": 8, "grey": 8}
	for i, n := range []string{"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white"} {
		m[n] = i
		m["bright-"+n] = i + 8
	}
	return m
}()

var hexRE = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// CoverColor is the value of the playing role that takes its colour from
// the cover of the playing album.
const CoverColor = "cover"

// ParseColor reads a colour: one of the 16 by name (red, bright-red,
// gray), by number (0 to 15), or as #rrggbb. It returns the number as a
// string, or the hex in lower case.
func ParseColor(s string) (string, error) {
	s = strings.TrimSpace(s)
	if hexRE.MatchString(s) {
		return strings.ToLower(s), nil
	}
	if n, ok := colorNames[strings.ToLower(s)]; ok {
		return strconv.Itoa(n), nil
	}
	if n, err := strconv.Atoi(s); err == nil && n >= 0 && n <= 15 {
		return strconv.Itoa(n), nil
	}
	return "", fmt.Errorf(
		"%q is not a colour: give a name such as red or bright-red, a number from 0 to 15, or #rrggbb",
		s,
	)
}

// UnmarshalYAML reads the block node by node, so a bad colour or an
// unknown role is a Problem at its place and not an error.
func (c *Colors) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		c.Problems = append(
			c.Problems,
			Problem{pos(n), "colors: must be a mapping of roles to colours"},
		)
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		key, val := n.Content[i], n.Content[i+1]
		var dst *string
		switch key.Value {
		case "playing":
			dst = &c.Playing
		case "key":
			dst = &c.Key
		case "error":
			dst = &c.Error
		case "selection":
			dst = &c.Selection
		default:
			c.Problems = append(c.Problems, Problem{
				pos(key),
				fmt.Sprintf(
					"colors: %q is not a role; the roles are playing, key, error and selection",
					key.Value,
				),
			})
			continue
		}
		if val.Kind != yaml.ScalarNode {
			c.Problems = append(
				c.Problems,
				Problem{pos(val), "colors." + key.Value + ": give a colour"},
			)
			continue
		}
		if dst == &c.Playing && strings.EqualFold(strings.TrimSpace(val.Value), CoverColor) {
			*dst = CoverColor
			continue
		}
		v, err := ParseColor(val.Value)
		if err != nil {
			c.Problems = append(
				c.Problems,
				Problem{pos(val), "colors." + key.Value + ": " + err.Error()},
			)
			continue
		}
		*dst = v
	}
	return nil
}
