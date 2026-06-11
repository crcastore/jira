# Reusing the Jira Issue Create UI

The Jira issue create button, dialog, form, CSS, JavaScript controller, and form parser live in [jiraissueui/](jiraissueui/). The package does not depend on this app's Jira client or HTTP routes, so another Go web project can import it and connect it to its own Jira service.

## Install

From another Go module:

```bash
go get github.com/ccastorena/jira-agent/jiraissueui
```

If you are developing against a local checkout, point your app at it temporarily:

```go
replace github.com/ccastorena/jira-agent => ../jira
```

## Add the Assets

The widget expects HTMX for async form posts and uses scoped class names under `.hx-jira-create`.

```html
<script src="https://unpkg.com/htmx.org@1.9.12"></script>
{{ .JiraCreateStyles }}
{{ .JiraCreateScript }}
```

Populate those template values from Go:

```go
data := map[string]any{
    "JiraCreateStyles": jiraissueui.StyleTag(),
    "JiraCreateScript": jiraissueui.ScriptTag(),
}
```

Use `ScriptTag` once per page. When you render a dialog on that page, set `DisableScript: true` so each dialog does not duplicate the controller script.

## Render a Drop-In Button and Dialog

```go
ui := jiraissueui.New()

dialog, err := ui.Dialog(jiraissueui.DialogData{
    ButtonLabel:   "Create",
    Title:         "Create Jira Issue",
    DisableScript: true,
    Form: jiraissueui.FormData{
        Endpoint: "/jira/create",
        Projects: []jiraissueui.Project{
            {Key: "SCRUM", Name: "Scrum Team"},
        },
        Assignees: []jiraissueui.User{
            {AccountID: "abc123", DisplayName: "Ada Lovelace"},
        },
    },
})
if err != nil {
    // Handle template/render error.
}
```

Place `dialog` wherever the create button should appear. The output includes:

- A launcher button
- A native `<dialog>` window
- A Jira issue form with normal `action`/`method` fallback
- HTMX attributes that POST to `Form.Endpoint` and swap the result target

## Render Only the Launcher Button

If your app owns the dialog markup, render only the reusable launcher button and point it at your dialog id:

```go
button, err := ui.Launcher(jiraissueui.LauncherData{
    Endpoint:    "/jira/create",
    DialogID:    "my-create-dialog",
    ButtonLabel: "Create",
})
```

The button includes `data-jira-create-open` and `data-jira-create-target="#my-create-dialog"`. Include `jiraissueui.ScriptTag()` once on the page so the controller can open and close the target dialog.

## Handle GET and POST

Your project owns Jira data loading and issue creation. The UI package only parses form values and renders HTML.

```go
func handleJiraCreate(w http.ResponseWriter, r *http.Request) {
    ui := jiraissueui.New()
    values := jiraissueui.IssueForm{}
    result := jiraissueui.Result{}

    if r.Method == http.MethodPost {
        form, err := jiraissueui.ParseRequest(r)
        values = form
        if err != nil {
            result.Err = err.Error()
        } else {
            key, url, createErr := createIssueInYourJiraClient(form)
            if createErr != nil {
                result.Err = createErr.Error()
            } else {
                result = jiraissueui.Result{Key: key, URL: url}
                w.Header().Set("HX-Trigger", "jiraIssueCreated")
            }
        }

        if r.Header.Get("HX-Request") == "true" {
            w.Header().Set("Content-Type", "text/html; charset=utf-8")
            _ = ui.RenderResult(w, jiraissueui.FormData{Result: result})
            return
        }
    }

    form, err := ui.Form(jiraissueui.FormData{
        Endpoint:  "/jira/create",
        Projects:  loadProjectsFromYourJiraClient(),
        Assignees: loadAssignableUsersFromYourJiraClient(),
        Values:    values,
        Result:    result,
    })
    if err != nil {
        http.Error(w, "create issue form unavailable", http.StatusInternalServerError)
        return
    }

    _ = pageTemplate.Execute(w, map[string]any{
        "JiraCreateStyles": jiraissueui.StyleTag(),
        "CreateForm":       form,
    })
}
```

Map `jiraissueui.IssueForm` to your Jira client's create-issue request. The parsed fields are `ProjectKey`, `Summary`, `IssueType`, `Description`, `Priority`, `Labels`, `AssigneeAccountID`, and `ReporterAccountID`.

## Styling Hooks

The CSS is scoped and themeable through custom properties:

```css
:root {
  --jira-create-border: #d1d5db;
  --jira-create-muted: #6b7280;
  --jira-create-ink: #1f2937;
  --jira-create-accent: linear-gradient(135deg, #115e59, #0f766e);
  --jira-create-success: #166534;
  --jira-create-error: #9f1239;
}
```

Use `jiraissueui.CSS()` or `jiraissueui.JS()` if your app bundles assets instead of rendering inline style/script tags.