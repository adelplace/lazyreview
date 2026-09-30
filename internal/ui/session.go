package ui

// session is where the user was, restored on the next start.
type session struct {
	PR        int
	File      string
	Line      int // new-side line number under the cursor, 0 when unknown
	HeadOID   string
	State     int // index in prStates
	HunksOnly bool
	Focus     pane
}

const sessionKey = "session"

// loadSession restores the last session, unless openPR asks for another PR.
func (m *Model) loadSession(openPR int) {
	var s session
	if !m.client.Cache.Load(sessionKey, &s) {
		return
	}
	m.saved = s
	if s.State >= 0 && s.State < len(prStates) {
		m.prs.state = s.State
	}
	m.view.hunksOnly = s.HunksOnly
	if s.PR <= 0 || (openPR != 0 && openPR != s.PR) {
		return
	}
	m.loadPR = s.PR
	m.restore = &s
	if s.Focus >= panePRs && s.Focus <= paneView {
		m.focus = s.Focus
	}
}

func (m *Model) snapshot() session {
	s := session{State: m.prs.state, HunksOnly: m.view.hunksOnly, Focus: m.focus}
	if m.detail != nil {
		s.PR, s.HeadOID = m.detail.Number, m.detail.HeadOID
		if f := m.currentFile(); f != nil {
			s.File, s.Line = f.Path, m.view.cursorLine()
		}
	}
	return s
}

// saveSession writes the session when it changed. It is written on every
// change rather than on quit, since a Neovim float may kill the process.
func (m *Model) saveSession() {
	if m.restore != nil {
		// Not restored yet: keep the saved position.
		return
	}
	if s := m.snapshot(); s != m.saved {
		m.saved = s
		m.client.Cache.Save(sessionKey, s)
	}
}

// applyRestore moves the cursor to the saved line once the restored file is
// displayed. The restore is dropped as soon as it cannot apply anymore.
func (m *Model) applyRestore() {
	r := m.restore
	if r == nil || m.detail == nil || m.detail.Number != r.PR {
		return
	}
	f := m.currentFile()
	if f == nil || f.Path != r.File || m.view.err != "" {
		m.restore = nil
		return
	}
	if m.view.data == nil {
		return // still loading
	}
	if m.detail.HeadOID == r.HeadOID && r.Line > 0 {
		m.view.gotoLine(r.Line)
	}
	m.restore = nil
}
