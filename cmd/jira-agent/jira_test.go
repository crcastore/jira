package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalizeJiraBaseURL(t *testing.T) {
	cases := map[string]string{
		"https://example.atlassian.net":                 "https://example.atlassian.net",
		"https://example.atlassian.net/":                "https://example.atlassian.net",
		"https://example.atlassian.net/browse/SCRUM-6":  "https://example.atlassian.net",
		" https://example.atlassian.net/jira/software ": "https://example.atlassian.net",
	}
	for in, want := range cases {
		got, err := normalizeJiraBaseURL(in)
		if err != nil {
			t.Fatalf("normalizeJiraBaseURL(%q) returned error: %v", in, err)
		}
		if got != want {
			t.Errorf("normalizeJiraBaseURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeJiraBaseURLRejectsNonJiraSites(t *testing.T) {
	for _, in := range []string{
		"",
		"admin.atlassian.com",
		"https://admin.atlassian.com",
		"https://home.atlassian.com/o/site/project",
		"https://example.com",
	} {
		if got, err := normalizeJiraBaseURL(in); err == nil {
			t.Errorf("normalizeJiraBaseURL(%q) = %q, want error", in, got)
		}
	}
}

func TestJiraSearchUsesEnhancedSearchGET(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/rest/api/3/search/jql" {
			t.Errorf("path = %s, want /rest/api/3/search/jql", r.URL.Path)
		}
		query := r.URL.Query()
		if got := query.Get("jql"); got != "assignee = currentUser()" {
			t.Errorf("jql = %q", got)
		}
		if got := query.Get("fields"); got != "summary,status" {
			t.Errorf("fields = %q", got)
		}
		if got := query.Get("maxResults"); got != "40" {
			t.Errorf("maxResults = %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "" {
			t.Errorf("Content-Type = %q, want empty", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		if len(body) != 0 {
			t.Errorf("body = %q, want empty", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"issues":[]}`))
	}))
	defer server.Close()

	client := &JiraClient{
		baseURL: server.URL,
		email:   "me@example.com",
		token:   "token",
		http:    server.Client(),
	}

	raw, err := client.Search("assignee = currentUser()", []string{"summary", "status"}, 40)
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	var payload struct {
		Issues []json.RawMessage `json:"issues"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("Search returned invalid JSON: %v", err)
	}
}

func TestJiraMyselfUsesMyselfEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/rest/api/3/myself" {
			t.Errorf("path = %s, want /rest/api/3/myself", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"displayName":"Ada"}`))
	}))
	defer server.Close()

	client := &JiraClient{
		baseURL: server.URL,
		email:   "me@example.com",
		token:   "token",
		http:    server.Client(),
	}

	if _, err := client.Myself(); err != nil {
		t.Fatalf("Myself returned error: %v", err)
	}
}

func TestJiraRequestReportsNonJSONResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<!doctype html><html><title>Administration</title></html>"))
	}))
	defer server.Close()

	client := &JiraClient{
		baseURL: server.URL,
		email:   "me@example.com",
		token:   "token",
		http:    server.Client(),
	}

	_, err := client.GetIssue("SCRUM-6")
	if err == nil {
		t.Fatal("GetIssue returned nil error for HTML response")
	}
	if got := err.Error(); !strings.Contains(got, "non-JSON response") || !strings.Contains(got, "text/html") {
		t.Fatalf("error = %q, want non-JSON response with content type", got)
	}
}
