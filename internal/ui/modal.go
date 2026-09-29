package ui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/shurcooL/githubv4"

	"github.com/adelplace/lazyreviewer/internal/gh"
	"github.com/adelplace/lazyreviewer/internal/term"
)

// modal is a dialog drawn over the main layout. update returns nil to close.
type modal interface {
	update(msg tea.Msg) (modal, tea.Cmd)
	view(maxW int) string
}

// Messages emitted by modals when confirmed.
type (
	commentConfirmedMsg struct{ thread gh.NewThread }
	submitConfirmedMsg  struct {
		event githubv4.PullRequestReviewEvent
		body  string
	}
	editorDoneMsg struct {
		text string
		err  error
	}
)

var modalBox = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(colAccent).
	Padding(0, 1)

func newTextarea(placeholder string) textarea.Model {
	ta := textarea.New()
	ta.Placeholder = placeholder
	ta.ShowLineNumbers = false
	ta.CharLimit = 0
	ta.SetHeight(8)
	ta.Focus()
	return ta
}

// openEditor edits text in $VISUAL / $EDITOR and reports the result as editorDoneMsg.
func openEditor(text string) tea.Cmd {
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}
	f, err := os.CreateTemp("", "lazyreviewer-*.md")
	if err != nil {
		return func() tea.Msg { return editorDoneMsg{err: err} }
	}
	_, err = f.WriteString(text)
	f.Close()
	if err != nil {
		return func() tea.Msg { return editorDoneMsg{err: err} }
	}
	args := append(strings.Fields(editor), f.Name())
	return tea.ExecProcess(exec.Command(args[0], args[1:]...), func(err error) tea.Msg {
		defer os.Remove(f.Name())
		if err != nil {
			return editorDoneMsg{err: err}
		}
		b, err := os.ReadFile(f.Name())
		return editorDoneMsg{text: strings.TrimRight(string(b), "\n"), err: err}
	})
}

// --- comment ---

type commentModal struct {
	target gh.NewThread
	desc   string
	ta     textarea.Model
}

func newCommentModal(t gh.NewThread, desc string) *commentModal {
	return &commentModal{target: t, desc: desc, ta: newTextarea("Leave a comment…")}
}

func (m *commentModal) update(msg tea.Msg) (modal, tea.Cmd) {
	switch msg := msg.(type) {
	case editorDoneMsg:
		if msg.err == nil {
			m.ta.SetValue(msg.text)
		}
		return m, m.ta.Focus()
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return nil, nil
		case "ctrl+s":
			body := strings.TrimSpace(m.ta.Value())
			if body == "" {
				return m, nil
			}
			t := m.target
			t.Body = body
			return nil, func() tea.Msg { return commentConfirmedMsg{t} }
		case "ctrl+e":
			return m, openEditor(m.ta.Value())
		}
	}
	var cmd tea.Cmd
	m.ta, cmd = m.ta.Update(msg)
	return m, cmd
}

func (m *commentModal) view(maxW int) string {
	w := min(maxW-4, 90)
	m.ta.SetWidth(w)
	return modalBox.Render(lipgloss.JoinVertical(lipgloss.Left,
		stAccent.Bold(true).Render("Comment on ")+term.Line(m.desc),
		"",
		m.ta.View(),
		"",
		hints("ctrl+s", "add to review", "ctrl+e", "$EDITOR", "esc", "cancel"),
	))
}

// --- submit ---

var reviewEvents = []struct {
	label string
	event githubv4.PullRequestReviewEvent
	style lipgloss.Style
}{
	{"Comment", githubv4.PullRequestReviewEventComment, stAccent},
	{"Approve", githubv4.PullRequestReviewEventApprove, stGreen},
	{"Request changes", githubv4.PullRequestReviewEventRequestChanges, stRed},
}

type submitModal struct {
	pending int
	choice  int
	ta      textarea.Model
}

func newSubmitModal(pending int) *submitModal {
	return &submitModal{pending: pending, ta: newTextarea("Review summary (optional for Approve)…")}
}

func (m *submitModal) update(msg tea.Msg) (modal, tea.Cmd) {
	switch msg := msg.(type) {
	case editorDoneMsg:
		if msg.err == nil {
			m.ta.SetValue(msg.text)
		}
		return m, m.ta.Focus()
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return nil, nil
		case "tab":
			m.choice = (m.choice + 1) % len(reviewEvents)
			return m, nil
		case "shift+tab":
			m.choice = (m.choice + len(reviewEvents) - 1) % len(reviewEvents)
			return m, nil
		case "ctrl+e":
			return m, openEditor(m.ta.Value())
		case "ctrl+s":
			ev := reviewEvents[m.choice].event
			body := strings.TrimSpace(m.ta.Value())
			// GitHub requires a body for these unless inline comments exist.
			if body == "" && m.pending == 0 && ev != githubv4.PullRequestReviewEventApprove {
				return m, nil
			}
			return nil, func() tea.Msg { return submitConfirmedMsg{event: ev, body: body} }
		}
	}
	var cmd tea.Cmd
	m.ta, cmd = m.ta.Update(msg)
	return m, cmd
}

func (m *submitModal) view(maxW int) string {
	w := min(maxW-4, 90)
	m.ta.SetWidth(w)
	var opts []string
	for i, e := range reviewEvents {
		if i == m.choice {
			opts = append(opts, e.style.Bold(true).Render("◉ "+e.label))
		} else {
			opts = append(opts, stDim.Render("○ "+e.label))
		}
	}
	return modalBox.Render(lipgloss.JoinVertical(lipgloss.Left,
		stAccent.Bold(true).Render("Submit review")+stDim.Render(pluralize(m.pending, " — %d pending comment", "s")),
		"",
		strings.Join(opts, "   "),
		"",
		m.ta.View(),
		"",
		hints("tab", "change verdict", "ctrl+s", "submit", "ctrl+e", "$EDITOR", "esc", "cancel"),
	))
}

// --- confirm ---

type confirmModal struct {
	question string
	onYes    tea.Cmd
}

func (m *confirmModal) update(msg tea.Msg) (modal, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "y", "Y", "enter":
			return nil, m.onYes
		case "n", "N", "esc", "q":
			return nil, nil
		}
	}
	return m, nil
}

func (m *confirmModal) view(int) string {
	return modalBox.Render(m.question + "\n\n" + hints("y", "yes", "n", "no"))
}

// --- help ---

type helpModal struct{}

var helpText = [][2]string{
	{"tab / shift+tab", "cycle panes"},
	{"h / l", "previous / next pane"},
	{"ctrl+h/j/k/l", "move to pane left / down / up / right"},
	{"j k ↑ ↓", "move"},
	{"g G ctrl+d ctrl+u", "top, bottom, half page (diff, also from files)"},
	{"enter", "open PR / file, fold directory"},
	{"/  s", "filter PRs, cycle PR state"},
	{"space", "toggle file viewed (then next unviewed)"},
	{"] [", "next / previous file"},
	{"n N", "next / previous change"},
	{"d", "full file ↔ hunks only"},
	{"v", "start / stop range selection"},
	{"c", "comment line or range"},
	{"x", "delete pending comment under cursor"},
	{"S", "submit review"},
	{"o", "open PR in browser"},
	{"r", "refresh"},
	{"q ctrl+c", "quit"},
}

func (helpModal) update(msg tea.Msg) (modal, tea.Cmd) {
	if _, ok := msg.(tea.KeyMsg); ok {
		return nil, nil
	}
	return helpModal{}, nil
}

func (helpModal) view(int) string {
	var b strings.Builder
	b.WriteString(stAccent.Bold(true).Render("Keys") + "\n\n")
	for _, h := range helpText {
		b.WriteString(stKey.Render(fit(h[0], 20)) + " " + h[1] + "\n")
	}
	b.WriteString("\n" + stDim.Render("press any key"))
	return modalBox.Render(b.String())
}

// --- helpers ---

func hints(kv ...string) string {
	var parts []string
	for i := 0; i+1 < len(kv); i += 2 {
		parts = append(parts, stKey.Render(kv[i])+" "+stDim.Render(kv[i+1]))
	}
	return strings.Join(parts, stDim.Render(" · "))
}

func pluralize(n int, format, suffix string) string {
	if n == 0 {
		return ""
	}
	s := fmt.Sprintf(format, n)
	if n > 1 {
		s += suffix
	}
	return s
}
