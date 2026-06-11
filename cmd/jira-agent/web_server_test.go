package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/ccastorena/jira-agent/chat"
)

type fakeChatService struct {
	defaultModel string
	models       []string
	resolveTo    string
	turn         chat.Turn
	err          error
}

func (f fakeChatService) DefaultModel() string { return f.defaultModel }

func (f fakeChatService) AvailableModels(context.Context) ([]string, error) {
	return append([]string(nil), f.models...), nil
}

func (f fakeChatService) ResolveModel(_ context.Context, requested string) string {
	if f.resolveTo != "" {
		return f.resolveTo
	}
	return requested
}

func (f fakeChatService) RunTurn(context.Context, string, string, string) (chat.Turn, error) {
	return f.turn, f.err
}

func (f fakeChatService) SetMaxContextTokens(int) {}

func (f fakeChatService) CurrentTokenUsage(string) int { return 0 }

func (f fakeChatService) ResetSession(string) {}

func TestHandleIndexRendersPage(t *testing.T) {
	app := &webApp{chat: fakeChatService{defaultModel: "llama", models: []string{"llama", "qwen"}}, llmTimeout: 5 * time.Second, maxContextTokens: 1234}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	app.handleIndex(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Jira + GitHub Agent") || !strings.Contains(body, "hx-chat") {
		t.Fatalf("unexpected body: %s", body)
	}
	if !strings.Contains(body, `value="1234"`) {
		t.Fatalf("expected configured token limit in page, got: %s", body)
	}
	if !strings.Contains(body, `href="/jira/create"`) || !strings.Contains(body, `Create Jira Issue`) {
		t.Fatalf("expected dashboard to link to dedicated create page, got: %s", body)
	}
}

func TestHandleIndexRendersCreateDialogWhenJiraAvailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/rest/api/3/project/search":
			_, _ = w.Write([]byte(`{"values":[{"key":"SCRUM","name":"My Team"}]}`))
		case "/rest/api/3/user/assignable/search":
			_, _ = w.Write([]byte(`[{"accountId":"712020:abc","displayName":"Chris","active":true}]`))
		default:
			t.Errorf("unexpected path = %s", r.URL.Path)
		}
	}))
	defer server.Close()

	app := &webApp{
		chat:       fakeChatService{defaultModel: "llama"},
		llmTimeout: 5 * time.Second,
		jc:         &JiraClient{baseURL: server.URL, email: "me@example.com", token: "token", http: server.Client()},
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	app.handleIndex(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{
		`window.JiraIssueCreate`,
		`data-jira-create-open`,
		`class="hx-jira-create-dialog"`,
		`<form class="hx-jira-create-form" action="/jira/create" method="post"`,
		`SCRUM - My Team`,
		`Chris`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("dashboard missing create dialog marker %q\n%s", want, body)
		}
	}
}

func TestHandleJiraCreatePageRendersProjectDropdown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/rest/api/3/project/search":
			_, _ = w.Write([]byte(`{"values":[{"key":"SCRUM","name":"My Team"}]}`))
		case "/rest/api/3/user/assignable/search":
			if r.URL.Query().Get("project") != "SCRUM" {
				t.Errorf("assignable search project = %q, want SCRUM", r.URL.Query().Get("project"))
			}
			_, _ = w.Write([]byte(`[{"accountId":"712020:abc","displayName":"Chris","active":true}]`))
		default:
			t.Errorf("unexpected path = %s", r.URL.Path)
		}
	}))
	defer server.Close()

	app := &webApp{jc: &JiraClient{baseURL: server.URL, email: "me@example.com", token: "token", http: server.Client()}}
	req := httptest.NewRequest(http.MethodGet, "/jira/create", nil)
	rr := httptest.NewRecorder()

	app.handleJiraCreatePage(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `<form class="hx-jira-create-form" action="/jira/create" method="post"`) {
		t.Fatalf("expected native create form, got: %s", body)
	}
	if !strings.Contains(body, `<select name="project_key"`) || !strings.Contains(body, `SCRUM - My Team`) {
		t.Fatalf("expected Jira project dropdown in create page, got: %s", body)
	}
	if !strings.Contains(body, `<select name="assignee_account_id"`) || !strings.Contains(body, `Chris`) || !strings.Contains(body, `Unassigned`) {
		t.Fatalf("expected Jira assignee dropdown in create page, got: %s", body)
	}
	if !strings.Contains(body, `<select name="reporter_account_id"`) || !strings.Contains(body, `Default`) {
		t.Fatalf("expected Jira reporter dropdown in create page, got: %s", body)
	}
}

func TestHandleChatPromptRequired(t *testing.T) {
	app := &webApp{chat: fakeChatService{defaultModel: "m"}, llmTimeout: 5 * time.Second}
	req := httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(url.Values{"model": {"m"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	app.chatHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d want 400", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "prompt required") {
		t.Fatalf("body: got %q", rr.Body.String())
	}
}

func TestHandleChatRendersFriendlyTimeout(t *testing.T) {
	app := &webApp{
		chat:       fakeChatService{defaultModel: "m", resolveTo: "m", err: context.DeadlineExceeded},
		llmTimeout: 3 * time.Second,
	}
	req := httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(url.Values{"prompt": {"hello"}, "model": {"m"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	app.chatHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "did not respond within") {
		t.Fatalf("expected timeout guidance, got: %s", rr.Body.String())
	}
}

func TestHandleJiraCreatePostsStandardIssueFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/rest/api/3/project/search":
			_, _ = w.Write([]byte(`{"values":[{"key":"SCRUM","name":"My Team"}]}`))
			return
		case "/rest/api/3/user/assignable/search":
			_, _ = w.Write([]byte(`[{"accountId":"712020:assignee","displayName":"Ada","active":true}]`))
			return
		case "/rest/api/3/issue":
			if r.Method != http.MethodPost {
				t.Errorf("method = %s, want POST", r.Method)
			}
		default:
			t.Errorf("unexpected path = %s", r.URL.Path)
			return
		}
		var payload struct {
			Fields map[string]any `json:"fields"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if nested(payload.Fields, "project", "key") != "SCRUM" {
			t.Errorf("project key = %v", payload.Fields["project"])
		}
		if payload.Fields["summary"] != "Fix login" {
			t.Errorf("summary = %v", payload.Fields["summary"])
		}
		if nested(payload.Fields, "issuetype", "name") != "Bug" {
			t.Errorf("issue type = %v", payload.Fields["issuetype"])
		}
		if nested(payload.Fields, "priority", "name") != "High" {
			t.Errorf("priority = %v", payload.Fields["priority"])
		}
		if nested(payload.Fields, "assignee", "accountId") != "712020:assignee" {
			t.Errorf("assignee = %v", payload.Fields["assignee"])
		}
		if nested(payload.Fields, "reporter", "accountId") != "712020:reporter" {
			t.Errorf("reporter = %v", payload.Fields["reporter"])
		}
		labels, ok := payload.Fields["labels"].([]any)
		if !ok || len(labels) != 2 || labels[0] != "auth" || labels[1] != "frontend" {
			t.Errorf("labels = %#v", payload.Fields["labels"])
		}
		_, _ = w.Write([]byte(`{"key":"SCRUM-12"}`))
	}))
	defer server.Close()

	app := &webApp{jc: &JiraClient{baseURL: server.URL, email: "me@example.com", token: "token", http: server.Client()}}
	req := httptest.NewRequest(http.MethodPost, "/jira/create", strings.NewReader(url.Values{
		"project_key":         {"scrum"},
		"issue_type":          {"Bug"},
		"summary":             {"Fix login"},
		"description":         {"Button fails"},
		"priority":            {"High"},
		"labels":              {"auth, frontend"},
		"assignee_account_id": {"712020:assignee"},
		"reporter_account_id": {"712020:reporter"},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	app.handleJiraCreatePage(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "SCRUM-12") {
		t.Fatalf("expected created key in response, got: %s", rr.Body.String())
	}
}

func TestHandleJiraCreateHTMXPostReturnsResultOnly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/issue" {
			t.Fatalf("unexpected Jira path for HTMX create: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"key":"SCRUM-22"}`))
	}))
	defer server.Close()

	app := &webApp{jc: &JiraClient{baseURL: server.URL, email: "me@example.com", token: "token", http: server.Client()}}
	req := httptest.NewRequest(http.MethodPost, "/jira/create", strings.NewReader(url.Values{
		"project_key": {"SCRUM"},
		"summary":     {"Fix login"},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	rr := httptest.NewRecorder()

	app.handleJiraCreatePage(rr, req)

	body := rr.Body.String()
	if strings.Contains(body, "<!doctype html>") {
		t.Fatalf("HTMX response should be a fragment, got full page: %s", body)
	}
	if rr.Header().Get("HX-Trigger") != "jiraIssueCreated" {
		t.Fatalf("HX-Trigger = %q, want jiraIssueCreated", rr.Header().Get("HX-Trigger"))
	}
	if !strings.Contains(body, `id="jira-create-result"`) || !strings.Contains(body, "SCRUM-22") {
		t.Fatalf("expected result fragment, got: %s", body)
	}
}

func TestJiraIssuesRenderLinks(t *testing.T) {
	items := []jiraIssueItem{{
		Key:      "SCRUM-7",
		URL:      "https://example.atlassian.net/browse/SCRUM-7",
		Summary:  "Fix login",
		Status:   "To Do",
		Assignee: "Chris",
		Updated:  "2026-06-10",
	}}
	var out strings.Builder
	if err := issuesTmpl.Execute(&out, map[string]any{"Issues": items}); err != nil {
		t.Fatalf("execute issues template: %v", err)
	}
	body := out.String()
	if !strings.Contains(body, `href="https://example.atlassian.net/browse/SCRUM-7"`) {
		t.Fatalf("expected issue link, got: %s", body)
	}
}

func TestFriendlyLLMErrorDefault(t *testing.T) {
	app := &webApp{llmTimeout: 2 * time.Second}
	msg := app.friendlyLLMError(errors.New("boom"), "x")
	if msg != "Error: boom" {
		t.Fatalf("unexpected message: %q", msg)
	}
}
