// Package diff parses GitHub unified patches and merges them with full file
// contents so a file can be displayed whole, annotated with its changes.
package diff

import (
	"fmt"
	"strconv"
	"strings"
)

// Kind is the role of a line in an annotated file.
type Kind uint8

const (
	Ctx Kind = iota // unchanged line
	Add             // line added in head
	Del             // line removed from base
)

// HunkLine is one line of a hunk body.
type HunkLine struct {
	Kind Kind
	Text string
}

// Hunk is one "@@ -a,b +c,d @@" block of a unified diff.
type Hunk struct {
	OldStart, OldLines int
	NewStart, NewLines int
	Header             string // text after the closing "@@", usually a function name
	Lines              []HunkLine
}

// Parse parses the patch GitHub returns for a single file (no "diff --git" /
// "---" / "+++" preamble, starts directly with "@@").
func Parse(patch string) ([]Hunk, error) {
	var hunks []Hunk
	var cur *Hunk
	for _, raw := range strings.Split(patch, "\n") {
		if strings.HasPrefix(raw, "@@") {
			h, err := parseHeader(raw)
			if err != nil {
				return nil, err
			}
			hunks = append(hunks, h)
			cur = &hunks[len(hunks)-1]
			continue
		}
		if cur == nil || raw == "" {
			// Preamble lines or the empty string after a trailing "\n".
			continue
		}
		switch raw[0] {
		case '+':
			cur.Lines = append(cur.Lines, HunkLine{Add, raw[1:]})
		case '-':
			cur.Lines = append(cur.Lines, HunkLine{Del, raw[1:]})
		case ' ':
			cur.Lines = append(cur.Lines, HunkLine{Ctx, raw[1:]})
		case '\\':
			// "\ No newline at end of file"
		}
	}
	return hunks, nil
}

func parseHeader(s string) (Hunk, error) {
	// @@ -oldStart[,oldLines] +newStart[,newLines] @@ header
	var h Hunk
	rest := strings.TrimPrefix(s, "@@")
	end := strings.Index(rest, "@@")
	if end < 0 {
		return h, fmt.Errorf("bad hunk header %q", s)
	}
	h.Header = strings.TrimSpace(rest[end+2:])
	fields := strings.Fields(rest[:end])
	if len(fields) != 2 || fields[0][0] != '-' || fields[1][0] != '+' {
		return h, fmt.Errorf("bad hunk header %q", s)
	}
	var err error
	if h.OldStart, h.OldLines, err = parseRange(fields[0][1:]); err != nil {
		return h, fmt.Errorf("bad hunk header %q: %w", s, err)
	}
	if h.NewStart, h.NewLines, err = parseRange(fields[1][1:]); err != nil {
		return h, fmt.Errorf("bad hunk header %q: %w", s, err)
	}
	return h, nil
}

func parseRange(s string) (start, n int, err error) {
	a, b, found := strings.Cut(s, ",")
	if start, err = strconv.Atoi(a); err != nil {
		return
	}
	n = 1
	if found {
		n, err = strconv.Atoi(b)
	}
	return
}
