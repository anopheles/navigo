// Package resume keeps the saved queue: the queue and where playback
// stood, in a small file per server and user in the state directory, so
// the next start can bring it back. The file is disposable: any trouble
// reading it means starting with an empty queue.
package resume

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/anopheles/navigo/internal/library"
)

// version changes whenever the stored shape does; a file with another
// version is thrown away.
const version = 1

// ErrNone means there is no usable saved queue: none yet, another version,
// or a damaged file.
var ErrNone = errors.New("no usable saved queue")

// Saved is what is kept. Current indexes Tracks, or is -1; Position is
// where Current stood. Volume is in percent and Repeat is the core's
// repeat mode as a number.
type Saved struct {
	Tracks   []library.TrackID
	Current  int
	Position time.Duration
	Volume   int
	Repeat   int
}

type file struct {
	Version int
	Server  string
	User    string
	Saved   Saved
}

// Path is the file for a server and user in dir.
func Path(dir, server, user string) string {
	sum := sha256.Sum256([]byte(user + "@" + server))
	return filepath.Join(dir, "queue-"+hex.EncodeToString(sum[:8])+".gob")
}

// Save writes s atomically, with mode 0600: readers see the old file or
// the new one, never half of one.
func Save(dir, server, user string, s Saved) error {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).
		Encode(file{Version: version, Server: server, User: user, Saved: s}); err != nil {
		return fmt.Errorf("saved queue: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".queue-*.tmp") // 0600
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after the rename
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), Path(dir, server, user))
}

// Load reads the saved queue for a server and user. Any problem with the
// file is ErrNone; a damaged or outdated file is removed.
func Load(dir, server, user string) (Saved, error) {
	path := Path(dir, server, user)
	b, err := os.ReadFile(path)
	if err != nil {
		return Saved{}, ErrNone
	}
	var f file
	if err := gob.NewDecoder(bytes.NewReader(b)).Decode(&f); err != nil ||
		f.Version != version || f.Server != server || f.User != user {
		_ = os.Remove(path)
		return Saved{}, ErrNone
	}
	return f.Saved, nil
}

// Delete removes the saved queue for a server and user, if there is one.
func Delete(dir, server, user string) error {
	err := os.Remove(Path(dir, server, user))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
