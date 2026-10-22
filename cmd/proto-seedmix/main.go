// PROTOTYPE, throwaway: lives only on branch prototype/seeded-mix, for
// "Does cache-only closeness feel like a radio on a real library?" (#68).
//
// It reads your real library cache, builds a Mix around a seed with a
// rough cache-only picker, and prints it next to what the server's
// getSimilarSongs2 returns for the same seed.
//
//	go run ./cmd/proto-seedmix artist "radiohead"
//	go run ./cmd/proto-seedmix album "ok computer"
//	go run ./cmd/proto-seedmix playlist "road trip"
//	go run ./cmd/proto-seedmix track "paranoid android"
//	go run ./cmd/proto-seedmix random      # one random seed of each kind
//
// Flags (before the kind): -n 25, -offline, -rand 1, -power 2, -seedcap 0
// (0 = a third of n), and the signal weights -w-seed, -w-artist,
// -w-linked, -w-playlist, -w-genre, -w-year.
package main

import (
	"cmp"
	"context"
	"flag"
	"fmt"
	"math/rand/v2"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/anopheles/navigo/internal/config"
	"github.com/anopheles/navigo/internal/creds"
	"github.com/anopheles/navigo/internal/library"
	"github.com/anopheles/navigo/internal/subsonic"
)

func main() {
	n := flag.Int("n", 25, "tracks per mix")
	offline := flag.Bool("offline", false, "skip the server")
	seedRand := flag.Uint64("rand", uint64(time.Now().UnixNano()), "random seed, to repeat a run")
	w := library.ProtoWeights{}
	flag.Float64Var(&w.Seed, "w-seed", 3, "weight: in the seed itself")
	flag.Float64Var(&w.Artist, "w-artist", 2, "weight: credits a seed artist (× √share)")
	flag.Float64Var(&w.Linked, "w-linked", 1.5, "weight: artist shares a track or small album with a seed artist")
	flag.Float64Var(&w.Playlist, "w-playlist", 2, "weight: shares playlists with seed tracks (up to 3)")
	flag.Float64Var(&w.Genre, "w-genre", 1, "weight: genre (× share of the seed with it)")
	flag.Float64Var(&w.Similar, "w-similar", 2.5, "weight: hybrid only, a server similar artist (× rank)")
	flag.Float64Var(&w.Year, "w-year", 0.5, "weight: close in year (only on top of another signal)")
	flag.Float64Var(&w.Power, "power", 2, "pick weight = closeness^power × Mix weight")
	flag.IntVar(&w.SeedCap, "seedcap", 0, "most tracks from the seed itself (0 = n/3)")
	flag.IntVar(&w.PerArtist, "per-artist", 3, "most tracks per other artist")
	flag.IntVar(&w.PerAlbum, "per-album", 2, "most tracks per other album")
	flag.IntVar(&w.MaxPlaylist, "max-playlist", 300, "playlists bigger than this don't count")
	flag.Parse()
	if w.SeedCap == 0 {
		w.SeedCap = max(1, *n/3)
	}
	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: proto-seedmix [flags] artist|album|playlist|track|random [name or id]")
		os.Exit(2)
	}
	r := rand.New(rand.NewPCG(*seedRand, 1))

	path, err := config.Path()
	must(err)
	cfg, err := config.Load(path)
	must(err)
	dir, err := os.UserCacheDir()
	must(err)
	snap, err := library.Load(filepath.Join(dir, "navigo"), cfg.Server.URL, cfg.Server.User)
	must(err)
	fmt.Printf("library: %d artists, %d albums, %d tracks, %d playlists (synced %s) · rand %d\n",
		len(snap.Artists), len(snap.Albums), len(snap.Tracks), len(snap.Playlists),
		snap.SyncedAt.Format("2006-01-02 15:04"), *seedRand)
	coverage(snap)

	var client *subsonic.Client
	if !*offline {
		ctx := context.Background()
		pw, err := creds.Password(ctx, cfg.Server, creds.OS)
		must(err)
		client = subsonic.New(subsonic.Credentials{URL: cfg.Server.URL, Username: cfg.Server.User, Password: pw})
		if err := client.Login(ctx); err != nil {
			fmt.Println("server: can't log in, going offline:", err)
			client = nil
		}
	}

	kind, q := flag.Arg(0), strings.Join(flag.Args()[1:], " ")
	kinds := []string{kind}
	if kind == "random" {
		kinds, q = []string{"artist", "album", "playlist", "track"}, ""
	}
	for _, k := range kinds {
		run(snap, client, k, q, *n, w, r)
	}
}

func run(snap *library.Snapshot, client *subsonic.Client, kind, q string, n int, w library.ProtoWeights, r *rand.Rand) {
	seed, others, err := snap.ProtoResolve(kind, q, r)
	if err != nil {
		fmt.Println("\n" + err.Error())
		return
	}
	rule("SEED " + strings.ToUpper(kind) + ": " + seed.Name)
	if len(others) > 0 {
		fmt.Printf("  (also matched: %s)\n", strings.Join(others, " · "))
	}
	fmt.Printf("  %d tracks · median year %d · genres %s · artists %s\n",
		len(seed.Tracks), seed.Year, shares(seed.Genres, nil), shares(artistShares(seed), snap))

	now := time.Now()
	scored := snap.ProtoScore(seed, w, now)
	sig := map[string]int{}
	for _, p := range scored {
		for _, why := range p.Why {
			sig[signal(why)]++
		}
	}
	fmt.Printf("  candidates: %d of %d tracks · by signal: %s\n", len(scored), len(snap.Tracks), counts(sig))

	picks := snap.ProtoMix(scored, n, w, r)
	fmt.Printf("\n  CACHE PICKER: %d tracks (seed cap %d)\n", len(picks), w.SeedCap)
	cacheIDs := map[library.TrackID]bool{}
	for i, p := range picks {
		cacheIDs[p.Track.ID] = true
		row(i+1, p.Track, fmt.Sprintf("c=%.1f", p.Closeness), p.Why)
	}
	summary(picks)

	if client == nil {
		return
	}
	ctx := context.Background()
	start := time.Now()
	songs, endpoint, err := similar(ctx, client, seed.ID, n)
	took := time.Since(start)
	fmt.Printf("\n  SERVER %s: %d tracks in %s\n", endpoint, len(songs), took.Round(time.Millisecond))
	if err != nil {
		fmt.Println("   ", err)
	}
	var server []library.ProtoPick
	overlap := 0
	for i, s := range songs {
		p, ok := scored[library.TrackID(s.ID)]
		t, inCache := snap.Track(library.TrackID(s.ID))
		if !inCache {
			t = library.TrackFrom(s)
		}
		why := p.Why
		if !ok {
			why = []string{"(no cache signal)"}
		}
		if cacheIDs[t.ID] {
			overlap++
		}
		row(i+1, t, fmt.Sprintf("c=%.1f", p.Closeness), why)
		server = append(server, library.ProtoPick{Track: t, Why: p.Why})
	}
	summary(server)
	fmt.Printf("  overlap with the cache picker: %d tracks\n", overlap)

	if a := mainArtist(seed); a != "" {
		seed.Similar = info(ctx, client, snap, a)
	}
	if len(seed.Similar) == 0 {
		return
	}
	hy := snap.ProtoScore(seed, w, now)
	hp := snap.ProtoMix(hy, n, w, r)
	fmt.Printf("\n  HYBRID PICKER (cache + server similar artists): %d tracks\n", len(hp))
	for i, p := range hp {
		row(i+1, p.Track, fmt.Sprintf("c=%.1f", p.Closeness), p.Why)
	}
	summary(hp)
}

// similar asks getSimilarSongs2, then getSimilarSongs if that fails.
func similar(ctx context.Context, c *subsonic.Client, id string, n int) ([]subsonic.Song, string, error) {
	p := url.Values{"id": {id}, "count": {strconv.Itoa(n)}}
	var r2 struct {
		S struct {
			Song []subsonic.Song `json:"song"`
		} `json:"similarSongs2"`
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if err := c.PrototypeCall(ctx, "getSimilarSongs2", p, &r2); err == nil {
		return r2.S.Song, "getSimilarSongs2", nil
	}
	var r1 struct {
		S struct {
			Song []subsonic.Song `json:"song"`
		} `json:"similarSongs"`
	}
	err := c.PrototypeCall(ctx, "getSimilarSongs", p, &r1)
	return r1.S.Song, "getSimilarSongs", err
}

// info prints the server's similar artists for a, marking the ones the
// library has, which hints at whether an agent is set up.
func info(ctx context.Context, c *subsonic.Client, snap *library.Snapshot, a library.ArtistID) map[string]float64 {
	var r struct {
		I struct {
			Similar []subsonic.Artist `json:"similarArtist"`
		} `json:"artistInfo2"`
	}
	start := time.Now()
	err := c.PrototypeCall(ctx, "getArtistInfo2",
		url.Values{"id": {string(a)}, "count": {"20"}, "includeNotPresent": {"true"}}, &r)
	name := string(a)
	if ar, ok := snap.Artist(a); ok {
		name = ar.Name
	}
	fmt.Printf("\n  SERVER similar artists of %s (%s):", name, time.Since(start).Round(time.Millisecond))
	if err != nil {
		fmt.Println(" ", err)
		return nil
	}
	if len(r.I.Similar) == 0 {
		fmt.Println(" none (likely no agent answering)")
		return nil
	}
	sim := map[string]float64{}
	var names []string
	for _, s := range r.I.Similar {
		mark := ""
		if s.ID == "-1" || s.ID == "" {
			mark = " (not in library)"
		}
		names = append(names, s.Name+mark)
		k := strings.ToLower(s.Name)
		if _, ok := sim[k]; !ok {
			sim[k] = 1 - 0.5*float64(len(sim))/float64(len(r.I.Similar))
		}
	}
	fmt.Println("\n    " + strings.Join(names, " · "))
	return sim
}

func mainArtist(seed *library.ProtoSeed) library.ArtistID {
	if seed.Kind == "artist" {
		return library.ArtistID(seed.ID)
	}
	var best library.ArtistID
	for a, s := range seed.Artists {
		if s > seed.Artists[best] || (s == seed.Artists[best] && a < best) {
			best = a
		}
	}
	return best
}

func row(i int, t library.Track, score string, why []string) {
	line := fmt.Sprintf("  %3d. %s — %s", i, t.Artist, t.Title)
	meta := fmt.Sprintf("(%s, %d, %s)", t.Album, t.Year, cmp.Or(t.Genre, "no genre"))
	fmt.Printf("%-70s %-48s %-6s %s\n", trunc(line, 70), trunc(meta, 48), score, strings.Join(why, " "))
}

func summary(ps []library.ProtoPick) {
	artists, genres, sig := map[string]int{}, map[string]int{}, map[string]int{}
	fromSeed := 0
	for _, p := range ps {
		artists[p.Track.Artist]++
		genres[cmp.Or(p.Track.Genre, "no genre")]++
		if p.FromSeed || slices.Contains(p.Why, "seed") {
			fromSeed++
		}
		for _, w := range p.Why {
			sig[signal(w)]++
		}
	}
	fmt.Printf("  → %d artists, %d genres, %d from the seed · top artists %s · top genres %s · signals %s\n",
		len(artists), len(genres), fromSeed, top(artists, 4), top(genres, 4), counts(sig))
}

func coverage(s *library.Snapshot) {
	genre, year, rated, played := 0, 0, 0, 0
	gs := map[string]int{}
	for _, t := range s.Tracks {
		if t.Genre != "" {
			genre++
			gs[t.Genre]++
		}
		if t.Year > 0 {
			year++
		}
		if t.Rating > 0 {
			rated++
		}
		if !t.Played.IsZero() {
			played++
		}
	}
	pct := func(k int) string { return fmt.Sprintf("%d%%", 100*k/max(1, len(s.Tracks))) }
	fmt.Printf("tracks with genre %s (%d distinct; top %s) · year %s · rated %s · played %s · starred %d\n",
		pct(genre), len(gs), top(gs, 5), pct(year), pct(rated), pct(played), len(s.Starred))
}

func artistShares(seed *library.ProtoSeed) map[string]float64 {
	out := map[string]float64{}
	for a, v := range seed.Artists {
		out[string(a)] = v
	}
	return out
}

func shares(m map[string]float64, snap *library.Snapshot) string {
	type kv struct {
		k string
		v float64
	}
	var l []kv
	for k, v := range m {
		if snap != nil {
			if a, ok := snap.Artist(library.ArtistID(k)); ok {
				k = a.Name
			}
		}
		l = append(l, kv{k, v})
	}
	slices.SortFunc(l, func(a, b kv) int { return cmp.Or(cmp.Compare(b.v, a.v), cmp.Compare(a.k, b.k)) })
	var parts []string
	for _, e := range l[:min(len(l), 5)] {
		parts = append(parts, fmt.Sprintf("%s %.0f%%", e.k, e.v*100))
	}
	if len(l) > 5 {
		parts = append(parts, fmt.Sprintf("+%d more", len(l)-5))
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}

func top(m map[string]int, k int) string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(a, b string) int { return cmp.Or(cmp.Compare(m[b], m[a]), cmp.Compare(a, b)) })
	var parts []string
	for _, key := range keys[:min(len(keys), k)] {
		parts = append(parts, fmt.Sprintf("%s ×%d", key, m[key]))
	}
	return strings.Join(parts, ", ")
}

func counts(m map[string]int) string {
	var parts []string
	for _, k := range []string{"seed", "artist", "similar", "linked", "playlist", "genre", "year", "(no cache signal)"} {
		if m[k] > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", k, m[k]))
		}
	}
	return strings.Join(parts, ", ")
}

func signal(why string) string {
	for _, k := range []string{"seed", "artist", "similar", "linked", "playlist", "genre", "year"} {
		if strings.HasPrefix(why, k) {
			return k
		}
	}
	return why
}

func rule(s string) { fmt.Printf("\n%s\n%s\n", s, strings.Repeat("─", min(len(s), 100))) }

func trunc(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "proto-seedmix:", err)
		os.Exit(1)
	}
}
