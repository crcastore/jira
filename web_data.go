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
