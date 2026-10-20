package library

import (
	"slices"
	"strings"
	"testing"
)

// found lists results as "name (artist)" so same-named albums differ.
func found(r Results) (artists, albums, tracks []string) {
	for _, a := range r.Artists {
		artists = append(artists, a.Name)
	}
	for _, a := range r.Albums {
		albums = append(albums, a.Name+" ("+a.Artist+")")
	}
	for _, t := range r.Tracks {
		tracks = append(tracks, t.Title)
	}
	return
}

func TestSearchRanking(t *testing.T) {
	s, _, _ := synced(t)
	for _, c := range []struct {
		query                   string
		artists, albums, tracks []string // a prefix of each group
		best                    Group
	}{
		// Exact names first, then names starting with the query; the two
		// "Blue" albums tie and go alphabetically by artist.
		{"blue", nil, []string{"Blue (Aurora Lane)", "Blue (The Quiet Ones)", "Blue Hour (Aurora Lane)"},
			[]string{"Blue", "Blue Again", "Blue Highway", "Blue Hour", "Blue Static"}, AlbumGroup},
		// Words start words; tracks on the album match through it, after
		// the title match, the one played first (play count breaks ties).
		{"blu ho", nil, []string{"Blue Hour (Aurora Lane)"}, []string{"Blue Hour", "Low Tide", "Afterglow"}, AlbumGroup},
		// Spread over artist and title.
		{"aurora low", nil, nil, []string{"Low Tide"}, TrackGroup},
		// Accents and case are ignored.
		{"angstrom", nil, []string{"Ångström Days (Zoë Brandt)"}, []string{"Ångström"}, TrackGroup},
		{"ZOE", []string{"Zoë Brandt"}, []string{"Ångström Days (Zoë Brandt)"}, []string{"Ångström"}, ArtistGroup},
		{"quiet", []string{"The Quiet Ones"}, []string{"Static Bloom (The Quiet Ones)", "Blue (The Quiet Ones)"}, []string{"Quiet Room"}, TrackGroup},
		{"夜明", nil, []string{"夜明け (森の音)"}, []string{"夜明け前"}, AlbumGroup},
	} {
		r := s.Search(c.query)
		artists, albums, tracks := found(r)
		check := func(group string, got, want []string) {
			if len(got) < len(want) || !slices.Equal(got[:len(want)], want) {
				t.Errorf("%q %s = %q, want them to start %q", c.query, group, got, want)
			}
			if len(want) == 0 && len(got) > 0 {
				t.Errorf("%q %s = %q, want none", c.query, group, got)
			}
		}
		check("artists", artists, c.artists)
		check("albums", albums, c.albums)
		check("tracks", tracks, c.tracks)
		if r.Best != c.best {
			t.Errorf("%q best in group %d, want %d", c.query, r.Best, c.best)
		}
	}
}

func TestSearchMisses(t *testing.T) {
	s, _, _ := synced(t)
	for _, q := range []string{"", "   ", "lue", "xyz", "blue xyz"} {
		r := s.Search(q)
		if a, b, c := found(r); len(a)+len(b)+len(c) > 0 || r.Best != -1 {
			t.Errorf("%q found %q %q %q", q, a, b, c)
		}
	}
}

func TestWords(t *testing.T) {
	for in, want := range map[string]string{
		"Ångström Days":    "angstrom days",
		"Über Alles Still": "uber alles still",
		"Don't Stop":       "dont stop",
		"Straße/Ørsted":    "strasse orsted",
		"Mile 042":         "mile 042",
	} {
		if got := strings.Join(words(in), " "); got != want {
			t.Errorf("words(%q) = %q, want %q", in, got, want)
		}
	}
}
