package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Tool dispatch: name resolution and CallTool guard rails (no network needed)
// ---------------------------------------------------------------------------

func TestCanonicalToolName(t *testing.T) {
	cases := map[string]string{
		"gh_list_repos":  "gh_list_my_repos",
		"gh_repos":       "gh_list_my_repos",
		"list_repos":     "gh_list_my_repos",
		"search_jira":    "search_issues",
		"jira_search":    "search_issues",
		"get_jira_issue": "get_issue",
		// Unknown names pass through unchanged.
		"gh_list_my_repos": "gh_list_my_repos",
		"search_issues":    "search_issues",
		"totally_made_up":  "totally_made_up",
	}
	for in, want := range cases {
		if got := canonicalToolName(in); got != want {
			t.Errorf("canonicalToolName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCallToolInvalidJSON(t *testing.T) {
	out := CallTool(nil, nil, "myself", "{not json")
	var got map[string]string
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("expected JSON error object, got %q", out)
	}
	if !strings.Contains(got["error"], "invalid JSON arguments") {
		t.Errorf("error = %q, want it to mention invalid JSON arguments", got["error"])
	}
}

func TestCallToolUnknownTool(t *testing.T) {
	out := CallTool(nil, nil, "no_such_tool", "{}")
	var got map[string]string
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("expected JSON error object, got %q", out)
	}
	if !strings.Contains(got["error"], "unknown tool") {
		t.Errorf("error = %q, want it to mention unknown tool", got["error"])
	}
}

func TestCallToolGitHubNotConfigured(t *testing.T) {
	// A GitHub tool with a nil client must report a configuration error
	// instead of panicking on a nil dereference.
	out := CallTool(nil, nil, "gh_list_my_repos", "{}")
	var got map[string]string
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("expected JSON error object, got %q", out)
	}
	if !strings.Contains(got["error"], "GitHub is not configured") {
		t.Errorf("error = %q, want it to mention GitHub is not configured", got["error"])
	}
}

func TestCallToolResolvesAliasBeforeDispatch(t *testing.T) {
	// "gh_list_repos" is an alias; with a nil client it should still route to
	// the GitHub branch (configuration error) rather than "unknown tool".
	out := CallTool(nil, nil, "gh_list_repos", "{}")
	if !strings.Contains(out, "GitHub is not configured") {
		t.Errorf("alias not resolved before dispatch: %q", out)
	}
}

// ---------------------------------------------------------------------------
// Argument coercion
// ---------------------------------------------------------------------------

func TestIntArg(t *testing.T) {
	cases := []struct {
		in   any
		want int
	}{
		{float64(7), 7},
		{int(3), 3},
		{"nope", 0},
		{nil, 0},
	}
	for _, c := range cases {
		if got := intArg(c.in); got != c.want {
			t.Errorf("intArg(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestStrOrIntArg(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{"test.yml", "test.yml"},
		{float64(12), "12"},
		{int(5), "5"},
		{int64(99), "99"},
		{nil, ""},
		{true, ""},
	}
	for _, c := range cases {
		if got := strOrIntArg(c.in); got != c.want {
			t.Errorf("strOrIntArg(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestErrJSON(t *testing.T) {
	out := errJSON("boom")
	var got map[string]string
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("errJSON did not produce valid JSON: %v", err)
	}
	if got["error"] != "boom" {
		t.Errorf("error = %q, want boom", got["error"])
	}
}

// ---------------------------------------------------------------------------
// Map helpers
// ---------------------------------------------------------------------------

func TestPickAndNested(t *testing.T) {
	m := map[string]any{
		"summary": "hello",
		"status":  map[string]any{"name": "Done"},
	}
	if got := pick(m, "summary"); got != "hello" {
		t.Errorf("pick summary = %v, want hello", got)
	}
	if got := pick(nil, "summary"); got != nil {
		t.Errorf("pick on nil map = %v, want nil", got)
	}
	if got := nested(m, "status", "name"); got != "Done" {
		t.Errorf("nested status.name = %v, want Done", got)
	}
	if got := nested(m, "missing", "name"); got != nil {
		t.Errorf("nested on missing key = %v, want nil", got)
	}
	if got := nested(nil, "status", "name"); got != nil {
		t.Errorf("nested on nil map = %v, want nil", got)
	}
}

func TestLabelNames(t *testing.T) {
	in := []any{
		map[string]any{"name": "bug"},
		map[string]any{"name": "p1"},
		map[string]any{"color": "red"}, // no name -> skipped
		"not-an-object",                // skipped
	}
	got := labelNames(in)
	if len(got) != 2 || got[0] != "bug" || got[1] != "p1" {
		t.Fatalf("labelNames = %v, want [bug p1]", got)
	}
	if labelNames("nope") != nil {
		t.Errorf("labelNames on non-array should be nil")
	}
}

// ---------------------------------------------------------------------------
// Payload trimming
// ---------------------------------------------------------------------------

func TestTrimSearch(t *testing.T) {
	raw := json.RawMessage(`{
		"nextPageToken": "abc",
		"issues": [
			{"key": "ABC-1", "fields": {
				"summary": "Fix it",
				"status": {"name": "In Progress"},
				"assignee": {"displayName": "Ada"},
				"priority": {"name": "High"},
				"issuetype": {"name": "Bug"},
				"updated": "2026-01-02T03:04:05Z",
				"noise": "drop me"
			}}
		]
	}`)
	out := trimSearch(raw)
	var got struct {
		Count         int              `json:"count"`
		NextPageToken string           `json:"next_page_token"`
		Issues        []map[string]any `json:"issues"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("trimSearch output not the expected shape: %v\n%s", err, out)
	}
	if got.Count != 1 || got.NextPageToken != "abc" {
		t.Fatalf("count=%d token=%q", got.Count, got.NextPageToken)
	}
	it := got.Issues[0]
	if it["key"] != "ABC-1" || it["summary"] != "Fix it" || it["status"] != "In Progress" {
		t.Errorf("unexpected trimmed issue: %v", it)
	}
	if it["assignee"] != "Ada" || it["type"] != "Bug" {
		t.Errorf("nested fields not flattened: %v", it)
	}
	if _, ok := it["noise"]; ok {
		t.Errorf("noisy field should have been dropped")
	}
}

func TestTrimSearchPassesThroughInvalid(t *testing.T) {
	raw := json.RawMessage(`not json`)
	if got := trimSearch(raw); string(got) != "not json" {
		t.Errorf("invalid input should pass through unchanged, got %s", got)
	}
}

func TestTrimIssue(t *testing.T) {
	raw := json.RawMessage(`{
		"key": "ABC-9",
		"fields": {
			"summary": "Title",
			"status": {"name": "Done"},
			"assignee": {"displayName": "Ada"},
			"reporter": {"displayName": "Bob"},
			"priority": {"name": "Low"},
			"issuetype": {"name": "Task"},
			"labels": ["a", "b"],
			"created": "2026-01-01",
			"updated": "2026-01-02",
			"description": "desc"
		}
	}`)
	out := trimIssue(raw)
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("trimIssue output invalid: %v", err)
	}
	if got["key"] != "ABC-9" || got["status"] != "Done" || got["reporter"] != "Bob" {
		t.Errorf("unexpected trimmed issue: %v", got)
	}
}

func TestSlimRepo(t *testing.T) {
	m := map[string]any{
		"full_name":         "octo/x",
		"stargazers_count":  float64(5),
		"open_issues_count": float64(2),
		"language":          "Go",
		"owner":             map[string]any{"login": "octo"}, // dropped
	}
	got := slimRepo(m)
	if got["full_name"] != "octo/x" || got["stargazers"].(float64) != 5 || got["open_issues"].(float64) != 2 {
		t.Errorf("slimRepo = %v", got)
	}
	if _, ok := got["owner"]; ok {
		t.Errorf("owner should be dropped")
	}
	if slimRepo(nil) != nil {
		t.Errorf("slimRepo(nil) should be nil")
	}
}

func TestSlimIssueMarksPRAndRepo(t *testing.T) {
	m := map[string]any{
		"number":       float64(3),
		"title":        "t",
		"user":         map[string]any{"login": "ada"},
		"labels":       []any{map[string]any{"name": "bug"}},
		"pull_request": map[string]any{"url": "x"},
		"repository":   map[string]any{"full_name": "octo/x"},
	}
	got := slimIssue(m)
	if got["user"] != "ada" {
		t.Errorf("user not flattened: %v", got["user"])
	}
	if got["is_pr"] != true {
		t.Errorf("is_pr should be true")
	}
	if got["repo"] != "octo/x" {
		t.Errorf("repo not extracted: %v", got["repo"])
	}
	if labels, ok := got["labels"].([]string); !ok || len(labels) != 1 || labels[0] != "bug" {
		t.Errorf("labels = %v", got["labels"])
	}
}

func TestTrimGitHubRunWorkflowConfirmation(t *testing.T) {
	// gh_run_workflow returns 204/empty; trimGitHub should synthesize a note.
	out := trimGitHub("gh_run_workflow", json.RawMessage(`{}`))
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("invalid output: %v", err)
	}
	if got["dispatched"] != true {
		t.Errorf("expected dispatched=true, got %v", got)
	}
}

func TestTrimGitHubListWorkflowRuns(t *testing.T) {
	raw := json.RawMessage(`{
		"total_count": 2,
		"workflow_runs": [
			{"id": 1, "name": "CI", "status": "completed", "conclusion": "success", "actor": {"login": "ada"}},
			{"id": 2, "name": "CI", "status": "in_progress"}
		]
	}`)
	out := trimGitHub("gh_list_workflow_runs", raw)
	var got struct {
		TotalCount int              `json:"total_count"`
		Returned   int              `json:"returned"`
		Runs       []map[string]any `json:"runs"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("invalid output: %v", err)
	}
	if got.TotalCount != 2 || got.Returned != 2 || len(got.Runs) != 2 {
		t.Fatalf("got %+v", got)
	}
	if got.Runs[0]["actor"] != "ada" {
		t.Errorf("actor not flattened: %v", got.Runs[0]["actor"])
	}
}

func TestTrimGitHubUnknownPassesThrough(t *testing.T) {
	raw := json.RawMessage(`{"anything":1}`)
	if got := trimGitHub("gh_not_handled", raw); string(got) != string(raw) {
		t.Errorf("unknown tool should pass raw through, got %s", got)
	}
}

func TestWrapWaitResult(t *testing.T) {
	out := wrapWaitResult(json.RawMessage(`{"id":1}`), true)
	var got struct {
		Completed bool           `json:"completed"`
		Run       map[string]any `json:"run"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("invalid output: %v", err)
	}
	if !got.Completed || got.Run["id"].(float64) != 1 {
		t.Errorf("wrapWaitResult = %+v", got)
	}

	// Empty run should become JSON null, not invalid JSON.
	out = wrapWaitResult(nil, false)
	if !json.Valid(out) {
		t.Errorf("wrapWaitResult(nil) produced invalid JSON: %s", out)
	}
}

func TestAsMapAndJSONOrRaw(t *testing.T) {
	if m := asMap(json.RawMessage(`{"a":1}`)); m == nil || m["a"].(float64) != 1 {
		t.Errorf("asMap failed: %v", m)
	}
	if asMap(json.RawMessage(`[1,2]`)) != nil {
		t.Errorf("asMap on non-object should be nil")
	}
	fallback := json.RawMessage(`"fallback"`)
	if got := jsonOrRaw(map[string]any{"ok": true}, fallback); !strings.Contains(string(got), "ok") {
		t.Errorf("jsonOrRaw should marshal value, got %s", got)
	}
}

// ---------------------------------------------------------------------------
// Web + env helpers
// ---------------------------------------------------------------------------

func TestTrimISODate(t *testing.T) {
	if got := trimISODate("2026-06-06T12:34:56.000Z"); got != "2026-06-06" {
		t.Errorf("trimISODate = %q", got)
	}
	if got := trimISODate("short"); got != "short" {
		t.Errorf("short string should pass through, got %q", got)
	}
}

func TestErrString(t *testing.T) {
	if errString(nil) != "" {
		t.Errorf("errString(nil) should be empty")
	}
	if got := errString(json.Unmarshal([]byte("x"), &struct{}{})); got == "" {
		t.Errorf("errString should surface a non-nil error message")
	}
}

func TestEnvOrInt(t *testing.T) {
	const key = "TEST_ENV_OR_INT_XYZ"
	t.Setenv(key, "")
	if got := envOrInt(key, 42); got != 42 {
		t.Errorf("empty env should use default, got %d", got)
	}
	t.Setenv(key, "15")
	if got := envOrInt(key, 42); got != 15 {
		t.Errorf("set env should parse, got %d", got)
	}
	t.Setenv(key, "-3")
	if got := envOrInt(key, 42); got != 42 {
		t.Errorf("negative should fall back to default, got %d", got)
	}
	t.Setenv(key, "notanint")
	if got := envOrInt(key, 42); got != 42 {
		t.Errorf("non-int should fall back to default, got %d", got)
	}
}

func TestEnvOr(t *testing.T) {
	const key = "TEST_ENV_OR_ABC"
	t.Setenv(key, "")
	if got := envOr(key, "def"); got != "def" {
		t.Errorf("empty env should use default, got %q", got)
	}
	t.Setenv(key, "set")
	if got := envOr(key, "def"); got != "set" {
		t.Errorf("set env should win, got %q", got)
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "", "third"); got != "third" {
		t.Errorf("firstNonEmpty = %q, want third", got)
	}
	if got := firstNonEmpty("first", "second"); got != "first" {
		t.Errorf("firstNonEmpty = %q, want first", got)
	}
	if got := firstNonEmpty("", ""); got != "" {
		t.Errorf("all-empty should be empty, got %q", got)
	}
}

func TestClampPerPage(t *testing.T) {
	cases := []struct {
		v, def, want int
	}{
		{0, 30, 30},
		{-5, 30, 30},
		{50, 30, 50},
		{500, 30, 100},
	}
	for _, c := range cases {
		if got := clampPerPage(c.v, c.def); got != c.want {
			t.Errorf("clampPerPage(%d, %d) = %d, want %d", c.v, c.def, got, c.want)
		}
	}
}
