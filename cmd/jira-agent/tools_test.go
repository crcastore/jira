package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestToolSchemasExposeFocusedGitHubWorkflow(t *testing.T) {
	want := []string{
		"gh_me",
		"gh_list_my_repos",
		"gh_get_repo",
		"gh_list_commits",
		"gh_list_pulls",
		"gh_get_pull",
		"gh_find_pull_requests",
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

func TestToolAliasesResolveMergeRequestLanguage(t *testing.T) {
	cases := map[string]string{
		"gh_list_mrs":            "gh_list_pulls",
		"gh_list_merge_requests": "gh_list_pulls",
		"gh_find_mrs":            "gh_find_pull_requests",
		"gh_list_all_mrs":        "gh_find_pull_requests",
		"find_merge_requests":    "gh_find_pull_requests",
		"list_mrs":               "gh_list_pulls",
		"gh_get_mr":              "gh_get_pull",
		"gh_get_merge_request":   "gh_get_pull",
		"get_mr":                 "gh_get_pull",
	}
	for input, want := range cases {
		if got := canonicalToolName(input); got != want {
			t.Errorf("canonicalToolName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestGitHubFindPullRequestsScansRepos(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/user/repos":
			query := request.URL.Query()
			checks := map[string]string{
				"visibility": "all",
				"sort":       "pushed",
				"per_page":   "100",
				"page":       "1",
			}
			for key, want := range checks {
				if got := query.Get(key); got != want {
					t.Errorf("query[%s] = %q, want %q", key, got, want)
				}
			}
			_, _ = response.Write([]byte(`[
				{"full_name":"octo/hello"},
				{"full_name":"acme/world"}
			]`))
		case "/repos/octo/hello/pulls":
			query := request.URL.Query()
			if got := query.Get("state"); got != "all" {
				t.Errorf("state = %q, want all", got)
			}
			if got := query.Get("per_page"); got != "50" {
				t.Errorf("per_page = %q, want 50", got)
			}
			_, _ = response.Write([]byte(`[
				{
					"number": 12,
					"title": "Add MR search",
					"state": "open",
					"user": {"login": "octocat"},
					"head": {"ref": "mr-search"},
					"base": {"ref": "main"},
					"html_url": "https://github.com/octo/hello/pull/12"
				}
			]`))
		case "/repos/acme/world/pulls":
			_, _ = response.Write([]byte(`[]`))
		default:
			t.Errorf("unexpected path %q", request.URL.Path)
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	client := &GitHubClient{baseURL: server.URL, token: "token", http: server.Client()}
	raw, err := client.FindPullRequests("all", 2, 50)
	if err != nil {
		t.Fatalf("FindPullRequests returned error: %v", err)
	}
	var got struct {
		State          string           `json:"state"`
		ReposScanned   int              `json:"repos_scanned"`
		ReposWithPulls int              `json:"repos_with_pulls"`
		Count          int              `json:"count"`
		Items          []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal raw pull search: %v", err)
	}
	if got.State != "all" || got.ReposScanned != 2 || got.ReposWithPulls != 1 || got.Count != 1 {
		t.Fatalf("unexpected summary: %+v", got)
	}
	if got.Items[0]["repository"] != "octo/hello" || got.Items[0]["number"].(float64) != 12 {
		t.Fatalf("unexpected item: %v", got.Items[0])
	}
}

func TestGitHubListCommitsBuildsRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/repos/octo/hello/commits" {
			t.Errorf("path = %q, want /repos/octo/hello/commits", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer token" {
			t.Errorf("Authorization header = %q", request.Header.Get("Authorization"))
		}
		query := request.URL.Query()
		checks := map[string]string{
			"sha":      "main",
			"path":     "cmd/main.go",
			"author":   "octocat",
			"since":    "2026-01-01T00:00:00Z",
			"until":    "2026-02-01T00:00:00Z",
			"per_page": "100",
		}
		for key, want := range checks {
			if got := query.Get(key); got != want {
				t.Errorf("query[%s] = %q, want %q", key, got, want)
			}
		}
		_, _ = response.Write([]byte(`[{"sha":"abc123"}]`))
	}))
	defer server.Close()

	client := &GitHubClient{baseURL: server.URL, token: "token", http: server.Client()}
	raw, err := client.ListCommits(
		"octo",
		"hello",
		"main",
		"cmd/main.go",
		"octocat",
		"2026-01-01T00:00:00Z",
		"2026-02-01T00:00:00Z",
		200,
	)
	if err != nil {
		t.Fatalf("ListCommits returned error: %v", err)
	}
	var commits []map[string]any
	if err := json.Unmarshal(raw, &commits); err != nil {
		t.Fatalf("unmarshal raw commits: %v", err)
	}
	if len(commits) != 1 || commits[0]["sha"] != "abc123" {
		t.Fatalf("unexpected commits: %v", commits)
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
			"sha": "abc123def456",
			"node_id": "noise",
			"commit": {
				"author": {"name": "Octo Cat", "email": "octo@example.com", "date": "2026-01-02T03:04:05Z"},
				"committer": {"name": "Hub Bot", "email": "bot@example.com", "date": "2026-01-02T04:05:06Z"},
				"message": "Add commit listing\n\nLonger body should be hidden",
				"tree": {"sha": "tree-sha"}
			},
			"author": {"login": "octocat", "id": 1},
			"committer": {"login": "hubot", "id": 2},
			"html_url": "https://github.com/octo/hello/commit/abc123def456",
			"parents": [{"sha": "parent"}]
		}
	]`)

	out := trimGitHub("gh_list_commits", in)

	var got struct {
		Count int              `json:"count"`
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("trimmed output is not the expected shape: %v\nraw=%s", err, string(out))
	}
	if got.Count != 1 || len(got.Items) != 1 {
		t.Fatalf("want one commit, got count=%d items=%d", got.Count, len(got.Items))
	}
	commit := got.Items[0]
	checks := map[string]any{
		"sha":          "abc123def456",
		"message":      "Add commit listing",
		"author":       "octocat",
		"author_name":  "Octo Cat",
		"authored_at":  "2026-01-02T03:04:05Z",
		"committer":    "hubot",
		"committed_at": "2026-01-02T04:05:06Z",
		"html_url":     "https://github.com/octo/hello/commit/abc123def456",
	}
	for key, want := range checks {
		if got := commit[key]; got != want {
			t.Errorf("%s = %v, want %v", key, got, want)
		}
	}
	for _, key := range []string{"node_id", "parents", "tree"} {
		if _, ok := commit[key]; ok {
			t.Errorf("expected field %q to be stripped, but it is present", key)
		}
	}
}

func TestTrimGitHubFindPullRequests(t *testing.T) {
	in := json.RawMessage(`{
		"state": "open",
		"max_repos": 50,
		"per_repo": 10,
		"repos_scanned": 2,
		"repos_with_pulls": 1,
		"count": 1,
		"items": [
			{
				"repository": "octo/hello",
				"number": 12,
				"title": "Add MR search",
				"state": "open",
				"draft": false,
				"user": {"login": "octocat", "id": 1},
				"head": {"ref": "mr-search", "sha": "noise"},
				"base": {"ref": "main", "sha": "noise"},
				"html_url": "https://github.com/octo/hello/pull/12",
				"body": "long body should be hidden"
			}
		]
	}`)

	out := trimGitHub("gh_find_pull_requests", in)

	var got struct {
		State        string           `json:"state"`
		ReposScanned int              `json:"repos_scanned"`
		Count        int              `json:"count"`
		Items        []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("trimmed output is not the expected shape: %v\nraw=%s", err, string(out))
	}
	if got.State != "open" || got.ReposScanned != 2 || got.Count != 1 || len(got.Items) != 1 {
		t.Fatalf("unexpected summary: %+v", got)
	}
	pull := got.Items[0]
	checks := map[string]any{
		"repo":     "octo/hello",
		"number":   float64(12),
		"title":    "Add MR search",
		"state":    "open",
		"user":     "octocat",
		"head":     "mr-search",
		"base":     "main",
		"html_url": "https://github.com/octo/hello/pull/12",
	}
	for key, want := range checks {
		if got := pull[key]; got != want {
			t.Errorf("%s = %v, want %v", key, got, want)
		}
	}
	for _, key := range []string{"body", "repository"} {
		if _, ok := pull[key]; ok {
			t.Errorf("expected field %q to be stripped, but it is present", key)
		}
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
