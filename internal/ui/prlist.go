package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/shurcooL/githubv4"

	"github.com/adelplace/lazyreview/internal/gh"
	"github.com/adelplace/lazyreview/internal/term"
)

var prStates = []struct {
	name   string
	states []githubv4.PullRequestState
}{
	{"open", []githubv4.PullRequestState{githubv4.PullRequestStateOpen}},
	{"merged", []githubv4.PullRequestState{githubv4.PullRequestStateMerged}},
	{"closed", []githubv4.PullRequestState{githubv4.PullRequestStateClosed}},
	{"all", []githubv4.PullRequestState{githubv4.PullRequestStateOpen, githubv4.PullRequestStateMerged, githubv4.PullRequestStateClosed}},
}

type prList struct {
	all       []gh.PR
	items     []gh.PR
	cursor    int
	offset    int
	state     int
	filter    textinput.Model
	filtering bool
	loading   bool
}

func newPRList() prList {
	ti := textinput.New()
	ti.Prompt = "/"
	ti.Placeholder = "filter"
	return prList{filter: ti}
}

func (l *prList) setPRs(prs []gh.PR) {
	var sel int
	if p := l.selected(); p != nil {
		sel = p.Number
	}
	l.all = prs
	l.applyFilter()
	l.selectNumber(sel)
}

func (l *prList) applyFilter() {
	q := strings.ToLower(strings.TrimSpace(l.filter.Value()))
	l.items = l.items[:0:0]
	for _, p := range l.all {
		hay := strings.ToLower(fmt.Sprintf("#%d %s %s %s", p.Number, p.Title, p.Author, p.HeadRef))
		if q == "" || strings.Contains(hay, q) {
			l.items = append(l.items, p)
		}
	}
	l.cursor = clamp(l.cursor, 0, len(l.items)-1)
}

func (l *prList) selected() *gh.PR {
	if l.cursor < 0 || l.cursor >= len(l.items) {
		return nil
	}
	return &l.items[l.cursor]
}

// selectNumber moves the cursor to PR number, if listed.
func (l *prList) selectNumber(number int) {
	for i, p := range l.items {
		if p.Number == number {
			l.cursor = i
		}
	}
}

func (l *prList) move(delta int) {
	l.cursor = clamp(l.cursor+delta, 0, len(l.items)-1)
}

func (l *prList) title() string {
	t := fmt.Sprintf("Pull requests [%s] %d", prStates[l.state].name, len(l.items))
	if l.loading {
		t += " …"
	}
	return t
}

func (l *prList) showFilter() bool { return l.filtering || l.filter.Value() != "" }

// rows is the number of PR rows that fit in a pane of inner height h.
func (l *prList) rows(h int) int {
	if l.showFilter() {
		return h - 1
	}
	return h
}

func (l *prList) view(w, h int, focused bool, active int) []string {
	rows := l.rows(h)
	var out []string
	if l.showFilter() {
		out = append(out, l.filter.View())
	}
	for i := l.offset; i < len(l.items) && i < l.offset+rows; i++ {
		p := l.items[i]
		num := fmt.Sprintf("#%-4d", p.Number)
		mark := " "
		if p.Number == active {
			mark = stAccent.Render("●")
		}
		badge := ""
		switch {
		case p.IsDraft:
			badge = stDim.Render("draft ")
		case p.ReviewDecision == "APPROVED":
			badge = stGreen.Render("✓ ")
		case p.ReviewDecision == "CHANGES_REQUESTED":
			badge = stRed.Render("± ")
		}
		line := mark + stMauve.Render(num) + " " + badge + term.Line(p.Title) + stDim.Render("  "+term.Line(p.Author))
		if i == l.cursor {
			line = selRow(line, w, focused)
		}
		out = append(out, line)
	}
	if len(l.items) == 0 && !l.loading {
		out = append(out, stDim.Render(" no pull requests"))
	}
	return out
}
