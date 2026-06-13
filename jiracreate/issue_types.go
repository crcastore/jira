package jiracreate

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// IssueType is the Jira metadata needed to choose parent and subtask issue types.
type IssueType struct {
	Name           string
	Subtask        bool
	HierarchyLevel int
}

type issueTypePayload struct {
	Name           string `json:"name"`
	Subtask        bool   `json:"subtask"`
	HierarchyLevel int    `json:"hierarchyLevel"`
}

// ParseIssueTypes accepts the common Jira createmeta response shapes used by
// Jira Cloud and older createmeta endpoints.
func ParseIssueTypes(raw json.RawMessage) ([]IssueType, error) {
	trimmed := strings.TrimSpace(string(raw))
	if strings.HasPrefix(trimmed, "[") {
		var payload []issueTypePayload
		if err := json.Unmarshal(raw, &payload); err != nil {
			return nil, err
		}
		return issueTypesFromPayload(payload)
	}

	var wrapper struct {
		Values     []issueTypePayload `json:"values"`
		IssueTypes []issueTypePayload `json:"issueTypes"`
		Projects   []struct {
			IssueTypes []issueTypePayload `json:"issuetypes"`
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
	return issueTypesFromPayload(payload)
}

func issueTypesFromPayload(payload []issueTypePayload) ([]IssueType, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("no Jira issue types returned")
	}
	items := make([]IssueType, 0, len(payload))
	for _, item := range payload {
		if item.Name == "" {
			continue
		}
		items = append(items, IssueType{
			Name:           item.Name,
			Subtask:        item.Subtask,
			HierarchyLevel: item.HierarchyLevel,
		})
	}
	return items, nil
}

// SortIssueTypes orders standard parent issue types first, then other parent
// types, then subtask types. Task-like types are preferred for create forms that
// may also create subtasks.
func SortIssueTypes(types []IssueType) {
	sort.Slice(types, func(i, j int) bool {
		if CanHaveSubtasks(types[i]) != CanHaveSubtasks(types[j]) {
			return CanHaveSubtasks(types[i])
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

// ParentIssueTypeNames returns issue types that can safely be used as parents
// for generated subtasks.
func ParentIssueTypeNames(types []IssueType) []string {
	names := make([]string, 0, len(types))
	for _, issueType := range types {
		if CanHaveSubtasks(issueType) {
			names = append(names, issueType.Name)
		}
	}
	return names
}

// FirstSubtaskIssueTypeName returns the first Jira issue type marked as a subtask.
func FirstSubtaskIssueTypeName(types []IssueType) string {
	for _, issueType := range types {
		if issueType.Subtask {
			return issueType.Name
		}
	}
	return ""
}

// ValidParentIssueType returns selected when it can host subtasks, otherwise the
// first available parent issue type after sorting.
func ValidParentIssueType(selected string, types []IssueType) string {
	selected = strings.TrimSpace(selected)
	first := ""
	for _, issueType := range types {
		if !CanHaveSubtasks(issueType) {
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

// CanHaveSubtasks excludes subtask types and higher-level issue types such as
// Epics, which Jira rejects as parents for subtasks.
func CanHaveSubtasks(issueType IssueType) bool {
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
