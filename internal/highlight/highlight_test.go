package highlight

import (
	"strings"
	"testing"

	"github.com/adelplace/lazyreview/internal/diff"
)

func TestLinesAlignment(t *testing.T) {
	head := "package main\n\n/* multi\nline */\nfunc main() {\n\tprintln(\"x\")\n}\n"
	hunks, _ := diff.Parse("@@ -5,3 +5,3 @@\n func main() {\n-\tprintln(\"old\")\n+\tprintln(\"x\")\n }")
	lines := diff.Annotate(head, hunks)
	segs := Lines("main.go", lines)
	if len(segs) != len(lines) {
		t.Fatalf("got %d seg lines for %d lines", len(segs), len(lines))
	}
	for i, l := range lines {
		var b strings.Builder
		for _, s := range segs[i] {
			b.WriteString(s.Text)
		}
		if want := clean(l.Text); b.String() != want {
			t.Errorf("line %d: got %q want %q", i, b.String(), want)
		}
	}
}

func TestExpandTabs(t *testing.T) {
	if got := expandTabs("a\tb\t\tc"); got != "a   b       c" {
		t.Errorf("got %q", got)
	}
}
