package config

import "testing"

func TestParseCoverArt(t *testing.T) {
	for in, want := range map[string]string{
		"":                        CoverAuto,
		"cover_art: true\n":       CoverAuto,
		"cover_art: auto\n":       CoverAuto,
		"cover_art: false\n":      CoverOff,
		"cover_art: blocks\n":     CoverBlocks,
		"cover_art: kitty\n":      CoverKitty,
		"cover_art: false # no\n": CoverOff,
	} {
		c, err := Parse([]byte(in))
		if err != nil || len(c.Problems()) != 0 || c.CoverArt.Mode != want {
			t.Errorf("%q: mode %q, problems %v, err %v; want %q",
				in, c.CoverArt.Mode, c.Problems(), err, want)
		}
	}
}

func TestParseCoverArtProblems(t *testing.T) {
	for _, in := range []string{"cover_art: sixel\n", "cover_art: [blocks]\n", "cover_art: 3\n"} {
		c, err := Parse([]byte(in))
		ps := c.Problems()
		if err != nil || len(ps) != 1 || ps[0].Line != 1 || c.CoverArt.Mode != CoverAuto {
			t.Errorf("%q: problems %v, mode %q, err %v", in, ps, c.CoverArt.Mode, err)
		}
	}
}
