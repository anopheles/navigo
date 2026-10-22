// Command genlibrary writes the synthetic fixture library as tagged, silent
// MP3 files (see mp3.go). The files are never committed. With -covers each
// album also gets a generated cover, embedded in its files, for the demo;
// the test fixtures have none.
//
//	go run ./tools/genlibrary -out ~/.cache/navigo-fixtures/music
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/anopheles/navigo/tools/internal/fixturelib"
)

// marker marks a directory genlibrary wrote, so it may be wiped and rewritten.
const marker = ".navigo-fixtures"

func main() {
	out := flag.String(
		"out",
		"",
		"directory to write the library into (must be empty or written by genlibrary)",
	)
	covers := flag.Bool("covers", false, "embed a generated cover in every album's files")
	flag.Parse()
	if *out == "" {
		log.Fatal("genlibrary: -out is required")
	}
	if err := run(*out, *covers); err != nil {
		log.Fatalf("genlibrary: %v", err)
	}
}

func run(out string, covers bool) error {
	if err := prepare(out); err != nil {
		return err
	}

	coverFiles := map[string][]byte{} // album artist and title to the cover PNG
	if covers {
		dir := filepath.Join(out, marker, "covers")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		for i, a := range fixturelib.Albums {
			p := filepath.Join(dir, strconv.Itoa(i)+".png")
			if err := writeCover(p, a); err != nil {
				return err
			}
			png, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			coverFiles[a.AlbumArtist+"\x00"+a.Title] = png
		}
	}

	type job struct {
		a fixturelib.Album
		t fixturelib.Track
	}
	jobs := make(chan job)
	errs := make(chan error, 1)
	var wg sync.WaitGroup
	for range runtime.NumCPU() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				if err := writeTrack(
					out,
					coverFiles[j.a.AlbumArtist+"\x00"+j.a.Title],
					j.a,
					j.t,
				); err != nil {
					select {
					case errs <- err:
					default:
					}
				}
			}
		}()
	}
	for _, a := range fixturelib.Albums {
		for _, t := range a.Tracks {
			jobs <- job{a, t}
		}
	}
	close(jobs)
	wg.Wait()
	select {
	case err := <-errs:
		return err
	default:
	}
	fmt.Printf(
		"wrote %d tracks in %d albums to %s\n",
		fixturelib.TrackCount(),
		len(fixturelib.Albums),
		out,
	)
	return nil
}

// prepare empties out if genlibrary wrote it before, and refuses any other
// non-empty directory.
func prepare(out string) error {
	entries, err := os.ReadDir(out)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return err
	case len(entries) > 0:
		if _, err := os.Stat(filepath.Join(out, marker)); err != nil {
			return fmt.Errorf("%s is not empty and was not written by genlibrary", out)
		}
		if err := os.RemoveAll(out); err != nil {
			return err
		}
	}
	return os.MkdirAll(filepath.Join(out, marker), 0o755)
}

func writeTrack(out string, cover []byte, a fixturelib.Album, t fixturelib.Track) error {
	dir := filepath.Join(out, clean(a.AlbumArtist), clean(a.Title))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	artist := t.Artist
	if artist == "" {
		artist = a.AlbumArtist
	}
	perDisc := 0
	for _, o := range a.Tracks {
		if o.Disc == t.Disc {
			perDisc++
		}
	}
	name := fmt.Sprintf("%d-%03d %s.mp3", t.Disc, t.Number, clean(t.Title))
	return writeMP3(filepath.Join(dir, name), tags{
		title:       t.Title,
		artist:      artist,
		album:       a.Title,
		albumArtist: a.AlbumArtist,
		track:       fmt.Sprintf("%d/%d", t.Number, perDisc),
		disc:        fmt.Sprintf("%d/%d", t.Disc, a.Discs),
		year:        a.Year,
		genre:       a.Genre,
		compilation: a.Compilation,
		cover:       cover,
	}, t.Seconds)
}

// clean makes a name safe as a file name on Linux, macOS and Windows.
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) {
			return '_'
		}
		return r
	}, s)
}
