package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGitHubListCommitsBuildsRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/repos/octo/hello/commits" {
			t.Errorf("path = %s, want /repos/octo/hello/commits", r.URL.Path)
		}
		query := r.URL.Query()
		want := map[string]string{
			"sha":      "main",
			"path":     "README.md",
			"author":   "ada",
			"since":    "2026-01-01T00:00:00Z",
			"until":    "2026-01-31T00:00:00Z",
			"per_page": "7",
		}
		for key, value := range want {
			if got := query.Get(key); got != value {
				t.Errorf("query %s = %q, want %q", key, got, value)
			}
		}
		if got := r.Header.Get("Accept"); got != "application/vnd.github+json" {
			t.Errorf("Accept = %q", got)
		}
		if got := r.Header.Get("X-GitHub-Api-Version"); got != "2022-11-28" {
			t.Errorf("X-GitHub-Api-Version = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	client := &GitHubClient{baseURL: server.URL, token: "token", http: server.Client()}
	raw, err := client.ListCommits("octo", "hello", "main", "README.md", "ada", "2026-01-01T00:00:00Z", "2026-01-31T00:00:00Z", 7)
	if err != nil {
		t.Fatalf("ListCommits returned error: %v", err)
	}
	var commits []json.RawMessage
	if err := json.Unmarshal(raw, &commits); err != nil {
		t.Fatalf("ListCommits returned invalid JSON: %v", err)
	}
}

func TestGitHubListCommitsClampsPerPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("per_page"); got != "100" {
			t.Errorf("per_page = %q, want 100", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	client := &GitHubClient{baseURL: server.URL, token: "token", http: server.Client()}
	if _, err := client.ListCommits("octo", "hello", "", "", "", "", "", 500); err != nil {
		t.Fatalf("ListCommits returned error: %v", err)
	}
}
