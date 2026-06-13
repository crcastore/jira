package main

import "github.com/ccastorena/jira-agent/jiracreate"

type jiraIssueTypeOption = jiracreate.IssueType

func (a *webApp) fetchJiraIssueTypes(projectKey string) ([]jiraIssueTypeOption, error) {
	if projectKey == "" {
		return nil, nil
	}
	raw, err := a.jc.ListIssueTypes(projectKey)
	if err != nil {
		return nil, err
	}
	types, err := jiracreate.ParseIssueTypes(raw)
	if err != nil {
		return nil, err
	}
	jiracreate.SortIssueTypes(types)
	return types, nil
}
