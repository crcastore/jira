package jiraissueui

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestParseValuesNormalizesAndValidatesForm(t *testing.T) {
	form, err := ParseValues(url.Values{
		"project_key":         {" scrum "},
		"summary":             {" Fix login "},
		"issue_type":          {" Bug "},
		"description":         {" Details "},
		"pull_request_repo":   {" octo/hello "},
		"pull_request":        {" octo/hello#12 "},
		"priority":            {" High "},
		"labels":              {" auth, frontend, , urgent "},
		"subtask_names":       {" Ada\nBob", " Grace;  "},
		"assignee_account_id": {" 712020:assignee "},
		"reporter_account_id": {" 712020:reporter "},
	})
	if err != nil {
		t.Fatalf("ParseValues returned error: %v", err)
	}
	if form.ProjectKey != "SCRUM" || form.Summary != "Fix login" || form.IssueType != "Bug" {
		t.Fatalf("unexpected normalized form: %+v", form)
	}
	if form.Description != "Details" || form.PullRequestRepo != "octo/hello" || form.PullRequest != "octo/hello#12" || form.Priority != "High" {
		t.Fatalf("unexpected text fields: %+v", form)
	}
	if form.AssigneeAccountID != "712020:assignee" || form.ReporterAccountID != "712020:reporter" {
		t.Fatalf("unexpected users: %+v", form)
	}
	wantLabels := []string{"auth", "frontend", "urgent"}
	if len(form.Labels) != len(wantLabels) {
		t.Fatalf("labels = %#v", form.Labels)
	}
	for i, want := range wantLabels {
		if form.Labels[i] != want {
			t.Fatalf("labels[%d] = %q, want %q", i, form.Labels[i], want)
		}
	}
	wantSubtasks := []string{"Ada", "Bob", "Grace"}
	if len(form.SubtaskNames) != len(wantSubtasks) {
		t.Fatalf("subtask names = %#v", form.SubtaskNames)
	}
	for i, want := range wantSubtasks {
		if form.SubtaskNames[i] != want {
			t.Fatalf("subtaskNames[%d] = %q, want %q", i, form.SubtaskNames[i], want)
		}
	}
}

func TestParseValuesRequiresProjectAndSummary(t *testing.T) {
	for _, values := range []url.Values{
		{"project_key": {"SCRUM"}},
		{"summary": {"Fix login"}},
	} {
		if _, err := ParseValues(values); err != ErrRequiredFields {
			t.Fatalf("ParseValues(%v) error = %v, want ErrRequiredFields", values, err)
		}
	}
}

func TestParseRequestRejectsInvalidFormBody(t *testing.T) {
	req := httptest.NewRequest("POST", "/jira/create", strings.NewReader("%zz"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if _, err := ParseRequest(req); err != ErrInvalidForm {
		t.Fatalf("ParseRequest error = %v, want ErrInvalidForm", err)
	}
}

func TestIssueFormLabelsCSV(t *testing.T) {
	form := IssueForm{Labels: []string{"auth", "frontend"}}
	if got := form.LabelsCSV(); got != "auth, frontend" {
		t.Fatalf("LabelsCSV = %q", got)
	}
}

func TestFormRendersHTMXAndNativeFormAttributes(t *testing.T) {
	html, err := New().Form(FormData{Endpoint: "/tickets/create"})
	if err != nil {
		t.Fatalf("Form returned error: %v", err)
	}
	got := string(html)
	for _, want := range []string{
		`class="hx-jira-create"`,
		`action="/tickets/create"`,
		`method="post"`,
		`hx-post="/tickets/create"`,
		`hx-target="#jira-create-result"`,
		`hx-indicator="#jira-create-working"`,
		`Creating issue`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("form missing %q\n%s", want, got)
		}
	}
	for _, blocked := range []string{
		`hx-disabled-elt=`,
		`find input, find textarea, find select, find button`,
	} {
		if strings.Contains(got, blocked) {
			t.Fatalf("form includes fragile inherited HTMX selector %q\n%s", blocked, got)
		}
	}
}

func TestFormRendersPullRequestRepoChangeTrigger(t *testing.T) {
	html, err := New().Form(FormData{
		PullRequestRepos: []PullRequestRepo{{FullName: "octo/hello"}},
	})
	if err != nil {
		t.Fatalf("Form returned error: %v", err)
	}
	got := string(html)
	for _, want := range []string{
		`name="pull_request_repo"`,
		`hx-get="/jira/create/pull-requests"`,
		`hx-trigger="change"`,
		`hx-target="#jira-create-pull-requests"`,
		`hx-include="this"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("repo select missing %q\n%s", want, got)
		}
	}
}

func TestLauncherRendersStandaloneCreateButton(t *testing.T) {
	html, err := New().Launcher(LauncherData{
		Endpoint:    "/tickets/create",
		DialogID:    "ticket-window",
		ButtonLabel: "Create ticket",
	})
	if err != nil {
		t.Fatalf("Launcher returned error: %v", err)
	}
	got := string(html)
	for _, want := range []string{
		`class="hx-jira-create-launcher"`,
		`data-jira-create-launcher`,
		`href="/tickets/create"`,
		`data-jira-create-target="#ticket-window"`,
		`Create ticket`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("launcher missing %q\n%s", want, got)
		}
	}
}

func TestDialogRendersLauncherWindowAndForm(t *testing.T) {
	html, err := New().Dialog(DialogData{
		DialogID:    "ticket-window",
		ButtonLabel: "Create ticket",
		Title:       "New ticket",
		Form: FormData{
			Endpoint: "/tickets/create",
			Projects: []Project{{Key: "SCRUM", Name: "My Team"}},
		},
	})
	if err != nil {
		t.Fatalf("Dialog returned error: %v", err)
	}
	got := string(html)
	for _, want := range []string{
		`class="hx-jira-create-launcher"`,
		`href="/tickets/create"`,
		`data-jira-create-open`,
		`<dialog class="hx-jira-create-dialog" id="ticket-window"`,
		`data-jira-create-close`,
		`New ticket`,
		`Create ticket`,
		`action="/tickets/create"`,
		`hx-post="/tickets/create"`,
		`dialog.showModal`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("dialog missing %q\n%s", want, got)
		}
	}
}

func TestDialogCanUseSharedScriptInsteadOfInlineScript(t *testing.T) {
	html, err := New().Dialog(DialogData{DisableScript: true})
	if err != nil {
		t.Fatalf("Dialog returned error: %v", err)
	}
	got := string(html)
	if strings.Contains(got, `<script>`) || strings.Contains(got, `window.JiraIssueCreate`) {
		t.Fatalf("dialog should not include inline script when DisableScript is set:\n%s", got)
	}
	if !strings.Contains(got, `data-jira-create-target="#jira-create-dialog"`) {
		t.Fatalf("dialog launcher missing target id:\n%s", got)
	}
}

func TestDialogUsesSafeDefaults(t *testing.T) {
	html, err := New().Dialog(DialogData{})
	if err != nil {
		t.Fatalf("Dialog returned error: %v", err)
	}
	got := string(html)
	for _, want := range []string{
		`id="jira-create-dialog"`,
		`href="/jira/create"`,
		`Create Jira Issue`,
		`action="/jira/create"`,
		`id="jira-create-result"`,
		`id="jira-create-working"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("dialog missing default %q\n%s", want, got)
		}
	}
}

func TestDialogEscapesTitleAndButtonLabel(t *testing.T) {
	html, err := New().Dialog(DialogData{
		ButtonLabel: `<script>alert(1)</script>`,
		Title:       `<img src=x onerror=alert(1)>`,
	})
	if err != nil {
		t.Fatalf("Dialog returned error: %v", err)
	}
	got := string(html)
	if strings.Contains(got, `<script>alert(1)</script>`) || strings.Contains(got, `<img src=x`) {
		t.Fatalf("dialog rendered unsafe raw text:\n%s", got)
	}
}

func TestFormRendersSearchPickersAndSelectedValues(t *testing.T) {
	html, err := New().Form(FormData{
		Endpoint:  "/jira/create",
		Projects:  []Project{{Key: "SCRUM", Name: "My Team"}, {Key: "OPS", Name: "Ops"}},
		Assignees: []User{{AccountID: "a1", DisplayName: "Ada"}, {AccountID: "b2", DisplayName: "Bob"}},
		Values: IssueForm{
			ProjectKey:        "OPS",
			Summary:           "Fix deploy",
			IssueType:         "Bug",
			Description:       "Deploy is blocked",
			PullRequestRepo:   "octo/hello",
			PullRequest:       "octo/hello#12",
			Priority:          "High",
			Labels:            []string{"deploy", "urgent"},
			SubtaskNames:      []string{"Ada", "Bob"},
			AssigneeAccountID: "a1",
			ReporterAccountID: "b2",
		},
		PullRequestRepos: []PullRequestRepo{{FullName: "octo/hello"}, {FullName: "octo/world"}},
		PullRequests:     []PullRequestOption{{Value: "octo/hello#12", Label: "#12 Add login fix"}},
	})
	if err != nil {
		t.Fatalf("Form returned error: %v", err)
	}
	got := string(html)
	for _, want := range []string{
		`<option value="OPS" selected>OPS - Ops</option>`,
		`<option value="Bug" selected>Bug</option>`,
		`value="Fix deploy"`,
		`Deploy is blocked`,
		`<select name="pull_request_repo"`,
		`<option value="octo/hello" selected>octo/hello</option>`,
		`<select name="pull_request"`,
		`<option value="octo/hello#12" selected>#12 Add login fix</option>`,
		`<option value="High" selected>High</option>`,
		`value="deploy, urgent"`,
		`data-jira-subtask-list`,
		`data-jira-subtask-items`,
		`name="subtask_names" type="text" value="Ada"`,
		`name="subtask_names" type="text" value="Bob"`,
		`data-jira-subtask-add>Add name</button>`,
		`data-jira-subtask-remove aria-label="Remove subtask name">Remove</button>`,
		`name="assignee_search" type="search" list="jira-create-assignee-options"`,
		`hx-get="/jira/create/users"`,
		`data-jira-user-target="assignee_account_id"`,
		`name="assignee_account_id" type="hidden" value="a1"`,
		`name="reporter_search" type="search" list="jira-create-reporter-options"`,
		`data-jira-user-target="reporter_account_id"`,
		`name="reporter_account_id" type="hidden" value="b2"`,
		`<option value="Ada" data-account-id="a1"></option>`,
		`<option value="Bob" data-account-id="b2"></option>`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("form missing %q\n%s", want, got)
		}
	}
	if strings.Contains(got, `<select name="assignee_account_id"`) || strings.Contains(got, `<select name="reporter_account_id"`) {
		t.Fatalf("user fields should not render bulk selects\n%s", got)
	}
	if strings.Contains(got, `<textarea name="subtask_names"`) {
		t.Fatalf("subtask names should render as list rows, not a textarea\n%s", got)
	}
}

func TestUserOptionsHTMLRendersReplaceableDatalist(t *testing.T) {
	html, err := New().UserOptionsHTML(UserOptionsData{
		OptionsID: "jira-user-results",
		Users:     []User{{AccountID: "a1", DisplayName: "Ada"}},
	})
	if err != nil {
		t.Fatalf("UserOptionsHTML returned error: %v", err)
	}
	got := string(html)
	for _, want := range []string{
		`<datalist id="jira-user-results">`,
		`<option value="Ada" data-account-id="a1"></option>`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("user options missing %q\n%s", want, got)
		}
	}
}

func TestFormFallsBackToProjectTextInput(t *testing.T) {
	html, err := New().Form(FormData{Values: IssueForm{ProjectKey: "SCRUM"}})
	if err != nil {
		t.Fatalf("Form returned error: %v", err)
	}
	got := string(html)
	if !strings.Contains(got, `name="project_key" type="text" value="SCRUM"`) {
		t.Fatalf("expected project text fallback, got:\n%s", got)
	}
}

func TestFormEscapesUserInput(t *testing.T) {
	html, err := New().Form(FormData{Values: IssueForm{
		ProjectKey:  `SCRUM"><script>alert(1)</script>`,
		Summary:     `<script>alert(1)</script>`,
		Description: `<img src=x onerror=alert(1)>`,
	}})
	if err != nil {
		t.Fatalf("Form returned error: %v", err)
	}
	got := string(html)
	if strings.Contains(got, `<script>alert(1)</script>`) || strings.Contains(got, `<img src=x`) {
		t.Fatalf("form rendered unsafe raw input:\n%s", got)
	}
}

func TestResultRendering(t *testing.T) {
	html, err := New().ResultHTML(FormData{Result: Result{
		Key: "SCRUM-12",
		URL: "https://example.atlassian.net/browse/SCRUM-12",
		Subtasks: []IssueLink{{
			Key: "SCRUM-13",
			URL: "https://example.atlassian.net/browse/SCRUM-13",
		}},
	}})
	if err != nil {
		t.Fatalf("ResultHTML returned error: %v", err)
	}
	got := string(html)
	if !strings.Contains(got, `Created <a href="https://example.atlassian.net/browse/SCRUM-12"`) || !strings.Contains(got, `SCRUM-12`) || !strings.Contains(got, `SCRUM-13`) {
		t.Fatalf("unexpected success result:\n%s", got)
	}

	html, err = New().ResultHTML(FormData{Result: Result{Err: "boom"}})
	if err != nil {
		t.Fatalf("ResultHTML returned error: %v", err)
	}
	if !strings.Contains(string(html), `hx-jira-create-warn`) || !strings.Contains(string(html), `boom`) {
		t.Fatalf("unexpected error result:\n%s", html)
	}
}

func TestStyleAndScriptTagsAreScoped(t *testing.T) {
	css := string(CSS())
	if !strings.Contains(css, ".hx-jira-create") {
		t.Fatalf("CSS is not scoped: %s", css)
	}
	if !strings.Contains(string(StyleTag()), "<style>") {
		t.Fatalf("StyleTag missing style wrapper")
	}
	if !strings.Contains(string(JS()), "window.JiraIssueCreate") {
		t.Fatalf("JS missing public controller namespace")
	}
	if !strings.Contains(string(ScriptTag()), "<script>") {
		t.Fatalf("ScriptTag missing script wrapper")
	}
}
