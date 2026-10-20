package library

import (
	"cmp"
	"slices"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Group is one kind of search result, in the order results show them.
type Group int

const (
	ArtistGroup Group = iota
	AlbumGroup
	TrackGroup
)

// Results are what a search found, each group best match first.
type Results struct {
	Artists []Artist
	Albums  []Album
	Tracks  []Track
	// Best is the group holding the best match overall, or -1 when
	// nothing matched. Its first entry is the match.
	Best Group
}

// Len is the number of results in a group.
func (r Results) Len(g Group) int {
	switch g {
	case ArtistGroup:
		return len(r.Artists)
	case AlbumGroup:
		return len(r.Albums)
	}
	return len(r.Tracks)
}

// Match tiers, best first.
const (
	exact      = iota // the name is the query
	namePrefix        // the name starts with the query
	nameWords         // every query word starts a word of the name
	spread            // ... or of the name, artist and album together
)

// rank orders matches: tier, then play count, then name.
type rank struct {
	tier, plays int
	name        string
}

func (a rank) compare(b rank) int {
	return cmp.Or(
		cmp.Compare(a.tier, b.tier),
		cmp.Compare(b.plays, a.plays),
		cmp.Compare(a.name, b.name),
	)
}

// keys is a name prepared for matching: folded words, and them joined.
type keys struct {
	words  []string
	joined string
}

func newKeys(s string) keys {
	w := words(s)
	return keys{w, strings.Join(w, " ")}
}

// searchIndex holds every artist, album and track prepared for matching.
// Tracks' extra words are their artist and album; albums' are the album
// artist.
type searchIndex struct {
	artists            []keys
	albums, albumExtra []keys
	tracks, trackExtra []keys
}

func (s *Snapshot) buildSearch() {
	x := &searchIndex{}
	for _, a := range s.Artists {
		x.artists = append(x.artists, newKeys(a.Name))
	}
	for _, a := range s.Albums {
		x.albums = append(x.albums, newKeys(a.Name))
		x.albumExtra = append(x.albumExtra, newKeys(a.Artist))
	}
	for _, t := range s.Tracks {
		x.tracks = append(x.tracks, newKeys(t.Title))
		x.trackExtra = append(x.trackExtra, newKeys(t.Artist+" "+t.Album))
	}
	s.search = x
}

// Search finds artists by name, albums by name and album artist, and
// tracks by title, artist and album, ignoring case and accents. Every
// query word must start a word. An empty query finds nothing.
func (s *Snapshot) Search(query string) Results {
	q := newKeys(query)
	res := Results{Best: -1}
	if len(q.words) == 0 {
		return res
	}
	x := s.search
	var best rank
	found := func(g Group, r rank) {
		if res.Best < 0 || r.compare(best) < 0 {
			res.Best, best = g, r
		}
	}

	var ar []rank
	for i, k := range x.artists {
		if t, ok := match(q, k, keys{}); ok {
			res.Artists = append(res.Artists, s.Artists[i])
			ar = append(ar, rank{t, 0, k.joined})
		}
	}
	sortBy(res.Artists, ar)
	if len(ar) > 0 {
		found(ArtistGroup, slices.MinFunc(ar, rank.compare))
	}

	var al []rank
	for i, k := range x.albums {
		if t, ok := match(q, k, x.albumExtra[i]); ok {
			res.Albums = append(res.Albums, s.Albums[i])
			al = append(al, rank{t, s.Albums[i].PlayCount, k.joined})
		}
	}
	sortBy(res.Albums, al)
	if len(al) > 0 {
		found(AlbumGroup, slices.MinFunc(al, rank.compare))
	}

	var tr []rank
	for i, k := range x.tracks {
		if t, ok := match(q, k, x.trackExtra[i]); ok {
			res.Tracks = append(res.Tracks, s.Tracks[i])
			tr = append(tr, rank{t, s.Tracks[i].PlayCount, k.joined})
		}
	}
	sortBy(res.Tracks, tr)
	if len(tr) > 0 {
		found(TrackGroup, slices.MinFunc(tr, rank.compare))
	}
	return res
}

// sortBy sorts items by their ranks, stably, so equal ranks keep library
// order.
func sortBy[T any](items []T, ranks []rank) {
	idx := make([]int, len(items))
	for i := range idx {
		idx[i] = i
	}
	slices.SortStableFunc(idx, func(a, b int) int { return ranks[a].compare(ranks[b]) })
	sorted := make([]T, len(items))
	for i, j := range idx {
		sorted[i] = items[j]
	}
	copy(items, sorted)
}

// match reports whether q matches a name (with extra words), and how well.
func match(q, name, extra keys) (int, bool) {
	switch {
	case name.joined == q.joined:
		return exact, true
	case strings.HasPrefix(name.joined, q.joined):
		return namePrefix, true
	case allStart(q.words, name.words, nil):
		return nameWords, true
	case allStart(q.words, name.words, extra.words):
		return spread, true
	}
	return 0, false
}

// allStart reports whether every query word starts one of the words.
func allStart(q []string, a, b []string) bool {
	for _, w := range q {
		starts := func(x string) bool { return strings.HasPrefix(x, w) }
		if !slices.ContainsFunc(a, starts) && !slices.ContainsFunc(b, starts) {
			return false
		}
	}
	return true
}

// words folds s (lower case, accents dropped) and splits it into words
// at anything that isn't a letter or digit. Apostrophes join: "don't"
// is "dont".
func words(s string) []string {
	return strings.FieldsFunc(
		fold(s),
		func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) },
	)
}

// letters that don't decompose into a base letter and an accent.
var special = strings.NewReplacer(
	"ß",
	"ss",
	"æ",
	"ae",
	"œ",
	"oe",
	"ø",
	"o",
	"ł",
	"l",
	"đ",
	"d",
	"ð",
	"d",
	"þ",
	"th",
	"'",
	"",
	"’",
	"",
)

func fold(s string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	out, _, err := transform.String(t, strings.ToLower(s))
	if err != nil {
		out = strings.ToLower(s)
	}
	return special.Replace(out)
}
