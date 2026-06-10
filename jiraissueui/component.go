// Package jiraissueui renders a reusable HTMX-friendly Jira issue creation form.
//
// The package deliberately knows nothing about a concrete Jira client. Host
// applications provide projects, assignable users, and POST the parsed IssueForm
// to their own Jira service. The rendered form has a normal action/method
// fallback and HTMX attributes, so it can work in plain HTML pages or be dropped
// into HTMX apps.
package jiraissueui

import (
	"bytes"
	"errors"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const (
	DefaultEndpoint    = "/jira/create"
	DefaultDialogID    = "jira-create-dialog"
	DefaultResultID    = "jira-create-result"
	DefaultWorkingID   = "jira-create-working"
	DefaultButtonLabel = "Create Jira Issue"
	DefaultTitle       = "Create Jira Issue"
	DefaultSubmitLabel = "Create Issue"
)

var (
	DefaultIssueTypes = []string{"Task", "Bug", "Story", "Epic"}
	DefaultPriorities = []string{"Highest", "High", "Medium", "Low", "Lowest"}

	ErrInvalidForm    = errors.New("Invalid form submission")
	ErrRequiredFields = errors.New("Project and summary are required")
)

// Project is a Jira project option shown in the project dropdown.
type Project struct {
	Key  string
	Name string
}

// User is a Jira user option shown in assignee and reporter dropdowns.
type User struct {
	AccountID   string
	DisplayName string
}

// IssueForm is the normalized form payload submitted by the create issue UI.
type IssueForm struct {
	ProjectKey        string
	Summary           string
	IssueType         string
	Description       string
	Priority          string
	Labels            []string
	AssigneeAccountID string
	ReporterAccountID string
}

// LabelsCSV returns labels in the same comma-separated format accepted by the form.
func (f IssueForm) LabelsCSV() string {
	return strings.Join(f.Labels, ", ")
}

// Result is rendered above the form after a create attempt.
type Result struct {
	Key string
	URL string
	Err string
}

// FormData configures the rendered create issue form.
type FormData struct {
	Endpoint    string
	ResultID    string
	WorkingID   string
	SubmitLabel string

	Projects     []Project
	ProjectsErr  string
	Assignees    []User
	AssigneesErr string

	IssueTypes []string
	Priorities []string
	Values     IssueForm
	Result     Result
}

// DialogData configures a drop-in create issue launcher and dialog window.
type DialogData struct {
	DialogID    string
	ButtonLabel string
	Title       string
	Form        FormData
}

func (d *DialogData) applyDefaults() {
	if d.DialogID == "" {
		d.DialogID = DefaultDialogID
	}
	if d.ButtonLabel == "" {
		d.ButtonLabel = DefaultButtonLabel
	}
	if d.Title == "" {
		d.Title = DefaultTitle
	}
	d.Form.applyDefaults()
}

func (d *FormData) applyDefaults() {
	if d.Endpoint == "" {
		d.Endpoint = DefaultEndpoint
	}
	if d.ResultID == "" {
		d.ResultID = DefaultResultID
	}
	if d.WorkingID == "" {
		d.WorkingID = DefaultWorkingID
	}
	if d.SubmitLabel == "" {
		d.SubmitLabel = DefaultSubmitLabel
	}
	if len(d.IssueTypes) == 0 {
		d.IssueTypes = append([]string(nil), DefaultIssueTypes...)
	}
	if len(d.Priorities) == 0 {
		d.Priorities = append([]string(nil), DefaultPriorities...)
	}
	if d.Values.ProjectKey == "" && len(d.Projects) > 0 {
		d.Values.ProjectKey = d.Projects[0].Key
	}
	if d.Values.IssueType == "" {
		d.Values.IssueType = d.IssueTypes[0]
	}
}

// Component renders Jira issue creation forms and result snippets.
type Component struct {
	tmpl *template.Template
}

// New returns a ready-to-use Component.
func New() *Component {
	return &Component{tmpl: template.Must(template.New("jiraissueui").Funcs(template.FuncMap{
		"selected": selected,
	}).Parse(formTmpl + resultTmpl + dialogTmpl))}
}

// RenderForm writes the full embeddable create issue form to w.
func (c *Component) RenderForm(w io.Writer, data FormData) error {
	data.applyDefaults()
	return c.tmpl.ExecuteTemplate(w, "jira-create-form", data)
}

// Form returns the full embeddable create issue form as template.HTML.
func (c *Component) Form(data FormData) (template.HTML, error) {
	var buf bytes.Buffer
	if err := c.RenderForm(&buf, data); err != nil {
		return "", err
	}
	return template.HTML(buf.String()), nil
}

// RenderResult writes only the result target. This is useful for HTMX responses.
func (c *Component) RenderResult(w io.Writer, data FormData) error {
	data.applyDefaults()
	return c.tmpl.ExecuteTemplate(w, "jira-create-result", data)
}

// ResultHTML returns only the result target as template.HTML.
func (c *Component) ResultHTML(data FormData) (template.HTML, error) {
	var buf bytes.Buffer
	if err := c.RenderResult(&buf, data); err != nil {
		return "", err
	}
	return template.HTML(buf.String()), nil
}

// RenderDialog writes a complete drop-in launcher and dialog window.
func (c *Component) RenderDialog(w io.Writer, data DialogData) error {
	data.applyDefaults()
	return c.tmpl.ExecuteTemplate(w, "jira-create-dialog", data)
}

// Dialog returns a complete drop-in launcher and dialog window as template.HTML.
func (c *Component) Dialog(data DialogData) (template.HTML, error) {
	var buf bytes.Buffer
	if err := c.RenderDialog(&buf, data); err != nil {
		return "", err
	}
	return template.HTML(buf.String()), nil
}

// ParseRequest parses and validates a create issue form request.
func ParseRequest(r *http.Request) (IssueForm, error) {
	if err := r.ParseForm(); err != nil {
		return IssueForm{}, ErrInvalidForm
	}
	return ParseValues(r.Form)
}

// ParseValues normalizes and validates posted create issue form values.
func ParseValues(values url.Values) (IssueForm, error) {
	form := IssueForm{
		ProjectKey:        strings.ToUpper(strings.TrimSpace(values.Get("project_key"))),
		Summary:           strings.TrimSpace(values.Get("summary")),
		IssueType:         strings.TrimSpace(values.Get("issue_type")),
		Description:       strings.TrimSpace(values.Get("description")),
		Priority:          strings.TrimSpace(values.Get("priority")),
		Labels:            splitCSV(values.Get("labels")),
		AssigneeAccountID: strings.TrimSpace(values.Get("assignee_account_id")),
		ReporterAccountID: strings.TrimSpace(values.Get("reporter_account_id")),
	}
	if form.ProjectKey == "" || form.Summary == "" {
		return form, ErrRequiredFields
	}
	return form, nil
}

// StyleTag returns scoped CSS wrapped in a <style> tag for easy embedding.
func StyleTag() template.HTML {
	return template.HTML("<style>" + componentCSS + "</style>")
}

// CSS returns the raw scoped CSS.
func CSS() template.CSS {
	return template.CSS(componentCSS)
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value != "" {
			values = append(values, value)
		}
	}
	return values
}

func selected(got, want string) bool {
	return got == want
}

const formTmpl = `{{define "jira-create-form"}}
<div class="hx-jira-create">
  {{template "jira-create-result" .}}
  {{if .ProjectsErr}}<div class="hx-jira-create-warn">Could not load Jira projects: {{.ProjectsErr}}</div>{{end}}
  {{if .AssigneesErr}}<div class="hx-jira-create-warn">Could not load Jira assignees: {{.AssigneesErr}}</div>{{end}}
  <form class="hx-jira-create-form" action="{{.Endpoint}}" method="post" hx-post="{{.Endpoint}}" hx-target="#{{.ResultID}}" hx-swap="outerHTML" hx-indicator="#{{.WorkingID}}" hx-disabled-elt="find input, find textarea, find select, find button">
    <div class="hx-jira-create-grid">
      <label class="hx-jira-create-field">Project
        {{if .Projects}}
        <select name="project_key" required>
          {{range .Projects}}<option value="{{.Key}}"{{if selected $.Values.ProjectKey .Key}} selected{{end}}>{{.Key}} - {{.Name}}</option>{{end}}
        </select>
        {{else}}
        <input name="project_key" type="text" value="{{.Values.ProjectKey}}" autocomplete="off" required>
        {{end}}
      </label>
      <label class="hx-jira-create-field">Issue type
        <select name="issue_type">
          {{range .IssueTypes}}<option value="{{.}}"{{if selected $.Values.IssueType .}} selected{{end}}>{{.}}</option>{{end}}
        </select>
      </label>
    </div>
    <label class="hx-jira-create-field">Summary<input name="summary" type="text" value="{{.Values.Summary}}" autocomplete="off" required></label>
    <label class="hx-jira-create-field">Description<textarea name="description">{{.Values.Description}}</textarea></label>
    <div class="hx-jira-create-grid">
      <label class="hx-jira-create-field">Priority
        <select name="priority">
          <option value="">None</option>
          {{range .Priorities}}<option value="{{.}}"{{if selected $.Values.Priority .}} selected{{end}}>{{.}}</option>{{end}}
        </select>
      </label>
      <label class="hx-jira-create-field">Labels<input name="labels" type="text" value="{{.Values.LabelsCSV}}" autocomplete="off"></label>
    </div>
    <div class="hx-jira-create-grid">
      <label class="hx-jira-create-field">Assignee
        <select name="assignee_account_id">
          <option value="">Unassigned</option>
          {{range .Assignees}}<option value="{{.AccountID}}"{{if selected $.Values.AssigneeAccountID .AccountID}} selected{{end}}>{{.DisplayName}}</option>{{end}}
        </select>
      </label>
      <label class="hx-jira-create-field">Reporter
        <select name="reporter_account_id">
          <option value="">Default</option>
          {{range .Assignees}}<option value="{{.AccountID}}"{{if selected $.Values.ReporterAccountID .AccountID}} selected{{end}}>{{.DisplayName}}</option>{{end}}
        </select>
      </label>
    </div>
    <div class="hx-jira-create-working htmx-indicator" id="{{.WorkingID}}" role="status" aria-live="polite">Creating issue...</div>
    <button type="submit">{{.SubmitLabel}}</button>
  </form>
</div>
{{end}}`

const resultTmpl = `{{define "jira-create-result"}}
<div class="hx-jira-create-result" id="{{.ResultID}}" aria-live="polite">
  {{if .Result.Err}}<div class="hx-jira-create-warn">{{.Result.Err}}</div>{{end}}
  {{if .Result.Key}}<div class="hx-jira-create-notice">Created <a href="{{.Result.URL}}" target="_blank" rel="noreferrer">{{.Result.Key}}</a></div>{{end}}
</div>
{{end}}`

const dialogTmpl = `{{define "jira-create-dialog"}}
<div class="hx-jira-create-launcher" data-jira-create-launcher>
	<a class="hx-jira-create-button" href="{{.Form.Endpoint}}" data-jira-create-open>{{.ButtonLabel}}</a>
	<dialog class="hx-jira-create-dialog" id="{{.DialogID}}" data-jira-create-dialog>
		<div class="hx-jira-create-window">
			<header class="hx-jira-create-window-head">
				<h2>{{.Title}}</h2>
				<button class="hx-jira-create-close" type="button" aria-label="Close" data-jira-create-close>&times;</button>
			</header>
			<div class="hx-jira-create-window-body">
				{{template "jira-create-form" .Form}}
			</div>
		</div>
	</dialog>
	<script>
	(function(){
		var root = document.currentScript ? document.currentScript.closest('[data-jira-create-launcher]') : null;
		if(!root || root.dataset.jiraCreateReady){ return; }
		root.dataset.jiraCreateReady = "1";
		var opener = root.querySelector('[data-jira-create-open]');
		var dialog = root.querySelector('[data-jira-create-dialog]');
		var close = root.querySelector('[data-jira-create-close]');
		if(!opener || !dialog || !dialog.showModal){ return; }
		opener.addEventListener('click', function(event){
			event.preventDefault();
			if(!dialog.open){ dialog.showModal(); }
		});
		if(close){ close.addEventListener('click', function(){ dialog.close(); }); }
		dialog.addEventListener('click', function(event){
			if(event.target === dialog){ dialog.close(); }
		});
		document.body.addEventListener('htmx:afterSwap', function(event){
			if(event.target && dialog.contains(event.target) && event.detail && event.detail.successful){
				var link = event.target.querySelector('.hx-jira-create-notice a');
				if(link){ setTimeout(function(){ dialog.close(); }, 700); }
			}
		});
	})();
	</script>
</div>
{{end}}`

const componentCSS = `
.hx-jira-create {
  display: grid;
  gap: 12px;
}
.hx-jira-create-form {
  display: grid;
  gap: 12px;
}
.hx-jira-create-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}
.hx-jira-create-field {
  display: grid;
  gap: 5px;
  font-size: 12px;
  color: var(--jira-create-muted, #6b7280);
}
.hx-jira-create-field input,
.hx-jira-create-field select,
.hx-jira-create-field textarea {
  width: 100%;
  border: 1px solid var(--jira-create-border, #d1d5db);
  border-radius: 8px;
  padding: 9px 10px;
  color: var(--jira-create-ink, #1f2937);
  font: inherit;
  font-size: 14px;
  background: var(--jira-create-field-bg, #fff);
}
.hx-jira-create-field textarea {
  min-height: 140px;
  resize: vertical;
}
.hx-jira-create button {
  justify-self: start;
  border: 0;
  border-radius: 10px;
  background: var(--jira-create-accent, linear-gradient(135deg, #115e59, #0f766e));
  color: #fff;
  font-weight: 700;
  padding: 10px 14px;
  cursor: pointer;
}
.hx-jira-create-button {
	display: inline-flex;
	align-items: center;
	min-height: 36px;
	padding: 8px 12px;
	border: 1px solid var(--jira-create-border, #d1d5db);
	border-radius: 8px;
	background: var(--jira-create-button-bg, #fff);
	color: var(--jira-create-ink, #1f2937);
	font-size: 13px;
	font-weight: 700;
	text-decoration: none;
}
.hx-jira-create-button:hover {
	border-color: var(--jira-create-hover-border, #9ca3af);
}
.hx-jira-create-dialog {
	width: min(92vw, 760px);
	border: 0;
	border-radius: 12px;
	padding: 0;
	color: var(--jira-create-ink, #1f2937);
	background: var(--jira-create-panel, #fff);
	box-shadow: 0 24px 80px rgba(15, 23, 42, 0.28);
}
.hx-jira-create-dialog::backdrop {
	background: rgba(15, 23, 42, 0.32);
}
.hx-jira-create-window {
	display: grid;
	max-height: min(86vh, 760px);
}
.hx-jira-create-window-head {
	display: flex;
	justify-content: space-between;
	align-items: center;
	gap: 12px;
	padding: 14px 16px;
	border-bottom: 1px solid var(--jira-create-border, #d1d5db);
}
.hx-jira-create-window-head h2 {
	margin: 0;
	font-size: 18px;
}
.hx-jira-create-close {
	border: 0;
	border-radius: 8px;
	background: transparent;
	color: var(--jira-create-muted, #6b7280);
	font-size: 22px;
	line-height: 1;
	padding: 4px 8px;
	cursor: pointer;
}
.hx-jira-create-window-body {
	overflow: auto;
	padding: 16px;
}
.hx-jira-create-result {
  min-height: 18px;
}
.hx-jira-create-warn {
  color: var(--jira-create-error, #9f1239);
  font-size: 13px;
}
.hx-jira-create-notice {
  color: var(--jira-create-success, #166534);
  font-size: 13px;
}
.hx-jira-create-notice a {
  color: var(--jira-create-success, #166534);
  font-weight: 700;
  text-decoration: none;
}
.hx-jira-create-working {
  display: none;
  font-size: 12px;
  color: var(--jira-create-muted, #6b7280);
}
.hx-jira-create-working.htmx-request {
  display: block;
}
@media (max-width: 720px) {
  .hx-jira-create-grid { grid-template-columns: 1fr; }
}
`
