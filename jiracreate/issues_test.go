package jiracreate

import (
	"encoding/json"
	"testing"

	"github.com/ccastorena/jira-agent/jiraissueui"
)

func TestSubtaskArgsFromFormUsesParentIDAndCopiesFields(t *testing.T) {
	form := jiraissueui.IssueForm{
		ProjectKey:        "SCRUM",
		Summary:           "Fix login",
		IssueType:         "Task",
		Description:       "Button fails",
		Priority:          "High",
		Labels:            []string{"auth"},
		AssigneeAccountID: "assignee",
		ReporterAccountID: "reporter",
	}
	args := SubtaskArgsFromForm(form, CreatedIssue{ID: "10012", Key: "SCRUM-12"}, "Ada", "Subtask")

	if args.ProjectKey != "SCRUM" || args.IssueType != "Subtask" || args.ParentID != "10012" || args.ParentKey != "SCRUM-12" {
		t.Fatalf("unexpected subtask args: %+v", args)
	}
	if args.Summary != "Fix login - Ada" || args.Description != "Button fails" || args.Priority != "High" {
		t.Fatalf("subtask did not copy expected fields: %+v", args)
	}
	if len(args.Labels) != 1 || args.Labels[0] != "auth" || args.AssigneeAccountID != "assignee" || args.ReporterAccountID != "reporter" {
		t.Fatalf("subtask did not copy labels/users: %+v", args)
	}
}

func TestParseCreatedIssue(t *testing.T) {
	created, ok := ParseCreatedIssue(json.RawMessage(`{"id":"10012","key":"SCRUM-12"}`))
	if !ok {
		t.Fatalf("ParseCreatedIssue returned !ok")
	}
	if created.ID != "10012" || created.Key != "SCRUM-12" {
		t.Fatalf("created = %+v", created)
	}
}
