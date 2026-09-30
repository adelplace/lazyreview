package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/shurcooL/githubv4"

	"github.com/adelplace/lazyreview/internal/diff"
	"github.com/adelplace/lazyreview/internal/gh"
	"github.com/adelplace/lazyreview/internal/highlight"
)

const netTimeout = 30 * time.Second

type (
	prsMsg struct {
		state  int
		prs    []gh.PR
		cached bool // read from the disk cache, a network reply follows
		err    error
	}
	detailMsg struct {
		number int
		d      *gh.Detail
		cached bool // read from the disk cache, a network reply follows
		err    error
	}
	contentMsg struct {
		key  string
		data *fileData
		err  error
	}
	highlightMsg struct {
		key  string
		segs [][]highlight.Seg
	}
	viewedMsg struct {
		number int
		path   string
		viewed bool
		err    error
	}
	editDoneMsg struct {
		warn string
		err  error
	}
	reviewMsg struct {
		number int
		rs     gh.ReviewState
		rsOK   bool // rs was reloaded successfully
		info   string
		err    error
	}
)

func prsKey(state int) string     { return fmt.Sprintf("prs-%s", prStates[state].name) }
func detailKey(number int) string { return fmt.Sprintf("pr-%d", number) }

func loadPRs(c *gh.Client, state int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), netTimeout)
		defer cancel()
		prs, err := c.ListPRs(ctx, prStates[state].states)
		if err == nil {
			c.Cache.Save(prsKey(state), prs)
		}
		return prsMsg{state: state, prs: prs, err: err}
	}
}

// cachedPRs replays the last PR list fetched for state, if any.
func cachedPRs(c *gh.Client, state int) tea.Cmd {
	return func() tea.Msg {
		var prs []gh.PR
		if !c.Cache.Load(prsKey(state), &prs) {
			return nil
		}
		return prsMsg{state: state, prs: prs, cached: true}
	}
}

func loadDetail(c *gh.Client, number int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), netTimeout)
		defer cancel()
		d, err := c.GetDetail(ctx, number)
		if err == nil {
			c.Cache.Save(detailKey(number), d)
		}
		return detailMsg{number: number, d: d, err: err}
	}
}

// cachedDetail replays the last detail fetched for PR number, if any.
func cachedDetail(c *gh.Client, number int) tea.Cmd {
	return func() tea.Msg {
		var d gh.Detail
		if !c.Cache.Load(detailKey(number), &d) || d.Number != number {
			return nil
		}
		return detailMsg{number: number, d: &d, cached: true}
	}
}

// fileKey identifies a processed file: "headOID:path".
func fileKey(d *gh.Detail, f *gh.File) string { return d.HeadOID + ":" + f.Path }

// loadFile fetches, annotates and highlights a file outside the UI goroutine.
func loadFile(c *gh.Client, d *gh.Detail, f gh.File) tea.Cmd {
	key := fileKey(d, &f)
	headOID, baseOID := d.HeadOID, d.BaseOID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), netTimeout)
		defer cancel()
		hunks, err := diff.Parse(f.Patch)
		if err != nil {
			return contentMsg{key: key, err: err}
		}
		fd := &fileData{hunks: hunks}
		removed := strings.EqualFold(f.Status, "removed") || strings.EqualFold(f.Status, "deleted")

		switch {
		case removed && f.Patch != "":
			// The patch holds every deleted line; nothing to fetch.
			fd.lines = diff.Annotate("", hunks)
		case removed:
			content, binary, err := c.FileContent(ctx, baseOID, f.Path)
			if err != nil {
				return contentMsg{key: key, err: err}
			}
			if binary {
				fd.note = "binary file deleted"
				return contentMsg{key: key, data: fd}
			}
			for i, t := range diff.SplitLines(content) {
				fd.lines = append(fd.lines, diff.Line{Kind: diff.Del, OldNo: i + 1, Text: t, Hunk: -1})
			}
			fd.note = "diff too large, comments disabled"
		default:
			content, binary, err := c.FileContent(ctx, headOID, f.Path)
			if err != nil {
				return contentMsg{key: key, err: err}
			}
			if binary {
				fd.note = "binary file"
				return contentMsg{key: key, data: fd}
			}
			if f.Patch == "" && f.Additions+f.Deletions > 0 {
				fd.note = "diff too large, showing head only"
			}
			fd.lines = diff.Annotate(content, hunks)
		}
		if highlight.Size(fd.lines) > syncHighlightBytes {
			// Show the file right away; colors follow in highlightMsg.
			fd.segs = highlight.Plain(fd.lines)
			fd.highlighting = true
		} else {
			fd.segs = highlight.Lines(f.Path, fd.lines)
		}
		return contentMsg{key: key, data: fd}
	}
}

// syncHighlightBytes is the size above which files are shown uncolored first.
const syncHighlightBytes = 64 << 10

func highlightFile(key, path string, lines []diff.Line) tea.Cmd {
	return func() tea.Msg {
		return highlightMsg{key: key, segs: highlight.Lines(path, lines)}
	}
}

func setViewed(c *gh.Client, number int, prID, path string, viewed bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), netTimeout)
		defer cancel()
		err := c.SetViewed(ctx, prID, path, viewed)
		return viewedMsg{number: number, path: path, viewed: viewed, err: err}
	}
}

// reviewOp runs a review mutation then reloads the review state, even when
// the mutation failed, since it may have partially succeeded (e.g. the
// pending review was created but the comment was rejected).
func reviewOp(c *gh.Client, number int, info string, op func(ctx context.Context) error) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), netTimeout)
		defer cancel()
		opErr := op(ctx)
		rs, err := c.GetReviewState(ctx, number)
		msg := reviewMsg{number: number, rs: rs, rsOK: err == nil, info: info, err: opErr}
		if opErr == nil {
			msg.err = err
		}
		return msg
	}
}

func addComment(c *gh.Client, d *gh.Detail, t gh.NewThread) tea.Cmd {
	prID, pendingID := d.ID, d.Review.PendingReviewID
	return reviewOp(c, d.Number, "comment added to pending review", func(ctx context.Context) error {
		if pendingID == "" {
			id, err := c.StartReview(ctx, prID)
			if err != nil {
				return err
			}
			pendingID = id
		}
		return c.AddThread(ctx, pendingID, t)
	})
}

func deleteComment(c *gh.Client, number int, id string) tea.Cmd {
	return reviewOp(c, number, "pending comment deleted", func(ctx context.Context) error {
		return c.DeleteComment(ctx, id)
	})
}

func submitReview(c *gh.Client, d *gh.Detail, ev githubv4.PullRequestReviewEvent, body string) tea.Cmd {
	prID, pendingID := d.ID, d.Review.PendingReviewID
	info := "review submitted: " + strings.ToLower(strings.ReplaceAll(string(ev), "_", " "))
	return reviewOp(c, d.Number, info, func(ctx context.Context) error {
		return c.SubmitReview(ctx, prID, pendingID, ev, body)
	})
}

func openBrowser(url string) tea.Cmd {
	return func() tea.Msg {
		name := "xdg-open"
		switch runtime.GOOS {
		case "darwin":
			name = "open"
		case "windows":
			name = "explorer"
		}
		_ = exec.Command(name, url).Start()
		return nil
	}
}

// openInEditor opens path (relative to the repository root) at line. Inside a
// Neovim terminal ($NVIM set, e.g. a LazyVim float) the file opens in the
// parent Neovim and the terminal window is hidden; otherwise $EDITOR runs.
func openInEditor(path string, line int, headOID string) tea.Cmd {
	root, err := gitOutput("rev-parse", "--show-toplevel")
	if err != nil {
		return msgCmd(editDoneMsg{err: fmt.Errorf("not in a git checkout: %w", err)})
	}
	abs := filepath.Join(root, filepath.FromSlash(path))
	if _, err := os.Stat(abs); err != nil {
		return msgCmd(editDoneMsg{err: fmt.Errorf("%s not found in the local checkout", path)})
	}
	var warn string
	if head, err := gitOutput("rev-parse", "HEAD"); err == nil && head != headOID {
		warn = "local checkout is not the PR head, lines may differ"
	}
	if server := os.Getenv("NVIM"); server != "" {
		return func() tea.Msg {
			err := exec.Command("nvim", "--server", server, "--remote-send", nvimOpenKeys(abs, line)).Run()
			return editDoneMsg{warn: warn, err: err}
		}
	}
	args := append(editorArgs(), fmt.Sprintf("+%d", line), abs)
	return tea.ExecProcess(exec.Command(args[0], args[1:]...), func(err error) tea.Msg {
		return editDoneMsg{warn: warn, err: err}
	})
}

func gitOutput(args ...string) (string, error) {
	out, err := exec.Command("git", args...).Output()
	return strings.TrimSpace(string(out)), err
}

func msgCmd(msg tea.Msg) tea.Cmd { return func() tea.Msg { return msg } }

// nvimOpenKeys is the --remote-send input hiding the current (terminal)
// window and editing path at line in the window underneath.
func nvimOpenKeys(path string, line int) string {
	cmd := fmt.Sprintf("silent! hide | edit +%d %s", line, vimEscape(path))
	return "<Cmd>" + strings.ReplaceAll(cmd, "<", "<lt>") + "<CR>"
}

// vimEscape escapes a file name for an Ex command, like fnameescape().
func vimEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune(" \t\n*?[{`$\\%#'\"|!<", r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}
