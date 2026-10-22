package library

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Recommendation is a track ListenBrainz suggests for the user, named as
// ListenBrainz names it: Artists is the artist credit, then each artist.
type Recommendation struct {
	MBID    string
	Title   string
	Artists []string
}

// Recommended finds the library tracks the Recommendations name, and how
// many Recommendations matched at least one. A track matches when one of
// its artists (display or credited) and its title equal a
// Recommendation's, both folded and with bracketed parts and a trailing
// " - …" stripped. Every copy of a song matches.
func (s *Snapshot) Recommended(recs []Recommendation) (map[TrackID]bool, int) {
	out := map[TrackID]bool{}
	if len(recs) == 0 {
		return out, 0
	}
	index := map[string][]TrackID{}
	for _, t := range s.Tracks {
		title := titleKey(t.Title)
		artists := map[string]bool{matchFold(t.Artist): true}
		for _, a := range t.Artists {
			artists[matchFold(a.Name)] = true
		}
		for a := range artists {
			k := a + "\x00" + title
			index[k] = append(index[k], t.ID)
		}
	}
	matched := 0
	for _, r := range recs {
		title, hit := titleKey(r.Title), false
		for _, a := range r.Artists {
			for _, id := range index[matchFold(a)+"\x00"+title] {
				out[id], hit = true, true
			}
		}
		if hit {
			matched++
		}
	}
	return out, matched
}

var (
	nonWord  = regexp.MustCompile(`[^\pL\pN]+`)
	brackets = regexp.MustCompile(`\s*[\(\[][^\)\]]*[\)\]]`)
	dash     = regexp.MustCompile(`\s+-\s+.*$`)
)

// titleKey is a title folded with bracketed parts and a trailing " - …"
// stripped, so "Heroes (2017 Remaster)" and "Heroes" meet. A title that is
// nothing but those parts keeps them.
func titleKey(title string) string {
	if k := matchFold(dash.ReplaceAllString(brackets.ReplaceAllString(title, ""), "")); k != "" {
		return k
	}
	return matchFold(title)
}

// matchFold lowercases, drops accents, reads & as "and", and makes every
// run of other characters one space.
func matchFold(s string) string {
	t := transform.Chain(norm.NFKD, runes.Remove(runes.In(unicode.Mn)))
	out, _, err := transform.String(t, strings.ToLower(s))
	if err != nil {
		out = strings.ToLower(s)
	}
	out = strings.ReplaceAll(out, "&", " and ")
	return strings.TrimSpace(nonWord.ReplaceAllString(out, " "))
}
