package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/adelplace/lazyreviewer/internal/diff"
	"github.com/adelplace/lazyreviewer/internal/gh"
	"github.com/adelplace/lazyreviewer/internal/highlight"
)

// mouseModel returns a 120×40 model: PRs pane rows y=1..13, files pane rows
// y=16..37 (x < 32), diff rows y=1..37 (x >= 32).
func mouseModel(t *testing.T) tea.Model {
	t.Helper()
	m := New(gh.New("x", "o", "r"), 0)
	m.prs.loading = false
	m.prs.setPRs([]gh.PR{{Number: 1}, {Number: 2}, {Number: 3}})
	m.detail = &gh.Detail{Number: 1, Files: []gh.File{{Path: "a.go"}, {Path: "dir/b.go"}, {Path: "dir/c.go"}}}
	m.tree = newFileTree(m.detail.Files)

	var sb strings.Builder
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&sb, "line %d\n", i)
	}
	lines := diff.Annotate(sb.String(), nil)
	for i := range m.detail.Files {
		m.cache[fileKey(m.detail, &m.detail.Files[i])] = &fileData{lines: lines, segs: highlight.Lines("a.go", lines)}
	}
	var tm tea.Model = m
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return tm
}

func press(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
}

func TestMouseClickFocus(t *testing.T) {
	tests := []struct {
		name string
		x, y int
		want pane
	}{
		{"prs border", 5, 0, panePRs},
		{"files border", 5, 15, paneFiles},
		{"view", 60, 10, paneView},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := mouseModel(t)
			mm := m.(Model)
			mm.focus = paneFiles
			if tt.want == paneFiles {
				mm.focus = paneView
			}
			m, _ = mm.Update(press(tt.x, tt.y))
			if got := m.(Model).focus; got != tt.want {
				t.Errorf("focus = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestMouseClickPR(t *testing.T) {
	m := mouseModel(t)
	m, cmd := m.Update(press(5, 2))
	got := m.(Model)
	if got.prs.cursor != 1 || got.loadPR != 2 || cmd == nil {
		t.Fatalf("cursor=%d loadPR=%d cmd=%v, want 1 2 non-nil", got.prs.cursor, got.loadPR, cmd != nil)
	}
	m = mouseModel(t)
	m, _ = m.Update(press(5, 1))
	if got := m.(Model); got.loadPR != 0 {
		t.Fatalf("click on open PR reloaded it: loadPR=%d", got.loadPR)
	}
}

func TestMouseClickFiles(t *testing.T) {
	// Entries: dir/ (y=16), b.go (17), c.go (18), a.go (19).
	m := mouseModel(t)
	m, _ = m.Update(press(5, 18))
	got := m.(Model)
	if got.tree.cursor != 2 || got.current != 2 || got.focus != paneFiles {
		t.Fatalf("cursor=%d current=%d focus=%d, want 2 2 files", got.tree.cursor, got.current, got.focus)
	}
	m, _ = m.Update(press(5, 16))
	if got := m.(Model); !got.tree.collapsed["dir"] || len(got.tree.entries) != 2 {
		t.Fatalf("dir not collapsed: entries=%d", len(got.tree.entries))
	}
}

func TestMouseDragSelectsRange(t *testing.T) {
	m := mouseModel(t)
	m, _ = m.Update(press(5, 19)) // open a.go
	m, _ = m.Update(press(60, 3))
	off := m.(Model).view.offset
	m, _ = m.Update(tea.MouseMsg{X: 60, Y: 7, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
	m, _ = m.Update(tea.MouseMsg{X: 60, Y: 7, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})
	got := m.(Model)
	if got.view.anchor != off+2 || got.view.cursor != off+6 || got.dragging {
		t.Fatalf("anchor=%d cursor=%d dragging=%v, want %d %d false", got.view.anchor, got.view.cursor, got.dragging, off+2, off+6)
	}
	// Motion after release does nothing; a plain click clears the range.
	m, _ = m.Update(tea.MouseMsg{X: 60, Y: 9, Action: tea.MouseActionMotion})
	m, _ = m.Update(press(60, 4))
	if v := m.(Model).view; v.anchor != -1 || v.cursor != off+3 {
		t.Fatalf("after click: anchor=%d cursor=%d, want -1 %d", v.anchor, v.cursor, off+3)
	}
}

func TestMouseWheelFilesKeepsFocus(t *testing.T) {
	m := mouseModel(t)
	mm := m.(Model)
	mm.focus = paneView
	start := mm.tree.cursor
	m, _ = mm.Update(tea.MouseMsg{X: 5, Y: 20, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	got := m.(Model)
	if got.focus != paneView || got.tree.cursor != start+1 {
		t.Fatalf("focus=%d cursor=%d, want view %d", got.focus, got.tree.cursor, start+1)
	}
}

func TestMouseIgnoredWithModal(t *testing.T) {
	m := mouseModel(t)
	mm := m.(Model)
	mm.modal = helpModal{}
	m, _ = mm.Update(press(5, 2))
	if got := m.(Model); got.prs.cursor != 0 || got.focus != mm.focus {
		t.Fatalf("mouse handled under modal: cursor=%d focus=%d", got.prs.cursor, got.focus)
	}
}
