package ui

import tea "github.com/charmbracelet/bubbletea"

// hit returns the pane under screen cell (x, y) and the row inside it, -1 on
// a border. ok is false outside every pane (e.g. the status bar).
func (m *Model) hit(x, y int) (p pane, row int, ok bool) {
	leftW, _, bodyH := m.layout()
	prH := m.prHeight()
	inner := func(top, h int) int {
		if r := y - top - 1; r >= 0 && r < h-2 {
			return r
		}
		return -1
	}
	switch {
	case x < 0 || y < 0 || y >= bodyH:
		return 0, -1, false
	case x >= leftW:
		return paneView, inner(0, bodyH), true
	case y < prH:
		return panePRs, inner(0, prH), true
	default:
		return paneFiles, inner(prH, bodyH-prH), true
	}
}

// prIndex maps a row of the PR pane to an index in prs.items, -1 when none.
func (m *Model) prIndex(row int) int {
	if row < 0 {
		return -1
	}
	if m.prs.showFilter() {
		row--
	}
	if i := m.prs.offset + row; row >= 0 && i < len(m.prs.items) {
		return i
	}
	return -1
}

// treeIndex maps a row of the files pane to an index in tree.entries, -1 when none.
func (m *Model) treeIndex(row int) int {
	if i := m.tree.offset + row; row >= 0 && i < len(m.tree.entries) {
		return i
	}
	return -1
}

// viewRow maps screen line y to a diff row, clamped to the rows, so that
// dragging above or below the pane extends the selection and scrolls.
func (m *Model) viewRow(y int) int {
	return clamp(m.view.offset+y-1, 0, len(m.view.rows)-1)
}

func (m *Model) onMouse(msg tea.MouseMsg) tea.Cmd {
	if msg.Action == tea.MouseActionRelease {
		m.dragging = false
		return nil
	}
	if msg.Action == tea.MouseActionMotion {
		if m.dragging && len(m.view.rows) > 0 {
			r := m.viewRow(msg.Y)
			if r != m.dragFrom && m.view.anchor < 0 {
				m.view.anchor = m.dragFrom
			}
			m.view.cursor = r
		}
		return nil
	}

	p, row, ok := m.hit(msg.X, msg.Y)
	if !ok {
		return nil
	}
	switch msg.Button {
	case tea.MouseButtonWheelUp, tea.MouseButtonWheelDown:
		delta := 1
		if msg.Button == tea.MouseButtonWheelUp {
			delta = -1
		}
		return m.wheel(p, delta)
	case tea.MouseButtonLeft:
		return m.click(p, row, msg.Y)
	}
	return nil
}

func (m *Model) wheel(p pane, delta int) tea.Cmd {
	switch p {
	case panePRs:
		m.prs.move(delta)
	case paneFiles:
		if m.detail == nil {
			return nil
		}
		m.tree.move(delta)
		if fi := m.tree.selectedFile(); fi >= 0 {
			return m.openFile(fi)
		}
	default:
		m.view.page(3 * delta)
	}
	return nil
}

func (m *Model) click(p pane, row, y int) tea.Cmd {
	if m.prs.filtering {
		m.prs.filtering = false
		m.prs.filter.Blur()
	}
	m.status = ""
	m.focus = p
	switch p {
	case panePRs:
		if i := m.prIndex(row); i >= 0 {
			m.prs.cursor = i
			return m.openPR()
		}
	case paneFiles:
		i := m.treeIndex(row)
		if i < 0 || m.detail == nil {
			return nil
		}
		m.tree.cursor = i
		if fi := m.tree.selectedFile(); fi >= 0 {
			return m.openFile(fi)
		}
		m.tree.toggleDir()
	default:
		if row < 0 || len(m.view.rows) == 0 {
			return nil
		}
		m.view.anchor = -1
		m.view.cursor = m.viewRow(y)
		m.dragging, m.dragFrom = true, m.view.cursor
	}
	return nil
}
