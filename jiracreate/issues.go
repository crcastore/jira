package jiracreate

import (
	"encoding/json"
	"fmt"

	"github.com/ccastorena/jira-agent/jiraissueui"
)

// CreateIssueArgs is the app-neutral Jira issue create request shape used by
// this repo's Jira client and the reusable create/subtask helpers.
type CreateIssueArgs struct {
	ProjectKey        string   `json:"project_key"`
	Summary           string   `json:"summary"`
	IssueType         string   `json:"issue_type,omitempty"`
	ParentID          string   `json:"parent_id,omitempty"`
	ParentKey         string   `json:"parent_key,omitempty"`
	Description       string   `json:"description,omitempty"`
	AssigneeAccountID string   `json:"assignee_account_id,omitempty"`
	ReporterAccountID string   `json:"reporter_account_id,omitempty"`
	Priority          string   `json:"priority,omitempty"`
	Labels            []string `json:"labels,omitempty"`
}

// CreatedIssue is the compact response needed after Jira creates an issue.
type CreatedIssue struct {
	ID  string `json:"id"`
	Key string `json:"key"`
}

func ArgsFromForm(form jiraissueui.IssueForm) CreateIssueArgs {
	return CreateIssueArgs{
		ProjectKey:        form.ProjectKey,
		Summary:           form.Summary,
		IssueType:         form.IssueType,
		Description:       form.Description,
		Priority:          form.Priority,
		Labels:            form.Labels,
		AssigneeAccountID: form.AssigneeAccountID,
		ReporterAccountID: form.ReporterAccountID,
	}
}

func SubtaskArgsFromForm(form jiraissueui.IssueForm, parent CreatedIssue, name, issueType string) CreateIssueArgs {
	args := ArgsFromForm(form)
	args.IssueType = issueType
	args.ParentID = parent.ID
	args.ParentKey = parent.Key
	args.Summary = fmt.Sprintf("%s - %s", form.Summary, name)
	return args
}

func ParseCreatedIssue(raw json.RawMessage) (CreatedIssue, bool) {
	var created CreatedIssue
	if err := json.Unmarshal(raw, &created); err != nil || created.Key == "" {
		return CreatedIssue{}, false
	}
	return created, true
}

func ParseCreatedIssueKey(raw json.RawMessage) (string, bool) {
	created, ok := ParseCreatedIssue(raw)
	if !ok {
		return "", false
	}
	return created.Key, true
}

func (i CreatedIssue) Link(baseURL string) jiraissueui.IssueLink {
	return jiraissueui.IssueLink{Key: i.Key, URL: baseURL + "/browse/" + i.Key}
}

func (i CreatedIssue) Result(baseURL string) jiraissueui.Result {
	link := i.Link(baseURL)
	return jiraissueui.Result{Key: link.Key, URL: link.URL}
}
