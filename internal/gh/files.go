package gh

import (
	"bytes"
	"context"
	"fmt"

	"github.com/shurcooL/githubv4"
)

// FileContent returns the raw content of path at the given commit.
// binary is true when the file contains NUL bytes.
func (c *Client) FileContent(ctx context.Context, oid, path string) (content string, binary bool, err error) {
	p := fmt.Sprintf("/repos/%s/%s/contents/%s?ref=%s", c.Owner, c.Name, escapePath(path), oid)
	body, err := c.rest(ctx, p, "application/vnd.github.raw+json", nil)
	if err != nil {
		return "", false, err
	}
	if bytes.IndexByte(body[:min(len(body), 8000)], 0) >= 0 {
		return "", true, nil
	}
	return string(body), false, nil
}

func (c *Client) SetViewed(ctx context.Context, prID, path string, viewed bool) error {
	if viewed {
		var m struct {
			MarkFileAsViewed struct {
				ClientMutationID string
			} `graphql:"markFileAsViewed(input: $input)"`
		}
		return c.gql.Mutate(ctx, &m, githubv4.MarkFileAsViewedInput{
			PullRequestID: githubv4.ID(prID), Path: githubv4.String(path),
		}, nil)
	}
	var m struct {
		UnmarkFileAsViewed struct {
			ClientMutationID string
		} `graphql:"unmarkFileAsViewed(input: $input)"`
	}
	return c.gql.Mutate(ctx, &m, githubv4.UnmarkFileAsViewedInput{
		PullRequestID: githubv4.ID(prID), Path: githubv4.String(path),
	}, nil)
}
