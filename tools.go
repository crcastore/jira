package main

import (
	"encoding/json"
	"fmt"

	openai "github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"
)

// ToolSchemas describes every tool exposed to the LLM.
var ToolSchemas = []openai.Tool{
	{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{
		Name:        "search_issues",
		Description: "Search Jira issues using JQL. Returns a compact list of matching issues.",
		Parameters: jsonschema.Definition{
			Type: jsonschema.Object,
			Properties: map[string]jsonschema.Definition{
				"jql":         {Type: jsonschema.String, Description: "JQL query, e.g. 'assignee = currentUser() AND status != Done'"},
				"max_results": {Type: jsonschema.Integer},
			},
			Required: []string{"jql"},
		},
	}},
	{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{
		Name:        "get_issue",
		Description: "Fetch full details for one issue by its key (e.g. ABC-123).",
		Parameters: jsonschema.Definition{
			Type:       jsonschema.Object,
			Properties: map[string]jsonschema.Definition{"key": {Type: jsonschema.String}},
			Required:   []string{"key"},
		},
	}},
	{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{
		Name:        "create_issue",
		Description: "Create a new Jira issue in the given project.",
		Parameters: jsonschema.Definition{
			Type: jsonschema.Object,
			Properties: map[string]jsonschema.Definition{
				"project_key":         {Type: jsonschema.String},
				"summary":             {Type: jsonschema.String},
				"issue_type":          {Type: jsonschema.String},
				"description":         {Type: jsonschema.String},
				"assignee_account_id": {Type: jsonschema.String},
				"priority":            {Type: jsonschema.String, Description: "e.g. Highest, High, Medium, Low, Lowest"},
				"labels":              {Type: jsonschema.Array, Items: &jsonschema.Definition{Type: jsonschema.String}},
			},
			Required: []string{"project_key", "summary"},
		},
	}},
	{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{
		Name:        "add_comment",
		Description: "Add a plain-text comment to an issue.",
		Parameters: jsonschema.Definition{
			Type: jsonschema.Object,
			Properties: map[string]jsonschema.Definition{
				"key":  {Type: jsonschema.String},
				"body": {Type: jsonschema.String},
			},
			Required: []string{"key", "body"},
		},
	}},
	{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{
		Name:        "list_transitions",
		Description: "List available workflow transitions for an issue (use before transition_issue).",
		Parameters: jsonschema.Definition{
			Type:       jsonschema.Object,
			Properties: map[string]jsonschema.Definition{"key": {Type: jsonschema.String}},
			Required:   []string{"key"},
		},
	}},
	{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{
		Name:        "transition_issue",
		Description: "Move an issue to a new status using a transition ID from list_transitions.",
		Parameters: jsonschema.Definition{
			Type: jsonschema.Object,
			Properties: map[string]jsonschema.Definition{
				"key":           {Type: jsonschema.String},
				"transition_id": {Type: jsonschema.String},
			},
			Required: []string{"key", "transition_id"},
		},
	}},
	{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{
		Name:        "update_issue_fields",
		Description: "Update arbitrary fields on an issue. `fields` must be a JSON object matching Jira's edit API.",
		Parameters: jsonschema.Definition{
			Type: jsonschema.Object,
			Properties: map[string]jsonschema.Definition{
				"key":    {Type: jsonschema.String},
				"fields": {Type: jsonschema.Object},
			},
			Required: []string{"key", "fields"},
		},
	}},
	{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{
		Name:        "search_users",
		Description: "Find users by name/email; returns accountIds usable as assignee.",
		Parameters: jsonschema.Definition{
			Type: jsonschema.Object,
			Properties: map[string]jsonschema.Definition{
				"query":       {Type: jsonschema.String},
				"max_results": {Type: jsonschema.Integer},
			},
			Required: []string{"query"},
		},
	}},
	{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{
		Name:        "list_projects",
		Description: "List Jira projects available to the current user.",
		Parameters:  jsonschema.Definition{Type: jsonschema.Object, Properties: map[string]jsonschema.Definition{}},
	}},
	{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{
		Name:        "myself",
		Description: "Return the authenticated user's profile (accountId, email, displayName).",
		Parameters:  jsonschema.Definition{Type: jsonschema.Object, Properties: map[string]jsonschema.Definition{}},
	}},
}

const toolResultMaxBytes = 15000

// CallTool dispatches a tool call coming back from the LLM to the JiraClient
// and returns a JSON string suitable for the "tool" message content.
func CallTool(jc *JiraClient, name, argsJSON string) string {
	if argsJSON == "" {
		argsJSON = "{}"
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return errJSON(fmt.Sprintf("invalid JSON arguments: %v", err))
	}

	var (
		raw json.RawMessage
		err error
	)

	switch name {
	case "search_issues":
		jql, _ := args["jql"].(string)
		max := intArg(args["max_results"])
		raw, err = jc.Search(jql, nil, max)
		if err == nil {
			return string(trimSearch(raw))
		}
	case "get_issue":
		key, _ := args["key"].(string)
		raw, err = jc.GetIssue(key)
		if err == nil {
			return string(trimIssue(raw))
		}
	case "create_issue":
		var a CreateIssueArgs
		b, _ := json.Marshal(args)
		if uerr := json.Unmarshal(b, &a); uerr != nil {
			return errJSON(fmt.Sprintf("bad arguments for create_issue: %v", uerr))
		}
		raw, err = jc.CreateIssue(a)
	case "add_comment":
		key, _ := args["key"].(string)
		body, _ := args["body"].(string)
		raw, err = jc.AddComment(key, body)
	case "list_transitions":
		key, _ := args["key"].(string)
		raw, err = jc.ListTransitions(key)
	case "transition_issue":
		key, _ := args["key"].(string)
		tid, _ := args["transition_id"].(string)
		raw, err = jc.TransitionIssue(key, tid)
	case "update_issue_fields":
		key, _ := args["key"].(string)
		fields, _ := args["fields"].(map[string]any)
		raw, err = jc.UpdateIssue(key, fields)
	case "search_users":
		q, _ := args["query"].(string)
		raw, err = jc.SearchUsers(q, intArg(args["max_results"]))
	case "list_projects":
		raw, err = jc.ListProjects()
	case "myself":
		raw, err = jc.Myself()
	default:
		return errJSON(fmt.Sprintf("unknown tool '%s'", name))
	}

	if err != nil {
		return errJSON(err.Error())
	}
	out := string(raw)
	if len(out) > toolResultMaxBytes {
		out = out[:toolResultMaxBytes]
	}
	return out
}

func errJSON(msg string) string {
	b, _ := json.Marshal(map[string]string{"error": msg})
	return string(b)
}

func intArg(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	}
	return 0
}

// trimSearch reduces the noisy Jira search payload to fields the LLM needs.
func trimSearch(raw json.RawMessage) json.RawMessage {
	var payload struct {
		Issues        []map[string]any `json:"issues"`
		NextPageToken any              `json:"nextPageToken"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return raw
	}
	slim := make([]map[string]any, 0, len(payload.Issues))
	for _, it := range payload.Issues {
		f, _ := it["fields"].(map[string]any)
		slim = append(slim, map[string]any{
			"key":      it["key"],
			"summary":  pick(f, "summary"),
			"status":   nested(f, "status", "name"),
			"assignee": nested(f, "assignee", "displayName"),
			"priority": nested(f, "priority", "name"),
			"type":     nested(f, "issuetype", "name"),
			"updated":  pick(f, "updated"),
		})
	}
	b, _ := json.Marshal(map[string]any{
		"count":           len(slim),
		"next_page_token": payload.NextPageToken,
		"issues":          slim,
	})
	return b
}

func trimIssue(raw json.RawMessage) json.RawMessage {
	var issue map[string]any
	if err := json.Unmarshal(raw, &issue); err != nil {
		return raw
	}
	f, _ := issue["fields"].(map[string]any)
	b, _ := json.Marshal(map[string]any{
		"key":         issue["key"],
		"summary":     pick(f, "summary"),
		"status":      nested(f, "status", "name"),
		"assignee":    nested(f, "assignee", "displayName"),
		"reporter":    nested(f, "reporter", "displayName"),
		"priority":    nested(f, "priority", "name"),
		"type":        nested(f, "issuetype", "name"),
		"labels":      pick(f, "labels"),
		"created":     pick(f, "created"),
		"updated":     pick(f, "updated"),
		"description": pick(f, "description"),
	})
	return b
}

func pick(m map[string]any, k string) any {
	if m == nil {
		return nil
	}
	return m[k]
}

func nested(m map[string]any, k1, k2 string) any {
	if m == nil {
		return nil
	}
	inner, _ := m[k1].(map[string]any)
	if inner == nil {
		return nil
	}
	return inner[k2]
}
