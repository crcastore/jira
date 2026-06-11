package main

import (
	"encoding/json"
	"fmt"

	openai "github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"
)

// ToolSchemas describes every tool exposed to the LLM.
// Trimmed to a focused 8-tool GitHub issue workflow. The removed tool schemas
// (all Jira tools + extra GitHub tools) are archived in REMOVED_TOOLS.md and can
// be pasted back here to restore them. CallTool still dispatches them if called.
var ToolSchemas = []openai.Tool{
	// ---------- GitHub ----------
	{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{
		Name:        "gh_me",
		Description: "Get the authenticated GitHub user.",
		Parameters:  jsonschema.Definition{Type: jsonschema.Object, Properties: map[string]jsonschema.Definition{}},
	}},
	{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{
		Name:        "gh_list_my_repos",
		Description: "List the user's GitHub repos (auto-paginates).",
		Parameters: jsonschema.Definition{
			Type: jsonschema.Object,
			Properties: map[string]jsonschema.Definition{
				"visibility": {Type: jsonschema.String, Description: "all|public|private"},
				"sort":       {Type: jsonschema.String, Description: "created|updated|pushed|full_name"},
				"max_total":  {Type: jsonschema.Integer},
			},
		},
	}},
	{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{
		Name:        "gh_get_repo",
		Description: "Get a GitHub repo's metadata.",
		Parameters: jsonschema.Definition{
			Type: jsonschema.Object,
			Properties: map[string]jsonschema.Definition{
				"owner": {Type: jsonschema.String},
				"repo":  {Type: jsonschema.String},
			},
			Required: []string{"owner", "repo"},
		},
	}},
	{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{
		Name:        "gh_list_issues",
		Description: "List repo issues (includes PRs).",
		Parameters: jsonschema.Definition{
			Type: jsonschema.Object,
			Properties: map[string]jsonschema.Definition{
				"owner":    {Type: jsonschema.String},
				"repo":     {Type: jsonschema.String},
				"state":    {Type: jsonschema.String, Description: "open|closed|all"},
				"labels":   {Type: jsonschema.String, Description: "comma-separated labels"},
				"assignee": {Type: jsonschema.String, Description: "login, '*', or 'none'"},
				"per_page": {Type: jsonschema.Integer},
			},
			Required: []string{"owner", "repo"},
		},
	}},
	{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{
		Name:        "gh_get_issue",
		Description: "Get one GitHub issue/PR by number.",
		Parameters: jsonschema.Definition{
			Type: jsonschema.Object,
			Properties: map[string]jsonschema.Definition{
				"owner":  {Type: jsonschema.String},
				"repo":   {Type: jsonschema.String},
				"number": {Type: jsonschema.Integer},
			},
			Required: []string{"owner", "repo", "number"},
		},
	}},
	{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{
		Name:        "gh_create_issue",
		Description: "Create a GitHub issue.",
		Parameters: jsonschema.Definition{
			Type: jsonschema.Object,
			Properties: map[string]jsonschema.Definition{
				"owner":     {Type: jsonschema.String},
				"repo":      {Type: jsonschema.String},
				"title":     {Type: jsonschema.String},
				"body":      {Type: jsonschema.String},
				"assignees": {Type: jsonschema.Array, Items: &jsonschema.Definition{Type: jsonschema.String}},
				"labels":    {Type: jsonschema.Array, Items: &jsonschema.Definition{Type: jsonschema.String}},
			},
			Required: []string{"owner", "repo", "title"},
		},
	}},
	{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{
		Name:        "gh_close_issue",
		Description: "Close a GitHub issue.",
		Parameters: jsonschema.Definition{
			Type: jsonschema.Object,
			Properties: map[string]jsonschema.Definition{
				"owner":  {Type: jsonschema.String},
				"repo":   {Type: jsonschema.String},
				"number": {Type: jsonschema.Integer},
			},
			Required: []string{"owner", "repo", "number"},
		},
	}},
	{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{
		Name:        "gh_comment_issue",
		Description: "Comment on a GitHub issue or PR.",
		Parameters: jsonschema.Definition{
			Type: jsonschema.Object,
			Properties: map[string]jsonschema.Definition{
				"owner":  {Type: jsonschema.String},
				"repo":   {Type: jsonschema.String},
				"number": {Type: jsonschema.Integer},
				"body":   {Type: jsonschema.String},
			},
			Required: []string{"owner", "repo", "number", "body"},
		},
	}},
}

const toolResultMaxBytes = 60000

// toolAliases maps common model-hallucinated tool names to the canonical name.
var toolAliases = map[string]string{
	"gh_list_repos":  "gh_list_my_repos",
	"gh_repos":       "gh_list_my_repos",
	"list_repos":     "gh_list_my_repos",
	"list_my_repos":  "gh_list_my_repos",
	"gh_my_repos":    "gh_list_my_repos",
	"search_jira":    "search_issues",
	"jira_search":    "search_issues",
	"get_jira_issue": "get_issue",
}

// canonicalToolName resolves an alias to its canonical tool name, returning the
// input unchanged when there is no alias.
func canonicalToolName(name string) string {
	if c, ok := toolAliases[name]; ok {
		return c
	}
	return name
}

// CallTool dispatches a tool call coming back from the LLM to the right client
// (Jira or GitHub) and returns a JSON string suitable for the "tool" message content.
// gc may be nil if no GitHub credentials are configured.
func CallTool(jc *JiraClient, gc *GitHubClient, name, argsJSON string) string {
	if argsJSON == "" {
		argsJSON = "{}"
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return errJSON(fmt.Sprintf("invalid JSON arguments: %v", err))
	}

	name = canonicalToolName(name)

	var (
		raw json.RawMessage
		err error
	)

	switch name {
	// ---------- Jira ----------
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

	// ---------- GitHub ----------
	case "gh_me", "gh_list_my_repos", "gh_get_repo", "gh_search_repos",
		"gh_list_issues", "gh_get_issue", "gh_create_issue", "gh_update_issue",
		"gh_close_issue", "gh_comment_issue", "gh_search_issues",
		"gh_list_pulls", "gh_get_pull", "gh_create_pull", "gh_merge_pull",
		"gh_list_pr_files", "gh_review_pull",
		"gh_list_workflows", "gh_run_workflow", "gh_list_workflow_runs", "gh_get_workflow_run",
		"gh_wait_for_workflow_run", "gh_run_workflow_and_wait":
		if gc == nil {
			return errJSON("GitHub is not configured (set GITHUB_TOKEN)")
		}
		raw, err = callGitHub(gc, name, args)
		if err == nil {
			raw = trimGitHub(name, raw)
		}

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

func callGitHub(gc *GitHubClient, name string, args map[string]any) (json.RawMessage, error) {
	owner, _ := args["owner"].(string)
	repo, _ := args["repo"].(string)
	number := intArg(args["number"])

	switch name {
	case "gh_me":
		return gc.Me()
	case "gh_list_my_repos":
		vis, _ := args["visibility"].(string)
		sort, _ := args["sort"].(string)
		max := intArg(args["max_total"])
		if max == 0 {
			max = intArg(args["per_page"]) // backward-compat if model still passes per_page
		}
		return gc.ListMyRepos(vis, sort, max)
	case "gh_get_repo":
		return gc.GetRepo(owner, repo)
	case "gh_search_repos":
		q, _ := args["query"].(string)
		return gc.SearchRepos(q, intArg(args["per_page"]))
	case "gh_list_issues":
		state, _ := args["state"].(string)
		labels, _ := args["labels"].(string)
		assignee, _ := args["assignee"].(string)
		return gc.ListIssues(owner, repo, state, labels, assignee, intArg(args["per_page"]))
	case "gh_get_issue":
		return gc.GetIssue(owner, repo, number)
	case "gh_create_issue":
		var a GHCreateIssueArgs
		b, _ := json.Marshal(args)
		if err := json.Unmarshal(b, &a); err != nil {
			return nil, fmt.Errorf("bad arguments for gh_create_issue: %w", err)
		}
		return gc.CreateIssue(a)
	case "gh_update_issue":
		fields, _ := args["fields"].(map[string]any)
		return gc.UpdateIssue(owner, repo, number, fields)
	case "gh_close_issue":
		return gc.CloseIssue(owner, repo, number)
	case "gh_comment_issue":
		body, _ := args["body"].(string)
		return gc.CommentIssue(owner, repo, number, body)
	case "gh_search_issues":
		q, _ := args["query"].(string)
		return gc.SearchIssues(q, intArg(args["per_page"]))
	case "gh_list_pulls":
		state, _ := args["state"].(string)
		return gc.ListPulls(owner, repo, state, intArg(args["per_page"]))
	case "gh_get_pull":
		return gc.GetPull(owner, repo, number)
	case "gh_create_pull":
		var a GHCreatePullArgs
		b, _ := json.Marshal(args)
		if err := json.Unmarshal(b, &a); err != nil {
			return nil, fmt.Errorf("bad arguments for gh_create_pull: %w", err)
		}
		return gc.CreatePull(a)
	case "gh_merge_pull":
		method, _ := args["merge_method"].(string)
		title, _ := args["commit_title"].(string)
		msg, _ := args["commit_message"].(string)
		return gc.MergePull(owner, repo, number, method, title, msg)
	case "gh_list_pr_files":
		return gc.ListPullFiles(owner, repo, number, intArg(args["per_page"]))
	case "gh_review_pull":
		event, _ := args["event"].(string)
		body, _ := args["body"].(string)
		return gc.ReviewPull(owner, repo, number, event, body)
	case "gh_list_workflows":
		return gc.ListWorkflows(owner, repo)
	case "gh_run_workflow":
		wid := strOrIntArg(args["workflow_id"])
		ref, _ := args["ref"].(string)
		if ref == "" {
			ref = "main"
		}
		if wid == "" {
			return nil, fmt.Errorf("gh_run_workflow: workflow_id is required (numeric ID or filename like 'test.yml')")
		}
		inputs, _ := args["inputs"].(map[string]any)
		return gc.RunWorkflow(owner, repo, wid, ref, inputs)
	case "gh_list_workflow_runs":
		wid := strOrIntArg(args["workflow_id"])
		status, _ := args["status"].(string)
		branch, _ := args["branch"].(string)
		return gc.ListWorkflowRuns(owner, repo, wid, status, branch, intArg(args["per_page"]))
	case "gh_get_workflow_run":
		runID := intArg(args["run_id"])
		return gc.GetWorkflowRun(owner, repo, runID)
	case "gh_wait_for_workflow_run":
		runID := intArg(args["run_id"])
		timeout := intArg(args["timeout_sec"])
		interval := intArg(args["interval_sec"])
		raw, ok, err := gc.WaitForWorkflowRun(owner, repo, runID, timeout, interval)
		if err != nil {
			return nil, err
		}
		return wrapWaitResult(raw, ok), nil
	case "gh_run_workflow_and_wait":
		wid := strOrIntArg(args["workflow_id"])
		ref, _ := args["ref"].(string)
		if ref == "" {
			ref = "main"
		}
		if wid == "" {
			return nil, fmt.Errorf("gh_run_workflow_and_wait: workflow_id is required (numeric ID or filename like 'test.yml')")
		}
		inputs, _ := args["inputs"].(map[string]any)
		timeout := intArg(args["timeout_sec"])
		interval := intArg(args["interval_sec"])
		raw, ok, err := gc.RunWorkflowAndWait(owner, repo, wid, ref, inputs, timeout, interval)
		if err != nil {
			return nil, err
		}
		return wrapWaitResult(raw, ok), nil
	}
	return nil, fmt.Errorf("unhandled github tool %q", name)
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

// strOrIntArg accepts a value that may arrive as a string or a JSON number
// (e.g. a workflow ID) and returns a string usable in a URL path.
func strOrIntArg(v any) string {
	switch n := v.(type) {
	case string:
		return n
	case float64:
		return fmt.Sprintf("%d", int64(n))
	case int:
		return fmt.Sprintf("%d", n)
	case int64:
		return fmt.Sprintf("%d", n)
	}
	return ""
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

// trimGitHub reduces noisy GitHub payloads to the fields the LLM actually needs,
// so list responses don't get truncated to 2-3 entries by toolResultMaxBytes.
func trimGitHub(name string, raw json.RawMessage) json.RawMessage {
	switch name {
	case "gh_me":
		return pickFields(raw, "login", "id", "name", "email", "html_url", "company", "bio")

	case "gh_list_my_repos":
		return slimArray(raw, slimRepo)

	case "gh_get_repo":
		return jsonOrRaw(slimRepo(asMap(raw)), raw)

	case "gh_search_repos":
		return slimSearchItems(raw, slimRepo)

	case "gh_list_issues":
		return slimArray(raw, slimIssue)

	case "gh_get_issue", "gh_create_issue", "gh_update_issue", "gh_close_issue":
		return jsonOrRaw(slimIssue(asMap(raw)), raw)

	case "gh_comment_issue":
		return pickFields(raw, "id", "html_url", "user", "body", "created_at")

	case "gh_search_issues":
		return slimSearchItems(raw, slimIssue)

	case "gh_list_pulls":
		return slimArray(raw, slimPull)

	case "gh_list_workflows":
		var payload struct {
			TotalCount int              `json:"total_count"`
			Workflows  []map[string]any `json:"workflows"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			return raw
		}
		slim := make([]map[string]any, 0, len(payload.Workflows))
		for _, w := range payload.Workflows {
			slim = append(slim, map[string]any{
				"id":       w["id"],
				"name":     w["name"],
				"path":     w["path"],
				"state":    w["state"],
				"html_url": w["html_url"],
			})
		}
		b, _ := json.Marshal(map[string]any{"count": payload.TotalCount, "workflows": slim})
		return b

	case "gh_run_workflow":
		// 204 No Content -> {}; surface a friendly confirmation instead.
		b, _ := json.Marshal(map[string]any{"dispatched": true,
			"note": "Workflow accepted. Use gh_list_workflow_runs to find the new run id."})
		return b

	case "gh_list_workflow_runs":
		var payload struct {
			TotalCount   int              `json:"total_count"`
			WorkflowRuns []map[string]any `json:"workflow_runs"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			return raw
		}
		slim := make([]map[string]any, 0, len(payload.WorkflowRuns))
		for _, r := range payload.WorkflowRuns {
			slim = append(slim, slimWorkflowRun(r))
		}
		b, _ := json.Marshal(map[string]any{
			"total_count": payload.TotalCount, "returned": len(slim), "runs": slim})
		return b

	case "gh_get_workflow_run":
		return jsonOrRaw(slimWorkflowRun(asMap(raw)), raw)

	case "gh_wait_for_workflow_run", "gh_run_workflow_and_wait":
		var w struct {
			Completed bool            `json:"completed"`
			Run       json.RawMessage `json:"run"`
		}
		if err := json.Unmarshal(raw, &w); err != nil {
			return raw
		}
		out, _ := json.Marshal(map[string]any{
			"completed": w.Completed,
			"run":       slimWorkflowRun(asMap(w.Run)),
		})
		return out

	case "gh_get_pull", "gh_create_pull":
		return jsonOrRaw(slimPull(asMap(raw)), raw)

	case "gh_merge_pull":
		return pickFields(raw, "sha", "merged", "message")

	case "gh_list_pr_files":
		return slimArray(raw, func(m map[string]any) map[string]any {
			return map[string]any{
				"filename":  m["filename"],
				"status":    m["status"],
				"additions": m["additions"],
				"deletions": m["deletions"],
				"changes":   m["changes"],
			}
		})

	case "gh_review_pull":
		return pickFields(raw, "id", "state", "html_url", "body", "submitted_at")
	}
	return raw
}

func slimRepo(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	return map[string]any{
		"full_name":      m["full_name"],
		"private":        m["private"],
		"description":    m["description"],
		"language":       m["language"],
		"default_branch": m["default_branch"],
		"stargazers":     m["stargazers_count"],
		"open_issues":    m["open_issues_count"],
		"updated_at":     m["updated_at"],
		"html_url":       m["html_url"],
	}
}

func slimIssue(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := map[string]any{
		"number":     m["number"],
		"title":      m["title"],
		"state":      m["state"],
		"user":       nested(m, "user", "login"),
		"assignee":   nested(m, "assignee", "login"),
		"labels":     labelNames(m["labels"]),
		"comments":   m["comments"],
		"created_at": m["created_at"],
		"updated_at": m["updated_at"],
		"html_url":   m["html_url"],
	}
	if repo, ok := m["repository"].(map[string]any); ok {
		out["repo"] = repo["full_name"]
	}
	if _, isPR := m["pull_request"]; isPR {
		out["is_pr"] = true
	}
	return out
}

func slimPull(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	return map[string]any{
		"number":        m["number"],
		"title":         m["title"],
		"state":         m["state"],
		"draft":         m["draft"],
		"user":          nested(m, "user", "login"),
		"head":          nested(m, "head", "ref"),
		"base":          nested(m, "base", "ref"),
		"merged":        m["merged"],
		"mergeable":     m["mergeable"],
		"additions":     m["additions"],
		"deletions":     m["deletions"],
		"changed_files": m["changed_files"],
		"created_at":    m["created_at"],
		"updated_at":    m["updated_at"],
		"html_url":      m["html_url"],
	}
}

func labelNames(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, l := range arr {
		if m, ok := l.(map[string]any); ok {
			if name, ok := m["name"].(string); ok {
				out = append(out, name)
			}
		}
	}
	return out
}

func slimWorkflowRun(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	return map[string]any{
		"id":          m["id"],
		"name":        m["name"],
		"workflow_id": m["workflow_id"],
		"event":       m["event"],
		"status":      m["status"],
		"conclusion":  m["conclusion"],
		"head_branch": m["head_branch"],
		"head_sha":    m["head_sha"],
		"run_number":  m["run_number"],
		"run_attempt": m["run_attempt"],
		"created_at":  m["created_at"],
		"updated_at":  m["updated_at"],
		"html_url":    m["html_url"],
		"actor":       nested(m, "actor", "login"),
	}
}

func slimArray(raw json.RawMessage, fn func(map[string]any) map[string]any) json.RawMessage {
	var arr []map[string]any
	if err := json.Unmarshal(raw, &arr); err != nil {
		return raw
	}
	slim := make([]map[string]any, 0, len(arr))
	for _, m := range arr {
		slim = append(slim, fn(m))
	}
	b, _ := json.Marshal(map[string]any{"count": len(slim), "items": slim})
	return b
}

func slimSearchItems(raw json.RawMessage, fn func(map[string]any) map[string]any) json.RawMessage {
	var payload struct {
		TotalCount        int              `json:"total_count"`
		IncompleteResults bool             `json:"incomplete_results"`
		Items             []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return raw
	}
	slim := make([]map[string]any, 0, len(payload.Items))
	for _, m := range payload.Items {
		slim = append(slim, fn(m))
	}
	b, _ := json.Marshal(map[string]any{
		"total_count":        payload.TotalCount,
		"incomplete_results": payload.IncompleteResults,
		"returned":           len(slim),
		"items":              slim,
	})
	return b
}

func pickFields(raw json.RawMessage, keys ...string) json.RawMessage {
	m := asMap(raw)
	if m == nil {
		return raw
	}
	out := make(map[string]any, len(keys))
	for _, k := range keys {
		if v, ok := m[k]; ok {
			out[k] = v
		}
	}
	b, _ := json.Marshal(out)
	return b
}

func asMap(raw json.RawMessage) map[string]any {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return m
}

func jsonOrRaw(v any, fallback json.RawMessage) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return fallback
	}
	return b
}

// wrapWaitResult packages a workflow run plus a completed flag so the LLM
// gets both pieces of info in a stable shape.
func wrapWaitResult(run json.RawMessage, completed bool) json.RawMessage {
	if len(run) == 0 {
		run = json.RawMessage("null")
	}
	b, _ := json.Marshal(map[string]any{
		"completed": completed,
		"run":       json.RawMessage(run),
	})
	return b
}
