package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestToolSchemasExposeFocusedGitHubIssueWorkflow(t *testing.T) {
	want := []string{
		"gh_me",
		"gh_list_my_repos",
		"gh_get_repo",
		"gh_list_issues",
		"gh_get_issue",
		"gh_list_pr_files",
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

func TestToolAliasesResolveMergeRequestFileLanguage(t *testing.T) {
	cases := map[string]string{
		"gh_list_mr_files":            "gh_list_pr_files",
		"gh_list_merge_request_files": "gh_list_pr_files",
		"gh_mr_changed_files":         "gh_list_pr_files",
		"list_mr_files":               "gh_list_pr_files",
		"changed_files":               "gh_list_pr_files",
		"mr_changed_files":            "gh_list_pr_files",
	}
	for input, want := range cases {
		if got := canonicalToolName(input); got != want {
			t.Errorf("canonicalToolName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestGitHubListPullFilesBuildsRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/repos/octo/hello/pulls/12/files" {
			t.Errorf("path = %q, want /repos/octo/hello/pulls/12/files", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer token" {
			t.Errorf("Authorization header = %q", request.Header.Get("Authorization"))
		}
		if got := request.URL.Query().Get("per_page"); got != "100" {
			t.Errorf("per_page = %q, want 100", got)
		}
		_, _ = response.Write([]byte(`[{"filename":"cmd/main.go"}]`))
	}))
	defer server.Close()

	client := &GitHubClient{baseURL: server.URL, token: "token", http: server.Client()}
	raw, err := client.ListPullFiles("octo", "hello", 12, 250)
	if err != nil {
		t.Fatalf("ListPullFiles returned error: %v", err)
	}
	var files []map[string]any
	if err := json.Unmarshal(raw, &files); err != nil {
		t.Fatalf("unmarshal raw files: %v", err)
	}
	if len(files) != 1 || files[0]["filename"] != "cmd/main.go" {
		t.Fatalf("unexpected files: %v", files)
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

func TestTrimGitHubListPRFiles(t *testing.T) {
	in := json.RawMessage(`[
		{
			"filename": "cmd/main.go",
			"status": "modified",
			"additions": 12,
			"deletions": 3,
			"changes": 15,
			"patch": "large diff should be hidden",
			"raw_url": "https://example.com/raw"
		}
	]`)

	out := trimGitHub("gh_list_pr_files", in)

	var got struct {
		Count int              `json:"count"`
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("trimmed output is not the expected shape: %v\nraw=%s", err, string(out))
	}
	if got.Count != 1 || len(got.Items) != 1 {
		t.Fatalf("want one file, got count=%d items=%d", got.Count, len(got.Items))
	}
	file := got.Items[0]
	checks := map[string]any{
		"filename":  "cmd/main.go",
		"status":    "modified",
		"additions": float64(12),
		"deletions": float64(3),
		"changes":   float64(15),
	}
	for key, want := range checks {
		if got := file[key]; got != want {
			t.Errorf("%s = %v, want %v", key, got, want)
		}
	}
	for _, key := range []string{"patch", "raw_url"} {
		if _, ok := file[key]; ok {
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
