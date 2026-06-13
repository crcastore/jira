# Copilot Extraction Guide

This is a detailed handoff guide for another Copilot or engineer who needs to pull pieces out of this repository and reuse them in another Go application. It focuses on the reusable Jira create UI, searchable Jira users, GitHub PR/MR enrichment, Jira parent issue + subtask creation, and the chat widgets/services.

The main rule: copy or import reusable packages first, then write thin app-specific adapters around your own Jira, GitHub, HTTP, and auth code. Do not start by copying `cmd/jira-agent` wholesale unless you want this exact app.

## Current Package Map

| Package or folder | Reusable? | Purpose |
| --- | --- | --- |
| `jiraissueui/` | yes | HTMX Jira issue create button/dialog/form, result fragments, scoped CSS/JS, form parsing into `IssueForm`. |
| `githubpr/` | yes | GitHub repository picker, PR/MR picker, PR/MR reference parsing, changed-file loading, and Jira description enrichment. |
| `jiracreate/` | yes | Jira issue type metadata parsing/filtering, parent/subtask issue type selection, `IssueForm` to Jira create args, subtask args, created issue parsing. |
| `chat/` | yes | Transport-independent tool-calling LLM engine and session store. |
| `agentcore/` | yes | Chat service wrapper and Ollama model catalog. |
| `chathttp/` | yes | Reusable HTTP handlers for HTMX chat, reset, and token limits. |
| `chatui/` | yes | Drop-in HTMX chat widget and result chunk rendering. |
| `cmd/jira-agent/` | reference only | App wiring: environment loading, concrete Jira/GitHub clients, route setup, templates, tool dispatch. |

## What To Extract For Common Goals

### Goal: Only The Jira Create Form UI

Copy or import:

- `jiraissueui/component.go`
- `jiraissueui/component_test.go`

You must provide:

- Jira projects as `[]jiraissueui.Project`
- Jira user search results as `[]jiraissueui.User`
- Optional GitHub repo/PR options as `[]jiraissueui.PullRequestRepo` and `[]jiraissueui.PullRequestOption`
- A POST handler that calls `jiraissueui.ParseRequest`
- HTMX on the page

Use when:

- You already have Jira create logic.
- You just need a reusable form/dialog/button.

### Goal: Jira Create Form Plus GitHub PR/MR Attachment

Copy or import:

- `jiraissueui/`
- `githubpr/`

You must provide a GitHub client implementing:

```go
type Client interface {
    ListMyRepos(visibility, sort string, maxTotal int) (json.RawMessage, error)
    ListPulls(owner, repo, state string, perPage int) (json.RawMessage, error)
    GetPull(owner, repo string, number int) (json.RawMessage, error)
    ListPullFiles(owner, repo string, number, perPage int) (json.RawMessage, error)
}
```

Use when:

- You want the repository dropdown.
- You want the dependent PR/MR dropdown.
- You want selected PR/MR metadata and changed files appended to the Jira description.

### Goal: Parent Jira Issue Plus One Subtask Per Name

Copy or import:

- `jiraissueui/`
- `jiracreate/`

Usually also copy/import `githubpr/` if PR/MR enrichment should remain.

You must provide:

- A Jira client that can create an issue.
- A Jira client method to fetch project issue types / createmeta.
- A small app-specific orchestration loop that creates the parent first, then creates subtasks.

Use when:

- You want `Subtask names` rows in the form.
- You want a parent issue with child subtasks under it.
- You need to avoid invalid Jira parent types like Epic.

### Goal: Chat Widget Or Agent Engine

Copy or import depending on depth:

- `chatui/` for just HTMX chat UI rendering.
- `chathttp/` for reusable HTTP handlers around a chat service.
- `chat/` for the LLM/tool-calling engine.
- `agentcore/` for the service wrapper and model catalog.

Use when:

- The target app wants the chat UI or agent loop separate from this Jira create feature.

## Importing Instead Of Copying

The module path is:

```go
github.com/ccastorena/jira-agent
```

Example imports:

```go
import (
    "github.com/ccastorena/jira-agent/githubpr"
    "github.com/ccastorena/jira-agent/jiracreate"
    "github.com/ccastorena/jira-agent/jiraissueui"
)
```

For local development against a sibling checkout:

```go
replace github.com/ccastorena/jira-agent => ../jira
```

## Copying Instead Of Importing

If the target project should own the code, copy these folders directly:

```text
jiraissueui/
githubpr/
jiracreate/
```

Then update imports if the target module path changes. The package names can stay the same.

Run this after copying:

```bash
gofmt -w jiraissueui githubpr jiracreate
go test ./...
```

## Jira Create UI Package: `jiraissueui`

### What It Owns

`jiraissueui` owns the browser-facing create form.

It renders:

- create button launcher
- native `<dialog>` wrapper
- full create form
- result fragment after create
- dependent PR/MR select fragment
- Jira user `<datalist>` fragments
- scoped CSS
- small JS controller for dialogs, user pickers, and subtask row add/remove

It parses:

- form submissions into `jiraissueui.IssueForm`
- comma/newline/semicolon-separated labels
- repeated `subtask_names` values
- pasted comma/newline/semicolon-separated subtask names

It does not know:

- how to call Jira
- how to call GitHub
- how to store data
- how to authenticate users
- how to send notifications

### Core Types

```go
type IssueForm struct {
    ProjectKey        string
    Summary           string
    IssueType         string
    Description       string
    PullRequestRepo   string
    PullRequest       string
    Priority          string
    Labels            []string
    SubtaskNames      []string
    AssigneeAccountID string
    ReporterAccountID string
}
```

```go
type FormData struct {
    Endpoint             string
    PullRequestsEndpoint string
    UsersEndpoint        string

    Projects     []Project
    Assignees    []User
    PullRequestRepos []PullRequestRepo
    PullRequests []PullRequestOption

    IssueTypes []string
    Priorities []string
    Values     IssueForm
    Result     Result
}
```

### Required Page Assets

Your host page must include HTMX and the component assets:

```html
<script src="https://unpkg.com/htmx.org@1.9.12"></script>
{{ .JiraCreateStyles }}
{{ .JiraCreateScript }}
```

In Go:

```go
map[string]any{
    "JiraCreateStyles": jiraissueui.StyleTag(),
    "JiraCreateScript": jiraissueui.ScriptTag(),
}
```

### Form Field Contract

| Field | HTML name | Notes |
| --- | --- | --- |
| Project | `project_key` | Required. Parsed uppercase/trimmed. |
| Summary | `summary` | Required. |
| Issue type | `issue_type` | Should be supplied from Jira project issue types. |
| Description | `description` | Plain text. `githubpr` may append PR details. |
| Repository | `pull_request_repo` | Selected GitHub repo. |
| PR/MR | `pull_request` | Usually `owner/repo#number`. |
| Priority | `priority` | Optional. |
| Labels | `labels` | Comma-separated in UI; parsed into `[]string`. |
| Subtask names | `subtask_names` | Repeated input values, one visible row per name. Pasted separators also work. |
| Assignee | `assignee_account_id` | Hidden account id synced from search input. |
| Reporter | `reporter_account_id` | Hidden account id synced from search input. |

### Minimal Render Example

```go
ui := jiraissueui.New()
formHTML, err := ui.Form(jiraissueui.FormData{
    Endpoint:             "/jira/create",
    PullRequestsEndpoint: "/jira/create/pull-requests",
    UsersEndpoint:        "/jira/create/users",
    Projects:             projects,
    IssueTypes:           []string{"Task", "Request"},
    Assignees:            users,
    PullRequestRepos:     repos,
    PullRequests:         pulls,
    Values:               values,
    Result:               result,
})
```

### Minimal Submit Parser

```go
form, err := jiraissueui.ParseRequest(r)
if err != nil {
    return form, jiraissueui.Result{Err: err.Error()}
}
```

## GitHub PR/MR Package: `githubpr`

### What It Owns

`githubpr` owns everything needed to turn GitHub repositories and PR/MRs into form options and Jira description text.

It provides:

- `Repositories()` for repository dropdown options.
- `PullRequests(repo)` for dependent PR/MR options.
- `ParseReference(raw)` for values like `owner/repo#12`, `owner/repo!12`, and GitHub pull URLs.
- `Details(rawReference)` to fetch PR/MR metadata and changed files.
- `EnrichIssue(form)` to append PR/MR details to `form.Description`.

### Minimal Usage

```go
picker := githubpr.NewPicker(githubClient)

repos, reposErr := picker.Repositories()
pulls, pullsErr := picker.PullRequests(values.PullRequestRepo)

enriched, err := picker.EnrichIssue(form)
```

### Host Client Interface

Your target GitHub client only needs this surface:

```go
type Client interface {
    ListMyRepos(visibility, sort string, maxTotal int) (json.RawMessage, error)
    ListPulls(owner, repo, state string, perPage int) (json.RawMessage, error)
    GetPull(owner, repo string, number int) (json.RawMessage, error)
    ListPullFiles(owner, repo string, number, perPage int) (json.RawMessage, error)
}
```

## Jira Create Helper Package: `jiracreate`

### What It Owns

`jiracreate` owns Jira create/subtask helper logic that should not be tied to HTTP handlers.

It provides:

- `ParseIssueTypes(raw)` for Jira issue type metadata.
- `SortIssueTypes(types)` for stable parent/subtask ordering.
- `ParentIssueTypeNames(types)` for safe parent issue type dropdown values.
- `ValidParentIssueType(selected, types)` to replace invalid/stale selections.
- `FirstSubtaskIssueTypeName(types)` to find Jira's actual subtask type name.
- `CanHaveSubtasks(issueType)` to exclude subtasks and higher-level issue types like Epics.
- `ArgsFromForm(form)` to map `jiraissueui.IssueForm` into create args.
- `SubtaskArgsFromForm(form, parent, name, issueType)` to create child issue args.
- `ParseCreatedIssue(raw)` to parse Jira create responses.

### Issue Type Metadata

Fetch issue types from Jira in the host app:

```text
GET /rest/api/3/issue/createmeta/{PROJECT_KEY}/issuetypes
```

Then parse and filter:

```go
raw, err := jiraClient.ListIssueTypes(projectKey)
types, err := jiracreate.ParseIssueTypes(raw)
jiracreate.SortIssueTypes(types)

parentIssueTypes := jiracreate.ParentIssueTypeNames(types)
selectedType := jiracreate.ValidParentIssueType(form.IssueType, types)
subtaskType := jiracreate.FirstSubtaskIssueTypeName(types)
```

Important behavior:

- `Epic` is excluded as a parent for generated subtasks.
- Any issue type with `HierarchyLevel > 0` is excluded as a parent.
- Subtask issue types are not shown as parent issue type options.
- `Task` is preferred first when available.

### Jira Create Args

Parent issue:

```go
args := jiracreate.ArgsFromForm(form)
raw, err := jiraClient.CreateIssue(args)
parent, ok := jiracreate.ParseCreatedIssue(raw)
```

Subtask:

```go
args := jiracreate.SubtaskArgsFromForm(form, parent, "Ada", subtaskType)
raw, err := jiraClient.CreateIssue(args)
child, ok := jiracreate.ParseCreatedIssue(raw)
```

`SubtaskArgsFromForm` sets both `ParentID` and `ParentKey`. The Jira client should prefer `ParentID` when building the REST payload because Jira Cloud accepts freshly-created parents more reliably by id.

### Jira REST Payload Mapping

The concrete Jira client should map `jiracreate.CreateIssueArgs` into Jira REST fields like this:

```go
fields := map[string]any{
    "project":   map[string]string{"key": args.ProjectKey},
    "summary":   args.Summary,
    "issuetype": map[string]string{"name": args.IssueType},
}
if args.ParentID != "" {
    fields["parent"] = map[string]string{"id": args.ParentID}
} else if args.ParentKey != "" {
    fields["parent"] = map[string]string{"key": args.ParentKey}
}
```

Then add optional description, priority, labels, assignee, and reporter.

## Searchable Jira Users

The reusable UI expects a user search route:

```text
GET /jira/create/users
```

Expected query parameters:

| Parameter | Meaning |
| --- | --- |
| `project_key` | Jira project key. |
| `field` | `assignee` or `reporter`. |
| `assignee_search` | Search text when `field=assignee`. |
| `reporter_search` | Search text when `field=reporter`. |

The handler should call Jira:

```text
GET /rest/api/3/user/assignable/search?project=SCRUM&query=ada&maxResults=20
```

Return a datalist fragment with the correct id:

```html
<datalist id="jira-create-assignee-options">
  <option value="Ada Lovelace" data-account-id="712020:ada"></option>
</datalist>
```

The JS in `jiraissueui` copies `data-account-id` into hidden fields named `assignee_account_id` and `reporter_account_id`.

## Minimal Host App Shape

A clean target app usually has these pieces:

```text
internal/jira/client.go          concrete Jira REST client
internal/github/client.go        concrete GitHub REST client
internal/web/jira_create.go      HTTP handlers and page glue
internal/web/templates.go        layout including HTMX + UI assets
```

Define a small Jira client interface for the create handler:

```go
type JiraClient interface {
    ListProjects() ([]jiraissueui.Project, error)
    ListIssueTypes(projectKey string) (json.RawMessage, error)
    SearchAssignableUsers(projectKey, query string, maxResults int) ([]jiraissueui.User, error)
    CreateIssue(jiracreate.CreateIssueArgs) (json.RawMessage, error)
}
```

Define a handler struct:

```go
type JiraCreateHandler struct {
    Jira   JiraClient
    GitHub githubpr.Client
    UI     *jiraissueui.Component
}
```

Build form data:

```go
func (h JiraCreateHandler) formData(values jiraissueui.IssueForm, result jiraissueui.Result) jiraissueui.FormData {
    projects, projectsErr := h.Jira.ListProjects()
    if values.ProjectKey == "" && len(projects) > 0 {
        values.ProjectKey = projects[0].Key
    }

    rawTypes, issueTypesErr := h.Jira.ListIssueTypes(values.ProjectKey)
    issueTypes, _ := jiracreate.ParseIssueTypes(rawTypes)
    jiracreate.SortIssueTypes(issueTypes)
    parentTypes := jiracreate.ParentIssueTypeNames(issueTypes)
    if len(parentTypes) > 0 {
        values.IssueType = jiracreate.ValidParentIssueType(values.IssueType, issueTypes)
    }

    users, usersErr := h.Jira.SearchAssignableUsers(values.ProjectKey, "", 20)
    picker := githubpr.NewPicker(h.GitHub)
    repos, reposErr := picker.Repositories()
    pulls, pullsErr := picker.PullRequests(values.PullRequestRepo)

    return jiraissueui.FormData{
        Endpoint:             "/jira/create",
        PullRequestsEndpoint: "/jira/create/pull-requests",
        UsersEndpoint:        "/jira/create/users",
        Projects:             projects,
        ProjectsErr:          errText(projectsErr),
        IssueTypes:           parentTypes,
        IssueTypesErr:        errText(issueTypesErr),
        Assignees:            users,
        AssigneesErr:         errText(usersErr),
        PullRequestRepos:     repos,
        PullRequestReposErr:  errText(reposErr),
        PullRequests:         pulls,
        PullRequestsErr:      errText(pullsErr),
        Values:               values,
        Result:               result,
    }
}
```

Create issues and subtasks:

```go
func (h JiraCreateHandler) create(form jiraissueui.IssueForm) jiraissueui.Result {
    enriched, err := githubpr.NewPicker(h.GitHub).EnrichIssue(form)
    if err != nil {
        return jiraissueui.Result{Err: err.Error()}
    }

    rawTypes, err := h.Jira.ListIssueTypes(enriched.ProjectKey)
    if err == nil {
        issueTypes, err := jiracreate.ParseIssueTypes(rawTypes)
        if err == nil {
            jiracreate.SortIssueTypes(issueTypes)
            enriched.IssueType = jiracreate.ValidParentIssueType(enriched.IssueType, issueTypes)
        }
    }

    parentRaw, err := h.Jira.CreateIssue(jiracreate.ArgsFromForm(enriched))
    if err != nil {
        return jiraissueui.Result{Err: err.Error()}
    }
    parent, ok := jiracreate.ParseCreatedIssue(parentRaw)
    if !ok {
        return jiraissueui.Result{Err: "Jira created the issue but returned an unexpected response"}
    }

    result := parent.Result("https://your-site.atlassian.net")
    if len(enriched.SubtaskNames) == 0 {
        return result
    }

    issueTypes, _ := jiracreate.ParseIssueTypes(rawTypes)
    jiracreate.SortIssueTypes(issueTypes)
    subtaskType := jiracreate.FirstSubtaskIssueTypeName(issueTypes)
    if subtaskType == "" {
        result.Err = "Jira project has no subtask issue type enabled"
        return result
    }

    for _, name := range enriched.SubtaskNames {
        childRaw, err := h.Jira.CreateIssue(jiracreate.SubtaskArgsFromForm(enriched, parent, name, subtaskType))
        if err != nil {
            result.Err = err.Error()
            return result
        }
        child, ok := jiracreate.ParseCreatedIssue(childRaw)
        if !ok {
            result.Err = "Jira returned an unexpected subtask create response"
            return result
        }
        result.Subtasks = append(result.Subtasks, child.Link("https://your-site.atlassian.net"))
    }
    return result
}
```

## Routes To Recreate

| Route | Method | Purpose |
| --- | --- | --- |
| `/jira/create` | `GET` | Render full page form. |
| `/jira/create` | `POST` | Parse form, enrich, create parent issue, create subtasks, render result. |
| `/jira/create/pull-requests` | `GET` | Return only the dependent PR/MR select fragment. |
| `/jira/create/users` | `GET` | Return only the user-search datalist for assignee or reporter. |

## Files In `cmd/jira-agent` To Use As References

Use these as examples, not copy-paste dependencies:

| File | Borrowable idea |
| --- | --- |
| `web_create.go` | Route handlers for create page, HTMX result fragment, PR/MR fragment, and user datalist fragment. |
| `web_create_issue.go` | Thin app orchestration around `githubpr` + `jiracreate` + concrete Jira client. |
| `web_issue_types.go` | Fetch Jira issue types and delegate parsing/filtering to `jiracreate`. |
| `web_data.go` | Map Jira projects and assignable users into `jiraissueui` options. |
| `jira.go` | Concrete Jira REST calls and ADF conversion. |
| `github.go` | Concrete GitHub REST calls. |
| `web_templates.go` | Example standalone page layout with HTMX and component assets. |

## Tests To Copy Or Recreate

Recommended reusable package tests:

```text
jiraissueui/component_test.go
githubpr/picker_test.go
jiracreate/issue_types_test.go
jiracreate/issues_test.go
```

Recommended host-app tests:

- create page renders project, issue type, user, repo, and PR/MR controls
- user search route sends `project`, `query`, and small `maxResults`
- PR/MR route returns all-state PR options
- form POST creates parent issue once
- form POST with subtask rows creates one child per row
- stale `Epic` issue type falls back to a subtask-compatible parent type
- subtask create payload sends `parent.id` when Jira returns an id
- PR/MR enrichment appends changed files to the description

## Common Extraction Mistakes

1. Copying `cmd/jira-agent/web_create.go` without `jiraissueui`, `githubpr`, and `jiracreate`.
2. Rendering the form without `jiraissueui.ScriptTag()`, which breaks dialog/user/subtask JS.
3. Using hardcoded issue types like `Bug` or `Sub-task` instead of Jira createmeta.
4. Creating subtasks under `Epic`; Jira rejects this.
5. Sending subtask parent by key immediately after parent creation; Jira Cloud is more reliable with parent id.
6. Loading all Jira users into a `<select>` instead of using `/user/assignable/search` with a query.
7. Forgetting that `subtask_names` is repeated by the list UI; parse all values, not only the first.
8. Forgetting that PR/MR options are repo-dependent and must be served as an HTMX fragment.

## Quick Copy Checklist For Copilot

When asked to rip this out into another Go + HTMX app:

1. Copy/import `jiraissueui/`.
2. Copy/import `jiracreate/`.
3. Copy/import `githubpr/` if PR/MR enrichment is needed.
4. Implement a Jira client with project lookup, issue type lookup, assignable-user search, and create issue.
5. Implement a GitHub client satisfying `githubpr.Client` if PR/MR support is needed.
6. Add `/jira/create`, `/jira/create/users`, and `/jira/create/pull-requests` routes.
7. Include HTMX plus `jiraissueui.StyleTag()` and `jiraissueui.ScriptTag()` in the page.
8. Fill `jiraissueui.FormData` from Jira/GitHub adapters.
9. On POST, parse with `jiraissueui.ParseRequest`.
10. Enrich with `githubpr.NewPicker(githubClient).EnrichIssue(form)` if PR/MR selected.
11. Use `jiracreate` to select valid issue types and build parent/subtask args.
12. Create parent issue, parse `CreatedIssue`, then create subtasks using `SubtaskArgsFromForm`.
13. Return `jiraissueui.Result` through `RenderResult` for HTMX.
14. Run copied package tests and add host-route tests.
