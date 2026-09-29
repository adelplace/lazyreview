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

func TestFocusDir(t *testing.T) {
	keys := map[string]tea.KeyType{"h": tea.KeyCtrlH, "j": tea.KeyCtrlJ, "k": tea.KeyCtrlK, "l": tea.KeyCtrlL}
	tests := []struct {
		name  string
		start pane
		keys  string
		want  pane
	}{
		{"prs down", panePRs, "j", paneFiles},
		{"prs right", panePRs, "l", paneView},
		{"prs left/up no-op", panePRs, "hk", panePRs},
		{"files up", paneFiles, "k", panePRs},
		{"files right", paneFiles, "l", paneView},
		{"files left/down no-op", paneFiles, "hj", paneFiles},
		{"view left defaults to files", paneView, "h", paneFiles},
		{"view up/down/right no-op", paneView, "jkl", paneView},
		{"view left returns to prs", panePRs, "lh", panePRs},
		{"round trip", panePRs, "jlhk", panePRs},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m tea.Model = New(gh.New("x", "o", "r"), 0)
			mm := m.(Model)
			mm.focus = tt.start
			m = mm
			for _, k := range tt.keys {
				m, _ = m.Update(tea.KeyMsg{Type: keys[string(k)]})
			}
			if got := m.(Model).focus; got != tt.want {
				t.Errorf("focus = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestFilesCtrlDScrollsDiff(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&sb, "line %d\n", i)
	}
	lines := diff.Annotate(sb.String(), nil)
	f := gh.File{Path: "a.go"}
	m := New(gh.New("x", "o", "r"), 0)
	m.detail = &gh.Detail{Files: []gh.File{f}}
	m.tree = newFileTree(m.detail.Files)
	m.focus = paneFiles
	m.view.w, m.view.h = 80, 30
	m.view.setFile(&m.detail.Files[0], &fileData{lines: lines, segs: highlight.Lines("a.go", lines)}, nil)

	var tm tea.Model = m
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	got := tm.(Model)
	if got.view.cursor != 15 || got.view.offset != 15 {
		t.Fatalf("after ctrl+d: cursor=%d offset=%d, want 15 15", got.view.cursor, got.view.offset)
	}
	if got.focus != paneFiles || got.tree.cursor != 0 {
		t.Fatalf("focus=%d tree cursor=%d, want files pane untouched", got.focus, got.tree.cursor)
	}
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	if v := tm.(Model).view; v.cursor != 0 || v.offset != 0 {
		t.Fatalf("after ctrl+u: cursor=%d offset=%d, want 0 0", v.cursor, v.offset)
	}
}
