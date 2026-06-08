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

// ListMyRepos returns all repositories the authenticated user has access to,
// auto-paginating through GitHub's per_page=100 cap. maxTotal bounds the total
// number of repos returned (use 0 for the default of 300).
func (c *GitHubClient) ListMyRepos(visibility, sort string, maxTotal int) (json.RawMessage, error) {
	if maxTotal <= 0 {
		maxTotal = 300
	}
	out := make([]any, 0, 100)
	page := 1
	for len(out) < maxTotal {
		q := url.Values{}
		if visibility != "" {
			q.Set("visibility", visibility)
		}
		if sort != "" {
			q.Set("sort", sort)
		}
		q.Set("per_page", "100")
		q.Set("page", fmt.Sprintf("%d", page))
		raw, err := c.request("GET", "/user/repos", q, nil)
		if err != nil {
			return nil, err
		}
		var batch []any
		if err := json.Unmarshal(raw, &batch); err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}
		out = append(out, batch...)
		if len(batch) < 100 {
			break
		}
		page++
	}
	if len(out) > maxTotal {
		out = out[:maxTotal]
	}
	return json.Marshal(out)
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

// ---------- Workflows (GitHub Actions) ----------

func (c *GitHubClient) ListWorkflows(owner, repo string) (json.RawMessage, error) {
	return c.request("GET", "/repos/"+owner+"/"+repo+"/actions/workflows", nil, nil)
}

// RunWorkflow triggers a workflow_dispatch event. workflowID is either the
// numeric ID or the filename (e.g. "test.yml"). ref is the branch/tag/sha.
func (c *GitHubClient) RunWorkflow(owner, repo, workflowID, ref string, inputs map[string]any) (json.RawMessage, error) {
	body := map[string]any{"ref": ref}
	if len(inputs) > 0 {
		body["inputs"] = inputs
	}
	return c.request("POST",
		fmt.Sprintf("/repos/%s/%s/actions/workflows/%s/dispatches", owner, repo, workflowID),
		nil, body)
}

func (c *GitHubClient) ListWorkflowRuns(owner, repo, workflowID, status, branch string, perPage int) (json.RawMessage, error) {
	q := url.Values{}
	if status != "" {
		q.Set("status", status)
	}
	if branch != "" {
		q.Set("branch", branch)
	}
	q.Set("per_page", fmt.Sprintf("%d", clampPerPage(perPage, 10)))
	path := "/repos/" + owner + "/" + repo + "/actions/runs"
	if workflowID != "" {
		path = "/repos/" + owner + "/" + repo + "/actions/workflows/" + workflowID + "/runs"
	}
	return c.request("GET", path, q, nil)
}

func (c *GitHubClient) GetWorkflowRun(owner, repo string, runID int) (json.RawMessage, error) {
	return c.request("GET", fmt.Sprintf("/repos/%s/%s/actions/runs/%d", owner, repo, runID), nil, nil)
}

// WaitForWorkflowRun polls GetWorkflowRun until the run's status is "completed"
// or the timeout elapses. Returns the final run payload (or the last one fetched
// on timeout) and ok=true if the run actually completed.
func (c *GitHubClient) WaitForWorkflowRun(owner, repo string, runID, timeoutSec, intervalSec int) (json.RawMessage, bool, error) {
	if timeoutSec <= 0 {
		timeoutSec = 600
	}
	if timeoutSec > 1800 {
		timeoutSec = 1800
	}
	if intervalSec <= 0 {
		intervalSec = 5
	}
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
	var last json.RawMessage
	for {
		raw, err := c.GetWorkflowRun(owner, repo, runID)
		if err != nil {
			return last, false, err
		}
		last = raw
		var run struct {
			Status string `json:"status"`
		}
		_ = json.Unmarshal(raw, &run)
		if run.Status == "completed" {
			return raw, true, nil
		}
		if time.Now().After(deadline) {
			return raw, false, nil
		}
		time.Sleep(time.Duration(intervalSec) * time.Second)
	}
}

// RunWorkflowAndWait dispatches a workflow_dispatch event, then polls
// list_workflow_runs to find the freshly created run (newer than the dispatch
// timestamp), then waits for it to complete.
func (c *GitHubClient) RunWorkflowAndWait(owner, repo, workflowID, ref string, inputs map[string]any, timeoutSec, intervalSec int) (json.RawMessage, bool, error) {
	startedAt := time.Now().UTC().Add(-2 * time.Second)
	if _, err := c.RunWorkflow(owner, repo, workflowID, ref, inputs); err != nil {
		return nil, false, err
	}
	if timeoutSec <= 0 {
		timeoutSec = 600
	}
	if timeoutSec > 1800 {
		timeoutSec = 1800
	}
	if intervalSec <= 0 {
		intervalSec = 5
	}
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)

	// Step 1: find the run id created after startedAt.
	var runID int
	for runID == 0 {
		if time.Now().After(deadline) {
			return nil, false, fmt.Errorf("timed out waiting for new workflow run to appear")
		}
		raw, err := c.ListWorkflowRuns(owner, repo, workflowID, "", ref, 10)
		if err != nil {
			return nil, false, err
		}
		var payload struct {
			WorkflowRuns []struct {
				ID        int    `json:"id"`
				Event     string `json:"event"`
				CreatedAt string `json:"created_at"`
			} `json:"workflow_runs"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			return nil, false, err
		}
		for _, r := range payload.WorkflowRuns {
			if r.Event != "workflow_dispatch" {
				continue
			}
			t, err := time.Parse(time.RFC3339, r.CreatedAt)
			if err == nil && t.After(startedAt) {
				runID = r.ID
				break
			}
		}
		if runID == 0 {
			time.Sleep(time.Duration(intervalSec) * time.Second)
		}
	}

	// Step 2: wait for completion with the remaining budget.
	remaining := int(time.Until(deadline).Seconds())
	if remaining < 10 {
		remaining = 10
	}
	return c.WaitForWorkflowRun(owner, repo, runID, remaining, intervalSec)
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
