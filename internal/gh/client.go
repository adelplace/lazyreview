// Package gh wraps the GitHub GraphQL and REST APIs used by lazyreviewer.
package gh

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"
)

const restBase = "https://api.github.com"

// Client talks to a single GitHub repository.
type Client struct {
	Owner, Name string
	gql         *githubv4.Client
	http        *http.Client
}

// Token returns GITHUB_TOKEN / GH_TOKEN, or falls back to `gh auth token`.
func Token() (string, error) {
	for _, k := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
		if t := os.Getenv(k); t != "" {
			return t, nil
		}
	}
	out, err := exec.Command("gh", "auth", "token").Output()
	if err != nil {
		return "", fmt.Errorf("no GITHUB_TOKEN set and `gh auth token` failed: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

var remoteRe = regexp.MustCompile(`github\.com[:/]([^/]+)/([^/]+?)(?:\.git)?/?$`)

// ParseRepo parses "owner/name" or a GitHub remote URL.
func ParseRepo(s string) (owner, name string, err error) {
	s = strings.TrimSpace(s)
	if m := remoteRe.FindStringSubmatch(s); m != nil {
		return m[1], m[2], nil
	}
	if o, n, ok := strings.Cut(s, "/"); ok && o != "" && n != "" && !strings.Contains(n, "/") {
		return o, n, nil
	}
	return "", "", fmt.Errorf("cannot parse GitHub repository from %q", s)
}

// RepoFromGit reads the "origin" remote of the git repository in the cwd.
func RepoFromGit() (owner, name string, err error) {
	out, err := exec.Command("git", "remote", "get-url", "origin").Output()
	if err != nil {
		return "", "", fmt.Errorf("not in a git repository with an origin remote (use --repo owner/name)")
	}
	return ParseRepo(string(out))
}

func New(token, owner, name string) *Client {
	hc := oauth2.NewClient(context.Background(), oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token}))
	hc.Timeout = 30 * time.Second
	return &Client{Owner: owner, Name: name, gql: githubv4.NewClient(hc), http: hc}
}

func (c *Client) repoVars() map[string]any {
	return map[string]any{
		"owner": githubv4.String(c.Owner),
		"name":  githubv4.String(c.Name),
	}
}

// rest performs a GET on the REST API. When out is nil the raw body is returned.
func (c *Client) rest(ctx context.Context, path, accept string, out any) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, restBase+path, nil)
	if err != nil {
		return nil, err
	}
	if accept == "" {
		accept = "application/vnd.github+json"
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		var e struct{ Message string }
		_ = json.Unmarshal(body, &e)
		return nil, fmt.Errorf("GET %s: %s: %s", path, resp.Status, e.Message)
	}
	if out != nil {
		return nil, json.Unmarshal(body, out)
	}
	return body, nil
}

func escapePath(p string) string {
	parts := strings.Split(p, "/")
	for i, s := range parts {
		parts[i] = url.PathEscape(s)
	}
	return strings.Join(parts, "/")
}
