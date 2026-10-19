package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseMPVArgs(t *testing.T) {
	c, err := Parse([]byte("player:\n  mpv_args: [--ao=null, '--volume-max=150']\nkeys: {}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"--ao=null", "--volume-max=150"}; !reflect.DeepEqual(c.Player.MPVArgs, want) {
		t.Errorf("mpv_args = %q, want %q", c.Player.MPVArgs, want)
	}
}

func TestParseRejects(t *testing.T) {
	for in, want := range map[string]string{
		"player:\n  mpv_args: [https://example/x.mp3]\n": "not an mpv option",
		"player:\n  mpv_args: [--input-ipc-server=/x]\n": "navigo sets --input-ipc-server",
		"player:\n  mpv_args: --ao=null\n":               "cannot unmarshal",
		"player:\n  mpv_args: [--ao=null\n":              "line",
	} {
		if _, err := Parse([]byte(in)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Parse(%q) = %v, want an error containing %q", in, err, want)
		}
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	if c, err := Load(filepath.Join(dir, "missing.yaml")); err != nil || c.Player.MPVArgs != nil {
		t.Errorf("Load(missing) = %+v, %v; want an empty config", c, err)
	}
	p := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(p, []byte("player: [\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil || !strings.Contains(err.Error(), p) {
		t.Errorf("Load(bad) = %v, want an error naming the file", err)
	}
}

func TestPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/cfg")
	t.Setenv("HOME", "/home/x")
	p, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(p) != "config.yaml" || filepath.Base(filepath.Dir(p)) != "navigo" {
		t.Errorf("Path = %s, want .../navigo/config.yaml", p)
	}
}
