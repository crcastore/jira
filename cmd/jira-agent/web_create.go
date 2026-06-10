package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

type jiraCreatePageData struct {
	Projects        []jiraProjectItem
	ProjectsErr     string
	SelectedProject string
	Assignees       []jiraUserItem
	AssigneesErr    string
	Result          jiraCreateResult
}

type jiraCreateResult struct {
	Key string
	URL string
	Err string
}

type jiraCreateForm struct {
	ProjectKey        string
	Summary           string
	IssueType         string
	Description       string
	Priority          string
	Labels            []string
	AssigneeAccountID string
	ReporterAccountID string
}

func (a *webApp) handleJiraCreatePage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	data := a.newJiraCreatePageData(r)
	if r.Method == http.MethodPost {
		data.Result = a.createJiraIssueFromRequest(r)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = createIssuePageTmpl.Execute(w, data)
}

func (a *webApp) newJiraCreatePageData(r *http.Request) jiraCreatePageData {
	projects, projectsErr := a.fetchJiraProjects()
	selectedProject := strings.ToUpper(strings.TrimSpace(r.FormValue("project_key")))
	if selectedProject == "" && len(projects) > 0 {
		selectedProject = projects[0].Key
	}

	assignees, assigneesErr := a.fetchJiraAssignableUsers(selectedProject)
	return jiraCreatePageData{
		Projects:        projects,
		ProjectsErr:     errString(projectsErr),
		SelectedProject: selectedProject,
		Assignees:       assignees,
		AssigneesErr:    errString(assigneesErr),
	}
}

func (a *webApp) createJiraIssueFromRequest(r *http.Request) jiraCreateResult {
	form, formErr := parseJiraCreateForm(r)
	if formErr != "" {
		return jiraCreateResult{Err: formErr}
	}

	raw, err := a.jc.CreateIssue(form.createArgs())
	if err != nil {
		return jiraCreateResult{Err: errString(err)}
	}

	key, ok := parseCreatedIssueKey(raw)
	if !ok {
		return jiraCreateResult{Err: "Jira created the issue but returned an unexpected response"}
	}
	return jiraCreateResult{Key: key, URL: a.jc.baseURL + "/browse/" + key}
}

func parseJiraCreateForm(r *http.Request) (jiraCreateForm, string) {
	if err := r.ParseForm(); err != nil {
		return jiraCreateForm{}, "Invalid form submission"
	}

	form := jiraCreateForm{
		ProjectKey:        strings.ToUpper(strings.TrimSpace(r.FormValue("project_key"))),
		Summary:           strings.TrimSpace(r.FormValue("summary")),
		IssueType:         strings.TrimSpace(r.FormValue("issue_type")),
		Description:       strings.TrimSpace(r.FormValue("description")),
		Priority:          strings.TrimSpace(r.FormValue("priority")),
		Labels:            splitCSV(r.FormValue("labels")),
		AssigneeAccountID: strings.TrimSpace(r.FormValue("assignee_account_id")),
		ReporterAccountID: strings.TrimSpace(r.FormValue("reporter_account_id")),
	}
	if form.ProjectKey == "" || form.Summary == "" {
		return jiraCreateForm{}, "Project and summary are required"
	}
	return form, ""
}

func (f jiraCreateForm) createArgs() CreateIssueArgs {
	return CreateIssueArgs{
		ProjectKey:        f.ProjectKey,
		Summary:           f.Summary,
		IssueType:         f.IssueType,
		Description:       f.Description,
		Priority:          f.Priority,
		Labels:            f.Labels,
		AssigneeAccountID: f.AssigneeAccountID,
		ReporterAccountID: f.ReporterAccountID,
	}
}

func parseCreatedIssueKey(raw json.RawMessage) (string, bool) {
	var created struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(raw, &created); err != nil || created.Key == "" {
		return "", false
	}
	return created.Key, true
}
