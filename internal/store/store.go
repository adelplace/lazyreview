// Package store is a best-effort on-disk cache for one repository: JSON
// documents (PR lists, PR details, UI session) and raw file contents keyed by
// commit. Every method is a no-op on a nil *Store and errors are swallowed,
// since a missing cache only costs a network round trip.
package store

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

type Store struct {
	dir string
}

// Open returns the store of owner/name under the user cache directory, or nil
// when it cannot be created.
func Open(owner, name string) *Store {
	base, err := os.UserCacheDir()
	if err != nil {
		return nil
	}
	return OpenDir(filepath.Join(base, "lazyreview", owner, name))
}

// OpenDir returns a store rooted at dir, or nil when it cannot be created.
func OpenDir(dir string) *Store {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil
	}
	return &Store{dir: dir}
}

// Load decodes document key into v and reports whether it succeeded.
func (s *Store) Load(key string, v any) bool {
	if s == nil {
		return false
	}
	b, err := os.ReadFile(filepath.Join(s.dir, key+".json"))
	return err == nil && json.Unmarshal(b, v) == nil
}

// Save encodes v as document key.
func (s *Store) Save(key string, v any) {
	if s == nil {
		return
	}
	if b, err := json.Marshal(v); err == nil {
		write(filepath.Join(s.dir, key+".json"), b)
	}
}

// Blob returns the content of path at commit oid, if cached.
func (s *Store) Blob(oid, path string) ([]byte, bool) {
	if s == nil {
		return nil, false
	}
	b, err := os.ReadFile(s.blobPath(oid, path))
	return b, err == nil
}

// PutBlob caches the content of path at commit oid. Contents at a commit never
// change, so blobs are never invalidated, only pruned.
func (s *Store) PutBlob(oid, path string, b []byte) {
	if s == nil {
		return
	}
	p := s.blobPath(oid, path)
	if os.MkdirAll(filepath.Dir(p), 0o700) == nil {
		write(p, b)
	}
}

func (s *Store) blobPath(oid, path string) string {
	h := sha1.Sum([]byte(path))
	return filepath.Join(s.dir, "blobs", filepath.Base(oid), hex.EncodeToString(h[:]))
}

// Prune removes files not modified for maxAge, then empty blob directories.
func (s *Store) Prune(maxAge time.Duration) {
	if s == nil {
		return
	}
	cutoff := time.Now().Add(-maxAge)
	var dirs []string
	_ = filepath.WalkDir(s.dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == s.dir {
			return nil
		}
		if d.IsDir() {
			dirs = append(dirs, p)
			return nil
		}
		if info, err := d.Info(); err == nil && info.ModTime().Before(cutoff) {
			_ = os.Remove(p)
		}
		return nil
	})
	// Deepest first; Remove fails harmlessly on non-empty directories.
	for i := len(dirs) - 1; i >= 0; i-- {
		_ = os.Remove(dirs[i])
	}
}

// write replaces p atomically so a concurrent reader never sees a partial file.
func write(p string, b []byte) {
	f, err := os.CreateTemp(filepath.Dir(p), ".tmp-*")
	if err != nil {
		return
	}
	_, err = f.Write(b)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(f.Name(), p)
	}
	if err != nil {
		_ = os.Remove(f.Name())
	}
}
