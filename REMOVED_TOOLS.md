# Removed Tools

`ToolSchemas` in `tools.go` was trimmed from **33 tools → 9 tools** to reduce the
schema sent to the LLM on every request. Only the schemas the model *sees* were
removed — the underlying client methods and the `CallTool` dispatch are still
present, so any tool below can be restored simply by pasting its schema block
back into the `ToolSchemas` slice in `tools.go`.

## Kept (9)

| Tool | Purpose |
| --- | --- |
| `gh_me` | Authenticated GitHub user |
| `gh_list_my_repos` | **Get repos** |
| `gh_get_repo` | Repo metadata |
| `gh_list_issues` | **See issues in a repo** |
| `gh_get_issue` | Get one issue/PR |
| `gh_list_pr_files` | See files changed in a pull request / merge request (MR) |
| `gh_create_issue` | **Open issue** |
| `gh_close_issue` | **Close issue** |
| `gh_comment_issue` | Comment on an issue/PR |

## Removed (24)

**Jira (10):** `search_issues`, `get_issue`, `create_issue`, `add_comment`,
`list_transitions`, `transition_issue`, `update_issue_fields`, `search_users`,
`list_projects`, `myself`

**GitHub (14):** `gh_search_repos`, `gh_update_issue`, `gh_search_issues`,
`gh_list_pulls`, `gh_get_pull`, `gh_create_pull`, `gh_merge_pull`,
`gh_review_pull`, `gh_list_workflows`, `gh_run_workflow`,
`gh_list_workflow_runs`, `gh_get_workflow_run`, `gh_wait_for_workflow_run`,
`gh_run_workflow_and_wait`

---

## Removed schema blocks (paste back to restore)

### Jira

```go
{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{
	Name:        "search_issues",
	Description: "Search Jira issues via JQL.",
	Parameters: jsonschema.Definition{
		Type: jsonschema.Object,
		Properties: map[string]jsonschema.Definition{
			"jql":         {Type: jsonschema.String, Description: "JQL query"},
			"max_results": {Type: jsonschema.Integer},
		},
		Required: []string{"jql"},
	},
}},
{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{
	Name:        "get_issue",
	Description: "Get one Jira issue by key (e.g. ABC-123).",
	Parameters: jsonschema.Definition{
		Type:       jsonschema.Object,
		Properties: map[string]jsonschema.Definition{"key": {Type: jsonschema.String}},
		Required:   []string{"key"},
	},
}},
{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{
	Name:        "create_issue",
	Description: "Create a Jira issue.",
	Parameters: jsonschema.Definition{
		Type: jsonschema.Object,
		Properties: map[string]jsonschema.Definition{
			"project_key":         {Type: jsonschema.String},
			"summary":             {Type: jsonschema.String},
			"issue_type":          {Type: jsonschema.String},
			"description":         {Type: jsonschema.String},
			"assignee_account_id": {Type: jsonschema.String},
			"priority":            {Type: jsonschema.String, Description: "Highest|High|Medium|Low|Lowest"},
			"labels":              {Type: jsonschema.Array, Items: &jsonschema.Definition{Type: jsonschema.String}},
		},
		Required: []string{"project_key", "summary"},
	},
}},
{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{
	Name:        "add_comment",
	Description: "Add a comment to a Jira issue.",
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
	Description: "List workflow transitions for a Jira issue.",
	Parameters: jsonschema.Definition{
		Type:       jsonschema.Object,
		Properties: map[string]jsonschema.Definition{"key": {Type: jsonschema.String}},
		Required:   []string{"key"},
	},
}},
{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{
	Name:        "transition_issue",
	Description: "Move a Jira issue to a new status (transition_id from list_transitions).",
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
	Description: "Update Jira issue fields (Jira edit-API JSON object).",
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
	Description: "Find Jira users; returns accountIds for assignee.",
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
	Description: "List Jira projects.",
	Parameters:  jsonschema.Definition{Type: jsonschema.Object, Properties: map[string]jsonschema.Definition{}},
}},
{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{
	Name:        "myself",
	Description: "Get the authenticated Jira user.",
	Parameters:  jsonschema.Definition{Type: jsonschema.Object, Properties: map[string]jsonschema.Definition{}},
}},
```

### GitHub

```go
{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{
	Name:        "gh_search_repos",
	Description: "Search GitHub repos (e.g. 'topic:cli language:go stars:>500').",
	Parameters: jsonschema.Definition{
		Type: jsonschema.Object,
		Properties: map[string]jsonschema.Definition{
			"query":    {Type: jsonschema.String},
			"per_page": {Type: jsonschema.Integer},
		},
		Required: []string{"query"},
	},
}},
{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{
	Name:        "gh_update_issue",
	Description: "Update a GitHub issue.",
	Parameters: jsonschema.Definition{
		Type: jsonschema.Object,
		Properties: map[string]jsonschema.Definition{
			"owner":  {Type: jsonschema.String},
			"repo":   {Type: jsonschema.String},
			"number": {Type: jsonschema.Integer},
			"fields": {Type: jsonschema.Object, Description: "fields to PATCH"},
		},
		Required: []string{"owner", "repo", "number", "fields"},
	},
}},
{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{
	Name:        "gh_search_issues",
	Description: "Search GitHub issues/PRs (e.g. 'is:open is:pr author:@me').",
	Parameters: jsonschema.Definition{
		Type: jsonschema.Object,
		Properties: map[string]jsonschema.Definition{
			"query":    {Type: jsonschema.String},
			"per_page": {Type: jsonschema.Integer},
		},
		Required: []string{"query"},
	},
}},
{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{
	Name:        "gh_list_pulls",
	Description: "List repo pull requests.",
	Parameters: jsonschema.Definition{
		Type: jsonschema.Object,
		Properties: map[string]jsonschema.Definition{
			"owner":    {Type: jsonschema.String},
			"repo":     {Type: jsonschema.String},
			"state":    {Type: jsonschema.String, Description: "open|closed|all"},
			"per_page": {Type: jsonschema.Integer},
		},
		Required: []string{"owner", "repo"},
	},
}},
{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{
	Name:        "gh_get_pull",
	Description: "Get one pull request.",
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
	Name:        "gh_create_pull",
	Description: "Open a pull request (head=source branch, base=target).",
	Parameters: jsonschema.Definition{
		Type: jsonschema.Object,
		Properties: map[string]jsonschema.Definition{
			"owner": {Type: jsonschema.String},
			"repo":  {Type: jsonschema.String},
			"title": {Type: jsonschema.String},
			"head":  {Type: jsonschema.String, Description: "source branch"},
			"base":  {Type: jsonschema.String, Description: "target branch"},
			"body":  {Type: jsonschema.String},
			"draft": {Type: jsonschema.Boolean},
		},
		Required: []string{"owner", "repo", "title", "head", "base"},
	},
}},
{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{
	Name:        "gh_merge_pull",
	Description: "Merge a PR (destructive; confirm first).",
	Parameters: jsonschema.Definition{
		Type: jsonschema.Object,
		Properties: map[string]jsonschema.Definition{
			"owner":          {Type: jsonschema.String},
			"repo":           {Type: jsonschema.String},
			"number":         {Type: jsonschema.Integer},
			"merge_method":   {Type: jsonschema.String, Description: "merge|squash|rebase"},
			"commit_title":   {Type: jsonschema.String},
			"commit_message": {Type: jsonschema.String},
		},
		Required: []string{"owner", "repo", "number"},
	},
}},
{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{
	Name:        "gh_list_pr_files",
	Description: "List files changed in a PR.",
	Parameters: jsonschema.Definition{
		Type: jsonschema.Object,
		Properties: map[string]jsonschema.Definition{
			"owner":    {Type: jsonschema.String},
			"repo":     {Type: jsonschema.String},
			"number":   {Type: jsonschema.Integer},
			"per_page": {Type: jsonschema.Integer},
		},
		Required: []string{"owner", "repo", "number"},
	},
}},
{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{
	Name:        "gh_review_pull",
	Description: "Review a PR.",
	Parameters: jsonschema.Definition{
		Type: jsonschema.Object,
		Properties: map[string]jsonschema.Definition{
			"owner":  {Type: jsonschema.String},
			"repo":   {Type: jsonschema.String},
			"number": {Type: jsonschema.Integer},
			"event":  {Type: jsonschema.String, Description: "APPROVE|REQUEST_CHANGES|COMMENT"},
			"body":   {Type: jsonschema.String},
		},
		Required: []string{"owner", "repo", "number", "event"},
	},
}},
{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{
	Name:        "gh_list_workflows",
	Description: "List Actions workflows in a repo.",
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
	Name:        "gh_run_workflow",
	Description: "Trigger a workflow_dispatch run (workflow must allow it).",
	Parameters: jsonschema.Definition{
		Type: jsonschema.Object,
		Properties: map[string]jsonschema.Definition{
			"owner":       {Type: jsonschema.String},
			"repo":        {Type: jsonschema.String},
			"workflow_id": {Type: jsonschema.String, Description: "numeric id or filename"},
			"ref":         {Type: jsonschema.String, Description: "branch/tag/sha (default main)"},
			"inputs":      {Type: jsonschema.Object},
		},
		Required: []string{"owner", "repo", "workflow_id"},
	},
}},
{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{
	Name:        "gh_list_workflow_runs",
	Description: "List recent workflow runs.",
	Parameters: jsonschema.Definition{
		Type: jsonschema.Object,
		Properties: map[string]jsonschema.Definition{
			"owner":       {Type: jsonschema.String},
			"repo":        {Type: jsonschema.String},
			"workflow_id": {Type: jsonschema.String},
			"status":      {Type: jsonschema.String, Description: "queued|in_progress|completed|success|failure|cancelled"},
			"branch":      {Type: jsonschema.String},
			"per_page":    {Type: jsonschema.Integer},
		},
		Required: []string{"owner", "repo"},
	},
}},
{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{
	Name:        "gh_get_workflow_run",
	Description: "Get one workflow run by id.",
	Parameters: jsonschema.Definition{
		Type: jsonschema.Object,
		Properties: map[string]jsonschema.Definition{
			"owner":  {Type: jsonschema.String},
			"repo":   {Type: jsonschema.String},
			"run_id": {Type: jsonschema.Integer},
		},
		Required: []string{"owner", "repo", "run_id"},
	},
}},
{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{
	Name:        "gh_wait_for_workflow_run",
	Description: "Wait until a workflow run completes; returns its conclusion.",
	Parameters: jsonschema.Definition{
		Type: jsonschema.Object,
		Properties: map[string]jsonschema.Definition{
			"owner":        {Type: jsonschema.String},
			"repo":         {Type: jsonschema.String},
			"run_id":       {Type: jsonschema.Integer},
			"timeout_sec":  {Type: jsonschema.Integer, Description: "max wait (default 600, cap 1800)"},
			"interval_sec": {Type: jsonschema.Integer, Description: "poll secs (default 5)"},
		},
		Required: []string{"owner", "repo", "run_id"},
	},
}},
{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{
	Name:        "gh_run_workflow_and_wait",
	Description: "Trigger a workflow and wait for it to finish; returns the completed run. Prefer when the user wants the result.",
	Parameters: jsonschema.Definition{
		Type: jsonschema.Object,
		Properties: map[string]jsonschema.Definition{
			"owner":        {Type: jsonschema.String},
			"repo":         {Type: jsonschema.String},
			"workflow_id":  {Type: jsonschema.String, Description: "numeric id or filename"},
			"ref":          {Type: jsonschema.String, Description: "branch/tag/sha (default main)"},
			"inputs":       {Type: jsonschema.Object},
			"timeout_sec":  {Type: jsonschema.Integer, Description: "max wait (default 600, cap 1800)"},
			"interval_sec": {Type: jsonschema.Integer, Description: "poll secs (default 5)"},
		},
		Required: []string{"owner", "repo", "workflow_id"},
	},
}},
```
