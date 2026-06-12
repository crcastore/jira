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
	DefaultEndpoint             = "/jira/create"
	DefaultPullRequestsEndpoint = "/jira/create/pull-requests"
	DefaultUsersEndpoint        = "/jira/create/users"
	DefaultDialogID             = "jira-create-dialog"
	DefaultResultID             = "jira-create-result"
	DefaultPullRequestsID       = "jira-create-pull-requests"
	DefaultAssigneeOptionsID    = "jira-create-assignee-options"
	DefaultReporterOptionsID    = "jira-create-reporter-options"
	DefaultWorkingID            = "jira-create-working"
	DefaultButtonLabel          = "Create Jira Issue"
	DefaultTitle                = "Create Jira Issue"
	DefaultSubmitLabel          = "Create Issue"
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

// User is a Jira user option shown in assignee and reporter search results.
type User struct {
	AccountID   string
	DisplayName string
}

// PullRequestRepo is a GitHub repository option shown before selecting a PR/MR.
type PullRequestRepo struct {
	FullName string
	Label    string
}

// DisplayLabel returns the visible label for a repository option.
func (r PullRequestRepo) DisplayLabel() string {
	if r.Label != "" {
		return r.Label
	}
	return r.FullName
}

// PullRequestOption is a pull request / merge request option for a selected repo.
type PullRequestOption struct {
	Value string
	Label string
}

// IssueForm is the normalized form payload submitted by the create issue UI.
type IssueForm struct {
	ProjectKey        string
	Summary           string
	IssueType         string
	Description       string
	PullRequestRepo   string
	PullRequest       string
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
	Endpoint             string
	PullRequestsEndpoint string
	UsersEndpoint        string
	ResultID             string
	PullRequestsID       string
	AssigneeOptionsID    string
	ReporterOptionsID    string
	WorkingID            string
	SubmitLabel          string

	Projects            []Project
	ProjectsErr         string
	Assignees           []User
	AssigneesErr        string
	PullRequestRepos    []PullRequestRepo
	PullRequestReposErr string
	PullRequests        []PullRequestOption
	PullRequestsErr     string

	IssueTypes []string
	Priorities []string
	Values     IssueForm
	Result     Result
}

// UserOptionsData configures a replaceable datalist of Jira user search results.
type UserOptionsData struct {
	OptionsID string
	Users     []User
}

// LauncherData configures a create issue button that opens a Jira create dialog.
type LauncherData struct {
	Endpoint    string
	DialogID    string
	ButtonLabel string
}

// DialogData configures a drop-in create issue launcher and dialog window.
type DialogData struct {
	DialogID    string
	ButtonLabel string
	Title       string
	Form        FormData

	// DisableScript omits the inline launcher controller. Use this when the host
	// page includes ScriptTag once globally.
	DisableScript bool
}

func (d DialogData) LauncherData() LauncherData {
	return LauncherData{
		Endpoint:    d.Form.Endpoint,
		DialogID:    d.DialogID,
		ButtonLabel: d.ButtonLabel,
	}
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

func (d *LauncherData) applyDefaults() {
	if d.Endpoint == "" {
		d.Endpoint = DefaultEndpoint
	}
	if d.DialogID == "" {
		d.DialogID = DefaultDialogID
	}
	if d.ButtonLabel == "" {
		d.ButtonLabel = DefaultButtonLabel
	}
}

func (d *FormData) applyDefaults() {
	if d.Endpoint == "" {
		d.Endpoint = DefaultEndpoint
	}
	if d.PullRequestsEndpoint == "" {
		d.PullRequestsEndpoint = DefaultPullRequestsEndpoint
	}
	if d.UsersEndpoint == "" {
		d.UsersEndpoint = DefaultUsersEndpoint
	}
	if d.ResultID == "" {
		d.ResultID = DefaultResultID
	}
	if d.PullRequestsID == "" {
		d.PullRequestsID = DefaultPullRequestsID
	}
	if d.AssigneeOptionsID == "" {
		d.AssigneeOptionsID = DefaultAssigneeOptionsID
	}
	if d.ReporterOptionsID == "" {
		d.ReporterOptionsID = DefaultReporterOptionsID
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
		"jiraCreateJS": JS,
		"selected":     selected,
	}).Parse(formTmpl + pullRequestFieldTmpl + userOptionsTmpl + resultTmpl + launcherTmpl + dialogTmpl))}
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

// RenderPullRequestField writes only the dependent PR/MR select field.
func (c *Component) RenderPullRequestField(w io.Writer, data FormData) error {
	data.applyDefaults()
	return c.tmpl.ExecuteTemplate(w, "jira-create-pull-request-field-wrapper", data)
}

// PullRequestFieldHTML returns only the dependent PR/MR select field.
func (c *Component) PullRequestFieldHTML(data FormData) (template.HTML, error) {
	var buf bytes.Buffer
	if err := c.RenderPullRequestField(&buf, data); err != nil {
		return "", err
	}
	return template.HTML(buf.String()), nil
}

// RenderUserOptions writes only a datalist of Jira user search results.
func (c *Component) RenderUserOptions(w io.Writer, data UserOptionsData) error {
	if data.OptionsID == "" {
		data.OptionsID = DefaultAssigneeOptionsID
	}
	return c.tmpl.ExecuteTemplate(w, "jira-create-user-options-list", data)
}

// UserOptionsHTML returns only a datalist of Jira user search results.
func (c *Component) UserOptionsHTML(data UserOptionsData) (template.HTML, error) {
	var buf bytes.Buffer
	if err := c.RenderUserOptions(&buf, data); err != nil {
		return "", err
	}
	return template.HTML(buf.String()), nil
}

// ResultHTML returns only the result target as template.HTML.
func (c *Component) ResultHTML(data FormData) (template.HTML, error) {
	var buf bytes.Buffer
	if err := c.RenderResult(&buf, data); err != nil {
		return "", err
	}
	return template.HTML(buf.String()), nil
}

// RenderLauncher writes a self-contained create button launcher for a dialog.
func (c *Component) RenderLauncher(w io.Writer, data LauncherData) error {
	data.applyDefaults()
	return c.tmpl.ExecuteTemplate(w, "jira-create-launcher", data)
}

// Launcher returns a self-contained create button launcher as template.HTML.
func (c *Component) Launcher(data LauncherData) (template.HTML, error) {
	var buf bytes.Buffer
	if err := c.RenderLauncher(&buf, data); err != nil {
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
		PullRequestRepo:   strings.TrimSpace(values.Get("pull_request_repo")),
		PullRequest:       strings.TrimSpace(values.Get("pull_request")),
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

// ScriptTag returns the launcher controller wrapped in a <script> tag.
func ScriptTag() template.HTML {
	return template.HTML("<script>" + componentJS + "</script>")
}

// JS returns the raw launcher controller script.
func JS() template.JS {
	return template.JS(componentJS)
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
	{{if .PullRequestReposErr}}<div class="hx-jira-create-warn">Could not load GitHub repositories: {{.PullRequestReposErr}}</div>{{end}}
	{{if .PullRequestsErr}}<div class="hx-jira-create-warn">Could not load pull requests: {{.PullRequestsErr}}</div>{{end}}
	<form class="hx-jira-create-form" action="{{.Endpoint}}" method="post" hx-post="{{.Endpoint}}" hx-target="#{{.ResultID}}" hx-swap="outerHTML" hx-indicator="#{{.WorkingID}}">
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
			<label class="hx-jira-create-field">Repository
				<select name="pull_request_repo" hx-get="{{.PullRequestsEndpoint}}" hx-trigger="change" hx-target="#{{.PullRequestsID}}" hx-swap="outerHTML" hx-include="this"{{if not .PullRequestRepos}} disabled{{end}}>
					<option value="">None</option>
					{{range .PullRequestRepos}}<option value="{{.FullName}}"{{if selected $.Values.PullRequestRepo .FullName}} selected{{end}}>{{.DisplayLabel}}</option>{{end}}
				</select>
			</label>
			{{template "jira-create-pull-request-field-wrapper" .}}
		</div>
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
				<input name="assignee_search" type="search" list="{{.AssigneeOptionsID}}" autocomplete="off" placeholder="Search users" hx-get="{{.UsersEndpoint}}" hx-trigger="input changed delay:250ms, focus once" hx-target="#{{.AssigneeOptionsID}}" hx-swap="outerHTML" hx-include="[name='project_key'], this" hx-vals='{"field":"assignee"}' data-jira-user-input data-jira-user-target="assignee_account_id">
				<input name="assignee_account_id" type="hidden" value="{{.Values.AssigneeAccountID}}">
				{{template "jira-create-assignee-options" .}}
      </label>
      <label class="hx-jira-create-field">Reporter
				<input name="reporter_search" type="search" list="{{.ReporterOptionsID}}" autocomplete="off" placeholder="Search users" hx-get="{{.UsersEndpoint}}" hx-trigger="input changed delay:250ms, focus once" hx-target="#{{.ReporterOptionsID}}" hx-swap="outerHTML" hx-include="[name='project_key'], this" hx-vals='{"field":"reporter"}' data-jira-user-input data-jira-user-target="reporter_account_id">
				<input name="reporter_account_id" type="hidden" value="{{.Values.ReporterAccountID}}">
				{{template "jira-create-reporter-options" .}}
      </label>
    </div>
    <div class="hx-jira-create-working htmx-indicator" id="{{.WorkingID}}" role="status" aria-live="polite">Creating issue...</div>
    <button type="submit">{{.SubmitLabel}}</button>
  </form>
</div>
{{end}}`

const pullRequestFieldTmpl = `{{define "jira-create-pull-request-field-wrapper"}}
<div id="{{.PullRequestsID}}">
	{{template "jira-create-pull-request-field" .}}
</div>
{{end}}

{{define "jira-create-pull-request-field"}}
<label class="hx-jira-create-field">PR / MR
	<select name="pull_request"{{if not .PullRequests}} disabled{{end}}>
		<option value="">None</option>
		{{range .PullRequests}}<option value="{{.Value}}"{{if selected $.Values.PullRequest .Value}} selected{{end}}>{{.Label}}</option>{{end}}
	</select>
</label>
{{end}}`

const userOptionsTmpl = `{{define "jira-create-assignee-options"}}
<datalist id="{{.AssigneeOptionsID}}">
	{{range .Assignees}}<option value="{{.DisplayName}}" data-account-id="{{.AccountID}}"></option>{{end}}
</datalist>
{{end}}

{{define "jira-create-reporter-options"}}
<datalist id="{{.ReporterOptionsID}}">
	{{range .Assignees}}<option value="{{.DisplayName}}" data-account-id="{{.AccountID}}"></option>{{end}}
</datalist>
{{end}}

{{define "jira-create-user-options-list"}}
<datalist id="{{.OptionsID}}">
	{{range .Users}}<option value="{{.DisplayName}}" data-account-id="{{.AccountID}}"></option>{{end}}
</datalist>
{{end}}`

const resultTmpl = `{{define "jira-create-result"}}
<div class="hx-jira-create-result" id="{{.ResultID}}" aria-live="polite">
  {{if .Result.Err}}<div class="hx-jira-create-warn">{{.Result.Err}}</div>{{end}}
  {{if .Result.Key}}<div class="hx-jira-create-notice">Created <a href="{{.Result.URL}}" target="_blank" rel="noreferrer">{{.Result.Key}}</a></div>{{end}}
</div>
{{end}}`

const launcherTmpl = `{{define "jira-create-launcher"}}
<div class="hx-jira-create-launcher" data-jira-create-launcher>
	{{template "jira-create-launcher-button" .}}
</div>
{{end}}

{{define "jira-create-launcher-button"}}
<a class="hx-jira-create-button" href="{{.Endpoint}}" data-jira-create-open data-jira-create-target="#{{.DialogID}}">{{.ButtonLabel}}</a>
{{end}}`

const dialogTmpl = `{{define "jira-create-dialog"}}
<div class="hx-jira-create-launcher" data-jira-create-launcher>
	{{template "jira-create-launcher-button" .LauncherData}}
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
	{{if not .DisableScript}}
	<script>
	{{jiraCreateJS}}
	</script>
	{{end}}
</div>
{{end}}`

const componentJS = `
(function(){
	if(window.JiraIssueCreate && window.JiraIssueCreate.ready){
		window.JiraIssueCreate.initAll(document);
		return;
	}

	function findDialog(root, opener){
		var target = opener.getAttribute('data-jira-create-target') || root.getAttribute('data-jira-create-target');
		if(target){
			try {
				var externalDialog = document.querySelector(target);
				if(externalDialog){ return externalDialog; }
			} catch(error) {}
		}
		return root.querySelector('[data-jira-create-dialog]');
	}

	function initLauncher(root){
		if(!root || root.dataset.jiraCreateReady){ return; }
		var opener = root.querySelector('[data-jira-create-open]');
		if(!opener){ return; }
		var dialog = findDialog(root, opener);
		if(!dialog || !dialog.showModal){ return; }
		var close = dialog.querySelector('[data-jira-create-close]') || root.querySelector('[data-jira-create-close]');
		root.dataset.jiraCreateReady = "1";
		opener.addEventListener('click', function(event){
			event.preventDefault();
			if(!dialog.open){ dialog.showModal(); }
		});
		if(close){ close.addEventListener('click', function(){ dialog.close(); }); }
		dialog.addEventListener('click', function(event){
			if(event.target === dialog){ dialog.close(); }
		});
	}

	function selectedAccountID(input){
		var listID = input.getAttribute('list');
		var list = listID ? document.getElementById(listID) : null;
		if(!list || !input.value){ return ""; }
		var options = list.querySelectorAll('option');
		for(var i = 0; i < options.length; i++){
			if(options[i].value === input.value){
				return options[i].getAttribute('data-account-id') || "";
			}
		}
		return "";
	}

	function syncUserPicker(input){
		var target = input.getAttribute('data-jira-user-target');
		var form = input.form;
		var hidden = form && target ? form.elements[target] : null;
		if(!hidden){ return; }
		hidden.value = selectedAccountID(input);
	}

	function initUserPickers(scope){
		var inputs = (scope || document).querySelectorAll('[data-jira-user-input]');
		for(var i = 0; i < inputs.length; i++){
			if(inputs[i].dataset.jiraUserReady){ continue; }
			inputs[i].dataset.jiraUserReady = "1";
			inputs[i].addEventListener('input', function(event){ syncUserPicker(event.currentTarget); });
			inputs[i].addEventListener('change', function(event){ syncUserPicker(event.currentTarget); });
		}
	}

	function syncUserPickers(scope){
		var inputs = (scope || document).querySelectorAll('[data-jira-user-input]');
		for(var i = 0; i < inputs.length; i++){ syncUserPicker(inputs[i]); }
	}

	function initAll(scope){
		initUserPickers(scope || document);
		var roots = (scope || document).querySelectorAll('[data-jira-create-launcher]');
		for(var i = 0; i < roots.length; i++){ initLauncher(roots[i]); }
	}

	function closeCreatedDialog(target, detail){
		if(!target || !detail || !detail.successful || !target.closest){ return; }
		var dialog = target.closest('[data-jira-create-dialog]');
		if(!dialog){ return; }
		var link = target.querySelector ? target.querySelector('.hx-jira-create-notice a') : null;
		if(link){ setTimeout(function(){ dialog.close(); }, 700); }
	}

	window.JiraIssueCreate = {
		ready: true,
		init: initLauncher,
		initAll: initAll
	};

	var currentRoot = document.currentScript ? document.currentScript.closest('[data-jira-create-launcher]') : null;
	if(currentRoot){ initLauncher(currentRoot); }

	if(document.readyState === "loading"){
		document.addEventListener('DOMContentLoaded', function(){ initAll(document); });
	} else {
		initAll(document);
	}

	document.addEventListener('htmx:afterSwap', function(event){
		initAll(document);
		syncUserPickers(document);
		closeCreatedDialog(event.target, event.detail);
	});
})();
`

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
