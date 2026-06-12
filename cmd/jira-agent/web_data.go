package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/ccastorena/jira-agent/githubpr"
	"github.com/ccastorena/jira-agent/jiraissueui"
)

const jiraUserSearchLimit = 20

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

type jiraIssueTypeOption struct {
	Name           string
	Subtask        bool
	HierarchyLevel int
}

type jiraIssueTypePayload struct {
	Name           string `json:"name"`
	Subtask        bool   `json:"subtask"`
	HierarchyLevel int    `json:"hierarchyLevel"`
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

func (a *webApp) fetchJiraIssueTypes(projectKey string) ([]jiraIssueTypeOption, error) {
	if projectKey == "" {
		return nil, nil
	}
	raw, err := a.jc.ListIssueTypes(projectKey)
	if err != nil {
		return nil, err
	}
	types, err := parseJiraIssueTypes(raw)
	if err != nil {
		return nil, err
	}
	sortJiraIssueTypes(types)
	return types, nil
}

func sortJiraIssueTypes(types []jiraIssueTypeOption) {
	sort.Slice(types, func(i, j int) bool {
		if isSubtaskParentIssueType(types[i]) != isSubtaskParentIssueType(types[j]) {
			return isSubtaskParentIssueType(types[i])
		}
		if types[i].Subtask != types[j].Subtask {
			return !types[i].Subtask
		}
		if issueTypeSortRank(types[i].Name) != issueTypeSortRank(types[j].Name) {
			return issueTypeSortRank(types[i].Name) < issueTypeSortRank(types[j].Name)
		}
		return types[i].Name < types[j].Name
	})
}

func parseJiraIssueTypes(raw json.RawMessage) ([]jiraIssueTypeOption, error) {
	trimmed := strings.TrimSpace(string(raw))
	if strings.HasPrefix(trimmed, "[") {
		var payload []jiraIssueTypePayload
		if err := json.Unmarshal(raw, &payload); err != nil {
			return nil, err
		}
		return issueTypeOptionsFromPayload(payload)
	}
	var wrapper struct {
		Values     []jiraIssueTypePayload `json:"values"`
		IssueTypes []jiraIssueTypePayload `json:"issueTypes"`
		Projects   []struct {
			IssueTypes []jiraIssueTypePayload `json:"issuetypes"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, err
	}
	payload := wrapper.Values
	if len(payload) == 0 {
		payload = wrapper.IssueTypes
	}
	if len(payload) == 0 {
		for _, project := range wrapper.Projects {
			payload = append(payload, project.IssueTypes...)
		}
	}
	return issueTypeOptionsFromPayload(payload)
}

func issueTypeOptionsFromPayload(payload []jiraIssueTypePayload) ([]jiraIssueTypeOption, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("no Jira issue types returned")
	}
	items := make([]jiraIssueTypeOption, 0, len(payload))
	for _, item := range payload {
		if item.Name == "" {
			continue
		}
		items = append(items, jiraIssueTypeOption{Name: item.Name, Subtask: item.Subtask, HierarchyLevel: item.HierarchyLevel})
	}
	return items, nil
}

func parentIssueTypeNames(types []jiraIssueTypeOption) []string {
	names := make([]string, 0, len(types))
	for _, issueType := range types {
		if isSubtaskParentIssueType(issueType) {
			names = append(names, issueType.Name)
		}
	}
	return names
}

func firstSubtaskIssueTypeName(types []jiraIssueTypeOption) string {
	for _, issueType := range types {
		if issueType.Subtask {
			return issueType.Name
		}
	}
	return ""
}

func validParentIssueType(selected string, types []jiraIssueTypeOption) string {
	selected = strings.TrimSpace(selected)
	first := ""
	for _, issueType := range types {
		if !isSubtaskParentIssueType(issueType) {
			continue
		}
		if first == "" {
			first = issueType.Name
		}
		if issueType.Name == selected {
			return issueType.Name
		}
	}
	return first
}

func isSubtaskParentIssueType(issueType jiraIssueTypeOption) bool {
	if issueType.Subtask || issueType.HierarchyLevel > 0 {
		return false
	}
	return !strings.EqualFold(issueType.Name, "Epic")
}

func issueTypeSortRank(name string) int {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "task":
		return 0
	case "request":
		return 1
	case "story":
		return 2
	case "bug":
		return 3
	case "epic":
		return 100
	default:
		return 50
	}
}

func (a *webApp) fetchJiraAssignableUsers(projectKey, query string) ([]jiraissueui.User, error) {
	if projectKey == "" {
		return nil, nil
	}
	raw, err := a.jc.SearchAssignableUsers(projectKey, strings.TrimSpace(query), jiraUserSearchLimit)
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

func (a *webApp) pullRequestPicker() githubpr.Picker {
	if a.gc == nil {
		return githubpr.NewPicker(nil)
	}
	return githubpr.NewPicker(a.gc)
}
