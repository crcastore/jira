package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/ccastorena/jira-agent/jiraissueui"
)

type repoItem struct {
	FullName string
	URL      string
	Updated  string
	Private  bool
}

type jiraIssueItem struct {
	Key      string
	URL      string
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
			URL:      a.jc.baseURL + "/browse/" + it.Key,
			Summary:  it.Fields.Summary,
			Status:   it.Fields.Status.Name,
			Assignee: assignee,
			Updated:  trimISODate(it.Fields.Updated),
		})
	}
	return items, nil
}

func (a *webApp) fetchJiraProjects() ([]jiraissueui.Project, error) {
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
	items := make([]jiraissueui.Project, 0, len(payload))
	for _, project := range payload {
		if project.Key == "" {
			continue
		}
		items = append(items, jiraissueui.Project{Key: project.Key, Name: project.Name})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Key < items[j].Key })
	return items, nil
}

func (a *webApp) fetchJiraAssignableUsers(projectKey string) ([]jiraissueui.User, error) {
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
	items := make([]jiraissueui.User, 0, len(payload))
	for _, user := range payload {
		if user.AccountID == "" || !user.Active {
			continue
		}
		items = append(items, jiraissueui.User{AccountID: user.AccountID, DisplayName: user.DisplayName})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].DisplayName < items[j].DisplayName })
	return items, nil
}

func (a *webApp) fetchPullRequestRepos() ([]jiraissueui.PullRequestRepo, error) {
	if a.gc == nil {
		return nil, nil
	}
	raw, err := a.gc.ListMyRepos("all", "pushed", 150)
	if err != nil {
		return nil, err
	}
	var payload []struct {
		FullName string `json:"full_name"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	items := make([]jiraissueui.PullRequestRepo, 0, len(payload))
	for _, repo := range payload {
		if repo.FullName == "" {
			continue
		}
		items = append(items, jiraissueui.PullRequestRepo{FullName: repo.FullName})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].FullName < items[j].FullName })
	return items, nil
}

func (a *webApp) fetchPullRequestsForRepo(fullName string) ([]jiraissueui.PullRequestOption, error) {
	fullName = strings.TrimSpace(fullName)
	if fullName == "" {
		return nil, nil
	}
	if a.gc == nil {
		return nil, fmt.Errorf("GITHUB_TOKEN not set")
	}
	owner, repo, ok := splitOwnerRepo(fullName)
	if !ok {
		return nil, fmt.Errorf("invalid repository %q", fullName)
	}
	raw, err := a.gc.ListPulls(owner, repo, "all", 100)
	if err != nil {
		return nil, err
	}
	var payload []struct {
		Number   int    `json:"number"`
		Title    string `json:"title"`
		State    string `json:"state"`
		MergedAt string `json:"merged_at"`
		Head     struct {
			Ref string `json:"ref"`
		} `json:"head"`
		Base struct {
			Ref string `json:"ref"`
		} `json:"base"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	items := make([]jiraissueui.PullRequestOption, 0, len(payload))
	for _, pr := range payload {
		if pr.Number <= 0 {
			continue
		}
		label := fmt.Sprintf("#%d", pr.Number)
		if pr.Title != "" {
			label += " " + pr.Title
		}
		if pr.MergedAt != "" {
			label += " [merged]"
		} else if pr.State != "" && pr.State != "open" {
			label += " [" + pr.State + "]"
		}
		if pr.Head.Ref != "" || pr.Base.Ref != "" {
			label += fmt.Sprintf(" (%s -> %s)", pr.Head.Ref, pr.Base.Ref)
		}
		items = append(items, jiraissueui.PullRequestOption{
			Value: fmt.Sprintf("%s#%d", fullName, pr.Number),
			Label: label,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Value > items[j].Value })
	return items, nil
}
