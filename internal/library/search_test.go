package library

import (
	"slices"
	"strings"
	"testing"
	"time"
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

func playlistLib() *Snapshot {
	return Build("https://m.example", "ann", time.Time{}, nil, nil,
		[]Track{{ID: "t1", Title: "Road Trip"}, {ID: "t2", Title: "Morning"}},
		[]Playlist{
			{ID: "p1", Name: "Road trip", Tracks: []TrackID{"t1"}},
			{ID: "p2", Name: "Road Trip Classics", Tracks: []TrackID{"t1", "t2"}},
			{ID: "p3", Name: "A road to nowhere"},
			{ID: "p4", Name: "Sängerfest"},
			{ID: "p5", Name: "Morning Mix"},
		}, nil)
}

func names(ps []Playlist) []string {
	var out []string
	for _, p := range ps {
		out = append(out, p.Name)
	}
	return out
}

func TestSearchPlaylists(t *testing.T) {
	s := playlistLib()
	// Same tiers as the other groups: exact, prefix, words; ties by name.
	got := names(s.Search("road trip").Playlists)
	if want := []string{
		"Road trip",
		"Road Trip Classics",
		"A road to nowhere",
	}; !slices.Equal(
		got[:2],
		want[:2],
	) ||
		slices.Contains(got, "Morning Mix") {
		t.Errorf("road trip = %q", got)
	}
	if got := names(s.Search("rd").Playlists); len(got) != 0 {
		t.Errorf("rd = %q, want none", got)
	}
	// Words start words; accents and case are ignored; only the name counts.
	if got := names(
		s.Search("ROAD no").Playlists,
	); !slices.Equal(
		got,
		[]string{"A road to nowhere"},
	) {
		t.Errorf("ROAD no = %q", got)
	}
	if got := names(s.Search("sangerfest").Playlists); !slices.Equal(got, []string{"Sängerfest"}) {
		t.Errorf("sangerfest = %q", got)
	}
	if got := names(s.Search("t1").Playlists); len(got) != 0 {
		t.Errorf("a track ID matched a playlist: %q", got)
	}
}

func TestSearchBestMatchCanBeAPlaylist(t *testing.T) {
	s := playlistLib()
	// "Road trip" is both a track and a playlist, exactly; the earlier
	// group wins the tie.
	if r := s.Search("road trip"); r.Best != PlaylistGroup {
		t.Errorf("best = %d, want the playlist", r.Best)
	}
	if r := s.Search("morning mix"); r.Best != PlaylistGroup || len(r.Tracks) != 0 {
		t.Errorf("morning mix: best %d, tracks %d", r.Best, len(r.Tracks))
	}
	if r := s.Search("morning"); r.Best != TrackGroup {
		t.Errorf("morning best = %d, want the track named exactly that", r.Best)
	}
}

func TestSearchSeesPlaylistChanges(t *testing.T) {
	s := playlistLib()
	n := s.WithPlaylist(Playlist{ID: "p6", Name: "Gym Rotation"})
	if len(n.Search("gym").Playlists) != 1 {
		t.Error("a created playlist isn't found")
	}
	if len(s.Search("gym").Playlists) != 0 {
		t.Error("the old snapshot learned of the new playlist")
	}
	n = n.WithPlaylist(Playlist{ID: "p6", Name: "Cardio"})
	if len(n.Search("gym").Playlists) != 0 || len(n.Search("cardio").Playlists) != 1 {
		t.Error("a renamed playlist is found by its old name")
	}
	n = n.WithoutPlaylist("p6")
	if len(n.Search("cardio").Playlists) != 0 {
		t.Error("a deleted playlist is found")
	}
	e := s.WithExtras([]Playlist{{ID: "p9", Name: "Fresh From Sync"}}, nil)
	if len(e.Search("fresh").Playlists) != 1 || len(e.Search("road").Playlists) != 0 {
		t.Error("playlists from a refresh aren't what is searched")
	}
}

// typoLib is a small library for typo-tolerant search.
func typoLib() *Snapshot {
	s := &Snapshot{
		Artists: []Artist{
			{ID: "a1", Name: "Blind Guardian"},
			{ID: "a2", Name: "Gear Daddies"},
		},
		Albums: []Album{{ID: "al1", Name: "Fly", Artist: "Blind Guardian"}},
		Tracks: []Track{
			{ID: "t1", Title: "Fly", Artist: "Blind Guardian", Album: "Fly"},
			{ID: "t2", Title: "Bald", Artist: "Gear Daddies", Album: "Zero"},
			{ID: "t3", Title: "Blind", Artist: "Gear Daddies", Album: "Zero"},
			{ID: "t4", Title: "Gear", Artist: "Gear Daddies", Album: "Zero"},
			{ID: "t5", Title: "Guardian", Artist: "Gear Daddies", Album: "Zero"},
		},
		Playlists: []Playlist{{ID: "p1", Name: "Gardening"}},
	}
	s.buildSearch()
	return s
}

func TestSearchForgivesTypos(t *testing.T) {
	s := typoLib()
	tracks := func(q string) []string { _, _, tr := found(s.Search(q)); return tr }
	artists := func(q string) []string { a, _, _ := found(s.Search(q)); return a }

	// Two edits off a word's start: "gaer" is "guar" with two letters wrong.
	if got := tracks("blind gaer fly"); !slices.Equal(got, []string{"Fly"}) {
		t.Errorf("blind gaer fly tracks = %q", got)
	}
	if _, al, _ := found(
		s.Search("blind gaer fly"),
	); !slices.Equal(
		al,
		[]string{"Fly (Blind Guardian)"},
	) {
		t.Errorf("blind gaer fly albums = %q", al)
	}
	// A swap of two letters is one edit, and so is a missing letter.
	for _, q := range []string{"gaurdian", "blnd guardian"} { //nolint:misspell // the typo is the test
		if got := artists(q); len(got) == 0 || got[0] != "Blind Guardian" {
			t.Errorf("%q artists = %q", q, got)
		}
	}
	// Playlists forgive typos too.
	if got := names(s.Search("gardneing").Playlists); !slices.Equal(got, []string{"Gardening"}) {
		t.Errorf("gardneing = %q", got)
	}
	// Words of three letters or fewer must be typed right.
	for _, q := range []string{"fyl", "gau", "blind fyl"} {
		if a, al, tr := found(s.Search(q)); len(a)+len(al)+len(tr) > 0 {
			t.Errorf("%q found %q %q %q", q, a, al, tr)
		}
	}
	// More than two edits is no match.
	if got := tracks("gxxxdian"); len(got) != 0 {
		t.Errorf("gxxxdian = %q", got)
	}
}

func TestSearchForgivesTyposOnlyWhenNothingElseMatches(t *testing.T) {
	s := typoLib()
	// "guard" matches as typed, so the playlist one edit off isn't shown.
	if r := s.Search("guard"); len(r.Tracks) == 0 || len(r.Playlists) != 0 {
		t.Errorf("guard: %d tracks, playlists %q", len(r.Tracks), names(r.Playlists))
	}
	// With typos, the fewest edits first: "blid" is one edit from "blin(d)"
	// and two from "bald"; alphabetical order alone would put Bald first.
	if _, _, got := found(
		s.Search("blid"),
	); len(got) < 2 ||
		!slices.Equal(got[:2], []string{"Blind", "Bald"}) {
		t.Errorf("blid tracks = %q", got)
	}
}
