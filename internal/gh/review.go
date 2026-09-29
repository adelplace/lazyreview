package gh

import (
	"context"
	"time"

	"github.com/shurcooL/githubv4"
)

type Comment struct {
	ID        string
	Author    string
	Body      string
	Pending   bool
	CreatedAt time.Time
}

// Thread is a review thread anchored on a diff line (or range).
type Thread struct {
	ID        string
	Path      string
	Line      int    // 0 when outdated
	StartLine int    // 0 for single-line threads
	Side      string // LEFT or RIGHT
	StartSide string
	Resolved  bool
	Outdated  bool
	Comments  []Comment
}

// ReviewState holds all threads and the viewer's pending review, if any.
type ReviewState struct {
	PendingReviewID string
	Threads         []Thread
}

func (c *Client) GetReviewState(ctx context.Context, number int) (ReviewState, error) {
	var q struct {
		Repository struct {
			PullRequest struct {
				Reviews struct {
					Nodes []struct{ ID string }
				} `graphql:"reviews(first: 1, states: [PENDING])"`
				ReviewThreads struct {
					PageInfo struct {
						HasNextPage bool
						EndCursor   string
					}
					Nodes []struct {
						ID            string
						Path          string
						Line          *int
						StartLine     *int
						DiffSide      string
						StartDiffSide *string
						IsResolved    bool
						IsOutdated    bool
						Comments      struct {
							Nodes []struct {
								ID        string
								Body      string
								State     string
								CreatedAt time.Time
								Author    struct{ Login string }
							}
						} `graphql:"comments(first: 100)"`
					}
				} `graphql:"reviewThreads(first: 100, after: $cursor)"`
			} `graphql:"pullRequest(number: $number)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}
	vars := c.repoVars()
	vars["number"] = githubv4.Int(number)
	vars["cursor"] = (*githubv4.String)(nil)
	var rs ReviewState
	for {
		if err := c.gql.Query(ctx, &q, vars); err != nil {
			return rs, err
		}
		pr := q.Repository.PullRequest
		if len(pr.Reviews.Nodes) > 0 {
			rs.PendingReviewID = pr.Reviews.Nodes[0].ID
		}
		for _, n := range pr.ReviewThreads.Nodes {
			t := Thread{ID: n.ID, Path: n.Path, Side: n.DiffSide, Resolved: n.IsResolved, Outdated: n.IsOutdated}
			if n.Line != nil {
				t.Line = *n.Line
			}
			if n.StartLine != nil {
				t.StartLine = *n.StartLine
			}
			if n.StartDiffSide != nil {
				t.StartSide = *n.StartDiffSide
			}
			for _, cm := range n.Comments.Nodes {
				t.Comments = append(t.Comments, Comment{
					ID: cm.ID, Author: cm.Author.Login, Body: cm.Body,
					Pending: cm.State == "PENDING", CreatedAt: cm.CreatedAt,
				})
			}
			rs.Threads = append(rs.Threads, t)
		}
		if !pr.ReviewThreads.PageInfo.HasNextPage {
			return rs, nil
		}
		vars["cursor"] = githubv4.String(pr.ReviewThreads.PageInfo.EndCursor)
	}
}

// StartReview creates an empty pending review and returns its ID.
func (c *Client) StartReview(ctx context.Context, prID string) (string, error) {
	var m struct {
		AddPullRequestReview struct {
			PullRequestReview struct{ ID string }
		} `graphql:"addPullRequestReview(input: $input)"`
	}
	err := c.gql.Mutate(ctx, &m, githubv4.AddPullRequestReviewInput{PullRequestID: githubv4.ID(prID)}, nil)
	return m.AddPullRequestReview.PullRequestReview.ID, err
}

// NewThread describes a comment to add on a line or a range of lines.
type NewThread struct {
	Path      string
	Line      int
	Side      string
	StartLine int // 0 for a single line
	StartSide string
	Body      string
}

// AddThread adds a thread to the pending review.
func (c *Client) AddThread(ctx context.Context, reviewID string, t NewThread) error {
	var m struct {
		AddPullRequestReviewThread struct {
			Thread struct{ ID string }
		} `graphql:"addPullRequestReviewThread(input: $input)"`
	}
	side := githubv4.DiffSide(t.Side)
	in := githubv4.AddPullRequestReviewThreadInput{
		PullRequestReviewID: githubv4.NewID(githubv4.ID(reviewID)),
		Path:                githubv4.NewString(githubv4.String(t.Path)),
		Body:                githubv4.String(t.Body),
		Line:                githubv4.NewInt(githubv4.Int(t.Line)),
		Side:                &side,
	}
	if t.StartLine != 0 {
		startSide := githubv4.DiffSide(t.StartSide)
		in.StartLine = githubv4.NewInt(githubv4.Int(t.StartLine))
		in.StartSide = &startSide
	}
	return c.gql.Mutate(ctx, &m, in, nil)
}

func (c *Client) DeleteComment(ctx context.Context, commentID string) error {
	var m struct {
		DeletePullRequestReviewComment struct {
			ClientMutationID string
		} `graphql:"deletePullRequestReviewComment(input: $input)"`
	}
	return c.gql.Mutate(ctx, &m, githubv4.DeletePullRequestReviewCommentInput{ID: githubv4.ID(commentID)}, nil)
}

// SubmitReview submits the pending review, or creates and submits a new one
// when there is none (e.g. approving without comments).
func (c *Client) SubmitReview(ctx context.Context, prID, pendingID string, event githubv4.PullRequestReviewEvent, body string) error {
	var b *githubv4.String
	if body != "" {
		b = githubv4.NewString(githubv4.String(body))
	}
	if pendingID == "" {
		var m struct {
			AddPullRequestReview struct {
				PullRequestReview struct{ ID string }
			} `graphql:"addPullRequestReview(input: $input)"`
		}
		return c.gql.Mutate(ctx, &m, githubv4.AddPullRequestReviewInput{
			PullRequestID: githubv4.ID(prID), Event: &event, Body: b,
		}, nil)
	}
	var m struct {
		SubmitPullRequestReview struct {
			ClientMutationID string
		} `graphql:"submitPullRequestReview(input: $input)"`
	}
	return c.gql.Mutate(ctx, &m, githubv4.SubmitPullRequestReviewInput{
		PullRequestReviewID: githubv4.NewID(githubv4.ID(pendingID)), Event: event, Body: b,
	}, nil)
}
