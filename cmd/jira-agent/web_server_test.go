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

const testIssueTypesJSON = `{"values":[{"name":"Epic","subtask":false,"hierarchyLevel":1},{"name":"Request","subtask":false,"hierarchyLevel":0},{"name":"Task","subtask":false,"hierarchyLevel":0},{"name":"Subtask","subtask":true,"hierarchyLevel":-1}]}`

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

func TestParseJiraIssueTypesAcceptsResponseShapes(t *testing.T) {
	cases := []json.RawMessage{
		json.RawMessage(`{"values":[{"name":"Task","subtask":false},{"name":"Subtask","subtask":true}]}`),
		json.RawMessage(`{"issueTypes":[{"name":"Task","subtask":false},{"name":"Subtask","subtask":true}]}`),
		json.RawMessage(`{"projects":[{"issuetypes":[{"name":"Task","subtask":false},{"name":"Subtask","subtask":true}]}]}`),
		json.RawMessage(`[{"name":"Task","subtask":false},{"name":"Subtask","subtask":true}]`),
	}
	for _, raw := range cases {
		types, err := parseJiraIssueTypes(raw)
		if err != nil {
			t.Fatalf("parseJiraIssueTypes(%s) returned error: %v", raw, err)
		}
		if len(types) != 2 || types[0].Name != "Task" || types[1].Name != "Subtask" || !types[1].Subtask {
			t.Fatalf("unexpected types from %s: %+v", raw, types)
		}
	}
}

func TestParentIssueTypeNamesExcludeEpicAndPreferTask(t *testing.T) {
	types, err := parseJiraIssueTypes(json.RawMessage(testIssueTypesJSON))
	if err != nil {
		t.Fatalf("parseJiraIssueTypes returned error: %v", err)
	}
	sortJiraIssueTypes(types)
	names := parentIssueTypeNames(types)
	if strings.Join(names, ",") != "Task,Request" {
		t.Fatalf("parent issue type names = %#v, want Task, Request", names)
	}
	if got := validParentIssueType("Epic", types); got != "Task" {
		t.Fatalf("validParentIssueType(Epic) = %q, want Task", got)
	}
}

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
		case "/rest/api/3/issue/createmeta/SCRUM/issuetypes":
			_, _ = w.Write([]byte(testIssueTypesJSON))
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
		case "/rest/api/3/issue/createmeta/SCRUM/issuetypes":
			_, _ = w.Write([]byte(testIssueTypesJSON))
		case "/rest/api/3/user/assignable/search":
			if r.URL.Query().Get("project") != "SCRUM" {
				t.Errorf("assignable search project = %q, want SCRUM", r.URL.Query().Get("project"))
			}
			if r.URL.Query().Get("maxResults") != "20" {
				t.Errorf("assignable search maxResults = %q, want 20", r.URL.Query().Get("maxResults"))
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
	if !strings.Contains(body, `https://unpkg.com/htmx.org@1.9.12`) || !strings.Contains(body, `hx-get="/jira/create/pull-requests"`) {
		t.Fatalf("expected HTMX-enabled create page, got: %s", body)
	}
	if !strings.Contains(body, `<select name="project_key"`) || !strings.Contains(body, `SCRUM - My Team`) {
		t.Fatalf("expected Jira project dropdown in create page, got: %s", body)
	}
	if !strings.Contains(body, `window.JiraIssueCreate`) {
		t.Fatalf("expected user picker script in create page, got: %s", body)
	}
	for _, want := range []string{
		`name="assignee_search" type="search"`,
		`list="jira-create-assignee-options"`,
		`name="assignee_account_id" type="hidden"`,
		`hx-get="/jira/create/users"`,
		`data-jira-user-target="assignee_account_id"`,
		`data-account-id="712020:abc"`,
		`Chris`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected Jira assignee search picker marker %q in create page, got: %s", want, body)
		}
	}
	for _, want := range []string{
		`name="reporter_search" type="search"`,
		`list="jira-create-reporter-options"`,
		`name="reporter_account_id" type="hidden"`,
		`data-jira-user-target="reporter_account_id"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected Jira reporter search picker marker %q in create page, got: %s", want, body)
		}
	}
}

func TestHandleJiraCreateUsersSearchesAssignableUsers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/rest/api/3/user/assignable/search" {
			t.Fatalf("unexpected Jira path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("project"); got != "SCRUM" {
			t.Errorf("project = %q, want SCRUM", got)
		}
		if got := r.URL.Query().Get("query"); got != "ada" {
			t.Errorf("query = %q, want ada", got)
		}
		if got := r.URL.Query().Get("maxResults"); got != "20" {
			t.Errorf("maxResults = %q, want 20", got)
		}
		_, _ = w.Write([]byte(`[{"accountId":"712020:ada","displayName":"Ada Lovelace","active":true}]`))
	}))
	defer server.Close()

	app := &webApp{jc: &JiraClient{baseURL: server.URL, email: "me@example.com", token: "token", http: server.Client()}}
	req := httptest.NewRequest(http.MethodGet, "/jira/create/users?field=assignee&project_key=SCRUM&assignee_search=ada", nil)
	rr := httptest.NewRecorder()

	app.handleJiraCreateUsers(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{
		`<datalist id="jira-create-assignee-options">`,
		`<option value="Ada Lovelace" data-account-id="712020:ada"></option>`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("user options missing %q\n%s", want, body)
		}
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
		case "/rest/api/3/issue/createmeta/SCRUM/issuetypes":
			_, _ = w.Write([]byte(testIssueTypesJSON))
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
		if nested(payload.Fields, "issuetype", "name") != "Task" {
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

func TestHandleJiraCreateCreatesSubtasksForNames(t *testing.T) {
	type createPayload struct {
		Fields map[string]any `json:"fields"`
	}
	var creates []createPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/rest/api/3/issue/createmeta/SCRUM/issuetypes" {
			_, _ = w.Write([]byte(`{"values":[{"name":"Task","subtask":false},{"name":"Subtask","subtask":true}]}`))
			return
		}
		if r.URL.Path != "/rest/api/3/issue" {
			t.Fatalf("unexpected path = %s", r.URL.Path)
		}
		var payload createPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		creates = append(creates, payload)
		switch len(creates) {
		case 1:
			_, _ = w.Write([]byte(`{"id":"10012","key":"SCRUM-12"}`))
		case 2:
			_, _ = w.Write([]byte(`{"id":"10013","key":"SCRUM-13"}`))
		case 3:
			_, _ = w.Write([]byte(`{"id":"10014","key":"SCRUM-14"}`))
		default:
			t.Fatalf("unexpected create count %d", len(creates))
		}
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
		"subtask_names":       {"Ada\nBob"},
		"assignee_account_id": {"712020:assignee"},
		"reporter_account_id": {"712020:reporter"},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	rr := httptest.NewRecorder()

	app.handleJiraCreatePage(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rr.Code)
	}
	if len(creates) != 3 {
		t.Fatalf("create count = %d, want 3", len(creates))
	}
	parent := creates[0].Fields
	if _, ok := parent["parent"]; ok {
		t.Fatalf("parent issue should not have a parent field: %#v", parent["parent"])
	}
	if parent["summary"] != "Fix login" || nested(parent, "issuetype", "name") != "Task" {
		t.Fatalf("unexpected parent payload: %#v", parent)
	}
	for i, want := range []string{"Ada", "Bob"} {
		subtask := creates[i+1].Fields
		if nested(subtask, "parent", "id") != "10012" {
			t.Fatalf("subtask parent = %#v, want id 10012", subtask["parent"])
		}
		if nested(subtask, "issuetype", "name") != "Subtask" {
			t.Fatalf("subtask issue type = %#v", subtask["issuetype"])
		}
		if subtask["summary"] != "Fix login - "+want {
			t.Fatalf("subtask summary = %v", subtask["summary"])
		}
		if nested(subtask, "assignee", "accountId") != "712020:assignee" || nested(subtask, "reporter", "accountId") != "712020:reporter" {
			t.Fatalf("subtask users were not copied: %#v", subtask)
		}
	}
	body := rr.Body.String()
	if !strings.Contains(body, "SCRUM-12") || !strings.Contains(body, "SCRUM-13") || !strings.Contains(body, "SCRUM-14") {
		t.Fatalf("expected parent and subtask keys in response, got: %s", body)
	}
}

func TestHandleJiraCreateAddsPullRequestDetailsToDescription(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/user/repos":
			_, _ = w.Write([]byte(`[{"full_name":"octo/hello"}]`))
			return
		case "/repos/octo/hello/pulls":
			_, _ = w.Write([]byte(`[
				{"number":12,"title":"Add login fix","head":{"ref":"fix-login"},"base":{"ref":"main"}}
			]`))
			return
		case "/repos/octo/hello/pulls/12":
			if r.Header.Get("Authorization") != "Bearer token" {
				t.Errorf("Authorization header = %q", r.Header.Get("Authorization"))
			}
			_, _ = w.Write([]byte(`{
				"number": 12,
				"title": "Add login fix",
				"state": "open",
				"html_url": "https://github.com/octo/hello/pull/12",
				"user": {"login": "octocat"},
				"head": {"ref": "fix-login"},
				"base": {"ref": "main"}
			}`))
			return
		case "/repos/octo/hello/pulls/12/files":
			if got := r.URL.Query().Get("per_page"); got != "100" {
				t.Errorf("per_page = %q, want 100", got)
			}
			_, _ = w.Write([]byte(`[
				{"filename":"cmd/main.go","status":"modified","additions":12,"deletions":3},
				{"filename":"README.md","status":"added","additions":5,"deletions":0}
			]`))
			return
		case "/rest/api/3/project/search":
			_, _ = w.Write([]byte(`{"values":[{"key":"SCRUM","name":"My Team"}]}`))
			return
		case "/rest/api/3/issue/createmeta/SCRUM/issuetypes":
			_, _ = w.Write([]byte(testIssueTypesJSON))
			return
		case "/rest/api/3/user/assignable/search":
			_, _ = w.Write([]byte(`[]`))
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
		descriptionJSON, err := json.Marshal(payload.Fields["description"])
		if err != nil {
			t.Fatalf("marshal description: %v", err)
		}
		description := string(descriptionJSON)
		for _, want := range []string{
			"Button fails",
			"Related pull request",
			"https://github.com/octo/hello/pull/12",
			"Add login fix",
			"cmd/main.go (modified, +12/-3)",
			"README.md (added, +5/-0)",
		} {
			if !strings.Contains(description, want) {
				t.Fatalf("description missing %q: %s", want, description)
			}
		}
		_, _ = w.Write([]byte(`{"key":"SCRUM-44"}`))
	}))
	defer server.Close()

	app := &webApp{
		jc: &JiraClient{baseURL: server.URL, email: "me@example.com", token: "token", http: server.Client()},
		gc: &GitHubClient{baseURL: server.URL, token: "token", http: server.Client()},
	}
	req := httptest.NewRequest(http.MethodPost, "/jira/create", strings.NewReader(url.Values{
		"project_key":       {"scrum"},
		"summary":           {"Fix login"},
		"description":       {"Button fails"},
		"pull_request_repo": {"octo/hello"},
		"pull_request":      {"octo/hello#12"},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	app.handleJiraCreatePage(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "SCRUM-44") {
		t.Fatalf("expected created key in response, got: %s", rr.Body.String())
	}
}

func TestHandleJiraCreatePullRequestsRendersOptions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/repos/octo/hello/pulls" {
			t.Fatalf("unexpected GitHub path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("state"); got != "all" {
			t.Errorf("state = %q, want all", got)
		}
		if got := r.URL.Query().Get("per_page"); got != "100" {
			t.Errorf("per_page = %q, want 100", got)
		}
		_, _ = w.Write([]byte(`[
			{"number":12,"title":"Add login fix","state":"open","head":{"ref":"fix-login"},"base":{"ref":"main"}},
			{"number":7,"title":"Clean up docs","state":"closed","merged_at":"2026-06-10T12:00:00Z","head":{"ref":"docs"},"base":{"ref":"main"}}
		]`))
	}))
	defer server.Close()

	app := &webApp{gc: &GitHubClient{baseURL: server.URL, token: "token", http: server.Client()}}
	req := httptest.NewRequest(http.MethodGet, "/jira/create/pull-requests?pull_request_repo=octo/hello", nil)
	rr := httptest.NewRecorder()

	app.handleJiraCreatePullRequests(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{
		`id="jira-create-pull-requests"`,
		`<select name="pull_request"`,
		`value="octo/hello#12"`,
		`#12 Add login fix (fix-login -&gt; main)`,
		`value="octo/hello#7"`,
		`#7 Clean up docs [merged] (docs -&gt; main)`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("partial missing %q\n%s", want, body)
		}
	}
}

func TestHandleJiraCreateHTMXPostReturnsResultOnly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/3/issue/createmeta/SCRUM/issuetypes" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(testIssueTypesJSON))
			return
		}
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
