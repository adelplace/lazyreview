package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/adelplace/lazyreview/internal/gh"
	"github.com/adelplace/lazyreview/internal/term"
)

type treeNode struct {
	name     string
	path     string
	file     int // index in files, -1 for directories
	children []*treeNode
}

type treeEntry struct {
	depth int
	name  string
	path  string
	file  int
}

type fileTree struct {
	files     []gh.File
	root      *treeNode
	collapsed map[string]bool
	entries   []treeEntry // visible rows
	order     []int       // file indices in display order, ignoring collapse
	cursor    int
	offset    int
}

func newFileTree(files []gh.File) fileTree {
	root := &treeNode{file: -1}
	for i, f := range files {
		n := root
		parts := strings.Split(f.Path, "/")
		for j, p := range parts {
			if j == len(parts)-1 {
				n.children = append(n.children, &treeNode{name: p, path: f.Path, file: i})
				break
			}
			var next *treeNode
			for _, c := range n.children {
				if c.file < 0 && c.name == p {
					next = c
					break
				}
			}
			if next == nil {
				next = &treeNode{name: p, path: strings.Join(parts[:j+1], "/"), file: -1}
				n.children = append(n.children, next)
			}
			n = next
		}
	}
	compact(root)
	sortTree(root)
	t := fileTree{files: files, root: root, collapsed: map[string]bool{}}
	var walk func(n *treeNode)
	walk = func(n *treeNode) {
		for _, c := range n.children {
			if c.file >= 0 {
				t.order = append(t.order, c.file)
			}
			walk(c)
		}
	}
	walk(root)
	t.rebuild()
	return t
}

// compact merges chains of single-child directories: a/b/c/file.go.
func compact(n *treeNode) {
	for _, c := range n.children {
		for c.file < 0 && len(c.children) == 1 && c.children[0].file < 0 {
			gc := c.children[0]
			c.name += "/" + gc.name
			c.path = gc.path
			c.children = gc.children
		}
		compact(c)
	}
}

func sortTree(n *treeNode) {
	sort.SliceStable(n.children, func(i, j int) bool {
		a, b := n.children[i], n.children[j]
		if (a.file < 0) != (b.file < 0) {
			return a.file < 0
		}
		return a.name < b.name
	})
	for _, c := range n.children {
		sortTree(c)
	}
}

func (t *fileTree) rebuild() {
	t.entries = t.entries[:0]
	var walk func(n *treeNode, depth int)
	walk = func(n *treeNode, depth int) {
		for _, c := range n.children {
			t.entries = append(t.entries, treeEntry{depth: depth, name: c.name, path: c.path, file: c.file})
			if c.file < 0 && !t.collapsed[c.path] {
				walk(c, depth+1)
			}
		}
	}
	walk(t.root, 0)
	t.cursor = clamp(t.cursor, 0, len(t.entries)-1)
}

func (t *fileTree) selectedFile() int {
	if t.cursor < 0 || t.cursor >= len(t.entries) {
		return -1
	}
	return t.entries[t.cursor].file
}

func (t *fileTree) move(delta int) {
	t.cursor = clamp(t.cursor+delta, 0, len(t.entries)-1)
}

// toggleDir collapses or expands the directory under the cursor.
func (t *fileTree) toggleDir() {
	if t.cursor >= len(t.entries) || t.entries[t.cursor].file >= 0 {
		return
	}
	p := t.entries[t.cursor].path
	t.collapsed[p] = !t.collapsed[p]
	t.rebuild()
}

// selectFile moves the cursor to file fi, expanding its parent directories.
func (t *fileTree) selectFile(fi int) {
	path := t.files[fi].Path
	for dir := range t.collapsed {
		if strings.HasPrefix(path, dir+"/") {
			delete(t.collapsed, dir)
		}
	}
	t.rebuild()
	for i, e := range t.entries {
		if e.file == fi {
			t.cursor = i
			return
		}
	}
}

// neighbor returns the file after (dir=1) or before (dir=-1) fi in display order.
// When unviewedOnly is set, viewed files are skipped. Returns -1 when none.
func (t *fileTree) neighbor(fi, dir int, unviewedOnly bool) int {
	pos := -1
	for i, f := range t.order {
		if f == fi {
			pos = i
		}
	}
	for i := pos + dir; i >= 0 && i < len(t.order); i += dir {
		f := t.order[i]
		if !unviewedOnly || t.files[f].Viewed != gh.Viewed {
			return f
		}
	}
	return -1
}

func (t *fileTree) viewedCount() int {
	n := 0
	for _, f := range t.files {
		if f.Viewed == gh.Viewed {
			n++
		}
	}
	return n
}

func (t *fileTree) view(w, h int, focused bool, current int, threads map[string]int) []string {
	var out []string
	for i := t.offset; i < len(t.entries) && i < t.offset+h; i++ {
		e := t.entries[i]
		indent := strings.Repeat("  ", e.depth)
		var line string
		if e.file < 0 {
			icon := "▾ "
			if t.collapsed[e.path] {
				icon = "▸ "
			}
			line = indent + stDim.Render(icon) + stAccent.Render(term.Line(e.name)+"/")
		} else {
			f := t.files[e.file]
			check := stDim.Render("○ ")
			switch f.Viewed {
			case gh.Viewed:
				check = stGreen.Render("✓ ")
			case gh.Dismissed:
				check = stYellow.Render("◐ ")
			}
			name := term.Line(e.name)
			if e.file == current {
				name = stBold.Render(name)
			}
			line = indent + check + statusLetter(f.Status) + " " + name +
				" " + stGreen.Render(fmt.Sprintf("+%d", f.Additions)) + stRed.Render(fmt.Sprintf("-%d", f.Deletions))
			if n := threads[f.Path]; n > 0 {
				line += stOrange.Render(fmt.Sprintf(" 💬%d", n))
			}
		}
		if i == t.cursor {
			line = selRow(line, w, focused)
		}
		out = append(out, line)
	}
	return out
}

func statusLetter(s string) string {
	switch strings.ToLower(s) {
	case "added":
		return stGreen.Render("A")
	case "removed", "deleted":
		return stRed.Render("D")
	case "renamed":
		return stMauve.Render("R")
	case "copied":
		return stMauve.Render("C")
	default:
		return stYellow.Render("M")
	}
}
