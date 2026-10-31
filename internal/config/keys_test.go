package config

import (
	"reflect"
	"strings"
	"testing"
)

const keysYAML = `server:
  url: https://m.example
keys:
  leader: '<Space>'
  timeout: 800
  hints: false
  hint-delay: 300
  defaults: true
  normal:
    global:
      '<C-d>': half-page-down
      '<leader>x': 'seek +60'
    queue:
      'J': ~
  visual:
    global: {}
  cmdline:
    '<C-k>': kill-to-end
`

func TestParseKeys(t *testing.T) {
	c, err := Parse([]byte(keysYAML))
	if err != nil {
		t.Fatal(err)
	}
	k := c.Keys
	if k.LeaderKey() != "<Space>" || k.Timeout != 800 || k.HintsOn() || k.HintDelay != 300 ||
		!k.DefaultsOn() {
		t.Errorf("options = %+v", k)
	}
	want := []Binding{
		{
			Mode:    "normal",
			Scope:   "global",
			Keys:    "<C-d>",
			Command: "half-page-down",
			Pos:     Pos{11, 7},
		},
		{Mode: "normal", Scope: "global", Keys: "<leader>x", Command: "seek +60", Pos: Pos{12, 7}},
		{Mode: "normal", Scope: "queue", Keys: "J", Delete: true, Pos: Pos{14, 7}},
		{Mode: "cmdline", Keys: "<C-k>", Command: "kill-to-end", Pos: Pos{18, 5}},
	}
	if !reflect.DeepEqual(k.Bindings, want) {
		t.Errorf("bindings =\n%+v\nwant\n%+v", k.Bindings, want)
	}
	if len(k.Problems) != 0 {
		t.Errorf("problems = %v", k.Problems)
	}

	// Defaults when the block is missing.
	c, _ = Parse([]byte("hints: true\n"))
	if c.Keys.LeaderKey() != `\` || !c.Keys.HintsOn() || !c.Keys.DefaultsOn() {
		t.Errorf("defaults = %+v", c.Keys)
	}
}

func TestKeysProblems(t *testing.T) {
	c, err := Parse([]byte(`keys:
  timeout: soon
  hints: maybe
  nromal:
    global: {}
  normal:
    global:
      'x': ''
      'y': [a, b]
  visual: up
  leader: ''
`))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range c.Keys.Problems {
		got = append(got, p.Format("config.yaml"))
	}
	want := []string{
		"config.yaml:2:12: timeout: give a number of milliseconds",
		"config.yaml:3:10: hints: give true or false",
		`config.yaml:4:3: unknown option or mode "nromal"; modes are normal, visual, cmdline and search`,
		"config.yaml:8:12: x: empty command; use nop for a dead key or ~ to delete the binding",
		"config.yaml:9:12: y: give a command line, nop for a dead key, or ~ to delete the binding",
		"config.yaml:10:11: visual: must map scopes, such as global, to bindings",
		`config.yaml:11:11: leader: give a key, such as '\' or '<Space>'`,
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("problems:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	// A YAML syntax error is fatal.
	if _, err := Parse([]byte("keys:\n  normal: [\n")); err == nil {
		t.Error("a syntax error parsed")
	}
}
