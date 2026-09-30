package ui

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/adelplace/lazyreviewer/internal/diff"
	"github.com/adelplace/lazyreviewer/internal/gh"
	"github.com/adelplace/lazyreviewer/internal/highlight"
	"github.com/adelplace/lazyreviewer/internal/term"
)

// fileData is a file ready for display, computed off the UI goroutine.
type fileData struct {
	lines []diff.Line
	hunks []diff.Hunk
	segs  [][]highlight.Seg
	note  string // e.g. "binary file"
	// highlighting is set while syntax colors are computed in the background.
	highlighting bool
}

type rowKind uint8

const (
	rowCode rowKind = iota
	rowSep
	rowComment
)

type row struct {
	kind      rowKind
	line      int    // index in data.lines (code rows and the anchor of comment rows)
	text      string // pre-rendered text for sep and comment rows
	commentID string
	pending   bool
}

type fileView struct {
	file      *gh.File
	data      *fileData
	threads   []gh.Thread
	outdated  int
	hunksOnly bool
	loading   bool
	err       string

	rows    []row
	changes []int // row indices where a block of changes starts
	cursor  int
	offset  int
	anchor  int // selection anchor row, -1 when not selecting
	w, h    int

	numW      int
	codeCache map[int]string
}

func newFileView() fileView { return fileView{anchor: -1} }

func (v *fileView) setLoading(f *gh.File) {
	*v = fileView{file: f, loading: true, anchor: -1, hunksOnly: v.hunksOnly, w: v.w, h: v.h}
}

func (v *fileView) setError(f *gh.File, err error) {
	*v = fileView{file: f, err: err.Error(), anchor: -1, hunksOnly: v.hunksOnly, w: v.w, h: v.h}
}

func (v *fileView) setFile(f *gh.File, d *fileData, threads []gh.Thread) {
	*v = fileView{file: f, data: d, anchor: -1, hunksOnly: v.hunksOnly, w: v.w, h: v.h}
	v.setThreads(threads)
	// Start on the first change, a third down the screen.
	if len(v.changes) > 0 {
		v.cursor = v.changes[0]
		v.offset = max(v.cursor-v.h/3, 0)
	}
}

// gotoLine moves the cursor to new-side line n, a third down the screen, when
// that line is displayed.
func (v *fileView) gotoLine(n int) {
	for r, rw := range v.rows {
		if rw.kind == rowCode && v.data.lines[rw.line].NewNo == n {
			v.cursor = r
			v.offset = max(r-v.h/3, 0)
			return
		}
	}
}

// cursorLine is the new-side line number under the cursor, 0 when none.
func (v *fileView) cursorLine() int {
	if v.data == nil || v.cursor >= len(v.rows) {
		return 0
	}
	return v.data.lines[v.rows[v.cursor].line].NewNo
}

func (v *fileView) resize(w, h int) {
	if w != v.w {
		v.w, v.h = w, h
		v.rebuild()
		return
	}
	v.h = h
}

// setThreads replaces review threads, keeping the cursor on the same line.
func (v *fileView) setThreads(all []gh.Thread) {
	v.threads = v.threads[:0:0]
	v.outdated = 0
	if v.file != nil {
		for _, t := range all {
			if t.Path != v.file.Path {
				continue
			}
			if t.Line == 0 {
				v.outdated++
				continue
			}
			v.threads = append(v.threads, t)
		}
	}
	v.rebuild()
}

func (v *fileView) toggleMode() {
	v.hunksOnly = !v.hunksOnly
	v.anchor = -1
	v.rebuild()
	v.offset = max(v.cursor-v.h/3, 0)
}

// rebuild recomputes display rows, keeping the cursor on the same code line.
func (v *fileView) rebuild() {
	v.codeCache = map[int]string{}
	if v.data == nil {
		v.rows, v.changes = nil, nil
		return
	}
	curLine := -1
	if v.cursor < len(v.rows) {
		curLine = v.rows[v.cursor].line
	}

	lines := v.data.lines
	maxNo := 1
	for _, l := range lines {
		maxNo = max(maxNo, l.OldNo, l.NewNo)
	}
	v.numW = max(len(strconv.Itoa(maxNo)), 3)

	byNew, byOld := map[int]int{}, map[int]int{}
	for i, l := range lines {
		if l.NewNo > 0 {
			byNew[l.NewNo] = i
		}
		if l.OldNo > 0 {
			byOld[l.OldNo] = i
		}
	}
	anchored := map[int][]gh.Thread{}
	for _, t := range v.threads {
		idx, ok := byNew[t.Line]
		if t.Side == "LEFT" {
			idx, ok = byOld[t.Line]
		}
		if ok {
			anchored[idx] = append(anchored[idx], t)
		}
	}

	v.rows, v.changes = v.rows[:0], v.changes[:0]
	prevHunk, prevKind := -2, diff.Ctx
	for i, l := range lines {
		if v.hunksOnly {
			if l.Hunk < 0 {
				continue
			}
			if l.Hunk != prevHunk {
				v.rows = append(v.rows, row{kind: rowSep, line: i, text: term.Line(diff.HunkTitle(v.data.hunks[l.Hunk]))})
			}
		}
		if l.Kind != diff.Ctx && (prevKind == diff.Ctx || l.Hunk != prevHunk) {
			v.changes = append(v.changes, len(v.rows))
		}
		prevHunk, prevKind = l.Hunk, l.Kind
		v.rows = append(v.rows, row{kind: rowCode, line: i})
		for _, t := range anchored[i] {
			v.rows = append(v.rows, v.threadRows(t, i)...)
		}
	}

	v.cursor = clamp(v.cursor, 0, len(v.rows)-1)
	if curLine >= 0 {
		for r, rw := range v.rows {
			if rw.kind == rowCode && rw.line == curLine {
				v.cursor = r
				break
			}
		}
	}
}

func (v *fileView) gutterW() int { return 2*v.numW + 4 }

func (v *fileView) threadRows(t gh.Thread, line int) []row {
	indent := strings.Repeat(" ", v.gutterW())
	textW := max(v.w-v.gutterW()-4, 10)
	pending := false
	for _, c := range t.Comments {
		pending = pending || c.Pending
	}
	bar := stDim
	if pending {
		bar = stOrange
	}
	loc := fmt.Sprintf("L%d", t.Line)
	if t.StartLine != 0 && t.StartLine != t.Line {
		loc = fmt.Sprintf("L%d–%d", t.StartLine, t.Line)
	}
	head := bar.Render("╭─ ") + stDim.Render(loc)
	if t.Resolved {
		head += stGreen.Render(" · resolved")
	}
	rows := []row{{kind: rowComment, line: line, text: indent + head}}
	for _, c := range t.Comments {
		who := stBold.Render(term.Line(c.Author))
		if c.Pending {
			who += stOrange.Render(" · pending")
		} else {
			who += stDim.Render(" · " + c.CreatedAt.Local().Format("2006-01-02 15:04"))
		}
		rows = append(rows, row{kind: rowComment, line: line, text: indent + bar.Render("│ ") + who, commentID: c.ID, pending: c.Pending})
		body := ansi.Wrap(strings.TrimRight(term.Sanitize(c.Body), "\n"), textW, " ")
		for _, bl := range strings.Split(body, "\n") {
			rows = append(rows, row{kind: rowComment, line: line, text: indent + bar.Render("│ ") + bl, commentID: c.ID, pending: c.Pending})
		}
	}
	rows = append(rows, row{kind: rowComment, line: line, text: indent + bar.Render("╰─")})
	return rows
}

func (v *fileView) move(delta int) {
	v.cursor = clamp(v.cursor+delta, 0, len(v.rows)-1)
}

// page moves the cursor and scrolls the view by the same amount, like vim.
func (v *fileView) page(delta int) {
	v.offset = clamp(v.offset+delta, 0, len(v.rows)-v.h)
	v.move(delta)
}

func (v *fileView) top()    { v.cursor = 0 }
func (v *fileView) bottom() { v.cursor = max(len(v.rows)-1, 0) }

// jumpChange moves to the next (dir=1) or previous (dir=-1) block of changes.
func (v *fileView) jumpChange(dir int) {
	if dir > 0 {
		for _, r := range v.changes {
			if r > v.cursor {
				v.cursor = r
				v.offset = max(r-v.h/3, 0)
				return
			}
		}
		return
	}
	for i := len(v.changes) - 1; i >= 0; i-- {
		if r := v.changes[i]; r < v.cursor {
			v.cursor = r
			v.offset = max(r-v.h/3, 0)
			return
		}
	}
}

func (v *fileView) toggleSelect() {
	if v.anchor >= 0 {
		v.anchor = -1
	} else {
		v.anchor = v.cursor
	}
}

// commentTarget describes where a comment typed now would be attached.
func (v *fileView) commentTarget() (gh.NewThread, string, error) {
	var t gh.NewThread
	if v.data == nil || v.file == nil || len(v.rows) == 0 {
		return t, "", errors.New("no file loaded")
	}
	lo, hi := v.cursor, v.cursor
	if v.anchor >= 0 {
		lo, hi = min(v.anchor, v.cursor), max(v.anchor, v.cursor)
	}
	first, last := -1, -1
	for r := lo; r <= hi; r++ {
		if v.rows[r].kind == rowCode {
			if first < 0 {
				first = v.rows[r].line
			}
			last = v.rows[r].line
		}
	}
	if first < 0 {
		return t, "", errors.New("move the cursor to a code line")
	}
	a, b := v.data.lines[first], v.data.lines[last]
	if a.Hunk < 0 || b.Hunk < 0 {
		return t, "", errors.New("GitHub only accepts comments on lines inside diff hunks (press d to show hunks only)")
	}
	if a.Hunk != b.Hunk {
		return t, "", errors.New("a multi-line comment must stay within one hunk")
	}
	sideOf := func(l diff.Line) (string, int) {
		if l.Kind == diff.Del {
			return "LEFT", l.OldNo
		}
		return "RIGHT", l.NewNo
	}
	t.Path = v.file.Path
	t.Side, t.Line = sideOf(b)
	desc := fmt.Sprintf("%s:%d", t.Path, t.Line)
	if first != last {
		t.StartSide, t.StartLine = sideOf(a)
		desc = fmt.Sprintf("%s:%d–%d", t.Path, t.StartLine, t.Line)
	}
	return t, desc, nil
}

// editTarget returns the head path and line to open in an editor: the line
// under the cursor, or the first change when fromCursor is false. Deleted
// lines resolve to the nearest head line, preferring the following one.
func (v *fileView) editTarget(fromCursor bool) (string, int, error) {
	if v.file == nil || v.data == nil {
		return "", 0, errors.New("no file loaded")
	}
	if v.file.Status == "removed" || v.file.Status == "DELETED" {
		return "", 0, errors.New("file is deleted in this PR")
	}
	start := v.cursor
	if !fromCursor {
		start = 0
		if len(v.changes) > 0 {
			start = v.changes[0]
		}
	}
	newNo := func(r int) int { return v.data.lines[v.rows[r].line].NewNo }
	for r := start; r < len(v.rows); r++ {
		if n := newNo(r); n > 0 {
			return v.file.Path, n, nil
		}
	}
	for r := min(start, len(v.rows)) - 1; r >= 0; r-- {
		if n := newNo(r); n > 0 {
			return v.file.Path, n, nil
		}
	}
	return v.file.Path, 1, nil
}

// pendingCommentAtCursor returns the pending comment under the cursor, if any.
func (v *fileView) pendingCommentAtCursor() string {
	if v.cursor < len(v.rows) && v.rows[v.cursor].pending {
		return v.rows[v.cursor].commentID
	}
	return ""
}

func (v *fileView) title() string {
	if v.file == nil {
		return "Diff"
	}
	f := v.file
	t := term.Line(f.Path)
	if f.PrevPath != "" && f.PrevPath != f.Path {
		t = term.Line(f.PrevPath) + " → " + t
	}
	t += fmt.Sprintf("  +%d -%d", f.Additions, f.Deletions)
	if v.hunksOnly {
		t += "  [hunks]"
	} else {
		t += "  [full]"
	}
	if v.outdated > 0 {
		t += fmt.Sprintf("  %d outdated thread(s)", v.outdated)
	}
	if v.data != nil && v.data.note != "" {
		t += "  (" + v.data.note + ")"
	}
	if v.data != nil && v.data.highlighting {
		t += "  (highlighting…)"
	}
	if len(v.rows) > 0 && v.cursor < len(v.rows) && v.data != nil && len(v.data.lines) > 0 {
		l := v.data.lines[v.rows[v.cursor].line]
		n := l.NewNo
		if n == 0 {
			n = l.OldNo
		}
		t += fmt.Sprintf("  L%d", n)
	}
	return t
}

func (v *fileView) view(focused bool) []string {
	switch {
	case v.file == nil:
		return []string{stDim.Render(" select a pull request, then a file")}
	case v.loading:
		return []string{stDim.Render(" loading " + term.Line(v.file.Path) + " …")}
	case v.err != "":
		return []string{stErr.Render(" " + term.Line(v.err))}
	case len(v.rows) == 0:
		msg := " no content"
		if v.data != nil && v.data.note != "" {
			msg = " " + v.data.note
		}
		return []string{stDim.Render(msg)}
	}
	lo, hi := -1, -1
	if v.anchor >= 0 {
		lo, hi = min(v.anchor, v.cursor), max(v.anchor, v.cursor)
	}
	out := make([]string, 0, v.h)
	for r := v.offset; r < len(v.rows) && r < v.offset+v.h; r++ {
		rw := v.rows[r]
		var line string
		switch rw.kind {
		case rowSep:
			line = paintBg(stAccent.Render(" ⋯ "+rw.text), v.w, colSelBg2)
		case rowComment:
			line = rw.text
		case rowCode:
			switch {
			case r == v.cursor && focused:
				line = v.renderCode(rw.line, colCursor)
			case r >= lo && r <= hi:
				line = v.renderCode(rw.line, colRangeBg)
			default:
				var ok bool
				if line, ok = v.codeCache[rw.line]; !ok {
					line = v.renderCode(rw.line, "")
					v.codeCache[rw.line] = line
				}
			}
		}
		if rw.kind == rowComment {
			if r == v.cursor && focused {
				line = paintBg(line, v.w, colCursor)
			} else if r >= lo && r <= hi {
				line = paintBg(line, v.w, colRangeBg)
			}
		}
		out = append(out, line)
	}
	return out
}

func (v *fileView) renderCode(i int, bg lipgloss.Color) string {
	l := v.data.lines[i]
	num := func(n int) string {
		if n == 0 {
			return strings.Repeat(" ", v.numW)
		}
		return fmt.Sprintf("%*d", v.numW, n)
	}
	sign, signSt := " ", stDim
	switch l.Kind {
	case diff.Add:
		sign, signSt = "+", stGreen
		if bg == "" {
			bg = colAddBg
		}
	case diff.Del:
		sign, signSt = "-", stRed
		if bg == "" {
			bg = colDelBg
		}
	}
	var b strings.Builder
	b.WriteString(stDim.Render(num(l.OldNo) + " " + num(l.NewNo) + " "))
	b.WriteString(signSt.Render(sign))
	b.WriteByte(' ')
	budget := v.w - v.gutterW()
	for _, s := range v.data.segs[i] {
		if budget <= 0 {
			break
		}
		text := s.Text
		if w := ansi.StringWidth(text); w > budget {
			text = ansi.Truncate(text, budget, "")
		}
		budget -= ansi.StringWidth(text)
		b.WriteString(s.Style.Render(text))
	}
	if bg == "" {
		return fit(b.String(), v.w)
	}
	return paintBg(b.String(), v.w, bg)
}
