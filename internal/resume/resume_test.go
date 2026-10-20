package resume

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/anopheles/navigo/internal/library"
)

func TestRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	want := Saved{
		Tracks:   []library.TrackID{"a", "b", "c"},
		Current:  1,
		Position: 134 * time.Second,
		Volume:   40,
		Repeat:   2,
	}
	if err := Save(dir, "https://m.example", "ann", want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir, "https://m.example", "ann")
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("Load = %+v, %v", got, err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(Path(dir, "https://m.example", "ann"))
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Errorf("mode = %v, %v", info.Mode().Perm(), err)
		}
	}
}

func TestEachServerAndUserHasOne(t *testing.T) {
	dir := t.TempDir()
	if err := Save(
		dir,
		"https://m.example",
		"ann",
		Saved{Tracks: []library.TrackID{"a"}},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir, "https://m.example", "bob"); !errors.Is(err, ErrNone) {
		t.Errorf("another user got %v", err)
	}
	if _, err := Load(dir, "https://other.example", "ann"); !errors.Is(err, ErrNone) {
		t.Errorf("another server got %v", err)
	}
}

func TestDamagedOrOutdatedFileIsDropped(t *testing.T) {
	dir := t.TempDir()
	path := Path(dir, "s", "u")
	if err := os.WriteFile(path, []byte("not a gob"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir, "s", "u"); !errors.Is(err, ErrNone) {
		t.Errorf("Load = %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the damaged file stayed: %v", err)
	}
}

func TestDelete(t *testing.T) {
	dir := t.TempDir()
	if err := Delete(dir, "s", "u"); err != nil { // none yet
		t.Fatal(err)
	}
	if err := Save(dir, "s", "u", Saved{}); err != nil {
		t.Fatal(err)
	}
	if err := Delete(dir, "s", "u"); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir, "s", "u"); !errors.Is(err, ErrNone) {
		t.Errorf("Load after Delete = %v", err)
	}
}
