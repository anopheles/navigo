package ui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/anopheles/navigo/internal/config"
	"github.com/anopheles/navigo/internal/keymap"
	"github.com/anopheles/navigo/internal/subsonic/subsonictest"
)

func parseKeys(t *testing.T, yaml string) config.Keys {
	t.Helper()
	c, err := config.Parse([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	return c.Keys
}

func TestBindingProblems(t *testing.T) {
	k := parseKeys(t, `keys:
  normal:
    nowhere:
      'x': stop
    global:
      '<nope>': stop
      'Q': queue-shred
      'W': kill-to-end
      '<C-x>': seek fast
      '<C-y>': stop now
      '<c-d>': stop
      '<C-d>': play-pause
      '"a': yank
      'gz': ~
      'g': [x]
  cmdline:
    'jk': cancel
    '<C-g>': stop
`)
	_, _, problems, err := buildKeys(k, commands())
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range problems {
		got = append(got, p.Format("config.yaml"))
	}
	want := []string{
		`config.yaml:4:7: unknown scope "nowhere"; scopes are ` + strings.Join(scopeNames, ", "),
		"config.yaml:6:7: unknown key name <nope>; write <lt> for the < key",
		`config.yaml:7:7: unknown command "queue-shred"`,
		"config.yaml:8:7: kill-to-end edits the command line: bind it under cmdline:",
		`config.yaml:9:7: seek: "fast" is not a number of seconds`,
		"config.yaml:10:7: stop takes no arguments",
		"config.yaml:12:7: <C-d> is bound already, at line 11",
		`config.yaml:13:7: "a: keys can't start with ", which names a register; navigo has one`,
		"config.yaml:14:7: gz isn't bound in normal.global, so there is nothing to delete",
		"config.yaml:15:12: g: give a command line, nop for a dead key, or ~ to delete the binding",
		"config.yaml:17:5: jk: command-line bindings are single keys",
		`config.yaml:18:5: "stop" isn't a command-line command; they are ` + strings.Join([]string{"accept", "cancel", "complete", "complete-back", "history-back", "history-forward", "kill-to-end", "kill-to-start", "kill-word", "line-end", "line-start", "nop"}, ", "),
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("problems:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestBindingsApply(t *testing.T) {
	k := parseKeys(t, `keys:
  leader: ','
  timeout: 300
  hint-delay: 200
  hints: false
  normal:
    global:
      '<leader>x': 'seek +60'
      '<C-n>': down
    tracks:
      'A': ~
    albums:
      'l': nop
  cmdline:
    '<C-k>': nop
`)
	tb, set, problems, err := buildKeys(k, commands())
	if err != nil || len(problems) > 0 {
		t.Fatalf("problems %v, %v", problems, err)
	}
	if set.timeout != 300*time.Millisecond || set.hintDelay != 200*time.Millisecond || set.prefixMenus || set.leader != "," {
		t.Errorf("settings = %+v", set)
	}
	if ks := tb.KeysFor(keymap.Normal, []string{"global"}, "seek +60"); len(ks) != 1 || ks[0] != ",x" {
		t.Errorf("leader binding = %q", ks)
	}
	// ~ falls through to the wider scope; nop is a dead key.
	e := keymap.NewEngine(tb)
	if r := e.Feed(keymap.Normal, []string{"album-tracks", "tracks", "global"}, "A"); r.Kind != keymap.NoMatch {
		t.Errorf("A after ~ = %+v", r)
	}
	if r := e.Feed(keymap.Normal, []string{"albums", "containers", "global"}, "l"); r.Command != "nop" {
		t.Errorf("l = %+v, want nop", r)
	}
	// A motion bound in normal works in Visual mode too.
	e.SetKinds(keymap.Kinds{Motion: func(n string) bool { return n == "down" }})
	if r := e.Feed(keymap.Visual, []string{"global"}, "<C-n>"); r.Command != "down" {
		t.Errorf("<C-n> in Visual = %+v", r)
	}
}

func TestNoQuitRefusesToStart(t *testing.T) {
	k := parseKeys(t, "keys:\n  defaults: false\n  normal:\n    global:\n      j: down\n")
	if _, _, _, err := buildKeys(k, commands()); !errors.Is(err, errNoQuit) {
		t.Errorf("err = %v, want errNoQuit", err)
	}
	_, err := New(Deps{Config: config.Config{Keys: k}})
	if !errors.Is(err, errNoQuit) {
		t.Errorf("New = %v, want errNoQuit", err)
	}
	// : is enough: :q quits.
	k = parseKeys(t, "keys:\n  defaults: false\n  normal:\n    global:\n      ':': cmdline\n")
	if _, _, _, err := buildKeys(k, commands()); err != nil {
		t.Errorf("with : bound: %v", err)
	}
	if problems, err := CheckConfig(config.Config{Keys: k}, "c.yaml"); len(problems) != 0 || err != nil {
		t.Errorf("CheckConfig = %v, %v", problems, err)
	}
}

// keysHarness starts navigo with a keys: block, written to config.yaml
// too, so :reload-config reads it.
func keysHarness(t *testing.T, yaml string) *harness {
	t.Helper()
	k := parseKeys(t, yaml)
	h := newHarness(t, 120, 40, func(c *config.Config, _ *subsonictest.Server) { c.Keys = k })
	if err := os.MkdirAll(filepath.Dir(h.path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(h.path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	return h
}

func TestRebindingShows(t *testing.T) {
	h := keysHarness(t, `keys:
  normal:
    tracks:
      'A': ~
      'Q': add-end
`)
	h.keys("<CR>")
	h.contains("Q add to queue")
	h.lacks("A add to queue")
	h.keys("Q")
	h.contains("1 track added to the end of the queue")
	h.keys("'?/")
	h.typeText("add-end")
	h.keys("<CR>")
	h.contains("Q          add-end")
	// On tracks, Q replaced A (search results and Visual mode keep theirs).
	heading := ""
	for _, r := range h.m.top().help.all {
		if r.heading != "" {
			heading = r.heading
		} else if r.key == "A" && heading == "On tracks" {
			t.Errorf("Help still lists A on tracks")
		}
	}
}

func TestProblemsPopup(t *testing.T) {
	h := keysHarness(t, "keys:\n  normal:\n    global:\n      'Q': queue-shred\n")
	h.contains("config.yaml has problems", `config.yaml:4:7: unknown command "queue-shred"`)
	h.keys("q")
	h.lacks("has problems")
}

func TestSessionMappings(t *testing.T) {
	h := keysHarness(t, "keys:\n  normal:\n    global:\n      '<C-n>': down\n")
	h.keys(":nmap --in tracks Q add-end<CR>")
	h.contains("Q now runs add-end (this session)")
	h.keys("<CR>Q")
	h.contains("1 track added")
	h.keys(":map<CR>")
	h.contains("normal.global", "<C-n>", "normal.tracks", "add-end")
	h.keys("q")

	// Bad ones are refused with the reason.
	h.keys(":nmap Q nope<CR>")
	h.contains(`unknown command "nope"`)
	h.keys(":nunmap --in tracks Z<CR>")
	h.contains("Z isn't bound in normal.tracks")

	// :cmap: <C-k> does nothing on the line now.
	h.keys(":")
	h.typeText("cmap <C-k> nop")
	h.keys("<CR>:")
	h.typeText("sync")
	h.keys("<C-a><C-k>")
	if v := h.m.line.Value(); v != "sync" {
		t.Errorf("<C-k> as nop changed the line to %q", v)
	}
	h.keys("<Esc>")

	// :reload-config drops the session's mappings and keeps the file's.
	h.keys(":reload-config<CR>")
	h.contains("config reloaded")
	h.keys("Q")
	h.lacks("2 tracks added")
	if ks := h.m.engine.Table().Keys(keymap.Normal, []string{"global"}, "down"); len(ks) < 2 {
		t.Errorf("down keys after reload = %q", ks)
	}
}
