package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/ccastorena/jira-agent/jiracreate"
)

// JiraClient is a thin wrapper around the Jira Cloud REST API v3.
type JiraClient struct {
	baseURL string
	email   string
	token   string
	http    *http.Client
}

func NewJiraClient() (*JiraClient, error) {
	base, err := normalizeJiraBaseURL(os.Getenv("JIRA_BASE_URL"))
	if err != nil {
		return nil, err
	}
	email := os.Getenv("JIRA_EMAIL")
	token := os.Getenv("JIRA_API_TOKEN")
	if email == "" || token == "" {
		return nil, fmt.Errorf("missing JIRA_EMAIL or JIRA_API_TOKEN")
	}
	return &JiraClient{
		baseURL: base,
		email:   email,
		token:   token,
		http:    &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func normalizeJiraBaseURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("missing JIRA_BASE_URL")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("JIRA_BASE_URL must be a full Jira site URL like https://your-domain.atlassian.net")
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "admin.atlassian.com" || host == "home.atlassian.com" || !strings.HasSuffix(host, ".atlassian.net") {
		return "", fmt.Errorf("JIRA_BASE_URL must be your Jira site URL like https://your-domain.atlassian.net, not %s", strings.TrimRight(trimmed, "/"))
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}

func (c *JiraClient) request(method, path string, query url.Values, body any) (json.RawMessage, error) {
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
	req.SetBasicAuth(c.email, c.token)
	req.Header.Set("Accept", "application/json")
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
		return nil, fmt.Errorf("jira %s %s -> %d: %s", method, path, resp.StatusCode, string(data))
	}
	if resp.StatusCode == 204 || len(data) == 0 {
		return json.RawMessage("{}"), nil
	}
	if !json.Valid(data) {
		contentType := resp.Header.Get("Content-Type")
		preview := strings.TrimSpace(string(data))
		if len(preview) > 240 {
			preview = preview[:240] + "..."
		}
		return nil, fmt.Errorf("jira %s %s returned non-JSON response (status %d, content-type %q): %s", method, path, resp.StatusCode, contentType, preview)
	}
	return json.RawMessage(data), nil
}

// ---------- Issues ----------

func (c *JiraClient) Search(jql string, fields []string, maxResults int) (json.RawMessage, error) {
	if len(fields) == 0 {
		fields = []string{"summary", "status", "assignee", "priority", "issuetype", "updated"}
	}
	if maxResults <= 0 {
		maxResults = 25
	}
	query := url.Values{}
	query.Set("jql", jql)
	query.Set("fields", strings.Join(fields, ","))
	query.Set("maxResults", fmt.Sprint(maxResults))
	return c.request("GET", "/rest/api/3/search/jql", query, nil)
}

func (c *JiraClient) GetIssue(key string) (json.RawMessage, error) {
	return c.request("GET", "/rest/api/3/issue/"+key, nil, nil)
}

func (c *JiraClient) ListIssueTypes(projectKey string) (json.RawMessage, error) {
	return c.request("GET", "/rest/api/3/issue/createmeta/"+url.PathEscape(projectKey)+"/issuetypes", nil, nil)
}

type CreateIssueArgs = jiracreate.CreateIssueArgs

func (c *JiraClient) CreateIssue(a CreateIssueArgs) (json.RawMessage, error) {
	if a.IssueType == "" {
		a.IssueType = "Task"
	}
	fields := map[string]any{
		"project":   map[string]string{"key": a.ProjectKey},
		"summary":   a.Summary,
		"issuetype": map[string]string{"name": a.IssueType},
	}
	if a.ParentID != "" {
		fields["parent"] = map[string]string{"id": a.ParentID}
	} else if a.ParentKey != "" {
		fields["parent"] = map[string]string{"key": a.ParentKey}
	}
	if a.Description != "" {
		fields["description"] = adf(a.Description)
	}
	if a.AssigneeAccountID != "" {
		fields["assignee"] = map[string]string{"accountId": a.AssigneeAccountID}
	}
	if a.ReporterAccountID != "" {
		fields["reporter"] = map[string]string{"accountId": a.ReporterAccountID}
	}
	if a.Priority != "" {
		fields["priority"] = map[string]string{"name": a.Priority}
	}
	if len(a.Labels) > 0 {
		fields["labels"] = a.Labels
	}
	return c.request("POST", "/rest/api/3/issue", nil, map[string]any{"fields": fields})
}

func (c *JiraClient) UpdateIssue(key string, fields map[string]any) (json.RawMessage, error) {
	return c.request("PUT", "/rest/api/3/issue/"+key, nil, map[string]any{"fields": fields})
}

func (c *JiraClient) AddComment(key, body string) (json.RawMessage, error) {
	return c.request("POST", "/rest/api/3/issue/"+key+"/comment", nil, map[string]any{"body": adf(body)})
}

func (c *JiraClient) ListTransitions(key string) (json.RawMessage, error) {
	return c.request("GET", "/rest/api/3/issue/"+key+"/transitions", nil, nil)
}

func (c *JiraClient) TransitionIssue(key, transitionID string) (json.RawMessage, error) {
	return c.request("POST", "/rest/api/3/issue/"+key+"/transitions", nil,
		map[string]any{"transition": map[string]string{"id": transitionID}})
}

// ---------- Lookups ----------

func (c *JiraClient) Myself() (json.RawMessage, error) {
	return c.request("GET", "/rest/api/3/myself", nil, nil)
}

func (c *JiraClient) SearchUsers(query string, maxResults int) (json.RawMessage, error) {
	if maxResults <= 0 {
		maxResults = 10
	}
	q := url.Values{}
	q.Set("query", query)
	q.Set("maxResults", fmt.Sprintf("%d", maxResults))
	return c.request("GET", "/rest/api/3/user/search", q, nil)
}

func (c *JiraClient) SearchAssignableUsers(projectKey, query string, maxResults int) (json.RawMessage, error) {
	if maxResults <= 0 {
		maxResults = 20
	}
	q := url.Values{}
	q.Set("project", projectKey)
	if query != "" {
		q.Set("query", query)
	}
	q.Set("maxResults", fmt.Sprintf("%d", maxResults))
	return c.request("GET", "/rest/api/3/user/assignable/search", q, nil)
}

func (c *JiraClient) ListProjects() (json.RawMessage, error) {
	raw, err := c.request("GET", "/rest/api/3/project/search", nil, nil)
	if err != nil {
		return nil, err
	}
	var wrap struct {
		Values json.RawMessage `json:"values"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return raw, nil
	}
	if len(wrap.Values) == 0 {
		return json.RawMessage("[]"), nil
	}
	return wrap.Values, nil
}

// adf wraps plain text in Atlassian Document Format (required by API v3).
func adf(text string) map[string]any {
	return map[string]any{
		"type":    "doc",
		"version": 1,
		"content": []any{
			map[string]any{
				"type":    "paragraph",
				"content": []any{map[string]any{"type": "text", "text": text}},
			},
		},
	}
}
