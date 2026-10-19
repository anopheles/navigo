package library

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// cacheVersion changes whenever the snapshot's stored shape does; a file
// with another version is thrown away and a full sync runs.
const cacheVersion = 1

// ErrNoCache means there is no usable cache file: none yet, another
// version, or a damaged one. The caller syncs from scratch.
var ErrNoCache = errors.New("no usable library cache")

type cacheFile struct {
	Version  int
	Snapshot Snapshot
}

// CachePath is the cache file for a server and user in dir.
func CachePath(dir, server, user string) string {
	sum := sha256.Sum256([]byte(user + "@" + server))
	return filepath.Join(dir, "library-"+hex.EncodeToString(sum[:8])+".gob")
}

// Save writes s to its cache file in dir, atomically: readers see the old
// file or the new one, never half of one.
func Save(dir string, s *Snapshot) error {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(cacheFile{Version: cacheVersion, Snapshot: *s}); err != nil {
		return fmt.Errorf("library cache: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".library-*.tmp")
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
	return os.Rename(tmp.Name(), CachePath(dir, s.Server, s.User))
}

// Load reads the cache file for a server and user. Any problem with the
// file is ErrNoCache; a damaged file is removed.
func Load(dir, server, user string) (*Snapshot, error) {
	path := CachePath(dir, server, user)
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, ErrNoCache
	}
	var f cacheFile
	if err := gob.NewDecoder(bytes.NewReader(b)).Decode(&f); err != nil || f.Version != cacheVersion ||
		f.Snapshot.Server != server || f.Snapshot.User != user {
		_ = os.Remove(path)
		return nil, ErrNoCache
	}
	s := f.Snapshot
	s.index()
	return &s, nil
}
