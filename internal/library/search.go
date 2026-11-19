package library

import (
	"cmp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Group is one kind of search result, in the order results show them.
type Group int

const (
	ArtistGroup Group = iota
	AlbumGroup
	PlaylistGroup
	TrackGroup
)

// Results are what a search found, each group best match first.
type Results struct {
	Artists   []Artist
	Albums    []Album
	Playlists []Playlist
	Tracks    []Track
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
	case PlaylistGroup:
		return len(r.Playlists)
	}
	return len(r.Tracks)
}

// Match tiers, best first.
const (
	exact       = iota // the name is the query
	namePrefix         // the name starts with the query
	nameWords          // every query word starts a word of the name
	spread             // ... or of the name, artist and album together
	fuzzyName          // every query word is a few edits off a word's start in the name
	fuzzySpread        // ... or in the name, artist and album together
)

// rank orders matches: tier, then edits, then play count, then name.
type rank struct {
	tier, edits, plays int
	name               string
}

func (a rank) compare(b rank) int {
	return cmp.Or(
		cmp.Compare(a.tier, b.tier),
		cmp.Compare(a.edits, b.edits),
		cmp.Compare(b.plays, a.plays),
		cmp.Compare(a.name, b.name),
	)
}

// keys is a name prepared for matching: folded words, and them joined.
// ids number the words in the index's vocabulary; playlists, reindexed on
// their own, have none.
type keys struct {
	words  []string
	joined string
	ids    []int32
}

func newKeys(s string) keys {
	w := words(s)
	return keys{words: w, joined: strings.Join(w, " ")}
}

// searchIndex holds every artist, album, playlist and track prepared for
// matching.
// Tracks' extra words are their artist and album; albums' are the album
// artist.
type searchIndex struct {
	artists            []keys
	albums, albumExtra []keys
	tracks, trackExtra []keys
	playlists          []keys
	// vocab is every distinct word of the artists, albums and tracks, so a
	// search works out a word's typos once however often it recurs.
	vocab []string
}

func (s *Snapshot) buildSearch() {
	x := &searchIndex{}
	ids := map[string]int32{}
	newKeys := func(s string) keys {
		k := newKeys(s)
		k.ids = make([]int32, len(k.words))
		for i, w := range k.words {
			id, ok := ids[w]
			if !ok {
				id = int32(len(x.vocab))
				ids[w] = id
				x.vocab = append(x.vocab, w)
			}
			k.ids[i] = id
		}
		return k
	}
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
	x.playlists = playlistKeys(s.Playlists)
	s.search = x
}

// playlistKeys prepares playlists for matching, by name only.
func playlistKeys(ps []Playlist) []keys {
	out := make([]keys, len(ps))
	for i, p := range ps {
		out[i] = newKeys(p.Name)
	}
	return out
}

// reindexPlaylists makes the search see s.Playlists after they changed. It
// gives s its own index, since the copy it came from shares the old one.
func (s *Snapshot) reindexPlaylists() {
	if s.search == nil {
		s.buildSearch()
		return
	}
	x := *s.search
	x.playlists = playlistKeys(s.Playlists)
	s.search = &x
}

// Search finds artists by name, albums by name and album artist,
// playlists by name, and tracks by title, artist and album, ignoring case
// and accents. Every query word must start a word; only when that finds
// nothing may each be a few typing mistakes off one (see [typoBudget]).
// An empty query finds nothing.
func (s *Snapshot) Search(query string) Results {
	q := newKeys(query)
	if len(q.words) == 0 {
		return Results{Best: -1}
	}
	m := newMatcher(q, len(s.search.vocab))
	res := s.searchWith(m.match)
	if res.Best < 0 && m.fuzzy {
		m.typosOK = true
		res = s.searchWith(m.match)
	}
	return res
}

// searchWith finds what match accepts, ranked.
func (s *Snapshot) searchWith(match func(name, extra keys) (how, bool)) Results {
	res := Results{Best: -1}
	x := s.search
	var best rank
	found := func(g Group, r rank) {
		if res.Best < 0 || r.compare(best) < 0 {
			res.Best, best = g, r
		}
	}

	var ar []rank
	for i, k := range x.artists {
		if t, ok := match(k, keys{}); ok {
			res.Artists = append(res.Artists, s.Artists[i])
			ar = append(ar, rank{t.tier, t.edits, 0, k.joined})
		}
	}
	sortBy(res.Artists, ar)
	if len(ar) > 0 {
		found(ArtistGroup, slices.MinFunc(ar, rank.compare))
	}

	var al []rank
	for i, k := range x.albums {
		if t, ok := match(k, x.albumExtra[i]); ok {
			res.Albums = append(res.Albums, s.Albums[i])
			al = append(al, rank{t.tier, t.edits, s.Albums[i].PlayCount, k.joined})
		}
	}
	sortBy(res.Albums, al)
	if len(al) > 0 {
		found(AlbumGroup, slices.MinFunc(al, rank.compare))
	}

	var pl []rank
	for i, k := range x.playlists {
		if t, ok := match(k, keys{}); ok {
			res.Playlists = append(res.Playlists, s.Playlists[i])
			pl = append(pl, rank{t.tier, t.edits, 0, k.joined})
		}
	}
	sortBy(res.Playlists, pl)
	if len(pl) > 0 {
		found(PlaylistGroup, slices.MinFunc(pl, rank.compare))
	}

	var tr []rank
	for i, k := range x.tracks {
		if t, ok := match(k, x.trackExtra[i]); ok {
			res.Tracks = append(res.Tracks, s.Tracks[i])
			tr = append(tr, rank{t.tier, t.edits, s.Tracks[i].PlayCount, k.joined})
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

// how is how well a name matched: its tier, and the typing mistakes
// forgiven.
type how struct{ tier, edits int }

// matcher matches one query against names, remembering each vocabulary
// word's distance from each query word, since the same words recur across
// a library.
type matcher struct {
	q       keys
	fuzzy   bool   // some query word could have typos
	typosOK bool   // and typos are forgiven
	budget  []int  // the edits each query word may be off by
	dist    []int8 // per query word and vocabulary word: typos+1, 0 if not yet known
	nvocab  int
	typos   typoTable
}

func newMatcher(q keys, nvocab int) *matcher {
	m := &matcher{q: q, budget: make([]int, len(q.words)), nvocab: nvocab}
	for i, w := range q.words {
		m.budget[i] = typoBudget(w)
		m.fuzzy = m.fuzzy || m.budget[i] > 0
	}
	if m.fuzzy {
		m.dist = make([]int8, len(q.words)*nvocab)
	}
	return m
}

// match reports whether the query matches a name (with extra words), and
// how well.
func (m *matcher) match(name, extra keys) (how, bool) {
	q := m.q
	switch {
	case name.joined == q.joined:
		return how{exact, 0}, true
	case strings.HasPrefix(name.joined, q.joined):
		return how{namePrefix, 0}, true
	case allStart(q.words, name.words, nil):
		return how{nameWords, 0}, true
	case allStart(q.words, name.words, extra.words):
		return how{spread, 0}, true
	case !m.typosOK:
		return how{}, false
	}
	if e, ok := m.edits(name, keys{}); ok {
		return how{fuzzyName, e}, true
	}
	if e, ok := m.edits(name, extra); ok {
		return how{fuzzySpread, e}, true
	}
	return how{}, false
}

// edits is the fewest typing mistakes that make every query word start
// one of the words, if each word is within its budget.
func (m *matcher) edits(a, b keys) (int, bool) {
	total := 0
	for i := range m.q.words {
		best := m.budget[i] + 1
		for _, k := range [2]keys{a, b} {
			for j, x := range k.words {
				best = min(best, m.distance(i, k, j, x))
			}
		}
		if best > m.budget[i] {
			return 0, false
		}
		total += best
	}
	return total, true
}

// distance is query word i's typos from x, word j of k.
func (m *matcher) distance(i int, k keys, j int, x string) int {
	if k.ids == nil {
		return m.typos.distance(m.q.words[i], x, m.budget[i])
	}
	at := i*m.nvocab + int(k.ids[j])
	if d := m.dist[at]; d > 0 {
		return int(d) - 1
	}
	d := m.typos.distance(m.q.words[i], x, m.budget[i])
	m.dist[at] = int8(d + 1)
	return d
}

// typoBudget is how many typing mistakes a query word may have: none up
// to three letters, two from four on.
func typoBudget(w string) int {
	if utf8.RuneCountInString(w) <= 3 {
		return 0
	}
	return 2
}

// typoTable works out typos, reusing its buffers from call to call.
type typoTable struct {
	a, b          []rune
	pp, prev, cur []int
}

// distance is the fewest typos that turn w into the start of x: a letter
// added, dropped or changed, or two neighbours swapped, each counts one
// (optimal string alignment distance against x's prefixes). Anything over
// limit comes back as limit+1.
func (t *typoTable) distance(w, x string, limit int) int {
	t.a, t.b = append(t.a[:0], []rune(w)...), append(t.b[:0], []rune(x)...)
	a, b := t.a, t.b
	if len(b) > len(a)+limit {
		b = b[:len(a)+limit] // a longer prefix needs more than limit drops
	}
	// rows i-2, i-1 and i of the table of distances between a[:i] and b[:j]
	n := len(b) + 1
	if cap(t.cur) < n {
		t.pp, t.prev, t.cur = make([]int, n), make([]int, n), make([]int, n)
	}
	pp, prev, cur := t.pp[:n], t.prev[:n], t.cur[:n]
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		low := cur[0]
		for j := 1; j <= len(b); j++ {
			sub := 1
			if a[i-1] == b[j-1] {
				sub = 0
			}
			d := min(prev[j]+1, cur[j-1]+1, prev[j-1]+sub)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				d = min(d, pp[j-2]+1)
			}
			cur[j] = d
			low = min(low, d)
		}
		if low > limit {
			return limit + 1
		}
		pp, prev, cur = prev, cur, pp
	}
	return min(slices.Min(prev), limit+1) // w against the best prefix of x
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
