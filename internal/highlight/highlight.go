// Package highlight turns annotated diff lines into syntax-colored segments.
package highlight

import (
	"strings"
	"sync"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/lipgloss"

	"github.com/adelplace/lazyreview/internal/diff"
	"github.com/adelplace/lazyreview/internal/term"
)

// Beyond this many bytes, files are shown without syntax colors.
const maxHighlightBytes = 2 << 20

const tabWidth = 4

// Seg is a run of text sharing one foreground style.
type Seg struct {
	Text  string
	Style lipgloss.Style // foreground / bold / italic only
}

var (
	styleName = "catppuccin-mocha"
	styleMu   sync.Mutex
	styleMap  = map[chroma.TokenType]lipgloss.Style{}
)

// SetStyle selects the chroma style. It must be called before any Lines call.
func SetStyle(name string) { styleName = name }

func tokenStyle(style *chroma.Style, tt chroma.TokenType) lipgloss.Style {
	styleMu.Lock()
	defer styleMu.Unlock()
	if s, ok := styleMap[tt]; ok {
		return s
	}
	e := style.Get(tt)
	s := lipgloss.NewStyle()
	if e.Colour.IsSet() {
		s = s.Foreground(lipgloss.Color(e.Colour.String()))
	}
	if e.Bold == chroma.Yes {
		s = s.Bold(true)
	}
	if e.Italic == chroma.Yes {
		s = s.Italic(true)
	}
	styleMap[tt] = s
	return s
}

// Lines returns one segment list per annotated line. Head-side lines (context
// and additions) are highlighted together, as are deleted lines, so that
// multi-line constructs such as comments or strings keep their color.
func Lines(filename string, lines []diff.Line) [][]Seg {
	out := make([][]Seg, len(lines))
	var newSide, oldSide []int
	size := 0
	for i, l := range lines {
		size += len(l.Text) + 1
		if l.Kind == diff.Del {
			oldSide = append(oldSide, i)
		} else {
			newSide = append(newSide, i)
		}
	}

	lexer := lexers.Match(filename)
	if lexer == nil || size > maxHighlightBytes {
		return Plain(lines)
	}
	lexer = chroma.Coalesce(lexer)
	style := styles.Get(styleName)
	for _, side := range [][]int{newSide, oldSide} {
		highlightSide(lexer, style, lines, side, out)
	}
	return out
}

// Plain returns uncolored segments, for instant display of large files.
func Plain(lines []diff.Line) [][]Seg {
	out := make([][]Seg, len(lines))
	for i, l := range lines {
		out[i] = []Seg{{Text: clean(l.Text), Style: lipgloss.NewStyle()}}
	}
	return out
}

// Size returns the number of bytes Lines would have to highlight.
func Size(lines []diff.Line) int {
	n := 0
	for _, l := range lines {
		n += len(l.Text) + 1
	}
	return n
}

func highlightSide(lexer chroma.Lexer, style *chroma.Style, lines []diff.Line, idx []int, out [][]Seg) {
	if len(idx) == 0 {
		return
	}
	var b strings.Builder
	for _, i := range idx {
		b.WriteString(clean(lines[i].Text))
		b.WriteByte('\n')
	}
	it, err := lexer.Tokenise(nil, b.String())
	if err != nil {
		for _, i := range idx {
			out[i] = []Seg{{Text: clean(lines[i].Text), Style: lipgloss.NewStyle()}}
		}
		return
	}
	n := 0 // position in idx
	for tok := it(); tok != chroma.EOF; tok = it() {
		st := tokenStyle(style, tok.Type)
		parts := strings.Split(tok.Value, "\n")
		for pi, p := range parts {
			if pi > 0 {
				n++
			}
			if p == "" || n >= len(idx) {
				continue
			}
			out[idx[n]] = append(out[idx[n]], Seg{Text: p, Style: st})
		}
	}
}

// clean makes one line of file content safe and tab-free for display.
func clean(s string) string { return expandTabs(term.Line(s)) }

func expandTabs(s string) string {
	if !strings.Contains(s, "\t") {
		return s
	}
	var b strings.Builder
	col := 0
	for _, r := range s {
		if r == '\t' {
			n := tabWidth - col%tabWidth
			b.WriteString(strings.Repeat(" ", n))
			col += n
			continue
		}
		b.WriteRune(r)
		col++
	}
	return b.String()
}
