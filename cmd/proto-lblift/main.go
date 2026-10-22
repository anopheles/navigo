// PROTOTYPE for "How strong is the lift on a real library?" (#79). Throwaway:
// lives on prototype/listenbrainz-lift and is never merged.
//
//	go run ./cmd/proto-lblift -server https://… -user david -lb anopheles
//
// It copies navigo's cache file to a temp dir (Load deletes a file that
// doesn't match), fetches the ListenBrainz Recommendations (1 + 10 calls),
// matches them by folded artist + title, and prints whole-library Mixes at
// several lift strengths side by side, with statistics over many Mixes.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/anopheles/navigo/internal/library"
)

type variant struct {
	name string
	lift func(rec bool, score float64) float64
}

func main() {
	server := flag.String(
		"server",
		"https://navidrome.anopheles.de",
		"navigo's server URL, as in the config",
	)
	user := flag.String("user", "david", "navigo's user")
	lb := flag.String("lb", "anopheles", "ListenBrainz user name")
	mixes := flag.Int("mixes", 200, "Mixes per variant for the statistics")
	size := flag.Int("n", 50, "tracks per Mix")
	seed := flag.Uint64("seed", 1, "seed of the sample Mix shown")
	flag.Parse()

	lib := load(*server, *user)
	recs := fetch(*lb)
	score := match(lib, recs) // track ID → best score among its Recommendations
	maxScore := 0.0
	for _, sc := range score {
		maxScore = max(maxScore, sc)
	}
	never := 0
	for _, t := range lib.Tracks {
		if _, ok := score[t.ID]; ok && t.Played.IsZero() {
			never++
		}
	}
	fmt.Printf(
		"library %d tracks · %d Recommendations · %d library tracks lifted (%.1f%%), %d never played\n\n",
		len(
			lib.Tracks,
		),
		len(recs),
		len(score),
		100*float64(len(score))/float64(len(lib.Tracks)),
		never,
	)

	fixed := func(f float64) func(bool, float64) float64 {
		return func(rec bool, _ float64) float64 {
			if rec {
				return f
			}
			return 1
		}
	}
	vs := []variant{
		{
			"none",
			fixed(1),
		},
		{"×1.5", fixed(1.5)},
		{"×2", fixed(2)},
		{"×3", fixed(3)},
		{"×5", fixed(5)},
		{"×10", fixed(10)},
		{"×(1+4·score)", func(rec bool, s float64) float64 {
			if rec {
				return 1 + 4*s/maxScore
			}
			return 1
		}},
	}
	now := time.Now()
	run := func(v variant, sd uint64) []library.Track {
		return lib.ProtoMixLifted(
			*size,
			now,
			rand.New(rand.NewPCG(sd, sd^0x9e3779b97f4a7c15)),
			func(t library.Track) float64 {
				sc, ok := score[t.ID]
				return v.lift(ok, sc)
			},
		)
	}

	fmt.Printf(
		"%-14s %10s %12s %14s %10s\n",
		"variant",
		"recs/Mix",
		"never/Mix",
		"Mixes w/ none",
		"artists",
	)
	for _, v := range vs {
		var recN, neverN, zero, artists int
		for sd := range uint64(*mixes) {
			got := run(v, sd+1000)
			k := 0
			seen := map[string]bool{}
			for _, t := range got {
				seen[strings.ToLower(t.Artist)] = true
				if _, ok := score[t.ID]; ok {
					k++
					if t.Played.IsZero() {
						neverN++
					}
				}
			}
			recN += k
			artists += len(seen)
			if k == 0 {
				zero++
			}
		}
		m := float64(*mixes)
		fmt.Printf(
			"%-14s %10.1f %12.1f %13.0f%% %10.1f\n",
			v.name,
			float64(recN)/m,
			float64(neverN)/m,
			100*float64(zero)/m,
			float64(artists)/m,
		)
	}

	show := []variant{vs[0], vs[2], vs[3], vs[5]}
	fmt.Printf(
		"\nsample Mix (seed %d), first 25 tracks · R = a Recommendation, R* = one never played\n\n",
		*seed,
	)
	cols := make([][]library.Track, len(show))
	for i, v := range show {
		cols[i] = run(v, *seed)
		fmt.Printf("%-37s", v.name)
	}
	fmt.Println()
	for row := range 25 {
		for _, c := range cols {
			cell := ""
			if row < len(c) {
				t := c[row]
				mark := "  "
				if _, ok := score[t.ID]; ok {
					mark = "R "
					if t.Played.IsZero() {
						mark = "R*"
					}
				}
				cell = mark + " " + cut(t.Artist, 14) + " – " + cut(t.Title, 15)
			}
			fmt.Printf("%-37s", cell)
		}
		fmt.Println()
	}
}

func cut(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

func load(server, user string) *library.Snapshot {
	home, _ := os.UserCacheDir()
	src := library.CachePath(filepath.Join(home, "navigo"), server, user)
	b, err := os.ReadFile(src)
	if err != nil {
		fail(err)
	}
	dir, _ := os.MkdirTemp("", "proto-lblift-")
	defer os.RemoveAll(dir)
	_ = os.WriteFile(filepath.Join(dir, filepath.Base(src)), b, 0o600)
	s, err := library.Load(dir, server, user)
	if err != nil {
		fail(err)
	}
	return s
}

type rec struct {
	MBID   string  `json:"recording_mbid"`
	Score  float64 `json:"score"`
	Title  string
	Artist []string
}

func fetch(user string) []rec {
	var r struct {
		Payload struct{ Mbids []rec } `json:"payload"`
	}
	get("https://api.listenbrainz.org/1/cf/recommendation/user/"+user+"/recording?count=1000", &r)
	recs := r.Payload.Mbids
	for i := 0; i < len(recs); i += 100 {
		var ids []string
		for _, x := range recs[i:min(i+100, len(recs))] {
			ids = append(ids, x.MBID)
		}
		var meta map[string]struct {
			Recording struct{ Name string }
			Artist    struct {
				Name    string
				Artists []struct{ Name string }
			}
		}
		get(
			"https://api.listenbrainz.org/1/metadata/recording/?inc=artist&recording_mbids="+strings.Join(
				ids,
				",",
			),
			&meta,
		)
		for j := i; j < min(i+100, len(recs)); j++ {
			m := meta[recs[j].MBID]
			recs[j].Title = m.Recording.Name
			recs[j].Artist = append(recs[j].Artist, m.Artist.Name)
			for _, a := range m.Artist.Artists {
				recs[j].Artist = append(recs[j].Artist, a.Name)
			}
		}
		time.Sleep(400 * time.Millisecond) // well under 30 requests per 10 s
	}
	return recs
}

func get(url string, v any) {
	resp, err := http.Get(url)
	if err != nil {
		fail(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		fail(fmt.Errorf("%s: %s", url, resp.Status))
	}
	if err := json.Unmarshal(b, v); err != nil {
		fail(err)
	}
}

var (
	nonWord  = regexp.MustCompile(`[^\pL\pN]+`)
	brackets = regexp.MustCompile(`\s*[\(\[][^\)\]]*[\)\]]`)
	dash     = regexp.MustCompile(`\s+-\s+.*$`)
)

func fold(s string) string {
	s = norm.NFKD.String(strings.ToLower(s))
	s = strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Mn, r) {
			return -1
		}
		return r
	}, s)
	s = strings.ReplaceAll(s, "&", " and ")
	return strings.TrimSpace(nonWord.ReplaceAllString(s, " "))
}

func loose(
	t string,
) string {
	return fold(dash.ReplaceAllString(brackets.ReplaceAllString(t, ""), ""))
}

// match finds each Recommendation's library tracks by folded artist
// (display or credited) plus title, exact or with suffixes stripped.
func match(lib *library.Snapshot, recs []rec) map[library.TrackID]float64 {
	idx := map[string][]library.TrackID{}
	for _, t := range lib.Tracks {
		arts := map[string]bool{fold(t.Artist): true}
		for _, a := range t.Artists {
			arts[fold(a.Name)] = true
		}
		for a := range arts {
			idx[a+"\x00"+fold(t.Title)] = append(idx[a+"\x00"+fold(t.Title)], t.ID)
			if l := loose(t.Title); l != fold(t.Title) {
				idx[a+"\x00~"+l] = append(idx[a+"\x00~"+l], t.ID)
			}
		}
	}
	out := map[library.TrackID]float64{}
	for _, r := range recs {
		var ids []library.TrackID
		for _, a := range r.Artist {
			ids = append(ids, idx[fold(a)+"\x00"+fold(r.Title)]...)
			ids = append(ids, idx[fold(a)+"\x00~"+loose(r.Title)]...)
			ids = append(ids, idx[fold(a)+"\x00"+loose(r.Title)]...)
		}
		for _, id := range ids {
			out[id] = max(out[id], r.Score)
		}
	}
	return out
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "proto-lblift:", err)
	os.Exit(1)
}
