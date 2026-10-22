package listenbrainz

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
// version is thrown away and the list fetched again.
const version = 1

// ErrNone means there is no usable stored list: none yet, another version,
// or a damaged file.
var ErrNone = errors.New("no usable stored Recommendations")

// Stored is the last list fetched: the API and user it was fetched for,
// when ListenBrainz made it and when navigo fetched it, and the
// Recommendations by name, not by track, so a sync can't leave it stale.
type Stored struct {
	URL, User   string
	LastUpdated time.Time
	FetchedAt   time.Time
	Recs        []library.Recommendation
}

type file struct {
	Version      int
	Server, User string // the Subsonic server and user it belongs to
	Stored       Stored
}

// Path is the file for a Subsonic server and user in dir.
func Path(dir, server, user string) string {
	sum := sha256.Sum256([]byte(user + "@" + server))
	return filepath.Join(dir, "recommendations-"+hex.EncodeToString(sum[:8])+".gob")
}

// Save writes s atomically, with mode 0600: readers see the old file or
// the new one, never half of one.
func Save(dir, server, user string, s Stored) error {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).
		Encode(file{Version: version, Server: server, User: user, Stored: s}); err != nil {
		return fmt.Errorf("stored Recommendations: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".recommendations-*.tmp") // 0600
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

// Load reads the stored list for a Subsonic server and user. Any problem
// with the file is ErrNone; a damaged or outdated file is removed.
func Load(dir, server, user string) (Stored, error) {
	path := Path(dir, server, user)
	b, err := os.ReadFile(path)
	if err != nil {
		return Stored{}, ErrNone
	}
	var f file
	if err := gob.NewDecoder(bytes.NewReader(b)).Decode(&f); err != nil ||
		f.Version != version || f.Server != server || f.User != user {
		_ = os.Remove(path)
		return Stored{}, ErrNone
	}
	return f.Stored, nil
}

// Delete removes the stored list for a Subsonic server and user, if any.
func Delete(dir, server, user string) error {
	err := os.Remove(Path(dir, server, user))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
