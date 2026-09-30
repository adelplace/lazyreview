//go:build integration

// Read-only checks against the real GitHub API:
//
//	LAZYREVIEW_TEST_REPO=owner/name go test -tags integration ./internal/gh/
package gh

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/shurcooL/githubv4"

	"github.com/adelplace/lazyreview/internal/diff"
)

func TestIntegrationAnnotateRealPRs(t *testing.T) {
	repo := os.Getenv("LAZYREVIEW_TEST_REPO")
	if repo == "" {
		t.Skip("LAZYREVIEW_TEST_REPO not set")
	}
	owner, name, err := ParseRepo(repo)
	if err != nil {
		t.Fatal(err)
	}
	token, err := Token()
	if err != nil {
		t.Fatal(err)
	}
	c := New(token, owner, name)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	prs, err := c.ListPRs(ctx, []githubv4.PullRequestState{githubv4.PullRequestStateOpen})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d open PRs", len(prs))
	checked := 0
	for _, pr := range prs[:min(len(prs), 3)] {
		d, err := c.GetDetail(ctx, pr.Number)
		if err != nil {
			t.Fatalf("#%d: %v", pr.Number, err)
		}
		t.Logf("#%d %q: %d files, %d threads, pending=%q", pr.Number, pr.Title, len(d.Files), len(d.Review.Threads), d.Review.PendingReviewID)
		for _, f := range d.Files[:min(len(d.Files), 5)] {
			if f.Patch == "" || strings.EqualFold(f.Status, "removed") {
				continue
			}
			content, binary, err := c.FileContent(ctx, d.HeadOID, f.Path)
			if err != nil {
				t.Fatalf("#%d %s: %v", pr.Number, f.Path, err)
			}
			if binary {
				continue
			}
			hunks, err := diff.Parse(f.Patch)
			if err != nil {
				t.Fatalf("#%d %s: %v", pr.Number, f.Path, err)
			}
			lines := diff.Annotate(content, hunks)
			adds, dels, head := 0, 0, 0
			for i, l := range lines {
				switch l.Kind {
				case diff.Add:
					adds++
				case diff.Del:
					dels++
				}
				if l.Kind != diff.Del {
					head++
					if l.NewNo != head {
						t.Errorf("#%d %s: line %d has NewNo %d, want %d", pr.Number, f.Path, i, l.NewNo, head)
						break
					}
				}
			}
			if want := len(diff.SplitLines(content)); head != want {
				t.Errorf("#%d %s: %d head lines, file has %d", pr.Number, f.Path, head, want)
			}
			if adds != f.Additions || dels != f.Deletions {
				t.Errorf("#%d %s: +%d -%d, GitHub says +%d -%d", pr.Number, f.Path, adds, dels, f.Additions, f.Deletions)
			}
			checked++
		}
	}
	t.Logf("checked %d files", checked)
}
