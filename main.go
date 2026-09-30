// lazyreviewer is a terminal UI to review GitHub pull requests.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/adelplace/lazyreviewer/internal/gh"
	"github.com/adelplace/lazyreviewer/internal/highlight"
	"github.com/adelplace/lazyreviewer/internal/store"
	"github.com/adelplace/lazyreviewer/internal/ui"
)

func main() {
	repo := flag.String("repo", "", "GitHub repository as owner/name (default: origin remote of the current git repo)")
	pr := flag.Int("pr", 0, "open this pull request number on start")
	theme := flag.String("theme", "catppuccin-mocha", "chroma syntax highlighting style")
	flag.Parse()

	if err := run(*repo, *pr, *theme); err != nil {
		fmt.Fprintln(os.Stderr, "lazyreviewer:", err)
		os.Exit(1)
	}
}

// cacheMaxAge is how long unused cached PRs and file contents are kept.
const cacheMaxAge = 30 * 24 * time.Hour

func run(repo string, pr int, theme string) error {
	var owner, name string
	var err error
	if repo != "" {
		owner, name, err = gh.ParseRepo(repo)
	} else {
		owner, name, err = gh.RepoFromGit()
	}
	if err != nil {
		return err
	}
	token, err := gh.Token()
	if err != nil {
		return err
	}
	highlight.SetStyle(theme)

	client := gh.New(token, owner, name)
	client.Cache = store.Open(owner, name)
	go client.Cache.Prune(cacheMaxAge)

	p := tea.NewProgram(ui.New(client, pr), tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err = p.Run()
	return err
}
