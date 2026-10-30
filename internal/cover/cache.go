package cover

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// Cache keeps the images the server sent, as files, so a cover is fetched
// once. It forgets the least recently used when it is over its size. The zero
// value and a nil *Cache keep nothing.
type Cache struct {
	dir string
	max int64
}

// OpenCache uses dir, creating it, and drops the least recently used files
// until what is left is within max bytes. An empty dir gives a cache that
// keeps nothing.
func OpenCache(dir string, max int64) *Cache {
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil
	}
	c := &Cache{dir: dir, max: max}
	c.evict()
	return c
}

func (c *Cache) path(id string) string {
	sum := sha256.Sum256([]byte(id))
	return filepath.Join(c.dir, hex.EncodeToString(sum[:16]))
}

// Get returns the image kept for id, and counts it as used.
func (c *Cache) Get(id string) ([]byte, bool) {
	if c == nil {
		return nil, false
	}
	b, err := os.ReadFile(c.path(id))
	if err != nil {
		return nil, false
	}
	now := time.Now()
	_ = os.Chtimes(c.path(id), now, now)
	return b, true
}

// Path returns the file kept for id, and counts it as used, or "" if there
// is none: a caller can hand it to something that objects to a missing file.
func (c *Cache) Path(id string) string {
	if c == nil {
		return ""
	}
	path := c.path(id)
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	now := time.Now()
	_ = os.Chtimes(path, now, now)
	return path
}

// Put keeps the image for id. A failure only means it is fetched again.
func (c *Cache) Put(id string, b []byte) {
	if c == nil {
		return
	}
	tmp, err := os.CreateTemp(c.dir, "put-*")
	if err != nil {
		return
	}
	_, werr := tmp.Write(b)
	if cerr := tmp.Close(); werr != nil || cerr != nil {
		_ = os.Remove(tmp.Name())
		return
	}
	if os.Rename(tmp.Name(), c.path(id)) != nil {
		_ = os.Remove(tmp.Name())
		return
	}
	c.evict()
}

func (c *Cache) evict() {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return
	}
	type file struct {
		path string
		size int64
		used time.Time
	}
	var files []file
	var total int64
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		files = append(files, file{filepath.Join(c.dir, e.Name()), info.Size(), info.ModTime()})
		total += info.Size()
	}
	slices.SortFunc(files, func(a, b file) int { return a.used.Compare(b.used) })
	for _, f := range files {
		if total <= c.max {
			return
		}
		if os.Remove(f.path) == nil {
			total -= f.size
		}
	}
}
