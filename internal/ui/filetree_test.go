package ui

import (
	"testing"

	"github.com/adelplace/lazyreview/internal/gh"
)

func entryPaths(t *fileTree) []string {
	var out []string
	for _, e := range t.entries {
		out = append(out, e.path)
	}
	return out
}

func TestTreeFilter(t *testing.T) {
	// Entries: dir/ b.go c.go, other/ b.txt, a.go.
	tr := newFileTree([]gh.File{{Path: "a.go"}, {Path: "dir/b.go"}, {Path: "dir/c.go"}, {Path: "other/b.txt"}})
	tr.collapsed["dir"] = true
	tr.rebuild()
	full := len(tr.entries)

	tr.filter.SetValue("DIR/")
	tr.applyFilter()
	// The full path is matched, and a collapsed parent is still expanded.
	if got := entryPaths(&tr); len(got) != 3 || got[0] != "dir" || got[1] != "dir/b.go" || got[2] != "dir/c.go" {
		t.Fatalf("entries = %v, want dir and its two files", got)
	}
	if fi := tr.selectedFile(); fi != 1 {
		t.Fatalf("selected file = %d, want the first match 1", fi)
	}
	if n := tr.neighbor(2, 1, false); n != -1 {
		t.Fatalf("neighbor after last match = %d, want -1", n)
	}
	if n := tr.neighbor(0, -1, false); n != 2 {
		t.Fatalf("neighbor before a.go = %d, want 2", n)
	}

	tr.filter.SetValue("b.")
	tr.applyFilter()
	if got := entryPaths(&tr); len(got) != 4 || tr.selectedFile() != 1 {
		t.Fatalf("entries = %v selected = %d, want 4 rows and dir/b.go kept", got, tr.selectedFile())
	}

	tr.filter.SetValue("nope")
	tr.applyFilter()
	if len(tr.entries) != 0 || tr.selectedFile() != -1 {
		t.Fatalf("entries = %v, want none", entryPaths(&tr))
	}

	tr.filter.SetValue("")
	tr.applyFilter()
	if len(tr.entries) != full || !tr.collapsed["dir"] {
		t.Fatalf("entries = %d, want %d with dir collapsed again", len(tr.entries), full)
	}
}
