package main

import (
	"encoding/json"
	"fmt"
	"sort"
)

type repoItem struct {
	FullName string
	URL      string
	Updated  string
	Private  bool
}

type jiraIssueItem struct {
	Key      string
	Summary  string
	Status   string
	Assignee string
	Updated  string
}

type jiraProjectItem struct {
	Key  string
	Name string
}

type jiraUserItem struct {
	AccountID   string
	DisplayName string
}

func (a *webApp) fetchRepos() ([]repoItem, error) {
	if a.gc == nil {
		return nil, fmt.Errorf("GITHUB_TOKEN not set")
	}
	raw, err := a.gc.ListMyRepos("", "updated", 150)
	if err != nil {
		return nil, err
	}
	var repos []struct {
		FullName string `json:"full_name"`
		HTMLURL  string `json:"html_url"`
		Updated  string `json:"updated_at"`
		Private  bool   `json:"private"`
	}
	if err := json.Unmarshal(raw, &repos); err != nil {
		return nil, err
	}
	items := make([]repoItem, 0, len(repos))
	for _, r := range repos {
		items = append(items, repoItem{
			FullName: r.FullName,
			URL:      r.HTMLURL,
			Updated:  trimISODate(r.Updated),
			Private:  r.Private,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Updated > items[j].Updated })
	if len(items) > 40 {
		items = items[:40]
	}
	return items, nil
}

func (a *webApp) fetchJiraIssues() ([]jiraIssueItem, error) {
	if _, err := a.jc.Myself(); err != nil {
		return nil, fmt.Errorf("Jira authentication failed: %w", err)
	}
	raw, err := a.jc.Search(
		"assignee = currentUser() AND statusCategory != Done ORDER BY updated DESC",
		[]string{"summary", "status", "assignee", "updated"},
		40,
	)
	if err != nil {
		return nil, err
	}

	var payload struct {
		Issues []struct {
			Key    string `json:"key"`
			Fields struct {
				Summary string `json:"summary"`
				Updated string `json:"updated"`
				Status  struct {
					Name string `json:"name"`
				} `json:"status"`
				Assignee struct {
					DisplayName string `json:"displayName"`
				} `json:"assignee"`
			} `json:"fields"`
		} `json:"issues"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	items := make([]jiraIssueItem, 0, len(payload.Issues))
	for _, it := range payload.Issues {
		assignee := it.Fields.Assignee.DisplayName
		if assignee == "" {
			assignee = "Unassigned"
		}
		items = append(items, jiraIssueItem{
			Key:      it.Key,
			Summary:  it.Fields.Summary,
			Status:   it.Fields.Status.Name,
			Assignee: assignee,
			Updated:  trimISODate(it.Fields.Updated),
		})
	}
	return items, nil
}

func (a *webApp) fetchJiraProjects() ([]jiraProjectItem, error) {
	raw, err := a.jc.ListProjects()
	if err != nil {
		return nil, err
	}
	var payload []struct {
		Key  string `json:"key"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	items := make([]jiraProjectItem, 0, len(payload))
	for _, project := range payload {
		if project.Key == "" {
			continue
		}
		items = append(items, jiraProjectItem{Key: project.Key, Name: project.Name})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Key < items[j].Key })
	return items, nil
}

func (a *webApp) fetchJiraAssignableUsers(projectKey string) ([]jiraUserItem, error) {
	if projectKey == "" {
		return nil, nil
	}
	raw, err := a.jc.SearchAssignableUsers(projectKey, 50)
	if err != nil {
		return nil, err
	}
	var payload []struct {
		AccountID   string `json:"accountId"`
		DisplayName string `json:"displayName"`
		Active      bool   `json:"active"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	items := make([]jiraUserItem, 0, len(payload))
	for _, user := range payload {
		if user.AccountID == "" || !user.Active {
			continue
		}
		items = append(items, jiraUserItem{AccountID: user.AccountID, DisplayName: user.DisplayName})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].DisplayName < items[j].DisplayName })
	return items, nil
}
