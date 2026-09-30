// Package ui implements the Bubble Tea interface of lazyreviewer.
package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/adelplace/lazyreviewer/internal/gh"
	"github.com/adelplace/lazyreviewer/internal/term"
)

type pane int

const (
	panePRs pane = iota
	paneFiles
	paneView
)

// prefetchAhead is how many following files are loaded in the background.
const prefetchAhead = 2

// maxCache bounds the number of processed files kept in memory.
const maxCache = 300

type Model struct {
	client *gh.Client
	w, h   int
	focus  pane
	// lastLeft is the left pane focused last, where ctrl+h returns to.
	lastLeft pane

	prs    prList
	detail *gh.Detail
	loadPR int // PR number whose detail is loading, 0 when idle
	// fromCache is set while detail comes from the disk cache and may be
	// stale; mutations wait for the network copy.
	fromCache bool
	tree      fileTree
	current   int // index in detail.Files, -1 when none
	view      fileView
	threads   map[string]int // thread count per path

	cache    map[string]*fileData
	inflight map[string]bool

	reviewBusy bool
	viewedBusy int

	modal     modal
	status    string
	statusErr bool
	spin      spinner.Model
	spinning  bool

	// dragging is set while the left button is held in the diff pane.
	dragging bool
	dragFrom int // diff row where the drag started

	restore *session // saved position waiting for its PR and file, nil when done
	saved   session  // last session written to disk
}

func New(c *gh.Client, openPR int) Model {
	sp := spinner.New()
	sp.Spinner = spinner.MiniDot
	sp.Style = stAccent
	m := Model{
		client:   c,
		prs:      newPRList(),
		view:     newFileView(),
		current:  -1,
		cache:    map[string]*fileData{},
		inflight: map[string]bool{},
		spin:     sp,
		loadPR:   openPR,
		lastLeft: paneFiles,
	}
	m.prs.loading = true
	m.loadSession(openPR)
	return m
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{cachedPRs(m.client, m.prs.state), loadPRs(m.client, m.prs.state), m.spin.Tick}
	if m.loadPR > 0 {
		cmds = append(cmds, cachedDetail(m.client, m.loadPR), loadDetail(m.client, m.loadPR))
	}
	return tea.Batch(cmds...)
}

func (m *Model) busy() bool {
	return m.prs.loading || m.loadPR != 0 || m.view.loading || m.reviewBusy || m.viewedBusy > 0
}

func (m *Model) setStatus(s string, isErr bool) {
	m.status, m.statusErr = s, isErr
}

func (m *Model) setErr(err error) { m.setStatus(err.Error(), true) }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmd := m.update(msg)
	m.syncScroll()
	if m.focus != paneView {
		m.lastLeft = m.focus
	}
	m.saveSession()
	if m.busy() && !m.spinning {
		m.spinning = true
		cmd = tea.Batch(cmd, m.spin.Tick)
	}
	return m, cmd
}

func (m *Model) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		_, rightW, bodyH := m.layout()
		m.view.resize(rightW-2, bodyH-2)
		return nil

	case spinner.TickMsg:
		if !m.busy() {
			m.spinning = false
			return nil
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return cmd

	case prsMsg:
		// A cached list arriving after the network one is older: drop it.
		if msg.state != m.prs.state || (msg.cached && !m.prs.loading) {
			return nil
		}
		m.prs.loading = msg.cached
		if msg.err != nil {
			m.setErr(msg.err)
			return nil
		}
		first := len(m.prs.all) == 0
		m.prs.setPRs(msg.prs)
		if first {
			m.prs.selectNumber(m.activePR())
		}
		return nil

	case detailMsg:
		return m.onDetail(msg)

	case contentMsg:
		delete(m.inflight, msg.key)
		if msg.err == nil {
			if len(m.cache) >= maxCache {
				m.cache = map[string]*fileData{}
			}
			m.cache[msg.key] = msg.data
		}
		var cmd tea.Cmd
		if msg.err == nil && msg.data.highlighting {
			_, path, _ := strings.Cut(msg.key, ":")
			cmd = highlightFile(msg.key, path, msg.data.lines)
		}
		if f := m.currentFile(); f != nil && fileKey(m.detail, f) == msg.key {
			if msg.err != nil {
				m.view.setError(f, msg.err)
			} else {
				m.view.setFile(f, msg.data, m.detail.Review.Threads)
			}
			m.applyRestore()
		}
		return cmd

	case highlightMsg:
		if fd, ok := m.cache[msg.key]; ok {
			fd.segs, fd.highlighting = msg.segs, false
			if m.view.data == fd {
				m.view.codeCache = map[int]string{}
			}
		}
		return nil

	case viewedMsg:
		m.viewedBusy--
		if msg.err != nil && m.detail != nil && m.detail.Number == msg.number {
			for i := range m.detail.Files {
				if m.detail.Files[i].Path == msg.path {
					m.detail.Files[i].Viewed = toViewed(!msg.viewed)
				}
			}
			m.setErr(fmt.Errorf("viewed %s: %w", msg.path, msg.err))
		}
		return nil

	case reviewMsg:
		m.reviewBusy = false
		if msg.err != nil {
			m.setErr(msg.err)
		} else if msg.info != "" {
			m.setStatus(msg.info, false)
		}
		if msg.rsOK && m.detail != nil && m.detail.Number == msg.number {
			m.detail.Review = msg.rs
			m.onThreadsChanged()
		}
		if msg.err == nil && strings.HasPrefix(msg.info, "review submitted") {
			m.prs.loading = true
			return loadPRs(m.client, m.prs.state)
		}
		return nil

	case editDoneMsg:
		if msg.err != nil {
			m.setErr(fmt.Errorf("edit: %w", msg.err))
		} else if msg.warn != "" {
			m.setStatus(msg.warn, true)
		}
		return nil

	case commentConfirmedMsg:
		if m.detail == nil {
			return nil
		}
		m.reviewBusy = true
		m.view.anchor = -1
		return addComment(m.client, m.detail, msg.thread)

	case deleteRequestMsg:
		m.reviewBusy = true
		return deleteComment(m.client, msg.number, msg.id)

	case submitConfirmedMsg:
		if m.detail == nil {
			return nil
		}
		m.reviewBusy = true
		return submitReview(m.client, m.detail, msg.event, msg.body)

	case tea.MouseMsg:
		if m.modal != nil {
			return nil
		}
		return m.onMouse(msg)

	case tea.KeyMsg:
		return m.onKey(msg)
	}

	if m.modal != nil {
		var cmd tea.Cmd
		m.modal, cmd = m.modal.update(msg)
		return cmd
	}
	return nil
}

func toViewed(v bool) gh.ViewedState {
	if v {
		return gh.Viewed
	}
	return gh.Unviewed
}

func (m *Model) onDetail(msg detailMsg) tea.Cmd {
	if msg.number != m.loadPR {
		return nil
	}
	if !msg.cached {
		m.loadPR = 0
	}
	if msg.err != nil {
		m.restore = nil
		m.setErr(fmt.Errorf("PR #%d: %w", msg.number, msg.err))
		return nil
	}
	m.fromCache = msg.cached
	if m.detail != nil && m.detail.Number == msg.number && sameFiles(m.detail, msg.d) {
		m.refreshDetail(msg.d)
		return nil
	}
	// Keep the current file when refreshing the same PR.
	var keepPath string
	if m.detail != nil && m.detail.Number == msg.number {
		if f := m.currentFile(); f != nil {
			keepPath = f.Path
		}
	} else if m.restore != nil && m.restore.PR == msg.number {
		keepPath = m.restore.File
	}
	m.detail = msg.d
	m.tree = newFileTree(msg.d.Files)
	m.current = -1
	m.view = fileView{anchor: -1, hunksOnly: m.view.hunksOnly, w: m.view.w, h: m.view.h}
	m.onThreadsChanged()

	target := -1
	for i, f := range msg.d.Files {
		if f.Path == keepPath {
			target = i
		}
	}
	if target < 0 && len(m.tree.order) > 0 {
		target = m.tree.order[0]
		if n := m.tree.neighbor(-1, 1, true); n >= 0 {
			target = n
		}
	}
	if target < 0 {
		m.restore = nil
		m.setStatus(fmt.Sprintf("PR #%d has no files", msg.number), false)
		return nil
	}
	if keepPath == "" {
		m.focus = paneFiles
	}
	cmd := m.openFile(target)
	m.applyRestore()
	return cmd
}

// sameFiles reports whether b has the same files and patches as a, so only
// its viewed states and review threads may differ.
func sameFiles(a, b *gh.Detail) bool {
	if a.HeadOID != b.HeadOID || a.BaseOID != b.BaseOID || len(a.Files) != len(b.Files) {
		return false
	}
	for i := range a.Files {
		if a.Files[i].Path != b.Files[i].Path {
			return false
		}
	}
	return true
}

// refreshDetail updates the displayed PR in place, keeping the file view, its
// scroll position and the tree selection.
func (m *Model) refreshDetail(d *gh.Detail) {
	for i := range m.detail.Files {
		m.detail.Files[i].Viewed = d.Files[i].Viewed
	}
	m.detail.PR, m.detail.Review = d.PR, d.Review
	m.onThreadsChanged()
}

// activePR is the PR displayed or being loaded, 0 when none.
func (m *Model) activePR() int {
	if m.loadPR != 0 {
		return m.loadPR
	}
	if m.detail != nil {
		return m.detail.Number
	}
	return 0
}

// stale reports, and explains in the status bar, that the displayed PR comes
// from the cache and must be refreshed before it can be changed: a stale
// pending review ID would start a second review, a stale viewed state would
// toggle the wrong way.
func (m *Model) stale() bool {
	if m.detail == nil || !m.fromCache {
		return false
	}
	if m.loadPR == m.detail.Number {
		m.setStatus("refreshing the PR, try again in a moment", true)
	} else {
		m.setStatus("PR shown from cache, press r to refresh", true)
	}
	return true
}

func (m *Model) onThreadsChanged() {
	m.threads = map[string]int{}
	for _, t := range m.detail.Review.Threads {
		m.threads[t.Path]++
	}
	m.view.setThreads(m.detail.Review.Threads)
}

func (m *Model) currentFile() *gh.File {
	if m.detail == nil || m.current < 0 || m.current >= len(m.detail.Files) {
		return nil
	}
	return &m.detail.Files[m.current]
}

// openFile shows file fi, loading it if needed, and prefetches the next ones.
func (m *Model) openFile(fi int) tea.Cmd {
	if m.detail == nil || fi < 0 {
		return nil
	}
	m.tree.selectFile(fi)
	var cmds []tea.Cmd
	if fi != m.current {
		m.current = fi
		f := &m.detail.Files[fi]
		if d, ok := m.cache[fileKey(m.detail, f)]; ok {
			m.view.setFile(f, d, m.detail.Review.Threads)
		} else {
			m.view.setLoading(f)
			cmds = append(cmds, m.fetch(fi))
		}
	}
	next := fi
	for i := 0; i < prefetchAhead; i++ {
		if next = m.tree.neighbor(next, 1, false); next < 0 {
			break
		}
		cmds = append(cmds, m.fetch(next))
	}
	return tea.Batch(cmds...)
}

func (m *Model) fetch(fi int) tea.Cmd {
	f := m.detail.Files[fi]
	key := fileKey(m.detail, &f)
	if m.inflight[key] || m.cache[key] != nil {
		return nil
	}
	m.inflight[key] = true
	return loadFile(m.client, m.detail, f)
}

func (m *Model) toggleViewed(fi int, advance bool) tea.Cmd {
	if m.detail == nil || fi < 0 || m.stale() {
		return nil
	}
	f := &m.detail.Files[fi]
	viewed := f.Viewed != gh.Viewed
	f.Viewed = toViewed(viewed)
	m.viewedBusy++
	cmds := []tea.Cmd{setViewed(m.client, m.detail.Number, m.detail.ID, f.Path, viewed)}
	if viewed && advance {
		if n := m.tree.neighbor(fi, 1, true); n >= 0 {
			cmds = append(cmds, m.openFile(n))
		} else if n := m.tree.neighbor(fi, -1, true); n >= 0 {
			cmds = append(cmds, m.openFile(n))
		} else {
			m.setStatus("all files viewed — press S to submit your review", false)
		}
	}
	return tea.Batch(cmds...)
}

func (m *Model) pendingCount() int {
	if m.detail == nil {
		return 0
	}
	n := 0
	for _, t := range m.detail.Review.Threads {
		for _, c := range t.Comments {
			if c.Pending {
				n++
			}
		}
	}
	return n
}

func (m *Model) onKey(k tea.KeyMsg) tea.Cmd {
	if m.modal != nil {
		var cmd tea.Cmd
		m.modal, cmd = m.modal.update(k)
		return cmd
	}
	key := k.String()
	if m.prs.filtering {
		switch key {
		case "enter", "esc":
			m.prs.filtering = false
			m.prs.filter.Blur()
			if key == "esc" {
				m.prs.filter.SetValue("")
				m.prs.applyFilter()
			}
			return nil
		}
		var cmd tea.Cmd
		m.prs.filter, cmd = m.prs.filter.Update(k)
		m.prs.applyFilter()
		return cmd
	}
	m.status = ""

	switch key {
	case "ctrl+c", "q":
		return tea.Quit
	case "?":
		m.modal = helpModal{}
		return nil
	case "tab":
		m.focus = (m.focus + 1) % 3
		return nil
	case "shift+tab":
		m.focus = (m.focus + 2) % 3
		return nil
	case "h", "left":
		m.focus = max(m.focus-1, panePRs)
		return nil
	case "l", "right":
		m.focus = min(m.focus+1, paneView)
		return nil
	case "ctrl+h", "ctrl+j", "ctrl+k", "ctrl+l":
		m.focusDir(key)
		return nil
	case "r":
		m.prs.loading = true
		cmds := []tea.Cmd{loadPRs(m.client, m.prs.state)}
		if m.detail != nil {
			m.loadPR = m.detail.Number
			cmds = append(cmds, loadDetail(m.client, m.detail.Number))
		}
		return tea.Batch(cmds...)
	case "o":
		if m.focus == panePRs {
			if p := m.prs.selected(); p != nil {
				return openBrowser(p.URL)
			}
		}
		if m.detail != nil {
			return openBrowser(m.detail.URL)
		}
		return nil
	case "S":
		if m.detail == nil || m.stale() {
			return nil
		}
		if m.reviewBusy {
			m.setStatus("a review operation is in progress", true)
			return nil
		}
		m.modal = newSubmitModal(m.pendingCount())
		return nil
	case "]":
		if m.detail != nil {
			return m.openFile(m.tree.neighbor(m.current, 1, false))
		}
		return nil
	case "[":
		if m.detail != nil {
			return m.openFile(m.tree.neighbor(m.current, -1, false))
		}
		return nil
	}

	switch m.focus {
	case panePRs:
		return m.keyPRs(key)
	case paneFiles:
		return m.keyFiles(key)
	default:
		return m.keyView(key)
	}
}

// focusDir moves focus spatially: PRs above Files on the left, Diff on the right.
func (m *Model) focusDir(key string) {
	switch {
	case key == "ctrl+l" && m.focus != paneView:
		m.lastLeft, m.focus = m.focus, paneView
	case key == "ctrl+h" && m.focus == paneView:
		m.focus = m.lastLeft
	case key == "ctrl+j" && m.focus == panePRs:
		m.focus = paneFiles
	case key == "ctrl+k" && m.focus == paneFiles:
		m.focus = panePRs
	}
}

func (m *Model) keyPRs(key string) tea.Cmd {
	page := max(m.prHeight()-2, 1)
	switch key {
	case "j", "down":
		m.prs.move(1)
	case "k", "up":
		m.prs.move(-1)
	case "g", "home":
		m.prs.cursor = 0
	case "G", "end":
		m.prs.move(len(m.prs.items))
	case "ctrl+d", "pgdown":
		m.prs.move(page / 2)
	case "ctrl+u", "pgup":
		m.prs.move(-page / 2)
	case "/":
		m.prs.filtering = true
		return m.prs.filter.Focus()
	case "s":
		m.prs.state = (m.prs.state + 1) % len(prStates)
		m.prs.loading = true
		m.prs.cursor = 0
		return tea.Batch(cachedPRs(m.client, m.prs.state), loadPRs(m.client, m.prs.state))
	case "enter", " ":
		if m.prOpen() {
			m.focus = paneFiles
			return nil
		}
		return m.openPR()
	}
	return nil
}

// prOpen reports whether the selected PR is the one already displayed.
func (m *Model) prOpen() bool {
	p := m.prs.selected()
	return p != nil && m.detail != nil && m.detail.Number == p.Number
}

// openPR loads the selected PR unless it is already displayed.
func (m *Model) openPR() tea.Cmd {
	p := m.prs.selected()
	if p == nil || m.prOpen() {
		return nil
	}
	m.loadPR = p.Number
	m.restore = nil
	return tea.Batch(cachedDetail(m.client, p.Number), loadDetail(m.client, p.Number))
}

func (m *Model) keyFiles(key string) tea.Cmd {
	if m.detail == nil {
		return nil
	}
	moved := false
	switch key {
	case "j", "down":
		m.tree.move(1)
		moved = true
	case "k", "up":
		m.tree.move(-1)
		moved = true
	case "g", "home":
		m.tree.cursor = 0
		moved = true
	case "G", "end":
		m.tree.move(len(m.tree.entries))
		moved = true
	case "ctrl+d":
		// Scroll the diff while keeping focus on the file list.
		m.view.page(max(m.view.h/2, 1))
	case "ctrl+u":
		m.view.page(-max(m.view.h/2, 1))
	case "enter":
		if fi := m.tree.selectedFile(); fi >= 0 {
			m.focus = paneView
			return m.openFile(fi)
		}
		m.tree.toggleDir()
	case " ":
		return m.toggleViewed(m.tree.selectedFile(), false)
	case "d":
		m.view.toggleMode()
	case "e":
		if fi := m.tree.selectedFile(); fi >= 0 && fi == m.current {
			return m.editFile(false)
		}
	}
	if moved {
		// Preview the file under the cursor, like lazygit.
		if fi := m.tree.selectedFile(); fi >= 0 {
			return m.openFile(fi)
		}
	}
	return nil
}

func (m *Model) keyView(key string) tea.Cmd {
	half := max(m.view.h/2, 1)
	switch key {
	case "j", "down":
		m.view.move(1)
	case "k", "up":
		m.view.move(-1)
	case "ctrl+d":
		m.view.page(half)
	case "ctrl+u":
		m.view.page(-half)
	case "ctrl+f", "pgdown":
		m.view.page(m.view.h)
	case "ctrl+b", "pgup":
		m.view.page(-m.view.h)
	case "g", "home":
		m.view.top()
	case "G", "end":
		m.view.bottom()
	case "n":
		m.view.jumpChange(1)
	case "N":
		m.view.jumpChange(-1)
	case "d":
		m.view.toggleMode()
	case "v":
		m.view.toggleSelect()
	case "esc":
		m.view.anchor = -1
	case " ":
		return m.toggleViewed(m.current, true)
	case "e":
		return m.editFile(true)
	case "c":
		if m.stale() {
			return nil
		}
		if m.reviewBusy {
			m.setStatus("a review operation is in progress", true)
			return nil
		}
		t, desc, err := m.view.commentTarget()
		if err != nil {
			m.setErr(err)
			return nil
		}
		m.modal = newCommentModal(t, desc)
	case "x":
		if m.stale() {
			return nil
		}
		id := m.view.pendingCommentAtCursor()
		if id == "" {
			m.setStatus("no pending comment under the cursor", true)
			return nil
		}
		number := m.detail.Number
		m.modal = &confirmModal{
			question: "Delete this pending comment?",
			onYes: func() tea.Msg {
				return deleteRequestMsg{number: number, id: id}
			},
		}
	}
	return nil
}

// editFile opens the displayed file in an editor, at the cursor line or at the
// first change.
func (m *Model) editFile(fromCursor bool) tea.Cmd {
	path, line, err := m.view.editTarget(fromCursor)
	if err != nil {
		m.setErr(err)
		return nil
	}
	return openInEditor(path, line, m.detail.HeadOID)
}

type deleteRequestMsg struct {
	number int
	id     string
}

// --- layout & rendering ---

// syncScroll keeps every cursor visible. It runs after each update because
// View has a value receiver and cannot persist scroll offsets.
func (m *Model) syncScroll() {
	leftW, _, _ := m.layout()
	prH := m.prHeight()
	m.prs.filter.Width = max(leftW-5, 1)
	m.prs.offset = scroll(m.prs.cursor, m.prs.offset, m.prs.rows(prH-2), len(m.prs.items))
	m.tree.offset = scroll(m.tree.cursor, m.tree.offset, m.filesHeight(), len(m.tree.entries))
	m.view.offset = scroll(m.view.cursor, m.view.offset, m.view.h, len(m.view.rows))
}

func (m *Model) layout() (leftW, rightW, bodyH int) {
	leftW = clamp(m.w/4, 32, 60)
	if m.w < 100 {
		leftW = m.w / 3
	}
	return leftW, m.w - leftW, m.h - 1
}

func (m *Model) prHeight() int {
	_, _, bodyH := m.layout()
	return clamp(bodyH*2/5, 5, bodyH-5)
}

// filesHeight is the number of rows inside the files pane.
func (m *Model) filesHeight() int {
	_, _, bodyH := m.layout()
	return max(bodyH-m.prHeight()-2, 1)
}

func (m Model) View() string {
	if m.w == 0 {
		return ""
	}
	leftW, rightW, bodyH := m.layout()
	prH := m.prHeight()
	filesH := bodyH - prH

	active := 0
	if m.detail != nil {
		active = m.detail.Number
	}
	prPanel := panel(m.prs.title(), m.prs.view(leftW-2, prH-2, m.focus == panePRs, active), leftW, prH, m.focus == panePRs)

	filesTitle := "Files"
	var files []string
	switch {
	case m.detail != nil && (m.loadPR == 0 || m.loadPR == m.detail.Number):
		filesTitle = fmt.Sprintf("Files #%d · %d/%d viewed", m.detail.Number, m.tree.viewedCount(), len(m.detail.Files))
		if m.loadPR != 0 {
			filesTitle += " …"
		}
		files = m.tree.view(leftW-2, filesH-2, m.focus == paneFiles, m.current, m.threads)
	case m.loadPR != 0:
		filesTitle = fmt.Sprintf("Files · loading #%d", m.loadPR)
	}
	filesPanel := panel(filesTitle, files, leftW, filesH, m.focus == paneFiles)

	viewPanel := panel(m.view.title(), m.view.view(m.focus == paneView), rightW, bodyH, m.focus == paneView)

	body := lipgloss.JoinHorizontal(lipgloss.Top, lipgloss.JoinVertical(lipgloss.Left, prPanel, filesPanel), viewPanel)
	out := body + "\n" + m.statusBar()
	if m.modal != nil {
		out = overlay(out, m.modal.view(m.w), m.w, m.h)
	}
	return out
}

func (m *Model) statusBar() string {
	var left string
	switch {
	case m.status != "" && m.statusErr:
		left = stErr.Render(" " + term.Line(m.status))
	case m.status != "":
		left = stGreen.Render(" " + term.Line(m.status))
	default:
		switch m.focus {
		case panePRs:
			left = " " + hints("enter", "open", "/", "filter", "s", "state", "o", "browser", "?", "help")
		case paneFiles:
			left = " " + hints("enter", "open", "space", "viewed", "]/[", "next/prev", "d", "full/hunks", "e", "edit", "?", "help")
		default:
			left = " " + hints("c", "comment", "v", "range", "space", "viewed", "n/N", "change", "e", "edit", "S", "submit", "?", "help")
		}
	}
	var right []string
	if m.busy() {
		right = append(right, m.spin.View())
	}
	if n := m.pendingCount(); n > 0 {
		right = append(right, stOrange.Render(fmt.Sprintf("%d pending", n)))
	}
	right = append(right, stDim.Render(m.client.Owner+"/"+m.client.Name+" "))
	r := strings.Join(right, "  ")
	gap := m.w - ansi.StringWidth(r)
	return fit(left, max(gap, 0)) + r
}

// overlay draws fg centered over bg.
func overlay(bg, fg string, w, h int) string {
	bgLines := strings.Split(bg, "\n")
	fgLines := strings.Split(fg, "\n")
	fw := 0
	for _, l := range fgLines {
		fw = max(fw, ansi.StringWidth(l))
	}
	x := max((w-fw)/2, 0)
	y := max((h-len(fgLines))/2, 0)
	for i, fl := range fgLines {
		by := y + i
		if by >= len(bgLines) {
			break
		}
		bl := bgLines[by]
		left := fit(ansi.Truncate(bl, x, ""), x)
		right := ansi.TruncateLeft(bl, x+fw, "")
		bgLines[by] = left + "\x1b[0m" + fit(fl, fw) + "\x1b[0m" + right
	}
	return strings.Join(bgLines, "\n")
}
