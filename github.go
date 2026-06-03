package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

// GitHubClient is a thin wrapper around the GitHub REST API (v3).
type GitHubClient struct {
	baseURL string
	token   string
	http    *http.Client
}

func NewGitHubClient() (*GitHubClient, error) {
	token := firstNonEmpty(os.Getenv("GITHUB_TOKEN"), os.Getenv("GH_TOKEN"))
	if token == "" {
		return nil, fmt.Errorf("missing GITHUB_TOKEN")
	}
	return &GitHubClient{
		baseURL: envOr("GITHUB_API_URL", "https://api.github.com"),
		token:   token,
		http:    &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (c *GitHubClient) request(method, path string, query url.Values, body any) (json.RawMessage, error) {
	var buf io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		buf = bytes.NewReader(b)
	}
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequest(method, u, buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "jira-agent-go")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("github %s %s -> %d: %s", method, path, resp.StatusCode, string(data))
	}
	if resp.StatusCode == 204 || len(data) == 0 {
		return json.RawMessage("{}"), nil
	}
	return json.RawMessage(data), nil
}

// ---------- Users ----------

func (c *GitHubClient) Me() (json.RawMessage, error) {
	return c.request("GET", "/user", nil, nil)
}

// ---------- Repos ----------

func (c *GitHubClient) ListMyRepos(visibility, sort string, perPage int) (json.RawMessage, error) {
	q := url.Values{}
	if visibility != "" {
		q.Set("visibility", visibility)
	}
	if sort != "" {
		q.Set("sort", sort)
	}
	q.Set("per_page", fmt.Sprintf("%d", clampPerPage(perPage, 30)))
	return c.request("GET", "/user/repos", q, nil)
}

func (c *GitHubClient) GetRepo(owner, repo string) (json.RawMessage, error) {
	return c.request("GET", "/repos/"+owner+"/"+repo, nil, nil)
}

func (c *GitHubClient) SearchRepos(query string, perPage int) (json.RawMessage, error) {
	q := url.Values{}
	q.Set("q", query)
	q.Set("per_page", fmt.Sprintf("%d", clampPerPage(perPage, 10)))
	return c.request("GET", "/search/repositories", q, nil)
}

// ---------- Issues ----------

func (c *GitHubClient) ListIssues(owner, repo, state, labels, assignee string, perPage int) (json.RawMessage, error) {
	q := url.Values{}
	if state != "" {
		q.Set("state", state)
	}
	if labels != "" {
		q.Set("labels", labels)
	}
	if assignee != "" {
		q.Set("assignee", assignee)
	}
	q.Set("per_page", fmt.Sprintf("%d", clampPerPage(perPage, 25)))
	return c.request("GET", "/repos/"+owner+"/"+repo+"/issues", q, nil)
}

func (c *GitHubClient) GetIssue(owner, repo string, number int) (json.RawMessage, error) {
	return c.request("GET", fmt.Sprintf("/repos/%s/%s/issues/%d", owner, repo, number), nil, nil)
}

type GHCreateIssueArgs struct {
	Owner     string   `json:"owner"`
	Repo      string   `json:"repo"`
	Title     string   `json:"title"`
	Body      string   `json:"body,omitempty"`
	Assignees []string `json:"assignees,omitempty"`
	Labels    []string `json:"labels,omitempty"`
}

func (c *GitHubClient) CreateIssue(a GHCreateIssueArgs) (json.RawMessage, error) {
	body := map[string]any{"title": a.Title}
	if a.Body != "" {
		body["body"] = a.Body
	}
	if len(a.Assignees) > 0 {
		body["assignees"] = a.Assignees
	}
	if len(a.Labels) > 0 {
		body["labels"] = a.Labels
	}
	return c.request("POST", "/repos/"+a.Owner+"/"+a.Repo+"/issues", nil, body)
}

func (c *GitHubClient) UpdateIssue(owner, repo string, number int, fields map[string]any) (json.RawMessage, error) {
	return c.request("PATCH", fmt.Sprintf("/repos/%s/%s/issues/%d", owner, repo, number), nil, fields)
}

func (c *GitHubClient) CloseIssue(owner, repo string, number int) (json.RawMessage, error) {
	return c.UpdateIssue(owner, repo, number, map[string]any{"state": "closed"})
}

func (c *GitHubClient) CommentIssue(owner, repo string, number int, body string) (json.RawMessage, error) {
	return c.request("POST", fmt.Sprintf("/repos/%s/%s/issues/%d/comments", owner, repo, number),
		nil, map[string]string{"body": body})
}

func (c *GitHubClient) SearchIssues(query string, perPage int) (json.RawMessage, error) {
	q := url.Values{}
	q.Set("q", query)
	q.Set("per_page", fmt.Sprintf("%d", clampPerPage(perPage, 25)))
	return c.request("GET", "/search/issues", q, nil)
}

// ---------- Pull Requests ----------

func (c *GitHubClient) ListPulls(owner, repo, state string, perPage int) (json.RawMessage, error) {
	q := url.Values{}
	if state != "" {
		q.Set("state", state)
	}
	q.Set("per_page", fmt.Sprintf("%d", clampPerPage(perPage, 25)))
	return c.request("GET", "/repos/"+owner+"/"+repo+"/pulls", q, nil)
}

func (c *GitHubClient) GetPull(owner, repo string, number int) (json.RawMessage, error) {
	return c.request("GET", fmt.Sprintf("/repos/%s/%s/pulls/%d", owner, repo, number), nil, nil)
}

type GHCreatePullArgs struct {
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
	Title string `json:"title"`
	Head  string `json:"head"`
	Base  string `json:"base"`
	Body  string `json:"body,omitempty"`
	Draft bool   `json:"draft,omitempty"`
}

func (c *GitHubClient) CreatePull(a GHCreatePullArgs) (json.RawMessage, error) {
	body := map[string]any{"title": a.Title, "head": a.Head, "base": a.Base, "draft": a.Draft}
	if a.Body != "" {
		body["body"] = a.Body
	}
	return c.request("POST", "/repos/"+a.Owner+"/"+a.Repo+"/pulls", nil, body)
}

func (c *GitHubClient) MergePull(owner, repo string, number int, method, commitTitle, commitMessage string) (json.RawMessage, error) {
	body := map[string]any{}
	if method != "" {
		body["merge_method"] = method // merge | squash | rebase
	}
	if commitTitle != "" {
		body["commit_title"] = commitTitle
	}
	if commitMessage != "" {
		body["commit_message"] = commitMessage
	}
	return c.request("PUT", fmt.Sprintf("/repos/%s/%s/pulls/%d/merge", owner, repo, number), nil, body)
}

func (c *GitHubClient) ListPullFiles(owner, repo string, number, perPage int) (json.RawMessage, error) {
	q := url.Values{}
	q.Set("per_page", fmt.Sprintf("%d", clampPerPage(perPage, 30)))
	return c.request("GET", fmt.Sprintf("/repos/%s/%s/pulls/%d/files", owner, repo, number), q, nil)
}

func (c *GitHubClient) ReviewPull(owner, repo string, number int, event, body string) (json.RawMessage, error) {
	payload := map[string]any{}
	if event != "" {
		payload["event"] = event // APPROVE | REQUEST_CHANGES | COMMENT
	}
	if body != "" {
		payload["body"] = body
	}
	return c.request("POST", fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews", owner, repo, number), nil, payload)
}

// Issue comment endpoint works for PRs too.
func (c *GitHubClient) CommentPull(owner, repo string, number int, body string) (json.RawMessage, error) {
	return c.CommentIssue(owner, repo, number, body)
}

// ---------- helpers ----------

func clampPerPage(v, def int) int {
	if v <= 0 {
		return def
	}
	if v > 100 {
		return 100
	}
	return v
}
