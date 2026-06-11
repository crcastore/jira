package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestToolSchemasExposeFocusedGitHubIssueWorkflow(t *testing.T) {
	want := []string{
		"gh_me",
		"gh_list_my_repos",
		"gh_get_repo",
		"gh_list_commits",
		"gh_list_issues",
		"gh_get_issue",
		"gh_create_issue",
		"gh_close_issue",
		"gh_comment_issue",
	}

	if len(ToolSchemas) != len(want) {
		t.Fatalf("ToolSchemas length = %d, want %d", len(ToolSchemas), len(want))
	}
	for i, tool := range ToolSchemas {
		if tool.Function == nil {
			t.Fatalf("ToolSchemas[%d].Function is nil", i)
		}
		if got := tool.Function.Name; got != want[i] {
			t.Fatalf("ToolSchemas[%d] = %q, want %q", i, got, want[i])
		}
	}
}

func TestTrimGitHubListMyRepos(t *testing.T) {
	// A realistic-ish GitHub /user/repos response with extra noise that should be stripped.
	in := json.RawMessage(`[
		{
			"id": 1,
			"full_name": "octo/hello",
			"private": false,
			"description": "first",
			"language": "Go",
			"default_branch": "main",
			"stargazers_count": 42,
			"open_issues_count": 3,
			"updated_at": "2026-01-02T03:04:05Z",
			"html_url": "https://github.com/octo/hello",
			"owner": {"login": "octo", "id": 99, "type": "User"},
			"permissions": {"admin": true, "push": true, "pull": true}
		},
		{
			"id": 2,
			"full_name": "octo/world",
			"private": true,
			"description": null,
			"language": "Python",
			"default_branch": "main",
			"stargazers_count": 0,
			"open_issues_count": 0,
			"updated_at": "2026-02-03T04:05:06Z",
			"html_url": "https://github.com/octo/world"
		}
	]`)

	out := trimGitHub("gh_list_my_repos", in)

	var got struct {
		Count int              `json:"count"`
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("trimmed output is not the expected shape: %v\nraw=%s", err, string(out))
	}
	if got.Count != 2 {
		t.Fatalf("want count=2, got %d", got.Count)
	}
	if len(got.Items) != 2 {
		t.Fatalf("want 2 items, got %d", len(got.Items))
	}
	first := got.Items[0]
	if first["full_name"] != "octo/hello" {
		t.Errorf("full_name: want octo/hello, got %v", first["full_name"])
	}
	if first["language"] != "Go" {
		t.Errorf("language: want Go, got %v", first["language"])
	}
	// stargazers_count is renamed to stargazers
	if v, ok := first["stargazers"]; !ok || v.(float64) != 42 {
		t.Errorf("stargazers: want 42, got %v (present=%v)", v, ok)
	}
	// Noisy fields must be gone.
	for _, k := range []string{"owner", "permissions", "id"} {
		if _, ok := first[k]; ok {
			t.Errorf("expected field %q to be stripped, but it is present", k)
		}
	}
}

func TestTrimGitHubListCommits(t *testing.T) {
	in := json.RawMessage(`[
		{
			"sha": "1234567890abcdef",
			"html_url": "https://github.com/octo/hello/commit/1234567890abcdef",
			"author": {"login": "ada"},
			"committer": {"login": "grace"},
			"commit": {
				"message": "Fix login\n\nLong details should be dropped",
				"author": {"name": "Ada", "email": "ada@example.com", "date": "2026-01-02T03:04:05Z"},
				"committer": {"name": "Grace", "date": "2026-01-02T04:05:06Z"}
			},
			"parents": [{"sha": "noise"}],
			"files": [{"filename": "drop-me"}]
		}
	]`)

	out := trimGitHub("gh_list_commits", in)
	var got struct {
		Count int              `json:"count"`
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("trimmed output invalid: %v\n%s", err, out)
	}
	if got.Count != 1 || len(got.Items) != 1 {
		t.Fatalf("unexpected count/items: %+v", got)
	}
	commit := got.Items[0]
	if commit["sha"] != "1234567890abcdef" || commit["short_sha"] != "1234567890ab" {
		t.Fatalf("unexpected sha fields: %v", commit)
	}
	if commit["message"] != "Fix login" || commit["author"] != "ada" || commit["committer"] != "grace" {
		t.Fatalf("commit not flattened: %v", commit)
	}
	if _, ok := commit["parents"]; ok {
		t.Fatalf("noisy parent data should be dropped: %v", commit)
	}
	if _, ok := commit["files"]; ok {
		t.Fatalf("noisy file data should be dropped: %v", commit)
	}
}

func TestCallToolGitHubListCommits(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/octo/hello/commits" {
			t.Errorf("path = %s, want /repos/octo/hello/commits", r.URL.Path)
		}
		if got := r.URL.Query().Get("sha"); got != "main" {
			t.Errorf("sha = %q, want main", got)
		}
		if got := r.URL.Query().Get("per_page"); got != "1" {
			t.Errorf("per_page = %q, want 1", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{
			"sha":"abcdef1234567890",
			"html_url":"https://github.com/octo/hello/commit/abcdef1234567890",
			"author":{"login":"ada"},
			"commit":{"message":"Initial commit","author":{"name":"Ada","date":"2026-01-02T03:04:05Z"}}
		}]`))
	}))
	defer server.Close()

	client := &GitHubClient{baseURL: server.URL, token: "token", http: server.Client()}
	out := CallTool(nil, client, "gh_list_commits", `{"owner":"octo","repo":"hello","sha":"main","per_page":1}`)
	if strings.Contains(out, "error") {
		t.Fatalf("CallTool returned error: %s", out)
	}
	if !strings.Contains(out, "abcdef1234567890") || !strings.Contains(out, "Initial commit") {
		t.Fatalf("CallTool did not return trimmed commit data: %s", out)
	}
}

func TestPickFields(t *testing.T) {
	in := json.RawMessage(`{"a":1,"b":2,"c":3}`)
	out := pickFields(in, "a", "c", "missing")

	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 keys, got %d (%v)", len(got), got)
	}
	if got["a"].(float64) != 1 || got["c"].(float64) != 3 {
		t.Errorf("unexpected values: %v", got)
	}
	if _, ok := got["missing"]; ok {
		t.Errorf("missing key should not be present")
	}
}
