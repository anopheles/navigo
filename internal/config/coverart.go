package config

import (
	"go.yaml.in/yaml/v3"
)

// Cover art modes: the values of Config.CoverArt.Mode.
const (
	CoverAuto   = ""       // the best the terminal does
	CoverOff    = "off"    // no cover art
	CoverBlocks = "blocks" // block characters, whatever the terminal can do
	CoverKitty  = "kitty"  // kitty graphics, whatever the terminal says
)

// CoverArt is the cover_art: setting: true or false, or the renderer to
// force, blocks or kitty.
type CoverArt struct {
	Mode     string
	Problems []Problem
}

// UnmarshalYAML reads the setting; anything else is a Problem and leaves
// cover art on, automatic.
func (c *CoverArt) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		switch n.Value {
		case "true", "auto":
			return nil
		case "false":
			c.Mode = CoverOff
			return nil
		case CoverBlocks, CoverKitty:
			c.Mode = n.Value
			return nil
		}
	}
	c.Problems = append(c.Problems, Problem{pos(n), "cover_art: give true, false, blocks or kitty"})
	return nil
}
