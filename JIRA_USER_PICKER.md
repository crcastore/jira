# Jira Create User Picker

This guide explains how the Jira create form handles assignee and reporter selection without rendering a huge dropdown of every Jira user. It also shows how to pull the same pattern into another Go + HTMX app.

## Why The Old Dropdown Can Break

A plain `<select>` looks simple, but it is a poor fit for large Jira user lists.

Common failure modes:

- Jira may return only the first page of assignable users. In this app the old code requested `maxResults=50`, so larger teams could silently lose users after the first 50.
- Native browser dropdowns are controlled by the OS/browser, not normal page layout. In embedded apps, iframes, or custom shells, long lists can appear clipped or truncated.
- Large option lists are slow to render, hard to search, and awkward on smaller screens.
- If another system wraps the form in a container with `overflow: hidden`, a native select menu can look cut off even when the HTML is valid.

The safer approach is: search Jira on demand, return only a small set of matching users, and submit Jira `accountId` values through hidden fields.

## Current Shape

The reusable UI lives in [jiraissueui/component.go](jiraissueui/component.go).

The app-specific Jira wiring lives in:

- [cmd/jira-agent/web_create.go](cmd/jira-agent/web_create.go)
- [cmd/jira-agent/web_data.go](cmd/jira-agent/web_data.go)
- [cmd/jira-agent/jira.go](cmd/jira-agent/jira.go)
- [cmd/jira-agent/web_server.go](cmd/jira-agent/web_server.go)

The create form no longer renders these as bulk selects:

```html
<select name="assignee_account_id">...</select>
<select name="reporter_account_id">...</select>
```

It now renders each user field as:

- a visible search input for the display name
- a small `<datalist>` of current search results
- a hidden input that carries the Jira `accountId`

The submitted field names did not change:

| Purpose | Posted field |
| --- | --- |
| Assignee Jira account id | `assignee_account_id` |
| Reporter Jira account id | `reporter_account_id` |

That means Jira issue creation still receives the same data it already expected.

## Request Flow

1. The create form renders a small initial datalist for assignee and reporter.
2. When the user focuses or types in the assignee field, HTMX calls `/jira/create/users`.
3. The request includes `project_key`, the search text, and `field=assignee`.
4. The server calls Jira's assignable user search endpoint with the project key, query text, and a small result limit.
5. The server returns one replacement `<datalist>` containing matching display names and `data-account-id` attributes.
6. A small JS controller checks whether the typed display name exactly matches one of the datalist options.
7. If it matches, the controller writes that option's `data-account-id` into the hidden field.
8. If it does not match, the controller clears the hidden field so stale account ids are not submitted.
9. Form submission posts `assignee_account_id` and `reporter_account_id` like before.

## Rendered Markup Contract

The assignee field uses this shape:

```html
<input
  name="assignee_search"
  type="search"
  list="jira-create-assignee-options"
  autocomplete="off"
  placeholder="Search users"
  hx-get="/jira/create/users"
  hx-trigger="input changed delay:250ms, focus once"
  hx-target="#jira-create-assignee-options"
  hx-swap="outerHTML"
  hx-include="[name='project_key'], this"
  hx-vals='{"field":"assignee"}'
  data-jira-user-input
  data-jira-user-target="assignee_account_id">

<input name="assignee_account_id" type="hidden" value="">

<datalist id="jira-create-assignee-options">
  <option value="Ada Lovelace" data-account-id="712020:ada"></option>
</datalist>
```

The reporter field is the same pattern with:

```html
name="reporter_search"
list="jira-create-reporter-options"
data-jira-user-target="reporter_account_id"
hx-vals='{"field":"reporter"}'
```

## Server Endpoint Contract

The UI expects this route:

```text
GET /jira/create/users
```

Expected query parameters:

| Parameter | Meaning |
| --- | --- |
| `project_key` | Jira project key, for example `SCRUM` |
| `field` | `assignee` or `reporter` |
| `assignee_search` | Search text when `field=assignee` |
| `reporter_search` | Search text when `field=reporter` |

Expected response:

```html
<datalist id="jira-create-assignee-options">
  <option value="Ada Lovelace" data-account-id="712020:ada"></option>
  <option value="Ada Byron" data-account-id="712020:byron"></option>
</datalist>
```

For reporter searches, return the reporter datalist id:

```html
<datalist id="jira-create-reporter-options">
  <option value="Grace Hopper" data-account-id="712020:grace"></option>
</datalist>
```

The datalist id matters because HTMX swaps the whole datalist using `hx-swap="outerHTML"`.

## Jira API Contract

The app calls Jira Cloud's assignable user search endpoint:

```text
GET /rest/api/3/user/assignable/search?project=SCRUM&query=ada&maxResults=20
```

Important details:

- Always pass `project`, because assignability depends on project permissions.
- Pass `query` when the user typed something.
- Keep `maxResults` small. This app uses `20`.
- Ignore inactive users and users with an empty `accountId`.
- Sort the returned list by display name for stable rendering.

This repo's client method is:

```go
func (c *JiraClient) SearchAssignableUsers(projectKey, query string, maxResults int) (json.RawMessage, error)
```

## Pull It Into Another App

### 1. Bring In The Reusable UI

Use the reusable package directly if the target app can import this module:

```go
import "github.com/ccastorena/jira-agent/jiraissueui"
```

Or vendor the package by bringing in:

- [jiraissueui/component.go](jiraissueui/component.go)
- [jiraissueui/component_test.go](jiraissueui/component_test.go)

The package is intentionally UI-only. It does not know about a concrete Jira client.

### 2. Include The Assets

The page that renders the create form needs HTMX and the Jira UI assets:

```html
<script src="https://unpkg.com/htmx.org@1.9.12"></script>
{{ .JiraCreateStyles }}
{{ .JiraCreateScript }}
```

In Go, those values come from:

```go
map[string]any{
    "JiraCreateStyles": jiraissueui.StyleTag(),
    "JiraCreateScript": jiraissueui.ScriptTag(),
}
```

The script is needed because it maps the selected display name back to the hidden Jira `accountId` field.

### 3. Set The Form Data Endpoint

When building `jiraissueui.FormData`, set `UsersEndpoint`:

```go
formData := jiraissueui.FormData{
    Endpoint:             "/jira/create",
    PullRequestsEndpoint: "/jira/create/pull-requests",
    UsersEndpoint:        "/jira/create/users",
    Projects:             projects,
    Assignees:            initialUsers,
    Values:               values,
    Result:               result,
}
```

`Assignees` should be a small initial list, not the entire company directory. It is only used to seed the first datalist before the user types.

### 4. Add The User Search Route

Register the route next to the normal create route:

```go
mux.HandleFunc("/jira/create", app.handleJiraCreatePage)
mux.HandleFunc("/jira/create/users", app.handleJiraCreateUsers)
```

A minimal handler looks like this:

```go
func (a *App) handleJiraCreateUsers(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }

    field := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("field")))
    if field != "reporter" {
        field = "assignee"
    }

    optionsID := jiraissueui.DefaultAssigneeOptionsID
    query := strings.TrimSpace(r.URL.Query().Get("assignee_search"))
    if field == "reporter" {
        optionsID = jiraissueui.DefaultReporterOptionsID
        query = strings.TrimSpace(r.URL.Query().Get("reporter_search"))
    }

    users, err := a.fetchJiraAssignableUsers(selectedProject(r), query)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    _ = a.jiraCreateUI.RenderUserOptions(w, jiraissueui.UserOptionsData{
        OptionsID: optionsID,
        Users:     users,
    })
}
```

### 5. Implement The Jira Lookup

The target app needs a helper that maps Jira JSON into `jiraissueui.User`:

```go
const jiraUserSearchLimit = 20

func (a *App) fetchJiraAssignableUsers(projectKey, query string) ([]jiraissueui.User, error) {
    if projectKey == "" {
        return nil, nil
    }

    raw, err := a.jira.SearchAssignableUsers(projectKey, strings.TrimSpace(query), jiraUserSearchLimit)
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

    users := make([]jiraissueui.User, 0, len(payload))
    for _, user := range payload {
        if user.AccountID == "" || !user.Active {
            continue
        }
        users = append(users, jiraissueui.User{
            AccountID:   user.AccountID,
            DisplayName: user.DisplayName,
        })
    }

    sort.Slice(users, func(i, j int) bool {
        return users[i].DisplayName < users[j].DisplayName
    })
    return users, nil
}
```

The Jira client method should call:

```go
func (c *JiraClient) SearchAssignableUsers(projectKey, query string, maxResults int) (json.RawMessage, error) {
    if maxResults <= 0 {
        maxResults = 20
    }

    q := url.Values{}
    q.Set("project", projectKey)
    if query != "" {
        q.Set("query", query)
    }
    q.Set("maxResults", fmt.Sprintf("%d", maxResults))

    return c.request("GET", "/rest/api/3/user/assignable/search", q, nil)
}
```

### 6. Keep Create Submission Unchanged

The create endpoint still parses the same fields:

```go
form, err := jiraissueui.ParseRequest(r)
```

The parsed form contains:

```go
form.AssigneeAccountID
form.ReporterAccountID
```

When creating the Jira issue, map those into Jira fields:

```go
if form.AssigneeAccountID != "" {
    fields["assignee"] = map[string]string{"accountId": form.AssigneeAccountID}
}
if form.ReporterAccountID != "" {
    fields["reporter"] = map[string]string{"accountId": form.ReporterAccountID}
}
```

## Small Checklist

Use this when pulling the picker into another project:

- The page includes HTMX.
- The page includes `jiraissueui.StyleTag()`.
- The page includes `jiraissueui.ScriptTag()`.
- `FormData.UsersEndpoint` points at your user search route.
- The route returns a `<datalist>` with the id requested by assignee or reporter.
- Each `<option>` has `value="Display Name"` and `data-account-id="..."`.
- The Jira lookup sends `project`, optional `query`, and a small `maxResults`.
- The create endpoint still reads `assignee_account_id` and `reporter_account_id`.
- Typing a name that is not in the datalist leaves the hidden account id empty.

## Why This Works Better With Lots Of Users

The browser never has to render every user. Jira receives a narrow search query. The returned HTML is tiny. The user can type the name they want. The form still submits stable Jira `accountId` values, not display names that may be duplicated or changed later.

This keeps the UI simple, avoids clipped native dropdowns, and scales to large Jira directories.
