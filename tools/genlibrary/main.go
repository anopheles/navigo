// Command genlibrary writes the synthetic fixture library as tagged, silent
// MP3 files, using ffmpeg. The files are never committed. With -covers each
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
	"os/exec"
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
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return errors.New("ffmpeg not found on PATH")
	}
	if err := prepare(out); err != nil {
		return err
	}

	silence := filepath.Join(out, marker, "silence")
	if err := os.MkdirAll(silence, 0o755); err != nil {
		return err
	}
	bases := map[int]string{}
	for _, a := range fixturelib.Albums {
		for _, t := range a.Tracks {
			if _, ok := bases[t.Seconds]; ok {
				continue
			}
			p := filepath.Join(silence, strconv.Itoa(t.Seconds)+".mp3")
			if err := ffmpeg(
				"-f",
				"lavfi",
				"-i",
				"anullsrc=r=22050:cl=mono",
				"-t",
				strconv.Itoa(t.Seconds),
				"-c:a",
				"libmp3lame",
				"-b:a",
				"16k",
				p,
			); err != nil {
				return err
			}
			bases[t.Seconds] = p
		}
	}

	coverFiles := map[string]string{} // album artist and title to the cover's path
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
			coverFiles[a.AlbumArtist+"\x00"+a.Title] = p
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
					bases[j.t.Seconds],
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

func writeTrack(out, base, cover string, a fixturelib.Album, t fixturelib.Track) error {
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
	args := []string{"-i", base}
	if cover != "" {
		args = append(args, "-i", cover, "-map", "0:a", "-map", "1:v",
			"-metadata:s:v", "title=Album cover", "-metadata:s:v", "comment=Cover (front)")
	}
	args = append(args, "-map_metadata", "-1", "-c", "copy", "-id3v2_version", "3",
		"-metadata", "title="+t.Title,
		"-metadata", "artist="+artist,
		"-metadata", "album="+a.Title,
		"-metadata", "album_artist="+a.AlbumArtist,
		"-metadata", fmt.Sprintf("track=%d/%d", t.Number, perDisc),
		"-metadata", fmt.Sprintf("disc=%d/%d", t.Disc, a.Discs),
		"-metadata", "date="+strconv.Itoa(a.Year),
		"-metadata", "genre="+a.Genre,
	)
	if a.Compilation {
		args = append(args, "-metadata", "compilation=1")
	}
	name := fmt.Sprintf("%d-%03d %s.mp3", t.Disc, t.Number, clean(t.Title))
	return ffmpeg(append(args, filepath.Join(dir, name))...)
}

func ffmpeg(args ...string) error {
	cmd := exec.Command(
		"ffmpeg",
		append([]string{"-nostdin", "-loglevel", "error", "-y"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg %s: %w\n%s", strings.Join(args, " "), err, out)
	}
	return nil
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
