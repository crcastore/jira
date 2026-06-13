package main

import (
	"errors"
	"fmt"

	"github.com/ccastorena/jira-agent/jiracreate"
	"github.com/ccastorena/jira-agent/jiraissueui"
)

const defaultSubtaskIssueType = "Sub-task"

type jiraIssueCreatePlan struct {
	Form             jiraissueui.IssueForm
	SubtaskIssueType string
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

	result := parent.Result(a.jc.baseURL)
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

	form.IssueType = jiracreate.ValidParentIssueType(form.IssueType, issueTypes)
	if form.IssueType == "" {
		return "", errors.New("Jira project has no creatable parent issue types")
	}

	if len(form.SubtaskNames) == 0 {
		return subtaskIssueType, nil
	}
	subtaskIssueType = jiracreate.FirstSubtaskIssueTypeName(issueTypes)
	if subtaskIssueType == "" {
		return "", errors.New("Jira project has no subtask issue type enabled")
	}
	return subtaskIssueType, nil
}

func (a *webApp) createParentJiraIssue(form jiraissueui.IssueForm) (jiracreate.CreatedIssue, error) {
	raw, err := a.jc.CreateIssue(createIssueArgsFromForm(form))
	if err != nil {
		return jiracreate.CreatedIssue{}, err
	}
	created, ok := jiracreate.ParseCreatedIssue(raw)
	if !ok {
		return jiracreate.CreatedIssue{}, errors.New("Jira created the issue but returned an unexpected response")
	}
	return created, nil
}

func (a *webApp) createJiraSubtasks(form jiraissueui.IssueForm, parent jiracreate.CreatedIssue, issueType string) ([]jiraissueui.IssueLink, error) {
	links := make([]jiraissueui.IssueLink, 0, len(form.SubtaskNames))
	for _, name := range form.SubtaskNames {
		created, err := a.createJiraSubtask(form, parent, name, issueType)
		if err != nil {
			return links, err
		}
		links = append(links, created.Link(a.jc.baseURL))
	}
	return links, nil
}

func (a *webApp) createJiraSubtask(form jiraissueui.IssueForm, parent jiracreate.CreatedIssue, name, issueType string) (jiracreate.CreatedIssue, error) {
	raw, err := a.jc.CreateIssue(createSubtaskArgsFromForm(form, parent, name, issueType))
	if err != nil {
		return jiracreate.CreatedIssue{}, fmt.Errorf("created %s but failed to create subtask for %s: %w", parent.Key, name, err)
	}
	subtask, ok := jiracreate.ParseCreatedIssue(raw)
	if !ok {
		return jiracreate.CreatedIssue{}, fmt.Errorf("created %s but Jira returned an unexpected response for subtask %s", parent.Key, name)
	}
	return subtask, nil
}

func createIssueArgsFromForm(form jiraissueui.IssueForm) CreateIssueArgs {
	return jiracreate.ArgsFromForm(form)
}

func createSubtaskArgsFromForm(form jiraissueui.IssueForm, parent jiracreate.CreatedIssue, name, issueType string) CreateIssueArgs {
	return jiracreate.SubtaskArgsFromForm(form, parent, name, issueType)
}
