package main

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ccastorena/jira-agent/jiraissueui"
)

const defaultSubtaskIssueType = "Sub-task"

type jiraIssueCreatePlan struct {
	Form             jiraissueui.IssueForm
	SubtaskIssueType string
}

type createdIssue struct {
	ID  string `json:"id"`
	Key string `json:"key"`
}

func (a *webApp) createJiraIssue(form jiraissueui.IssueForm) jiraissueui.Result {
	plan, err := a.planJiraIssueCreate(form)
	if err != nil {
		return jiraissueui.Result{Err: errString(err)}
	}

	parent, err := a.createParentJiraIssue(plan.Form)
	if err != nil {
		return jiraissueui.Result{Err: errString(err)}
	}

	result := parent.result(a.jc.baseURL)
	subtasks, err := a.createJiraSubtasks(plan.Form, parent, plan.SubtaskIssueType)
	result.Subtasks = subtasks
	if err != nil {
		result.Err = err.Error()
	}
	return result
}

func (a *webApp) planJiraIssueCreate(form jiraissueui.IssueForm) (jiraIssueCreatePlan, error) {
	enriched, err := a.pullRequestPicker().EnrichIssue(form)
	if err != nil {
		return jiraIssueCreatePlan{}, err
	}

	subtaskIssueType, err := a.applyProjectIssueTypes(&enriched)
	if err != nil {
		return jiraIssueCreatePlan{}, err
	}
	return jiraIssueCreatePlan{Form: enriched, SubtaskIssueType: subtaskIssueType}, nil
}

func (a *webApp) applyProjectIssueTypes(form *jiraissueui.IssueForm) (string, error) {
	subtaskIssueType := defaultSubtaskIssueType
	issueTypes, issueTypesErr := a.fetchJiraIssueTypes(form.ProjectKey)
	if issueTypesErr != nil || len(issueTypes) == 0 {
		return subtaskIssueType, nil
	}

	form.IssueType = validParentIssueType(form.IssueType, issueTypes)
	if form.IssueType == "" {
		return "", errors.New("Jira project has no creatable parent issue types")
	}

	if len(form.SubtaskNames) == 0 {
		return subtaskIssueType, nil
	}
	subtaskIssueType = firstSubtaskIssueTypeName(issueTypes)
	if subtaskIssueType == "" {
		return "", errors.New("Jira project has no subtask issue type enabled")
	}
	return subtaskIssueType, nil
}

func (a *webApp) createParentJiraIssue(form jiraissueui.IssueForm) (createdIssue, error) {
	raw, err := a.jc.CreateIssue(createIssueArgsFromForm(form))
	if err != nil {
		return createdIssue{}, err
	}
	created, ok := parseCreatedIssue(raw)
	if !ok {
		return createdIssue{}, errors.New("Jira created the issue but returned an unexpected response")
	}
	return created, nil
}

func (a *webApp) createJiraSubtasks(form jiraissueui.IssueForm, parent createdIssue, issueType string) ([]jiraissueui.IssueLink, error) {
	links := make([]jiraissueui.IssueLink, 0, len(form.SubtaskNames))
	for _, name := range form.SubtaskNames {
		created, err := a.createJiraSubtask(form, parent, name, issueType)
		if err != nil {
			return links, err
		}
		links = append(links, created.link(a.jc.baseURL))
	}
	return links, nil
}

func (a *webApp) createJiraSubtask(form jiraissueui.IssueForm, parent createdIssue, name, issueType string) (createdIssue, error) {
	raw, err := a.jc.CreateIssue(createSubtaskArgsFromForm(form, parent, name, issueType))
	if err != nil {
		return createdIssue{}, fmt.Errorf("created %s but failed to create subtask for %s: %w", parent.Key, name, err)
	}
	subtask, ok := parseCreatedIssue(raw)
	if !ok {
		return createdIssue{}, fmt.Errorf("created %s but Jira returned an unexpected response for subtask %s", parent.Key, name)
	}
	return subtask, nil
}

func createIssueArgsFromForm(form jiraissueui.IssueForm) CreateIssueArgs {
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

func createSubtaskArgsFromForm(form jiraissueui.IssueForm, parent createdIssue, name, issueType string) CreateIssueArgs {
	args := createIssueArgsFromForm(form)
	args.IssueType = issueType
	args.ParentID = parent.ID
	args.ParentKey = parent.Key
	args.Summary = fmt.Sprintf("%s - %s", form.Summary, name)
	return args
}

func parseCreatedIssue(raw json.RawMessage) (createdIssue, bool) {
	var created createdIssue
	if err := json.Unmarshal(raw, &created); err != nil || created.Key == "" {
		return createdIssue{}, false
	}
	return created, true
}

func parseCreatedIssueKey(raw json.RawMessage) (string, bool) {
	created, ok := parseCreatedIssue(raw)
	if !ok {
		return "", false
	}
	return created.Key, true
}

func (i createdIssue) link(baseURL string) jiraissueui.IssueLink {
	return jiraissueui.IssueLink{Key: i.Key, URL: baseURL + "/browse/" + i.Key}
}

func (i createdIssue) result(baseURL string) jiraissueui.Result {
	link := i.link(baseURL)
	return jiraissueui.Result{Key: link.Key, URL: link.URL}
}
