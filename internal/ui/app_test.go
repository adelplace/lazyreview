package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/adelplace/lazyreviewer/internal/diff"
	"github.com/adelplace/lazyreviewer/internal/gh"
	"github.com/adelplace/lazyreviewer/internal/highlight"
	"github.com/adelplace/lazyreviewer/internal/store"
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

// cacheTestModel returns a model backed by a temporary disk cache and a PR
// with two 200-line files.
func cacheTestModel(t *testing.T) (Model, *gh.Detail, *fileData) {
	t.Helper()
	c := gh.New("x", "o", "r")
	c.Cache = store.OpenDir(t.TempDir())
	var sb strings.Builder
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&sb, "line %d\n", i)
	}
	lines := diff.Annotate(sb.String(), nil)
	fd := &fileData{lines: lines, segs: highlight.Lines("a.go", lines)}
	d := &gh.Detail{
		PR:      gh.PR{Number: 7},
		HeadOID: "head",
		Files:   []gh.File{{Path: "a.go"}, {Path: "b.go"}},
	}
	return New(c, 0), d, fd
}

func sendAll(m tea.Model, msgs ...tea.Msg) Model {
	for _, msg := range msgs {
		m, _ = m.Update(msg)
	}
	return m.(Model)
}

func cloneDetail(d *gh.Detail) *gh.Detail {
	c := *d
	c.Files = append([]gh.File(nil), d.Files...)
	return &c
}

func TestCachedDetailRefreshedInPlace(t *testing.T) {
	m, d, fd := cacheTestModel(t)
	m.w, m.h = 120, 40
	m.view.w, m.view.h = 80, 30
	m.loadPR = 7
	m = sendAll(m,
		detailMsg{number: 7, d: cloneDetail(d), cached: true},
		contentMsg{key: "head:a.go", data: fd},
	)
	if !m.fromCache || m.loadPR != 7 {
		t.Fatalf("after cached detail: fromCache=%v loadPR=%d, want true 7", m.fromCache, m.loadPR)
	}
	if cmd := m.toggleViewed(0, false); cmd != nil || m.detail.Files[0].Viewed == gh.Viewed {
		t.Fatal("viewed toggled on a cached PR")
	}
	m.focus = paneView
	m = sendAll(m, tea.KeyMsg{Type: tea.KeyCtrlD})
	cursor, offset := m.view.cursor, m.view.offset

	fresh := cloneDetail(d)
	fresh.Files[1].Viewed = gh.Viewed
	fresh.Review.PendingReviewID = "rev"
	m = sendAll(m, detailMsg{number: 7, d: fresh})
	if m.fromCache || m.loadPR != 0 {
		t.Fatalf("after network detail: fromCache=%v loadPR=%d, want false 0", m.fromCache, m.loadPR)
	}
	if m.view.cursor != cursor || m.view.offset != offset || m.view.data != fd {
		t.Fatalf("view reset: cursor=%d offset=%d, want %d %d", m.view.cursor, m.view.offset, cursor, offset)
	}
	if m.tree.files[1].Viewed != gh.Viewed || m.detail.Review.PendingReviewID != "rev" {
		t.Fatal("viewed state or review not refreshed")
	}
}

func TestSessionRestore(t *testing.T) {
	m, d, fd := cacheTestModel(t)
	m.client.Cache.Save(sessionKey, session{PR: 7, File: "b.go", Line: 120, HeadOID: "head", HunksOnly: false, Focus: paneView})

	m = New(m.client, 0)
	m.w, m.h = 120, 40
	m.view.w, m.view.h = 80, 30
	if m.loadPR != 7 || m.focus != paneView {
		t.Fatalf("loadPR=%d focus=%d, want 7 %d", m.loadPR, m.focus, paneView)
	}
	m = sendAll(m,
		detailMsg{number: 7, d: cloneDetail(d), cached: true},
		contentMsg{key: "head:b.go", data: fd},
	)
	if f := m.currentFile(); f == nil || f.Path != "b.go" {
		t.Fatalf("current file = %v, want b.go", f)
	}
	if got := m.view.cursorLine(); got != 120 {
		t.Fatalf("cursor line = %d, want 120", got)
	}
	if m.restore != nil {
		t.Fatal("restore not consumed")
	}

	m = sendAll(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	var s session
	if !m.client.Cache.Load(sessionKey, &s) || s.PR != 7 || s.File != "b.go" || s.Line != 121 {
		t.Fatalf("saved session = %+v, want PR 7 b.go line 121", s)
	}
}

func TestSessionIgnoredForOtherPR(t *testing.T) {
	m, _, _ := cacheTestModel(t)
	m.client.Cache.Save(sessionKey, session{PR: 7, File: "b.go", State: 1})
	m = New(m.client, 9)
	if m.loadPR != 9 || m.restore != nil || m.prs.state != 1 {
		t.Fatalf("loadPR=%d restore=%v state=%d, want 9 nil 1", m.loadPR, m.restore, m.prs.state)
	}
}
