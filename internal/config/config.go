// Package config reads navigo's config.yaml from the per-OS config
// directory. Settings appear here as the features that use them land.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Config is the content of config.yaml. Keys it doesn't know yet are
// ignored.
type Config struct {
	Player Player `yaml:"player"`
}

// Player configures playback.
type Player struct {
	// MPVArgs are extra mpv options, such as --audio-device=... or
	// --ao=null. navigo's own options come first, so these can override
	// them, except the IPC ones navigo can't run without.
	MPVArgs []string `yaml:"mpv_args"`
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
			return Config{}, fmt.Errorf("player.mpv_args: %q is not an mpv option; options start with --", a)
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
