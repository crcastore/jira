# Jira Ticket Create Button Extraction Playbook

This is a handoff guide for another Copilot or engineer who needs to pull the Jira ticket creation button/dialog/form out of this repo and put it into another Go + HTMX project.

The reusable code is already isolated in [jiraissueui/component.go](jiraissueui/component.go#L1-L600). The app-specific files under [cmd/jira-agent/](cmd/jira-agent/) are examples of how this repo wires that reusable package to its own Jira client and templates.

## What To Copy

Copy or import the reusable package first. Do not start by copying the dashboard handler, because the handler depends on this app's `webApp`, `JiraClient`, `CreateIssueArgs`, and helper functions.

| Source | Target | Required | Notes |
| --- | --- | --- | --- |
| [jiraissueui/component.go](jiraissueui/component.go#L1-L600) | `jiraissueui/component.go` or `internal/jiraissueui/component.go` | yes | This is the whole reusable package: button, dialog, form, parser, CSS, and JS controller. Copy the entire file if you are vendoring the code. |
| [jiraissueui/component_test.go](jiraissueui/component_test.go#L1-L289) | `jiraissueui/component_test.go` or `internal/jiraissueui/component_test.go` | recommended | Copy with the package so the target project can prove the extraction still renders and parses correctly. |
| [cmd/jira-agent/web_create.go](cmd/jira-agent/web_create.go#L17-L163) | target app handler, for example `internal/web/jira_create.go` | reference only | Use as a model for GET, POST, HTMX fragment response, and Jira create mapping. Do not copy unchanged. |
| [cmd/jira-agent/web_data.go](cmd/jira-agent/web_data.go#L109-L157) | target Jira adapter, for example `internal/jira/options.go` | reference only | Shows how Jira REST JSON is mapped into `jiraissueui.Project` and `jiraissueui.User`. |
| [cmd/jira-agent/web_server.go](cmd/jira-agent/web_server.go#L15-L21) | target app state struct | reference only | Shows import + storing `*jiraissueui.Component` on the app. |
| [cmd/jira-agent/web_server.go](cmd/jira-agent/web_server.go#L56-L68) | target app startup | reference only | Shows `jiraissueui.New()` and route registration for `/jira/create`. |
| [cmd/jira-agent/web_server.go](cmd/jira-agent/web_server.go#L112-L122) | target page data | reference only | Shows passing styles, script, and rendered dialog into the main page template. |
| [cmd/jira-agent/web_templates.go](cmd/jira-agent/web_templates.go#L220-L222) | target layout `<head>` | reference only | Shows where to render `jiraissueui.StyleTag()` and `jiraissueui.ScriptTag()`. |
| [cmd/jira-agent/web_templates.go](cmd/jira-agent/web_templates.go#L258-L262) | target page body | reference only | Shows where the rendered create button/dialog is inserted. |
| [cmd/jira-agent/web_templates.go](cmd/jira-agent/web_templates.go#L324-L413) | optional dedicated create page | reference only | Shows a full-page form route in addition to the modal button. |

## Important Source Line Map

Use these line ranges when you need to explain or surgically copy a section.

| Concern | Source range | Why it matters |
| --- | --- | --- |
| Package purpose and imports | [jiraissueui/component.go](jiraissueui/component.go#L1-L18) | Proves the package is UI-only and has no concrete Jira client dependency. |
| Defaults | [jiraissueui/component.go](jiraissueui/component.go#L20-L36) | Default endpoint, ids, labels, issue types, priorities, and parse errors. |
| Public data models | [jiraissueui/component.go](jiraissueui/component.go#L38-L108) | `Project`, `User`, `IssueForm`, `Result`, `FormData`, `LauncherData`, and `DialogData`. |
| Defaulting logic | [jiraissueui/component.go](jiraissueui/component.go#L111-L169) | Fills endpoint, DOM ids, submit label, issue types, priorities, and default project/type. |
| Renderer API | [jiraissueui/component.go](jiraissueui/component.go#L171-L242) | `New`, `Form`, `RenderResult`, `Launcher`, and `Dialog`. These are the main host-app entry points. |
| Form parser | [jiraissueui/component.go](jiraissueui/component.go#L244-L268) | Converts posted form fields into `IssueForm` and validates required project/summary fields. |
| Asset exports | [jiraissueui/component.go](jiraissueui/component.go#L270-L288) | `StyleTag`, `CSS`, `ScriptTag`, and `JS` for inline or bundled assets. |
| Form template | [jiraissueui/component.go](jiraissueui/component.go#L306-L357) | Native + HTMX form markup and all submitted field names. |
| Result template | [jiraissueui/component.go](jiraissueui/component.go#L359-L364) | The HTMX fragment returned after create attempts. |
| Launcher template | [jiraissueui/component.go](jiraissueui/component.go#L366-L374) | The standalone create button markup and `data-jira-create-target` contract. |
| Dialog template | [jiraissueui/component.go](jiraissueui/component.go#L376-L396) | The button + native `<dialog>` + form composition. |
| Dialog controller JS | [jiraissueui/component.go](jiraissueui/component.go#L398-L467) | Opens/closes dialogs, supports HTMX-loaded markup, and closes after successful create. |
| Scoped CSS | [jiraissueui/component.go](jiraissueui/component.go#L469-L600) | Styles are scoped under `.hx-jira-create` and themeable with CSS custom properties. |

## Choose Import Or Copy

### Option A: Import From This Module

Use this when the target project can depend on this repo as a Go module. The module path is declared in [go.mod](go.mod#L1-L5).

```bash
go get github.com/ccastorena/jira-agent/jiraissueui
```

In target Go files:

```go
import "github.com/ccastorena/jira-agent/jiraissueui"
```

For local development against a checkout next to the target project:

```go
replace github.com/ccastorena/jira-agent => ../jira
```

### Option B: Copy The Package Into The Target Project

Use this when the target project should own the code and not depend on this whole repo.

1. Copy [jiraissueui/component.go](jiraissueui/component.go#L1-L600) into target `jiraissueui/component.go`.
2. Copy [jiraissueui/component_test.go](jiraissueui/component_test.go#L1-L289) into target `jiraissueui/component_test.go`.
3. Keep `package jiraissueui` if the target folder is named `jiraissueui`.
4. If you put it under `internal/jiraissueui`, `package jiraissueui` is still fine. Import it from the target module as `your/module/internal/jiraissueui`.
5. Run `gofmt` and `go test ./...` in the target project.

## Target Project File Plan

A clean target app usually needs these files or equivalents:

| Target file | Purpose |
| --- | --- |
| `jiraissueui/component.go` | Copied reusable package, or omit if importing from this module. |
| `jiraissueui/component_test.go` | Copied tests for parser/rendering/assets. |
| `internal/web/jira_create.go` | Target app handler that renders the dialog/form and handles POSTs. |
| `internal/jira/client.go` | Target app Jira client. It must list projects, list assignable users, and create issues. |
| `internal/web/templates.go` or `.gohtml` templates | Page layout with HTMX, `JiraCreateStyles`, `JiraCreateScript`, and the rendered button/dialog. |
| `cmd/<app>/main.go` | Route registration for `/jira/create` and the page containing the button. |

Names are flexible. The important part is that the reusable package stays separate from the Jira client and HTTP app wiring.

## Minimal Target Handler

Create a target file like `internal/web/jira_create.go`. This is the shape another Copilot should implement after importing or copying `jiraissueui`.

```go
package web

import (
    "html/template"
    "net/http"
    "strings"

    "your/module/jiraissueui"
)

type JiraCreateService interface {
    ListProjects() ([]jiraissueui.Project, error)
    SearchAssignableUsers(projectKey string) ([]jiraissueui.User, error)
    CreateIssue(form jiraissueui.IssueForm) (key string, browseURL string, err error)
}

type JiraCreateHandler struct {
    Jira     JiraCreateService
    UI       *jiraissueui.Component
    Endpoint string
}

func NewJiraCreateHandler(jira JiraCreateService) JiraCreateHandler {
    return JiraCreateHandler{
        Jira:     jira,
        UI:       jiraissueui.New(),
        Endpoint: "/jira/create",
    }
}

func (h JiraCreateHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet && r.Method != http.MethodPost {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }

    values := jiraissueui.IssueForm{ProjectKey: selectedProject(r)}
    result := jiraissueui.Result{}

    if r.Method == http.MethodPost {
        values, result = h.createFromRequest(r)
        if r.Header.Get("HX-Request") == "true" {
            if result.Err == "" {
                w.Header().Set("HX-Trigger", "jiraIssueCreated")
            }
            w.Header().Set("Content-Type", "text/html; charset=utf-8")
            _ = h.component().RenderResult(w, jiraissueui.FormData{Result: result})
            return
        }
    }

    form, err := h.component().Form(h.formData(values, result))
    if err != nil {
        http.Error(w, "create issue form unavailable", http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    _ = createIssuePageTemplate.Execute(w, map[string]any{
        "CreateStyles": jiraissueui.StyleTag(),
        "CreateForm":   form,
    })
}

func (h JiraCreateHandler) Dialog() template.HTML {
    dialog, err := h.component().Dialog(jiraissueui.DialogData{
        ButtonLabel:   "Create",
        Title:         "Create Jira Issue",
        Form:          h.formData(jiraissueui.IssueForm{}, jiraissueui.Result{}),
        DisableScript: true,
    })
    if err != nil {
        return template.HTML(`<a href="/jira/create">Create</a>`)
    }
    return dialog
}

func (h JiraCreateHandler) formData(values jiraissueui.IssueForm, result jiraissueui.Result) jiraissueui.FormData {
    endpoint := h.Endpoint
    if endpoint == "" {
        endpoint = "/jira/create"
    }

    projects, projectsErr := h.Jira.ListProjects()
    if values.ProjectKey == "" && len(projects) > 0 {
        values.ProjectKey = projects[0].Key
    }

    assignees, assigneesErr := h.Jira.SearchAssignableUsers(values.ProjectKey)

    return jiraissueui.FormData{
        Endpoint:     endpoint,
        Projects:     projects,
        ProjectsErr:  errorText(projectsErr),
        Assignees:    assignees,
        AssigneesErr: errorText(assigneesErr),
        Values:       values,
        Result:       result,
    }
}

func (h JiraCreateHandler) createFromRequest(r *http.Request) (jiraissueui.IssueForm, jiraissueui.Result) {
    form, err := jiraissueui.ParseRequest(r)
    if err != nil {
        return form, jiraissueui.Result{Err: err.Error()}
    }

    key, browseURL, err := h.Jira.CreateIssue(form)
    if err != nil {
        return form, jiraissueui.Result{Err: err.Error()}
    }
    return form, jiraissueui.Result{Key: key, URL: browseURL}
}

func (h JiraCreateHandler) component() *jiraissueui.Component {
    if h.UI != nil {
        return h.UI
    }
    return jiraissueui.New()
}

func selectedProject(r *http.Request) string {
    return strings.ToUpper(strings.TrimSpace(r.FormValue("project_key")))
}

func errorText(err error) string {
    if err == nil {
        return ""
    }
    return err.Error()
}
```

This handler is based on [cmd/jira-agent/web_create.go](cmd/jira-agent/web_create.go#L17-L163), but it removes this repo's `webApp` coupling and replaces it with a small `JiraCreateService` interface.

## Target Template Wiring

The page with the button needs HTMX and the Jira create assets. Use this in the target layout `<head>`:

```html
<script src="https://unpkg.com/htmx.org@1.9.12"></script>
{{ .JiraCreateStyles }}
{{ .JiraCreateScript }}
```

This mirrors [cmd/jira-agent/web_templates.go](cmd/jira-agent/web_templates.go#L220-L222). The script should appear once per full page. If the target app bundles assets instead of inline tags, use `jiraissueui.CSS()` and `jiraissueui.JS()` from [jiraissueui/component.go](jiraissueui/component.go#L270-L288).

Place the rendered dialog where the button should appear:

```html
<div class="toolbar">
  {{ .JiraCreateDialog }}
</div>
```

This mirrors [cmd/jira-agent/web_templates.go](cmd/jira-agent/web_templates.go#L258-L262).

The page handler that renders this layout should pass:

```go
createHandler := web.NewJiraCreateHandler(jiraClient)

data := map[string]any{
    "JiraCreateStyles": jiraissueui.StyleTag(),
    "JiraCreateScript": jiraissueui.ScriptTag(),
    "JiraCreateDialog": createHandler.Dialog(),
}
```

This mirrors [cmd/jira-agent/web_server.go](cmd/jira-agent/web_server.go#L112-L122).

## Route Wiring

Register the create endpoint in the target app startup:

```go
jiraCreate := web.NewJiraCreateHandler(jiraClient)

mux := http.NewServeMux()
mux.Handle("/jira/create", jiraCreate)
```

This mirrors [cmd/jira-agent/web_server.go](cmd/jira-agent/web_server.go#L66-L68). The endpoint must match `FormData.Endpoint`; the default is `/jira/create` from [jiraissueui/component.go](jiraissueui/component.go#L20-L28).

## Jira Client Mapping

The reusable UI package intentionally does not know how to talk to Jira. The target project must map its Jira client responses into these UI models:

```go
[]jiraissueui.Project{
    {Key: "SCRUM", Name: "Scrum Team"},
}

[]jiraissueui.User{
    {AccountID: "712020:abc", DisplayName: "Ada Lovelace"},
}
```

The source app's mapping examples are:

- Projects: [cmd/jira-agent/web_data.go](cmd/jira-agent/web_data.go#L109-L130)
- Assignable users: [cmd/jira-agent/web_data.go](cmd/jira-agent/web_data.go#L132-L157)

When creating an issue, convert `jiraissueui.IssueForm` into the target Jira client's create payload. The source app's mapping is [cmd/jira-agent/web_create.go](cmd/jira-agent/web_create.go#L129-L153). The important fields are:

| `IssueForm` field | Form input name | Notes |
| --- | --- | --- |
| `ProjectKey` | `project_key` | Required. `ParseValues` uppercases and trims it. |
| `Summary` | `summary` | Required. |
| `IssueType` | `issue_type` | Defaults to `Task` unless overridden. |
| `Description` | `description` | Plain text from the textarea. |
| `Priority` | `priority` | Empty string means no priority. |
| `Labels` | `labels` | Comma-separated input parsed into `[]string`. |
| `AssigneeAccountID` | `assignee_account_id` | Empty string means unassigned/default behavior. |
| `ReporterAccountID` | `reporter_account_id` | Empty string means Jira default behavior. |

The form field names are rendered in [jiraissueui/component.go](jiraissueui/component.go#L311-L354), and the parser expects them in [jiraissueui/component.go](jiraissueui/component.go#L253-L267). Keep those in sync if you customize the template.

## Button-Only Mode

If the target app already owns the `<dialog>` markup, use only the reusable button launcher:

```go
button, err := jiraissueui.New().Launcher(jiraissueui.LauncherData{
    Endpoint:    "/jira/create",
    DialogID:    "my-create-dialog",
    ButtonLabel: "Create",
})
```

The output uses the launcher markup from [jiraissueui/component.go](jiraissueui/component.go#L366-L374). The target dialog must have the matching id:

```html
{{ .JiraCreateButton }}

<dialog id="my-create-dialog" data-jira-create-dialog>
  <button type="button" data-jira-create-close>Close</button>
  {{ .CreateForm }}
</dialog>
```

Include `jiraissueui.ScriptTag()` once so the controller in [jiraissueui/component.go](jiraissueui/component.go#L398-L467) can open and close the dialog.

## Full Dialog Mode

Most Go/HTMX projects should use full dialog mode. It produces the button, native dialog, close control, form, result target, and optional inline controller script.

```go
dialog, err := jiraissueui.New().Dialog(jiraissueui.DialogData{
    DialogID:      "jira-create-dialog",
    ButtonLabel:   "Create",
    Title:         "Create Jira Issue",
    DisableScript: true,
    Form: jiraissueui.FormData{
        Endpoint:  "/jira/create",
        Projects:  projects,
        Assignees: users,
    },
})
```

Use `DisableScript: true` when the page includes `jiraissueui.ScriptTag()` globally. Leave it false only for a tiny drop-in page where the dialog is the only place that should include the controller script. The dialog template is [jiraissueui/component.go](jiraissueui/component.go#L376-L396).

## Dedicated Create Page Mode

The target app can also expose a full page at `/jira/create`. That is useful when JavaScript is disabled or when someone opens the button link in a new tab. The source page template is [cmd/jira-agent/web_templates.go](cmd/jira-agent/web_templates.go#L324-L413).

The key template pieces are:

```html
{{ .CreateStyles }}
...
{{ .CreateForm }}
```

The handler sample above already supports non-HTMX GET and POST. For HTMX POSTs it returns only `RenderResult`, following [cmd/jira-agent/web_create.go](cmd/jira-agent/web_create.go#L22-L29).

## Tests To Copy Or Recreate

Copy [jiraissueui/component_test.go](jiraissueui/component_test.go#L1-L289) if you copy the package. If the target project imports the package, write integration tests around the target handler instead.

Useful test ranges:

- Parser normalization and validation: [jiraissueui/component_test.go](jiraissueui/component_test.go#L10-L68)
- Form HTMX/native attributes: [jiraissueui/component_test.go](jiraissueui/component_test.go#L70-L89)
- Standalone launcher button: [jiraissueui/component_test.go](jiraissueui/component_test.go#L91-L112)
- Full dialog rendering and default behavior: [jiraissueui/component_test.go](jiraissueui/component_test.go#L114-L178)
- Escaping user-controlled text: [jiraissueui/component_test.go](jiraissueui/component_test.go#L180-L254)
- Result fragment and asset tags: [jiraissueui/component_test.go](jiraissueui/component_test.go#L256-L289)

Minimum target verification:

```bash
go test ./...
```

Manual HTMX smoke test after starting the target app:

```bash
curl -i -X POST http://localhost:8080/jira/create \
  -H 'HX-Request: true' \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  --data 'project_key=SCRUM&summary=Smoke+test&issue_type=Task'
```

Expected response:

- HTTP 200
- `Content-Type: text/html; charset=utf-8`
- `HX-Trigger: jiraIssueCreated` on success
- Body contains `<div class="hx-jira-create-result" id="jira-create-result"`

## Common Pitfalls

- Do not copy [cmd/jira-agent/web_create.go](cmd/jira-agent/web_create.go#L17-L163) unchanged into another project. It is tied to this app's `webApp`, `JiraClient`, and helpers.
- Do copy [jiraissueui/component.go](jiraissueui/component.go#L1-L600) unchanged first. Customize after the target project passes tests.
- Keep the form `Endpoint` and the registered route identical. Defaults are in [jiraissueui/component.go](jiraissueui/component.go#L20-L28).
- Include HTMX on the page. Without HTMX, the form still submits natively, but modal result swapping will not happen.
- Include `jiraissueui.ScriptTag()` once on pages with modal launchers. Otherwise `data-jira-create-open` links behave like normal links.
- Use `DisableScript: true` when `ScriptTag()` is already in the page head. This avoids duplicated controller scripts.
- Preserve `hx-target="#jira-create-result"` and matching result id if you customize ids. The defaults are in [jiraissueui/component.go](jiraissueui/component.go#L20-L28), and the form/result templates are [jiraissueui/component.go](jiraissueui/component.go#L306-L364).
- If project or assignee loading fails, pass `ProjectsErr` or `AssigneesErr` in `FormData`; the component renders warnings instead of crashing.

## Prompt For Another Copilot

Use this prompt in the target Go/HTMX project:

```text
I need to integrate the Jira ticket creation button from another repo. Copy or import the reusable jiraissueui package described in JIRA_ISSUE_UI.md. Start with jiraissueui/component.go lines 1-600 and component_test.go lines 1-289. Then create a target HTTP handler equivalent to the sample in this guide using this project's Jira client. Wire /jira/create, include jiraissueui.StyleTag() and jiraissueui.ScriptTag() in the page head, render jiraissueui.Dialog() where the create button belongs, and run go test ./.... Do not copy cmd/jira-agent/web_create.go unchanged because it is app-specific.
```