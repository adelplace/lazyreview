package diff

import (
	"fmt"
	"strings"
)

// Line is one displayable line of an annotated file.
type Line struct {
	Kind  Kind
	OldNo int // 0 when the line does not exist in base
	NewNo int // 0 when the line does not exist in head
	Text  string
	// Hunk is the index of the hunk the line belongs to, or -1 when the line
	// is outside every hunk. GitHub only accepts review comments on lines
	// that belong to a hunk.
	Hunk int
}

// Annotate merges the full head content of a file with its hunks, producing
// every line of the head file plus the deleted lines at their position.
// head is empty for deleted files.
func Annotate(head string, hunks []Hunk) []Line {
	headLines := SplitLines(head)
	out := make([]Line, 0, len(headLines)+8)

	newPos, oldPos := 1, 1 // next head / base line number to emit
	emitGap := func(until int) {
		for ; newPos < until && newPos <= len(headLines); newPos++ {
			out = append(out, Line{Kind: Ctx, OldNo: oldPos, NewNo: newPos, Text: headLines[newPos-1], Hunk: -1})
			oldPos++
		}
	}

	for hi, h := range hunks {
		// A zero-length range points at the line *before* the change.
		newFirst, oldFirst := h.NewStart, h.OldStart
		if h.NewLines == 0 {
			newFirst++
		}
		if h.OldLines == 0 {
			oldFirst++
		}
		emitGap(newFirst)
		newPos, oldPos = newFirst, oldFirst
		for _, l := range h.Lines {
			switch l.Kind {
			case Ctx:
				out = append(out, Line{Kind: Ctx, OldNo: oldPos, NewNo: newPos, Text: l.Text, Hunk: hi})
				oldPos++
				newPos++
			case Add:
				out = append(out, Line{Kind: Add, NewNo: newPos, Text: l.Text, Hunk: hi})
				newPos++
			case Del:
				out = append(out, Line{Kind: Del, OldNo: oldPos, Text: l.Text, Hunk: hi})
				oldPos++
			}
		}
	}
	emitGap(len(headLines) + 1)
	return out
}

// HunkTitle renders the "@@ -a,b +c,d @@ header" line of a hunk.
func HunkTitle(h Hunk) string {
	return strings.TrimSpace(fmt.Sprintf("@@ -%d,%d +%d,%d @@ %s", h.OldStart, h.OldLines, h.NewStart, h.NewLines, h.Header))
}

// SplitLines splits file content into lines, ignoring the final newline and
// stripping "\r" from CRLF endings.
func SplitLines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.TrimSuffix(s, "\n")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSuffix(l, "\r")
	}
	return lines
}
