package gh

import (
	"context"
	"fmt"
	"time"

	"github.com/shurcooL/githubv4"
	"golang.org/x/sync/errgroup"
)

type PR struct {
	ID             string
	Number         int
	Title          string
	Author         string
	HeadRef        string
	BaseRef        string
	URL            string
	State          string
	IsDraft        bool
	ReviewDecision string
	UpdatedAt      time.Time
	Additions      int
	Deletions      int
	ChangedFiles   int
}

type prNode struct {
	ID     string
	Number int
	Title  string
	Author struct {
		Login string
	}
	HeadRefName    string
	BaseRefName    string
	URL            string
	State          string
	IsDraft        bool
	ReviewDecision string
	UpdatedAt      time.Time
	Additions      int
	Deletions      int
	ChangedFiles   int
}

func (n prNode) toPR() PR {
	return PR{
		ID: n.ID, Number: n.Number, Title: n.Title, Author: n.Author.Login,
		HeadRef: n.HeadRefName, BaseRef: n.BaseRefName, URL: n.URL, State: n.State,
		IsDraft: n.IsDraft, ReviewDecision: n.ReviewDecision, UpdatedAt: n.UpdatedAt,
		Additions: n.Additions, Deletions: n.Deletions, ChangedFiles: n.ChangedFiles,
	}
}

// ListPRs returns the 100 most recently updated PRs in the given states.
func (c *Client) ListPRs(ctx context.Context, states []githubv4.PullRequestState) ([]PR, error) {
	var q struct {
		Repository struct {
			PullRequests struct {
				Nodes []prNode
			} `graphql:"pullRequests(first: 100, states: $states, orderBy: {field: UPDATED_AT, direction: DESC})"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}
	vars := c.repoVars()
	vars["states"] = states
	if err := c.gql.Query(ctx, &q, vars); err != nil {
		return nil, err
	}
	prs := make([]PR, len(q.Repository.PullRequests.Nodes))
	for i, n := range q.Repository.PullRequests.Nodes {
		prs[i] = n.toPR()
	}
	return prs, nil
}

type ViewedState string

const (
	Viewed    ViewedState = "VIEWED"
	Unviewed  ViewedState = "UNVIEWED"
	Dismissed ViewedState = "DISMISSED" // viewed, but changed since
)

type File struct {
	Path      string
	PrevPath  string // set for renames
	Status    string // added, removed, modified, renamed, ...
	Additions int
	Deletions int
	Viewed    ViewedState
	Patch     string // empty for binary or too large diffs
}

// Detail is everything needed to review a PR.
type Detail struct {
	PR
	HeadOID string
	BaseOID string
	Files   []File
	Review  ReviewState
}

// GetDetail fetches PR metadata, files with viewed state, patches and review
// threads concurrently.
func (c *Client) GetDetail(ctx context.Context, number int) (*Detail, error) {
	var d Detail
	var gqlFiles []File
	var patches map[string]restFile

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		var err error
		gqlFiles, err = c.gqlFiles(ctx, number, &d)
		return err
	})
	g.Go(func() error {
		var err error
		patches, err = c.restFiles(ctx, number)
		return err
	})
	g.Go(func() error {
		var err error
		d.Review, err = c.GetReviewState(ctx, number)
		return err
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}
	for i := range gqlFiles {
		if rf, ok := patches[gqlFiles[i].Path]; ok {
			gqlFiles[i].Patch = rf.Patch
			gqlFiles[i].Status = rf.Status
			gqlFiles[i].PrevPath = rf.PreviousFilename
		}
	}
	d.Files = gqlFiles
	return &d, nil
}

func (c *Client) gqlFiles(ctx context.Context, number int, d *Detail) ([]File, error) {
	var q struct {
		Repository struct {
			PullRequest struct {
				prNode
				HeadRefOid string
				BaseRefOid string
				Files      struct {
					PageInfo struct {
						HasNextPage bool
						EndCursor   string
					}
					Nodes []struct {
						Path              string
						Additions         int
						Deletions         int
						ChangeType        string
						ViewerViewedState string
					}
				} `graphql:"files(first: 100, after: $cursor)"`
			} `graphql:"pullRequest(number: $number)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}
	vars := c.repoVars()
	vars["number"] = githubv4.Int(number)
	vars["cursor"] = (*githubv4.String)(nil)
	var files []File
	for {
		if err := c.gql.Query(ctx, &q, vars); err != nil {
			return nil, err
		}
		pr := q.Repository.PullRequest
		d.PR = pr.prNode.toPR()
		d.HeadOID, d.BaseOID = pr.HeadRefOid, pr.BaseRefOid
		for _, n := range pr.Files.Nodes {
			files = append(files, File{
				Path: n.Path, Additions: n.Additions, Deletions: n.Deletions,
				Status: n.ChangeType, Viewed: ViewedState(n.ViewerViewedState),
			})
		}
		if !pr.Files.PageInfo.HasNextPage {
			return files, nil
		}
		vars["cursor"] = githubv4.String(pr.Files.PageInfo.EndCursor)
	}
}

type restFile struct {
	Filename         string
	PreviousFilename string `json:"previous_filename"`
	Status           string
	Patch            string
}

func (c *Client) restFiles(ctx context.Context, number int) (map[string]restFile, error) {
	out := map[string]restFile{}
	for page := 1; ; page++ {
		var files []restFile
		path := fmt.Sprintf("/repos/%s/%s/pulls/%d/files?per_page=100&page=%d", c.Owner, c.Name, number, page)
		if _, err := c.rest(ctx, path, "", &files); err != nil {
			return nil, err
		}
		for _, f := range files {
			out[f.Filename] = f
		}
		// GitHub caps this endpoint at 3000 files.
		if len(files) < 100 || page >= 30 {
			return out, nil
		}
	}
}
