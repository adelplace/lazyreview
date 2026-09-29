package diff

import (
	"fmt"
	"strings"
	"testing"
)

// render prints lines as "old new sign text" for compact comparisons.
func render(lines []Line) string {
	var b strings.Builder
	for _, l := range lines {
		sign := map[Kind]string{Ctx: " ", Add: "+", Del: "-"}[l.Kind]
		fmt.Fprintf(&b, "%d %d %s%s h%d\n", l.OldNo, l.NewNo, sign, l.Text, l.Hunk)
	}
	return b.String()
}

func TestParseHeader(t *testing.T) {
	hs, err := Parse("@@ -1 +1,2 @@ func main() {\n-a\n+b\n+c\n\\ No newline at end of file")
	if err != nil {
		t.Fatal(err)
	}
	h := hs[0]
	if h.OldStart != 1 || h.OldLines != 1 || h.NewStart != 1 || h.NewLines != 2 || h.Header != "func main() {" {
		t.Fatalf("bad header %+v", h)
	}
	if len(h.Lines) != 3 {
		t.Fatalf("want 3 lines, got %d", len(h.Lines))
	}
}

func TestParseBadHeader(t *testing.T) {
	if _, err := Parse("@@ nope @@"); err == nil {
		t.Fatal("want error")
	}
}

func TestAnnotate(t *testing.T) {
	tests := []struct {
		name  string
		head  string
		patch string
		want  string
	}{
		{
			name:  "change in middle with gaps both sides",
			head:  "1\n2\n3\nX\n5\n6\n7\n",
			patch: "@@ -3,3 +3,3 @@\n 3\n-4\n+X\n 5",
			want: "1 1  1 h-1\n2 2  2 h-1\n3 3  3 h0\n4 0 -4 h0\n0 4 +X h0\n5 5  5 h0\n" +
				"6 6  6 h-1\n7 7  7 h-1\n",
		},
		{
			name:  "multiple hunks shift old numbers",
			head:  "a\nNEW\nb\nc\nd\n",
			patch: "@@ -1,2 +1,3 @@\n a\n+NEW\n b\n@@ -4 +4,0 @@\n-gone\n",
			// base: a b c gone d   head: a NEW b c d
			want: "1 1  a h0\n0 2 +NEW h0\n2 3  b h0\n3 4  c h-1\n4 0 -gone h1\n5 5  d h-1\n",
		},
		{
			name:  "added file",
			head:  "x\ny",
			patch: "@@ -0,0 +1,2 @@\n+x\n+y",
			want:  "0 1 +x h0\n0 2 +y h0\n",
		},
		{
			name:  "deleted file",
			head:  "",
			patch: "@@ -1,2 +0,0 @@\n-x\n-y",
			want:  "1 0 -x h0\n2 0 -y h0\n",
		},
		{
			name:  "insertion at top",
			head:  "new\nold\n",
			patch: "@@ -0,0 +1 @@\n+new",
			want:  "0 1 +new h0\n1 2  old h-1\n",
		},
		{
			name:  "no patch (binary or too large)",
			head:  "a\r\nb\r\n",
			patch: "",
			want:  "1 1  a h-1\n2 2  b h-1\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hs, err := Parse(tt.patch)
			if err != nil {
				t.Fatal(err)
			}
			if got := render(Annotate(tt.head, hs)); got != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestHunkTitle(t *testing.T) {
	hs, _ := Parse("@@ -1,2 +1,3 @@ fn\n a\n+NEW\n b\n@@ -4 +4,0 @@\n-gone\n")
	if got := HunkTitle(hs[0]); got != "@@ -1,2 +1,3 @@ fn" {
		t.Errorf("got %q", got)
	}
	if got := HunkTitle(hs[1]); got != "@@ -4,1 +4,0 @@" {
		t.Errorf("got %q", got)
	}
}
