package config

import "testing"

func TestParseColor(t *testing.T) {
	for in, want := range map[string]string{
		"red": "1", "Red": "1", "bright-red": "9", "gray": "8", "grey": "8", "white": "7",
		"bright-white": "15", "0": "0", "15": "15",
		"#FF9E64": "#ff9e64", "#7dcfff": "#7dcfff",
	} {
		if got, err := ParseColor(in); err != nil || got != want {
			t.Errorf("ParseColor(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if got, err := ParseColor(" 5 "); err != nil || got != "5" { // padding is ignored
		t.Errorf("ParseColor(\" 5 \") = %q, %v", got, err)
	}
	for _, in := range []string{"", "reddish", "16", "-1", "#fff", "#12345g", "bright-gray", "1.5"} {
		if got, err := ParseColor(in); err == nil {
			t.Errorf("ParseColor(%q) = %q, want an error", in, got)
		}
	}
}

func TestParseColors(t *testing.T) {
	c, err := Parse(
		[]byte("colors:\n  playing: green\n  key: '#7DCFFF'\n  error: 9\n  selection: gray\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	got := c.Colors
	if got.Playing != "2" || got.Key != "#7dcfff" || got.Error != "9" || got.Selection != "8" ||
		len(c.Problems()) != 0 {
		t.Errorf("colors = %+v, problems %v", got, c.Problems())
	}
}

func TestParseColorsProblems(t *testing.T) {
	c, err := Parse([]byte(`colors:
  playing: greenish
  accent: red
  key: [1]
  selection: blue
`))
	if err != nil {
		t.Fatal(err)
	}
	ps := c.Problems()
	if len(ps) != 3 {
		t.Fatalf("problems = %v", ps)
	}
	for i, want := range []struct {
		line, col int
		msg       string
	}{
		{2, 12, `colors.playing: "greenish" is not a colour`},
		{3, 3, `colors: "accent" is not a role; the roles are playing, key, error and selection`},
		{4, 8, "colors.key: give a colour"},
	} {
		p := ps[i]
		if p.Line != want.line || p.Col != want.col || len(p.Msg) < len(want.msg) ||
			p.Msg[:len(want.msg)] != want.msg {
			t.Errorf("problem %d = %+v, want %+v", i, p, want)
		}
	}
	// What was fine in the block still applies.
	if c.Colors.Selection != "4" || c.Colors.Playing != "" || c.Colors.Key != "" {
		t.Errorf("colors = %+v", c.Colors)
	}
}

func TestColorsNotAMapping(t *testing.T) {
	c, err := Parse([]byte("colors: red\n"))
	if err != nil || len(c.Problems()) != 1 ||
		c.Problems()[0].Msg != "colors: must be a mapping of roles to colours" {
		t.Errorf("problems = %v, %v", c.Problems(), err)
	}
}
