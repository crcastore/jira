# Jira Create + PR/MR Extraction Guide

This guide explains how Jira ticket creation works in this repo and how to copy
the reusable pieces into another Go + HTMX project. It focuses on the full flow:
render a Jira create form, choose a GitHub repository, choose a PR/MR, fetch the
PR/MR changed files, and append those details to the Jira ticket description.

## The Extractable Pieces

There are three reusable packages:

| Package | Purpose | Copy when |
| --- | --- | --- |
| [jiraissueui/](jiraissueui/) | Renders the create button, dialog, form, dependent PR/MR field, result fragment, CSS, JS, and parses submitted form fields into `IssueForm`. | You want the Jira ticket creation UI. |
| [githubpr/](githubpr/) | Loads GitHub repositories, loads all-state PR/MR options for a selected repository, parses PR/MR references, fetches PR/MR metadata, fetches changed files, and appends those details to `IssueForm.Description`. | You want the repository + PR/MR picker and changed-file enrichment. |
| [jiracreate/](jiracreate/) | Parses Jira issue type metadata, chooses parent/subtask issue types, maps `IssueForm` into Jira create args, builds subtask args, and parses Jira create responses. | You want parent issue + generated subtask creation helpers without this app's HTTP wiring. |

The app-specific files under [cmd/jira-agent/](cmd/jira-agent/) only wire those
packages to this repo's Jira and GitHub clients.

The create/subtask wiring is split by concern:

| File | Purpose |
| --- | --- |
| [cmd/jira-agent/web_create.go](cmd/jira-agent/web_create.go) | HTTP form/page/fragment handlers. |
| [cmd/jira-agent/web_create_issue.go](cmd/jira-agent/web_create_issue.go) | App-specific orchestration around `jiracreate`: enrich form, create parent, create subtasks. |
| [cmd/jira-agent/web_issue_types.go](cmd/jira-agent/web_issue_types.go) | Thin adapter that fetches Jira createmeta and delegates parsing/filtering to `jiracreate`. |
| [cmd/jira-agent/web_data.go](cmd/jira-agent/web_data.go) | General Jira/GitHub panel data and assignable-user mapping. |

## Current Runtime Flow

1. The dashboard or dedicated create page asks [cmd/jira-agent/web_create.go](cmd/jira-agent/web_create.go) for form data.
2. `jiraCreateFormData` loads Jira projects, project-valid issue types, and assignable users from this app's Jira client.
3. `jiraCreateFormData` creates a `githubpr.Picker` from this app's GitHub client.
4. `Picker.Repositories()` provides the repository dropdown values.
5. If a repository is already selected, `Picker.PullRequests(repo)` provides the PR/MR dropdown values.
6. [jiraissueui/component.go](jiraissueui/component.go) renders the form and dependent dropdown markup.
7. When the user changes the repository dropdown, HTMX calls `/jira/create/pull-requests?pull_request_repo=owner/repo`.
8. `handleJiraCreatePullRequests` calls `Picker.PullRequests(repo)` and returns only the PR/MR select fragment.
9. When the user submits the form, `jiraissueui.ParseRequest` returns an `IssueForm`.
10. `Picker.EnrichIssue(form)` parses `form.PullRequest`, fetches the PR/MR, fetches changed files, and appends a text block to `form.Description`.
11. This app converts the enriched `IssueForm` into Jira REST create fields and calls Jira to create the parent issue.
12. If `subtask_names` was submitted, the app uses Jira's actual subtask issue type for the project and creates one child issue per name, copying the same description, priority, labels, assignee, and reporter. Each subtask summary is `Parent summary - Name`.

## Important Contracts

The UI uses these submitted field names:

| Field | HTML name | Owner |
| --- | --- | --- |
| Jira project | `project_key` | `jiraissueui` |
| Summary | `summary` | `jiraissueui` |
| Issue type | `issue_type` | `jiraissueui` |
| Description | `description` | `jiraissueui`, enriched by `githubpr` |
| GitHub repository | `pull_request_repo` | `jiraissueui`, options from `githubpr.Repositories()` |
| PR/MR reference | `pull_request` | `jiraissueui`, options from `githubpr.PullRequests(repo)` |
| Priority | `priority` | `jiraissueui` |
| Labels | `labels` | `jiraissueui` |
| Subtask names | `subtask_names` | `jiraissueui`, one submitted value per list row; pasted comma/newline/semicolon-separated names are also accepted |
| Assignee | `assignee_account_id` | `jiraissueui` |
| Reporter | `reporter_account_id` | `jiraissueui` |

The PR/MR option value format is always `owner/repo#number`. `githubpr.ParseReference`
also accepts GitHub pull URLs, `owner/repo!number`, `owner/repo/pull/number`, and
`owner/repo/number`.

## GitHub Client Interface

To reuse [githubpr/](githubpr/), provide a client with this interface:

```go
type Client interface {
    ListMyRepos(visibility, sort string, maxTotal int) (json.RawMessage, error)
    ListPulls(owner, repo, state string, perPage int) (json.RawMessage, error)
    GetPull(owner, repo string, number int) (json.RawMessage, error)
    ListPullFiles(owner, repo string, number, perPage int) (json.RawMessage, error)
}
```

This repo's [cmd/jira-agent/github.go](cmd/jira-agent/github.go) already satisfies it.
In another project, your GitHub client can be a thin wrapper around these REST calls:

| Method | GitHub REST endpoint |
| --- | --- |
| `ListMyRepos("all", "pushed", 150)` | `GET /user/repos?visibility=all&sort=pushed&per_page=100...` |
| `ListPulls(owner, repo, "all", 100)` | `GET /repos/{owner}/{repo}/pulls?state=all&per_page=100` |
| `GetPull(owner, repo, number)` | `GET /repos/{owner}/{repo}/pulls/{number}` |
| `ListPullFiles(owner, repo, number, 100)` | `GET /repos/{owner}/{repo}/pulls/{number}/files?per_page=100` |

Use `state=all`. The picker intentionally shows open, closed, and merged PR/MRs
because a Jira ticket may need to link to already-merged work.

## Minimal Target Wiring

The target app needs one handler that owns both the create page and the dependent
PR/MR fragment endpoint.

```go
type JiraCreateService interface {
    ListProjects() ([]jiraissueui.Project, error)
    SearchAssignableUsers(projectKey string) ([]jiraissueui.User, error)
    CreateIssue(form jiraissueui.IssueForm) (key string, browseURL string, err error)
}

type JiraCreateHandler struct {
    Jira   JiraCreateService
    GitHub githubpr.Client
    UI     *jiraissueui.Component
}

func (h JiraCreateHandler) formData(values jiraissueui.IssueForm, result jiraissueui.Result) jiraissueui.FormData {
    projects, projectsErr := h.Jira.ListProjects()
    if values.ProjectKey == "" && len(projects) > 0 {
        values.ProjectKey = projects[0].Key
    }

    users, usersErr := h.Jira.SearchAssignableUsers(values.ProjectKey)
    if values.PullRequestRepo == "" && values.PullRequest != "" {
        if ref, err := githubpr.ParseReference(values.PullRequest); err == nil {
            values.PullRequestRepo = ref.FullName()
        }
    }

    picker := githubpr.NewPicker(h.GitHub)
    repos, reposErr := picker.Repositories()
    pulls, pullsErr := picker.PullRequests(values.PullRequestRepo)

    return jiraissueui.FormData{
        Endpoint:             "/jira/create",
        PullRequestsEndpoint: "/jira/create/pull-requests",
        Projects:             projects,
        ProjectsErr:          errorText(projectsErr),
        Assignees:            users,
        AssigneesErr:         errorText(usersErr),
        PullRequestRepos:     repos,
        PullRequestReposErr:  errorText(reposErr),
        PullRequests:         pulls,
        PullRequestsErr:      errorText(pullsErr),
        Values:               values,
        Result:               result,
    }
}
```

The dependent endpoint is just as small:

```go
func (h JiraCreateHandler) ServePullRequests(w http.ResponseWriter, r *http.Request) {
    values := jiraissueui.IssueForm{
        PullRequestRepo: strings.TrimSpace(r.URL.Query().Get("pull_request_repo")),
        PullRequest:     strings.TrimSpace(r.URL.Query().Get("pull_request")),
    }
    pulls, err := githubpr.NewPicker(h.GitHub).PullRequests(values.PullRequestRepo)

    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    _ = h.UI.RenderPullRequestField(w, jiraissueui.FormData{
        PullRequestsEndpoint: "/jira/create/pull-requests",
        PullRequests:         pulls,
        PullRequestsErr:      errorText(err),
        Values:               values,
    })
}
```

Submit handling should enrich before Jira creation:

```go
func (h JiraCreateHandler) createFromRequest(r *http.Request) (jiraissueui.IssueForm, jiraissueui.Result) {
    form, err := jiraissueui.ParseRequest(r)
    if err != nil {
        return form, jiraissueui.Result{Err: err.Error()}
    }

    form, err = githubpr.NewPicker(h.GitHub).EnrichIssue(form)
    if err != nil {
        return form, jiraissueui.Result{Err: err.Error()}
    }

    key, browseURL, err := h.Jira.CreateIssue(form)
    if err != nil {
        return form, jiraissueui.Result{Err: err.Error()}
    }
    return form, jiraissueui.Result{Key: key, URL: browseURL}
}
```

Route both endpoints:

```go
create := JiraCreateHandler{Jira: jiraClient, GitHub: githubClient, UI: jiraissueui.New()}

mux.HandleFunc("/jira/create", create.ServeHTTP)
mux.HandleFunc("/jira/create/pull-requests", create.ServePullRequests)
```

## Template Requirements

The page that contains the form must load HTMX and the Jira UI assets:

```html
<script src="https://unpkg.com/htmx.org@1.9.12"></script>
{{ .JiraCreateStyles }}
{{ .JiraCreateScript }}
```

The standalone create page and dashboard dialog both use the same `jiraissueui`
form. The repository select includes:

```html
hx-get="/jira/create/pull-requests"
hx-trigger="change"
hx-target="#jira-create-pull-requests"
hx-swap="outerHTML"
hx-include="this"
```

Do not put an inherited `hx-disabled-elt="find ..."` on the create form. HTMX
1.9.12 can inherit that onto the repository select request and fail before it
sends `/jira/create/pull-requests`.

## Description Block Added To Jira

`githubpr.EnrichIssue` appends text like this to the submitted description:

```text
Related pull request
- PR: https://github.com/octo/hello/pull/12
- Repository: octo/hello
- Number: #12
- Title: Add login fix
- State: open
- Author: octocat
- Branches: fix-login -> main

Changed files
- cmd/main.go (modified, +12/-3)
- README.md (added, +5/-0)
```

The Jira client in the target project can then convert that plain text into ADF,
Markdown, or whatever its Jira API wrapper expects. This repo converts plain text
to Atlassian Document Format in [cmd/jira-agent/jira.go](cmd/jira-agent/jira.go).

## Files To Copy

For the complete reusable create + PR/MR flow, copy or import:

1. [jiraissueui/component.go](jiraissueui/component.go)
2. [jiraissueui/component_test.go](jiraissueui/component_test.go)
3. [githubpr/picker.go](githubpr/picker.go)
4. [githubpr/picker_test.go](githubpr/picker_test.go)

Then add your app-specific:

1. Jira client adapter for projects, users, and create issue.
2. GitHub client adapter satisfying `githubpr.Client`.
3. Web handler that fills `jiraissueui.FormData`, serves the PR/MR fragment, and calls `githubpr.EnrichIssue` before Jira create.
4. Page template that includes HTMX, `jiraissueui.StyleTag()`, and `jiraissueui.ScriptTag()`.

## Verification Checklist

After extraction, test these cases:

1. The create page renders with a repository dropdown.
2. Selecting a repository sends `GET /jira/create/pull-requests?pull_request_repo=owner%2Frepo`.
3. The PR/MR dropdown becomes enabled and contains `owner/repo#number` values.
4. Merged PR/MRs show a `[merged]` label and closed unmerged PR/MRs show `[closed]`.
5. Submitting with a selected PR/MR calls `GetPull` and `ListPullFiles`.
6. The created Jira description contains the original description, repository, PR/MR URL/number/title, branches, and changed files.
7. Submitting without a selected PR/MR still creates a Jira ticket without calling GitHub PR detail endpoints.

The reusable package tests in [githubpr/picker_test.go](githubpr/picker_test.go)
cover the most important GitHub behavior without network calls.