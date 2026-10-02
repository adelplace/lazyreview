package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	colAccent  = lipgloss.Color("#89b4fa")
	colBorder  = lipgloss.Color("#45475a")
	colDim     = lipgloss.Color("#7f849c")
	colText    = lipgloss.Color("#cdd6f4")
	colGreen   = lipgloss.Color("#a6e3a1")
	colRed     = lipgloss.Color("#f38ba8")
	colYellow  = lipgloss.Color("#f9e2af")
	colOrange  = lipgloss.Color("#fab387")
	colMauve   = lipgloss.Color("#cba6f7")
	colSelBg   = lipgloss.Color("#313244")
	colSelBg2  = lipgloss.Color("#26263a")
	colCursor  = lipgloss.Color("#45475a")
	colAddBg   = lipgloss.Color("#1e3326")
	colDelBg   = lipgloss.Color("#3b2029")
	colRangeBg = lipgloss.Color("#34385a")

	stDim     = lipgloss.NewStyle().Foreground(colDim)
	stAccent  = lipgloss.NewStyle().Foreground(colAccent)
	stGreen   = lipgloss.NewStyle().Foreground(colGreen)
	stRed     = lipgloss.NewStyle().Foreground(colRed)
	stYellow  = lipgloss.NewStyle().Foreground(colYellow)
	stOrange  = lipgloss.NewStyle().Foreground(colOrange)
	stMauve   = lipgloss.NewStyle().Foreground(colMauve)
	stBold    = lipgloss.NewStyle().Bold(true)
	stErr     = lipgloss.NewStyle().Foreground(colRed).Bold(true)
	stKey     = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	stMatch   = lipgloss.NewStyle().Foreground(lipgloss.Color("#1e1e2e")).Background(colYellow)
	stStatusB = lipgloss.NewStyle().Background(lipgloss.Color("#181825"))
)

// fit truncates or pads s (which may contain ANSI codes) to exactly w cells.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if ansi.StringWidth(s) > w {
		s = ansi.Truncate(s, w, "…")
	}
	if pad := w - ansi.StringWidth(s); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}

// panel draws a rounded box of outer size w×h with a title in the top border.
func panel(title string, body []string, w, h int, focused bool) string {
	if w < 4 || h < 2 {
		return ""
	}
	bc := colBorder
	if focused {
		bc = colAccent
	}
	border := lipgloss.NewStyle().Foreground(bc)
	inner := w - 2

	title = ansi.Truncate(title, inner-3, "…")
	var ts string
	if focused {
		ts = stAccent.Bold(true).Render(title)
	} else {
		ts = stDim.Render(title)
	}
	fill := inner - 3 - ansi.StringWidth(title)
	var b strings.Builder
	b.WriteString(border.Render("╭─ "))
	b.WriteString(ts)
	b.WriteString(border.Render(" " + strings.Repeat("─", max(fill, 0)) + "╮"))
	side := border.Render("│")
	for i := 0; i < h-2; i++ {
		b.WriteByte('\n')
		line := ""
		if i < len(body) {
			line = body[i]
		}
		b.WriteString(side)
		b.WriteString(fit(line, inner))
		b.WriteString(side)
	}
	b.WriteByte('\n')
	b.WriteString(border.Render("╰" + strings.Repeat("─", inner) + "╯"))
	return b.String()
}

// scroll returns the offset keeping cursor inside a window of the given height.
func scroll(cursor, offset, height, total int) int {
	if height <= 0 {
		return 0
	}
	if cursor < offset {
		offset = cursor
	}
	if cursor >= offset+height {
		offset = cursor - height + 1
	}
	if maxOff := total - height; offset > maxOff {
		offset = max(maxOff, 0)
	}
	return max(offset, 0)
}

func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	return max(lo, min(v, hi))
}

// bgSeq returns the truecolor SGR sequence setting background c ("#rrggbb").
func bgSeq(c lipgloss.Color) string {
	v, err := strconv.ParseUint(strings.TrimPrefix(string(c), "#"), 16, 32)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", v>>16, (v>>8)&0xff, v&0xff)
}

// paintBg fits s to w cells and paints background c across the whole line,
// surviving the resets emitted by inner styled segments.
func paintBg(s string, w int, c lipgloss.Color) string {
	seq := bgSeq(c)
	s = fit(s, w)
	s = strings.ReplaceAll(s, "\x1b[0m", "\x1b[0m"+seq)
	s = strings.ReplaceAll(s, "\x1b[m", "\x1b[m"+seq)
	return seq + s + "\x1b[0m"
}

func selRow(line string, w int, focused bool) string {
	if focused {
		return paintBg(line, w, colSelBg)
	}
	return paintBg(line, w, colSelBg2)
}
