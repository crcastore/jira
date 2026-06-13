package jiracreate

import (
	"encoding/json"
	"strings"
	"testing"
)

const issueTypesJSON = `{"values":[{"name":"Epic","subtask":false,"hierarchyLevel":1},{"name":"Request","subtask":false,"hierarchyLevel":0},{"name":"Task","subtask":false,"hierarchyLevel":0},{"name":"Subtask","subtask":true,"hierarchyLevel":-1}]}`

func TestParseIssueTypesAcceptsResponseShapes(t *testing.T) {
	cases := []json.RawMessage{
		json.RawMessage(`{"values":[{"name":"Task","subtask":false},{"name":"Subtask","subtask":true}]}`),
		json.RawMessage(`{"issueTypes":[{"name":"Task","subtask":false},{"name":"Subtask","subtask":true}]}`),
		json.RawMessage(`{"projects":[{"issuetypes":[{"name":"Task","subtask":false},{"name":"Subtask","subtask":true}]}]}`),
		json.RawMessage(`[{"name":"Task","subtask":false},{"name":"Subtask","subtask":true}]`),
	}
	for _, raw := range cases {
		types, err := ParseIssueTypes(raw)
		if err != nil {
			t.Fatalf("ParseIssueTypes(%s) returned error: %v", raw, err)
		}
		if len(types) != 2 || types[0].Name != "Task" || types[1].Name != "Subtask" || !types[1].Subtask {
			t.Fatalf("unexpected types from %s: %+v", raw, types)
		}
	}
}

func TestParentIssueTypeNamesExcludeEpicAndPreferTask(t *testing.T) {
	types, err := ParseIssueTypes(json.RawMessage(issueTypesJSON))
	if err != nil {
		t.Fatalf("ParseIssueTypes returned error: %v", err)
	}
	SortIssueTypes(types)
	names := ParentIssueTypeNames(types)
	if strings.Join(names, ",") != "Task,Request" {
		t.Fatalf("parent issue type names = %#v, want Task, Request", names)
	}
	if got := ValidParentIssueType("Epic", types); got != "Task" {
		t.Fatalf("ValidParentIssueType(Epic) = %q, want Task", got)
	}
}
