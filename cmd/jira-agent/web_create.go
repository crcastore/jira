package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/ccastorena/jira-agent/githubpr"
	"github.com/ccastorena/jira-agent/jiraissueui"
)

type jiraCreatePageData struct {
	CreateStyles template.HTML
	CreateScript template.HTML
	CreateForm   template.HTML
}

func (a *webApp) handleJiraCreatePage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.Method == http.MethodPost && isHTMXRequest(r) {
		_, result := a.createJiraIssueFromRequest(r)
		if result.Err == "" {
			w.Header().Set("HX-Trigger", "jiraIssueCreated")
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = a.jiraCreateComponent().RenderResult(w, jiraissueui.FormData{Result: result})
		return
	}

	data, err := a.newJiraCreatePageData(r)
	if err != nil {
		http.Error(w, "create issue form unavailable", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = createIssuePageTmpl.Execute(w, data)
}

func (a *webApp) newJiraCreatePageData(r *http.Request) (jiraCreatePageData, error) {
	values := jiraissueui.IssueForm{ProjectKey: selectedJiraProject(r)}
	result := jiraissueui.Result{}
	if r.Method == http.MethodPost {
		values, result = a.createJiraIssueFromRequest(r)
	}

	formData := a.jiraCreateFormData(values, result)
	form, err := a.jiraCreateComponent().Form(formData)
	if err != nil {
		return jiraCreatePageData{}, err
	}

	return jiraCreatePageData{
		CreateStyles: jiraissueui.StyleTag(),
		CreateScript: jiraissueui.ScriptTag(),
		CreateForm:   form,
	}, nil
}

func (a *webApp) jiraCreateFormData(values jiraissueui.IssueForm, result jiraissueui.Result) jiraissueui.FormData {
	projects, projectsErr := a.fetchJiraProjects()
	if values.ProjectKey == "" && len(projects) > 0 {
		values.ProjectKey = projects[0].Key
	}
	issueTypes, issueTypesErr := a.fetchJiraIssueTypes(values.ProjectKey)
	parentIssueTypes := parentIssueTypeNames(issueTypes)
	if len(parentIssueTypes) > 0 {
		values.IssueType = validParentIssueType(values.IssueType, issueTypes)
	}
	assignees, assigneesErr := a.fetchJiraAssignableUsers(values.ProjectKey, "")
	if values.PullRequestRepo == "" && values.PullRequest != "" {
		if ref, err := githubpr.ParseReference(values.PullRequest); err == nil {
			values.PullRequestRepo = ref.FullName()
		}
	}
	picker := a.pullRequestPicker()
	pullRequestRepos, pullRequestReposErr := picker.Repositories()
	pullRequests, pullRequestsErr := picker.PullRequests(values.PullRequestRepo)
	return jiraissueui.FormData{
		Endpoint:             "/jira/create",
		PullRequestsEndpoint: "/jira/create/pull-requests",
		UsersEndpoint:        "/jira/create/users",
		Projects:             projects,
		Assignees:            assignees,
		AssigneesErr:         errString(assigneesErr),
		IssueTypesErr:        errString(issueTypesErr),
		PullRequestRepos:     pullRequestRepos,
		PullRequestReposErr:  errString(pullRequestReposErr),
		PullRequests:         pullRequests,
		PullRequestsErr:      errString(pullRequestsErr),
		IssueTypes:           parentIssueTypes,
		ProjectsErr:          errString(projectsErr),
		Values:               values,
		Result:               result,
	}
}

func (a *webApp) handleJiraCreateUsers(w http.ResponseWriter, r *http.Request) {
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

	users, err := a.fetchJiraAssignableUsers(selectedJiraProject(r), query)
	if err != nil {
		http.Error(w, errString(err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = a.jiraCreateComponent().RenderUserOptions(w, jiraissueui.UserOptionsData{
		OptionsID: optionsID,
		Users:     users,
	})
}

func (a *webApp) handleJiraCreatePullRequests(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	values := jiraissueui.IssueForm{
		PullRequestRepo: strings.TrimSpace(r.URL.Query().Get("pull_request_repo")),
		PullRequest:     strings.TrimSpace(r.URL.Query().Get("pull_request")),
	}
	pullRequests, pullRequestsErr := a.pullRequestPicker().PullRequests(values.PullRequestRepo)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = a.jiraCreateComponent().RenderPullRequestField(w, jiraissueui.FormData{
		PullRequestsEndpoint: "/jira/create/pull-requests",
		PullRequests:         pullRequests,
		PullRequestsErr:      errString(pullRequestsErr),
		Values:               values,
	})
}

func (a *webApp) jiraCreateDialog() template.HTML {
	if a.jc == nil {
		return jiraCreateFallbackLink()
	}
	dialog, err := a.jiraCreateComponent().Dialog(jiraissueui.DialogData{
		ButtonLabel:   "Create",
		Title:         "Create Jira Issue",
		Form:          a.jiraCreateFormData(jiraissueui.IssueForm{}, jiraissueui.Result{}),
		DisableScript: true,
	})
	if err != nil {
		return jiraCreateFallbackLink()
	}
	return dialog
}

func jiraCreateFallbackLink() template.HTML {
	return template.HTML(`<a class="nav-tab" href="/jira/create">Create</a>`)
}

func (a *webApp) jiraCreateComponent() *jiraissueui.Component {
	if a.jiraCreateUI != nil {
		return a.jiraCreateUI
	}
	return jiraissueui.New()
}

func selectedJiraProject(r *http.Request) string {
	return strings.ToUpper(strings.TrimSpace(r.FormValue("project_key")))
}

func isHTMXRequest(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

func (a *webApp) createJiraIssueFromRequest(r *http.Request) (jiraissueui.IssueForm, jiraissueui.Result) {
	form, err := jiraissueui.ParseRequest(r)
	if err != nil {
		return form, jiraissueui.Result{Err: err.Error()}
	}
	return form, a.createJiraIssue(form)
}

func (a *webApp) createJiraIssue(form jiraissueui.IssueForm) jiraissueui.Result {
	enriched, err := a.pullRequestPicker().EnrichIssue(form)
	if err != nil {
		return jiraissueui.Result{Err: errString(err)}
	}
	issueTypes, issueTypesErr := a.fetchJiraIssueTypes(enriched.ProjectKey)
	if issueTypesErr == nil && len(issueTypes) > 0 {
		enriched.IssueType = validParentIssueType(enriched.IssueType, issueTypes)
		if enriched.IssueType == "" {
			return jiraissueui.Result{Err: "Jira project has no creatable parent issue types"}
		}
	}
	subtaskIssueType := "Sub-task"
	if len(enriched.SubtaskNames) > 0 && issueTypesErr == nil && len(issueTypes) > 0 {
		subtaskIssueType = firstSubtaskIssueTypeName(issueTypes)
		if subtaskIssueType == "" {
			return jiraissueui.Result{Err: "Jira project has no subtask issue type enabled"}
		}
	}

	raw, err := a.jc.CreateIssue(createIssueArgsFromForm(enriched))
	if err != nil {
		return jiraissueui.Result{Err: errString(err)}
	}

	created, ok := parseCreatedIssue(raw)
	if !ok {
		return jiraissueui.Result{Err: "Jira created the issue but returned an unexpected response"}
	}
	key := created.Key
	result := jiraissueui.Result{Key: key, URL: a.jc.baseURL + "/browse/" + key}
	for _, name := range enriched.SubtaskNames {
		subtaskRaw, err := a.jc.CreateIssue(createSubtaskArgsFromForm(enriched, created, name, subtaskIssueType))
		if err != nil {
			result.Err = fmt.Sprintf("created %s but failed to create subtask for %s: %v", key, name, err)
			return result
		}
		subtask, ok := parseCreatedIssue(subtaskRaw)
		if !ok {
			result.Err = fmt.Sprintf("created %s but Jira returned an unexpected response for subtask %s", key, name)
			return result
		}
		result.Subtasks = append(result.Subtasks, jiraissueui.IssueLink{
			Key: subtask.Key,
			URL: a.jc.baseURL + "/browse/" + subtask.Key,
		})
	}
	return result
}

func createIssueArgsFromForm(form jiraissueui.IssueForm) CreateIssueArgs {
	return CreateIssueArgs{
		ProjectKey:        form.ProjectKey,
		Summary:           form.Summary,
		IssueType:         form.IssueType,
		Description:       form.Description,
		Priority:          form.Priority,
		Labels:            form.Labels,
		AssigneeAccountID: form.AssigneeAccountID,
		ReporterAccountID: form.ReporterAccountID,
	}
}

func createSubtaskArgsFromForm(form jiraissueui.IssueForm, parent createdIssue, name, issueType string) CreateIssueArgs {
	args := createIssueArgsFromForm(form)
	args.IssueType = issueType
	args.ParentID = parent.ID
	args.ParentKey = parent.Key
	args.Summary = fmt.Sprintf("%s - %s", form.Summary, name)
	return args
}

type createdIssue struct {
	ID  string `json:"id"`
	Key string `json:"key"`
}

func parseCreatedIssue(raw json.RawMessage) (createdIssue, bool) {
	var created createdIssue
	if err := json.Unmarshal(raw, &created); err != nil || created.Key == "" {
		return createdIssue{}, false
	}
	return created, true
}

func parseCreatedIssueKey(raw json.RawMessage) (string, bool) {
	created, ok := parseCreatedIssue(raw)
	if !ok {
		return "", false
	}
	return created.Key, true
}
