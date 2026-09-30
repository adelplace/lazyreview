package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDocRoundTrip(t *testing.T) {
	s := OpenDir(t.TempDir())
	type doc struct {
		N int
		S string
	}
	var got doc
	if s.Load("d", &got) {
		t.Fatal("Load of a missing document succeeded")
	}
	s.Save("d", doc{N: 42, S: "x"})
	if !s.Load("d", &got) || got != (doc{N: 42, S: "x"}) {
		t.Fatalf("Load = %+v", got)
	}
}

func TestBlob(t *testing.T) {
	s := OpenDir(t.TempDir())
	if _, ok := s.Blob("abc", "a/b.go"); ok {
		t.Fatal("Blob of a missing file succeeded")
	}
	s.PutBlob("abc", "a/b.go", []byte("hello"))
	if b, ok := s.Blob("abc", "a/b.go"); !ok || string(b) != "hello" {
		t.Fatalf("Blob = %q, %v", b, ok)
	}
	if _, ok := s.Blob("def", "a/b.go"); ok {
		t.Fatal("Blob matched another commit")
	}
}

func TestPrune(t *testing.T) {
	dir := t.TempDir()
	s := OpenDir(dir)
	s.PutBlob("old", "f", []byte("1"))
	s.PutBlob("new", "f", []byte("2"))
	s.Save("doc", 1)
	past := time.Now().Add(-48 * time.Hour)
	oldDir := filepath.Join(dir, "blobs", "old")
	entries, _ := os.ReadDir(oldDir)
	for _, e := range entries {
		_ = os.Chtimes(filepath.Join(oldDir, e.Name()), past, past)
	}

	s.Prune(24 * time.Hour)
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Errorf("old blob dir still exists: %v", err)
	}
	if _, ok := s.Blob("new", "f"); !ok {
		t.Error("recent blob pruned")
	}
	var n int
	if !s.Load("doc", &n) {
		t.Error("recent document pruned")
	}
}

func TestNilStore(t *testing.T) {
	var s *Store
	s.Save("d", 1)
	s.PutBlob("a", "b", nil)
	s.Prune(0)
	var n int
	if s.Load("d", &n) {
		t.Fatal("nil store loaded a document")
	}
	if _, ok := s.Blob("a", "b"); ok {
		t.Fatal("nil store returned a blob")
	}
}
