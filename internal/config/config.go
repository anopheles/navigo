// Package config reads navigo's config.yaml from the per-OS config
// directory. Settings appear here as the features that use them land.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Config is the content of config.yaml. Keys it doesn't know yet are
// ignored.
type Config struct {
	Server Server `yaml:"server"`
	Player Player `yaml:"player"`
	// Hints shows the hint line; on unless set to false.
	Hints *bool `yaml:"hints"`
	// Resume keeps the queue between runs; on unless set to false.
	Resume Resume `yaml:"resume"`
	// Theme names a palette for the colour roles; Colors changes single
	// roles on top of it.
	Theme  Theme  `yaml:"theme"`
	Colors Colors `yaml:"colors"`
	// CoverArt turns the playing track's cover on or off, or forces a
	// renderer; on, automatic, unless set.
	CoverArt CoverArt `yaml:"cover_art"`
	Keys     Keys     `yaml:"keys"`
}

// Resume is the resume: setting.
type Resume struct {
	Off      bool
	Problems []Problem
}

// UnmarshalYAML reads true or false; anything else is a Problem and
// leaves Resume on.
func (r *Resume) UnmarshalYAML(n *yaml.Node) error {
	var v bool
	if n.Kind != yaml.ScalarNode || n.Decode(&v) != nil {
		r.Problems = append(r.Problems, Problem{pos(n), "resume: give true or false"})
		return nil
	}
	r.Off = !v
	return nil
}

// ResumeOn reports whether the queue is kept between runs.
func (c Config) ResumeOn() bool { return !c.Resume.Off }

// Problems are what was wrong with the values of the settings outside the
// keys block, each at its place; the settings concerned keep their
// defaults.
func (c Config) Problems() []Problem {
	return slices.Concat(
		c.Player.Problems, c.Resume.Problems, c.Theme.Problems, c.Colors.Problems,
		c.CoverArt.Problems,
	)
}

// Server is the one server navigo connects to. The password comes from
// PasswordCommand, then Password, then the OS keyring.
type Server struct {
	URL             string `yaml:"url"`
	User            string `yaml:"user"`
	Password        string `yaml:"password"`
	PasswordCommand string `yaml:"password_command"`
}

// HintsOn reports whether the hint line is shown.
func (c Config) HintsOn() bool { return c.Hints == nil || *c.Hints }

// Player configures playback.
type Player struct {
	// MPVPath is the mpv to run when it isn't on PATH.
	MPVPath string `yaml:"mpv_path"`
	// MPVArgs are extra mpv options, such as --audio-device=... or
	// --ao=null. navigo's own options come first, so these can override
	// them, except the IPC ones navigo can't run without.
	MPVArgs []string `yaml:"mpv_args"`
	// ReplayGain is off, track or album, and ReplayGainPreamp is in dB;
	// bad values are Problems and leave the defaults.
	ReplayGain       string  `yaml:"-"`
	ReplayGainPreamp float64 `yaml:"-"`
	// Problems are what was wrong with the block's ReplayGain values.
	Problems []Problem `yaml:"-"`
}

// GainMode is the ReplayGain mode, track unless set.
func (p Player) GainMode() string {
	if p.ReplayGain == "" {
		return "track"
	}
	return p.ReplayGain
}

// maxPreamp is the largest preamp in dB, either way.
const maxPreamp = 15

// UnmarshalYAML reads the block node by node, so a bad ReplayGain value
// is a Problem at its place and not an error.
func (p *Player) UnmarshalYAML(n *yaml.Node) error {
	type plain Player
	if err := n.Decode((*plain)(p)); err != nil {
		return err
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		key, val := n.Content[i], n.Content[i+1]
		bad := func(msg string) {
			p.Problems = append(p.Problems, Problem{pos(val), msg})
		}
		switch key.Value {
		case "replaygain":
			switch val.Value {
			case "off", "track", "album":
				p.ReplayGain = val.Value
			default:
				bad("player.replaygain: give off, track or album")
			}
		case "replaygain_preamp":
			v, err := strconv.ParseFloat(val.Value, 64)
			switch {
			case val.Kind != yaml.ScalarNode || err != nil || math.IsNaN(v):
				bad("player.replaygain_preamp: give a number of dB")
			case v < -maxPreamp || v > maxPreamp:
				bad(
					fmt.Sprintf(
						"player.replaygain_preamp: give dB from -%d to %d",
						maxPreamp,
						maxPreamp,
					),
				)
			default:
				p.ReplayGainPreamp = v
			}
		}
	}
	return nil
}

// Path is where config.yaml lives: navigo/config.yaml under
// os.UserConfigDir (XDG_CONFIG_HOME or ~/.config on Linux, ~/Library/
// Application Support on macOS, %AppData% on Windows).
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "navigo", "config.yaml"), nil
}

// StateDir is where navigo keeps history: navigo/ under XDG_STATE_HOME or
// ~/.local/state on Linux, and under os.UserCacheDir elsewhere, since Go
// has no per-OS state directory.
func StateDir() (string, error) {
	if runtime.GOOS == "linux" {
		if dir := os.Getenv("XDG_STATE_HOME"); filepath.IsAbs(dir) {
			return filepath.Join(dir, "navigo"), nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".local", "state", "navigo"), nil
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "navigo"), nil
}

// Load reads the config at path. A missing file is an empty config.
func Load(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	c, err := Parse(b)
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

// reserved are mpv options navigo sets itself and must keep.
var reserved = []string{"--input-ipc-server", "--input-ipc-client"}

// Parse reads a config from YAML.
func Parse(b []byte) (Config, error) {
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return Config{}, err
	}
	for _, a := range c.Player.MPVArgs {
		if !strings.HasPrefix(a, "--") {
			return Config{}, fmt.Errorf(
				"player.mpv_args: %q is not an mpv option; options start with --",
				a,
			)
		}
		name, _, _ := strings.Cut(a, "=")
		for _, r := range reserved {
			if name == r {
				return Config{}, fmt.Errorf("player.mpv_args: navigo sets %s itself", r)
			}
		}
	}
	return c, nil
}

// SaveServer writes the server block into the config at path, keeping
// everything else in the file. An empty password removes server.password.
// The file is created with mode 0600 and its directory with 0700.
func SaveServer(path string, url, user, password string) error {
	var doc yaml.Node
	b, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return err
	default:
		if err := yaml.Unmarshal(b, &doc); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	if doc.Kind == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return fmt.Errorf("%s: not a YAML mapping", path)
	}
	server := child(root, "server")
	set(server, "url", url)
	set(server, "user", user)
	if password != "" {
		set(server, "password", password)
	} else {
		remove(server, "password")
	}
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o600)
}

// LooseWithPassword reports whether the config at path holds a password
// and can be read by group or others. Windows has no such mode bits.
func LooseWithPassword(path string, c Config) bool {
	if c.Server.Password == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Mode().Perm()&fs.FileMode(0o077) != 0 && !isWindows
}

// child returns the mapping under key in m, adding it if missing.
func child(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			v := m.Content[i+1]
			if v.Kind != yaml.MappingNode {
				*v = yaml.Node{Kind: yaml.MappingNode}
			}
			return v
		}
	}
	v := &yaml.Node{Kind: yaml.MappingNode}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, v)
	return v
}

func set(m *yaml.Node, key, value string) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content[i+1] = &yaml.Node{Kind: yaml.ScalarNode, Value: value}
			return
		}
	}
	m.Content = append(
		m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Value: value},
	)
}

func remove(m *yaml.Node, key string) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content = append(m.Content[:i], m.Content[i+2:]...)
			return
		}
	}
}
