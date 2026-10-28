package config

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestParseMPVArgs(t *testing.T) {
	c, err := Parse([]byte("player:\n  mpv_args: [--ao=null, '--volume-max=150']\nkeys: {}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{
		"--ao=null",
		"--volume-max=150",
	}; !reflect.DeepEqual(
		c.Player.MPVArgs,
		want,
	) {
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

func TestStateDir(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("XDG_STATE_HOME is Linux only")
	}
	t.Setenv("HOME", "/home/x")
	for env, want := range map[string]string{"/state": "/state/navigo", "": "/home/x/.local/state/navigo", "relative": "/home/x/.local/state/navigo"} {
		t.Setenv("XDG_STATE_HOME", env)
		if got, err := StateDir(); err != nil || got != want {
			t.Errorf("XDG_STATE_HOME=%q: StateDir = %q, %v; want %q", env, got, err, want)
		}
	}
}

func TestSaveServerKeepsTheRest(t *testing.T) {
	p := filepath.Join(t.TempDir(), "navigo", "config.yaml")
	if err := SaveServer(p, "https://music.example", "ann", ""); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(p); info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}
	orig := "# my config\nplayer:\n  mpv_args: [--ao=null] # quiet\nserver:\n  url: old\n  password_command: pass show navidrome\n"
	if err := os.WriteFile(p, []byte(orig), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := SaveServer(p, "https://music.example", "ann", "s3cret"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	for _, want := range []string{"# my config", "# quiet", "password_command: pass show navidrome", "password: s3cret"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("saved config lacks %q:\n%s", want, b)
		}
	}
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	want := Server{
		URL:             "https://music.example",
		User:            "ann",
		Password:        "s3cret",
		PasswordCommand: "pass show navidrome",
	}
	if c.Server != want || len(c.Player.MPVArgs) != 1 {
		t.Errorf("loaded %+v", c)
	}
	if err := SaveServer(p, "https://music.example", "ann", ""); err != nil {
		t.Fatal(err)
	}
	if c, _ := Load(p); c.Server.Password != "" {
		t.Error("empty password didn't remove server.password")
	}
}

func TestLooseWithPassword(t *testing.T) {
	if isWindows {
		t.Skip("no mode bits")
	}
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte("server: {password: x}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, _ := Load(p)
	if !LooseWithPassword(p, c) {
		t.Error("0644 with a password should be loose")
	}
	_ = os.Chmod(p, 0o600)
	if LooseWithPassword(p, c) {
		t.Error("0600 is not loose")
	}
}

func TestHints(t *testing.T) {
	c, _ := Parse([]byte("{}"))
	off, _ := Parse([]byte("hints: false"))
	if !c.HintsOn() || off.HintsOn() {
		t.Error("hints default on, false turns them off")
	}
}

func TestParseMPVPath(t *testing.T) {
	c, err := Parse([]byte("player:\n  mpv_path: /opt/mpv/bin/mpv\n"))
	if err != nil || c.Player.MPVPath != "/opt/mpv/bin/mpv" {
		t.Errorf("mpv_path = %q, %v", c.Player.MPVPath, err)
	}
}

func TestParseReplayGain(t *testing.T) {
	c, err := Parse(nil)
	if err != nil || c.Player.GainMode() != "track" || c.Player.ReplayGainPreamp != 0 {
		t.Fatalf("defaults = %q, %v, %v", c.Player.GainMode(), c.Player.ReplayGainPreamp, err)
	}
	c, err = Parse(
		[]byte("player:\n  mpv_path: /x/mpv\n  replaygain: album\n  replaygain_preamp: -3.5\n"),
	)
	if err != nil || c.Player.GainMode() != "album" || c.Player.ReplayGainPreamp != -3.5 ||
		c.Player.MPVPath != "/x/mpv" || len(c.Player.Problems) != 0 {
		t.Fatalf("set = %+v, %v", c.Player, err)
	}
}

func TestParseReplayGainProblems(t *testing.T) {
	for _, tc := range []struct {
		yaml, msg string
		line, col int
	}{
		{"player:\n  replaygain: loud\n", "player.replaygain: give off, track or album", 2, 15},
		{"player:\n  replaygain_preamp: lots\n", "player.replaygain_preamp: give a number of dB", 2, 22},
		{"player:\n  replaygain_preamp: 40\n", "player.replaygain_preamp: give dB from -15 to 15", 2, 22},
		{"player:\n  replaygain_preamp: -16\n", "player.replaygain_preamp: give dB from -15 to 15", 2, 22},
		{"player:\n  radio: sometimes\n", "player.radio: give true or false", 2, 10},
	} {
		c, err := Parse([]byte(tc.yaml))
		if err != nil {
			t.Errorf("%q: %v", tc.yaml, err)
			continue
		}
		if len(c.Player.Problems) != 1 {
			t.Errorf("%q: problems = %v", tc.yaml, c.Player.Problems)
			continue
		}
		p := c.Player.Problems[0]
		if p.Msg != tc.msg || p.Line != tc.line || p.Col != tc.col {
			t.Errorf("%q: problem = %+v, want %q at %d:%d", tc.yaml, p, tc.msg, tc.line, tc.col)
		}
		// The bad value leaves the defaults.
		if c.Player.GainMode() != "track" || c.Player.ReplayGainPreamp != 0 {
			t.Errorf("%q: kept %q, %v", tc.yaml, c.Player.GainMode(), c.Player.ReplayGainPreamp)
		}
	}
}

func TestParseRadio(t *testing.T) {
	for yaml, want := range map[string]bool{
		"":                          false,
		"player:\n  radio: true\n":  true,
		"player:\n  radio: false\n": false,
	} {
		c, err := Parse([]byte(yaml))
		if err != nil || c.Player.Radio != want || len(c.Player.Problems) != 0 {
			t.Errorf(
				"%q: radio %v, %v, %v; want %v",
				yaml,
				c.Player.Radio,
				c.Player.Problems,
				err,
				want,
			)
		}
	}
}

func TestParseSyncQueue(t *testing.T) {
	for yaml, want := range map[string]bool{
		"": false, "sync_queue: true\n": true, "sync_queue: false\n": false, "sync_queue: maybe\n": false,
	} {
		c, err := Parse([]byte(yaml))
		if err != nil || c.SyncQueue.On != want {
			t.Errorf("%q: SyncQueue = %v, err %v", yaml, c.SyncQueue.On, err)
		}
	}
	c, _ := Parse([]byte("sync_queue: [1]\n"))
	if ps := c.Problems(); len(ps) != 1 || ps[0].Msg != "sync_queue: give true or false" {
		t.Errorf("problems = %v", ps)
	}
}

func TestParseResume(t *testing.T) {
	for _, tc := range []struct {
		yaml string
		on   bool
		msg  string
	}{
		{"", true, ""},
		{"resume: true\n", true, ""},
		{"resume: false\n", false, ""},
		{"resume: maybe\n", true, "resume: give true or false"},
		{"resume: [1]\n", true, "resume: give true or false"},
	} {
		c, err := Parse([]byte(tc.yaml))
		if err != nil {
			t.Errorf("%q: %v", tc.yaml, err)
			continue
		}
		if c.ResumeOn() != tc.on {
			t.Errorf("%q: ResumeOn = %v", tc.yaml, c.ResumeOn())
		}
		ps := c.Problems()
		if tc.msg == "" && len(ps) != 0 ||
			tc.msg != "" && (len(ps) != 1 || ps[0].Msg != tc.msg || ps[0].Line != 1) {
			t.Errorf("%q: problems = %v", tc.yaml, ps)
		}
	}
}
