package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

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
		items = append(items, jiraIssueTypeOption{
			Name:           item.Name,
			Subtask:        item.Subtask,
			HierarchyLevel: item.HierarchyLevel,
		})
	}
	return items, nil
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
