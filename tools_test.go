package main

import (
	"encoding/json"
	"testing"
)

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
		Count int                      `json:"count"`
		Items []map[string]any         `json:"items"`
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
