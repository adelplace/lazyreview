package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/adelplace/lazyreview/internal/diff"
	"github.com/adelplace/lazyreview/internal/gh"
	"github.com/adelplace/lazyreview/internal/highlight"
)

// head: a NEW b c d   base: a b c gone d
func testView(t testing.TB, threads []gh.Thread) *fileView {
	t.Helper()
	hunks, err := diff.Parse("@@ -1,2 +1,3 @@\n a\n+NEW\n b\n@@ -4 +4,0 @@\n-gone\n")
	if err != nil {
		t.Fatal(err)
	}
	lines := diff.Annotate("a\nNEW\nb\nc\nd\n", hunks)
	f := &gh.File{Path: "x.go"}
	v := &fileView{w: 80, h: 20}
	v.setFile(f, &fileData{lines: lines, hunks: hunks, segs: highlight.Lines("x.go", lines)}, threads)
	return v
}

// cursorOn moves the cursor to the code row showing line index li.
func cursorOn(v *fileView, li int) {
	for r, rw := range v.rows {
		if rw.kind == rowCode && rw.line == li {
			v.cursor = r
			return
		}
	}
}

func TestCommentTarget(t *testing.T) {
	// lines: 0 " a" 1 "+NEW" 2 " b" 3 " c"(no hunk) 4 "-gone" 5 " d"(no hunk)
	tests := []struct {
		name        string
		anchor, cur int // line indices, anchor -1 for single line
		want        gh.NewThread
		wantErr     string
	}{
		{name: "added line", anchor: -1, cur: 1, want: gh.NewThread{Path: "x.go", Line: 2, Side: "RIGHT"}},
		{name: "context line", anchor: -1, cur: 0, want: gh.NewThread{Path: "x.go", Line: 1, Side: "RIGHT"}},
		{name: "deleted line", anchor: -1, cur: 4, want: gh.NewThread{Path: "x.go", Line: 4, Side: "LEFT"}},
		{name: "range", anchor: 0, cur: 2, want: gh.NewThread{Path: "x.go", StartLine: 1, StartSide: "RIGHT", Line: 3, Side: "RIGHT"}},
		{name: "reverse range", anchor: 2, cur: 1, want: gh.NewThread{Path: "x.go", StartLine: 2, StartSide: "RIGHT", Line: 3, Side: "RIGHT"}},
		{name: "outside hunk", anchor: -1, cur: 3, wantErr: "inside diff hunks"},
		{name: "across hunks", anchor: 2, cur: 4, wantErr: "within one hunk"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := testView(t, nil)
			if tt.anchor >= 0 {
				cursorOn(v, tt.anchor)
				v.toggleSelect()
			}
			cursorOn(v, tt.cur)
			got, _, err := v.commentTarget()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("want error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestThreadAnchoring(t *testing.T) {
	threads := []gh.Thread{
		{Path: "x.go", Line: 2, Side: "RIGHT", Comments: []gh.Comment{{ID: "c1", Body: "on NEW", Pending: true}}},
		{Path: "x.go", Line: 4, Side: "LEFT", Comments: []gh.Comment{{ID: "c2", Body: "on gone"}}},
		{Path: "x.go", Line: 0, Side: "RIGHT"}, // outdated
		{Path: "other.go", Line: 1, Side: "RIGHT"},
	}
	v := testView(t, threads)
	if v.outdated != 1 {
		t.Errorf("outdated = %d, want 1", v.outdated)
	}
	// The comment rows must directly follow the line they are anchored to.
	for i, rw := range v.rows {
		if rw.kind == rowComment && rw.commentID == "c1" {
			if v.data.lines[rw.line].Text != "NEW" {
				t.Errorf("c1 anchored on %q", v.data.lines[rw.line].Text)
			}
			if !rw.pending {
				t.Error("c1 should be pending")
			}
			v.cursor = i
			if id := v.pendingCommentAtCursor(); id != "c1" {
				t.Errorf("pendingCommentAtCursor = %q", id)
			}
		}
		if rw.kind == rowComment && rw.commentID == "c2" {
			if v.data.lines[rw.line].Text != "gone" {
				t.Errorf("c2 anchored on %q", v.data.lines[rw.line].Text)
			}
			v.cursor = i
			if id := v.pendingCommentAtCursor(); id != "" {
				t.Errorf("submitted comment must not be deletable, got %q", id)
			}
		}
	}
}

func TestHunksOnlyRows(t *testing.T) {
	v := testView(t, nil)
	v.toggleMode()
	var got []string
	for _, rw := range v.rows {
		if rw.kind == rowSep {
			got = append(got, "@")
		} else {
			got = append(got, v.data.lines[rw.line].Text)
		}
	}
	if want := "@ a NEW b @ gone"; strings.Join(got, " ") != want {
		t.Errorf("rows = %q, want %q", strings.Join(got, " "), want)
	}
}

func BenchmarkViewLargeFile(b *testing.B) {
	var sb strings.Builder
	for i := 0; i < 20000; i++ {
		fmt.Fprintf(&sb, "func f%d(x int) int { return x * %d } // comment\n", i, i)
	}
	hunks, _ := diff.Parse("@@ -10000,1 +10000,2 @@\n-old\n+new1\n+new2")
	lines := diff.Annotate(sb.String(), hunks)
	v := &fileView{w: 120, h: 50}
	v.setFile(&gh.File{Path: "big.go"}, &fileData{lines: lines, hunks: hunks, segs: highlight.Lines("big.go", lines)}, nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Scrolling: every frame shows a new window of lines.
		v.cursor = (i * 7) % len(v.rows)
		v.offset = scroll(v.cursor, v.offset, v.h, len(v.rows))
		_ = v.view(true)
	}
}

func BenchmarkLoadLargeFile(b *testing.B) {
	var sb strings.Builder
	for i := 0; i < 20000; i++ {
		fmt.Fprintf(&sb, "func f%d(x int) int { return x * %d } // comment\n", i, i)
	}
	content := sb.String()
	hunks, _ := diff.Parse("@@ -10000,1 +10000,2 @@\n-old\n+new1\n+new2")
	for i := 0; i < b.N; i++ {
		lines := diff.Annotate(content, hunks)
		_ = highlight.Lines("big.go", lines)
	}
}

func TestEditTarget(t *testing.T) {
	// lines: 0 " a" 1 "+NEW" 2 " b" 3 " c" 4 "-gone" 5 " d"
	tests := []struct {
		name string
		cur  int // line index, -1 for the first change
		want int
	}{
		{name: "context line", cur: 0, want: 1},
		{name: "added line", cur: 1, want: 2},
		{name: "deleted line resolves to next", cur: 4, want: 5},
		{name: "first change", cur: -1, want: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := testView(t, nil)
			if tt.cur >= 0 {
				cursorOn(v, tt.cur)
			}
			path, line, err := v.editTarget(tt.cur >= 0)
			if err != nil || path != "x.go" || line != tt.want {
				t.Errorf("got %q %d %v, want x.go %d", path, line, err, tt.want)
			}
		})
	}

	t.Run("comment row uses its anchor line", func(t *testing.T) {
		v := testView(t, []gh.Thread{{Path: "x.go", Line: 3, Comments: []gh.Comment{{Body: "hi"}}}})
		for r, rw := range v.rows {
			if rw.kind == rowComment {
				v.cursor = r
				break
			}
		}
		if _, line, _ := v.editTarget(true); line != 3 {
			t.Errorf("line = %d, want 3", line)
		}
	})

	t.Run("deleted at end resolves to previous", func(t *testing.T) {
		hunks, _ := diff.Parse("@@ -1,2 +1,1 @@\n a\n-gone\n")
		lines := diff.Annotate("a\n", hunks)
		v := &fileView{w: 80, h: 20}
		v.setFile(&gh.File{Path: "y.go"}, &fileData{lines: lines, hunks: hunks, segs: highlight.Lines("y.go", lines)}, nil)
		v.bottom()
		if _, line, _ := v.editTarget(true); line != 1 {
			t.Errorf("line = %d, want 1", line)
		}
	})

	t.Run("errors", func(t *testing.T) {
		v := newFileView()
		if _, _, err := v.editTarget(true); err == nil {
			t.Error("want error without a file")
		}
		v = *testView(t, nil)
		v.file = &gh.File{Path: "x.go", Status: "removed"}
		if _, _, err := v.editTarget(true); err == nil {
			t.Error("want error for a deleted file")
		}
	})
}

func TestNvimOpenKeys(t *testing.T) {
	got := nvimOpenKeys("/r/a b|<c>%.go", 7)
	want := `<Cmd>silent! hide | edit +7 /r/a\ b\|\<lt>c>\%.go<CR>`
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}
