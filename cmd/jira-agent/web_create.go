package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/ccastorena/jira-agent/jiraissueui"
)

type jiraCreatePageData struct {
	CreateStyles template.HTML
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
		CreateForm:   form,
	}, nil
}

func (a *webApp) jiraCreateFormData(values jiraissueui.IssueForm, result jiraissueui.Result) jiraissueui.FormData {
	projects, projectsErr := a.fetchJiraProjects()
	if values.ProjectKey == "" && len(projects) > 0 {
		values.ProjectKey = projects[0].Key
	}
	assignees, assigneesErr := a.fetchJiraAssignableUsers(values.ProjectKey)
	if values.PullRequestRepo == "" && values.PullRequest != "" {
		if ref, err := parsePullRequestReference(values.PullRequest); err == nil {
			values.PullRequestRepo = ref.FullName()
		}
	}
	pullRequestRepos, pullRequestReposErr := a.fetchPullRequestRepos()
	pullRequests, pullRequestsErr := a.fetchPullRequestsForRepo(values.PullRequestRepo)
	return jiraissueui.FormData{
		Endpoint:             "/jira/create",
		PullRequestsEndpoint: "/jira/create/pull-requests",
		Projects:             projects,
		ProjectsErr:          errString(projectsErr),
		Assignees:            assignees,
		AssigneesErr:         errString(assigneesErr),
		PullRequestRepos:     pullRequestRepos,
		PullRequestReposErr:  errString(pullRequestReposErr),
		PullRequests:         pullRequests,
		PullRequestsErr:      errString(pullRequestsErr),
		Values:               values,
		Result:               result,
	}
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
	pullRequests, pullRequestsErr := a.fetchPullRequestsForRepo(values.PullRequestRepo)
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

func jiraCreateStyleTag() template.HTML {
	return jiraissueui.StyleTag()
}

func jiraCreateScriptTag() template.HTML {
	return jiraissueui.ScriptTag()
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
	enriched, err := a.enrichIssueWithPullRequest(form)
	if err != nil {
		return jiraissueui.Result{Err: errString(err)}
	}

	raw, err := a.jc.CreateIssue(createIssueArgsFromForm(enriched))
	if err != nil {
		return jiraissueui.Result{Err: errString(err)}
	}

	key, ok := parseCreatedIssueKey(raw)
	if !ok {
		return jiraissueui.Result{Err: "Jira created the issue but returned an unexpected response"}
	}
	return jiraissueui.Result{Key: key, URL: a.jc.baseURL + "/browse/" + key}
}

type pullRequestRef struct {
	Owner  string
	Repo   string
	Number int
}

func (r pullRequestRef) FullName() string {
	return r.Owner + "/" + r.Repo
}

func (a *webApp) enrichIssueWithPullRequest(form jiraissueui.IssueForm) (jiraissueui.IssueForm, error) {
	if strings.TrimSpace(form.PullRequest) == "" {
		return form, nil
	}
	if a.gc == nil {
		return form, fmt.Errorf("GitHub is not configured (set GITHUB_TOKEN) to attach PR details")
	}

	ref, err := parsePullRequestReference(form.PullRequest)
	if err != nil {
		return form, err
	}
	pull, err := a.gc.GetPull(ref.Owner, ref.Repo, ref.Number)
	if err != nil {
		return form, err
	}
	files, err := a.gc.ListPullFiles(ref.Owner, ref.Repo, ref.Number, 100)
	if err != nil {
		return form, err
	}
	block, err := pullRequestDescriptionBlock(ref, pull, files)
	if err != nil {
		return form, err
	}
	form.Description = appendDescriptionBlock(form.Description, block)
	return form, nil
}

func parsePullRequestReference(raw string) (pullRequestRef, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return pullRequestRef{}, fmt.Errorf("PR / MR reference is required")
	}
	if u, err := url.Parse(value); err == nil && u.Scheme != "" && u.Host != "" {
		if ref, ok := parsePullRequestPath(strings.Trim(u.Path, "/")); ok {
			return ref, nil
		}
	}

	normalized := strings.Trim(value, " /")
	normalized = strings.TrimPrefix(normalized, "https://")
	normalized = strings.TrimPrefix(normalized, "http://")
	parts := strings.Split(normalized, "/")
	if len(parts) >= 5 && strings.Contains(parts[0], ".") {
		normalized = strings.Join(parts[1:], "/")
	}

	for _, sep := range []string{"#", "!"} {
		ownerRepo, numberText, found := strings.Cut(normalized, sep)
		if !found {
			continue
		}
		owner, repo, ok := splitOwnerRepo(ownerRepo)
		if !ok {
			break
		}
		number, ok := positiveInt(numberText)
		if ok {
			return pullRequestRef{Owner: owner, Repo: repo, Number: number}, nil
		}
	}

	if ref, ok := parsePullRequestPath(strings.Trim(normalized, "/")); ok {
		return ref, nil
	}
	return pullRequestRef{}, fmt.Errorf("invalid PR / MR reference %q; use owner/repo#123 or a GitHub pull request URL", raw)
}

func parsePullRequestPath(path string) (pullRequestRef, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 4 && (parts[2] == "pull" || parts[2] == "pulls" || parts[2] == "merge_requests") {
		number, ok := positiveInt(parts[3])
		return pullRequestRef{Owner: parts[0], Repo: parts[1], Number: number}, ok && parts[0] != "" && parts[1] != ""
	}
	if len(parts) == 3 {
		number, ok := positiveInt(parts[2])
		return pullRequestRef{Owner: parts[0], Repo: parts[1], Number: number}, ok && parts[0] != "" && parts[1] != ""
	}
	return pullRequestRef{}, false
}

func splitOwnerRepo(value string) (string, string, bool) {
	parts := strings.Split(strings.Trim(value, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func positiveInt(value string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	return n, err == nil && n > 0
}

func pullRequestDescriptionBlock(ref pullRequestRef, rawPull, rawFiles json.RawMessage) (string, error) {
	var pull struct {
		Number  int    `json:"number"`
		Title   string `json:"title"`
		State   string `json:"state"`
		HTMLURL string `json:"html_url"`
		User    struct {
			Login string `json:"login"`
		} `json:"user"`
		Head struct {
			Ref string `json:"ref"`
		} `json:"head"`
		Base struct {
			Ref string `json:"ref"`
		} `json:"base"`
	}
	if err := json.Unmarshal(rawPull, &pull); err != nil {
		return "", err
	}

	var files []struct {
		Filename  string `json:"filename"`
		Status    string `json:"status"`
		Additions int    `json:"additions"`
		Deletions int    `json:"deletions"`
	}
	if err := json.Unmarshal(rawFiles, &files); err != nil {
		return "", err
	}

	number := pull.Number
	if number == 0 {
		number = ref.Number
	}
	pullURL := pull.HTMLURL
	if pullURL == "" {
		pullURL = fmt.Sprintf("%s#%d", ref.FullName(), number)
	}

	lines := []string{
		"Related pull request",
		fmt.Sprintf("- PR: %s", pullURL),
		fmt.Sprintf("- Repository: %s", ref.FullName()),
		fmt.Sprintf("- Number: #%d", number),
	}
	if pull.Title != "" {
		lines = append(lines, "- Title: "+pull.Title)
	}
	if pull.State != "" {
		lines = append(lines, "- State: "+pull.State)
	}
	if pull.User.Login != "" {
		lines = append(lines, "- Author: "+pull.User.Login)
	}
	if pull.Head.Ref != "" || pull.Base.Ref != "" {
		lines = append(lines, fmt.Sprintf("- Branches: %s -> %s", pull.Head.Ref, pull.Base.Ref))
	}
	lines = append(lines, "", "Changed files")
	if len(files) == 0 {
		lines = append(lines, "- No changed files returned.")
	}
	for _, file := range files {
		status := file.Status
		if status == "" {
			status = "changed"
		}
		lines = append(lines, fmt.Sprintf("- %s (%s, +%d/-%d)", file.Filename, status, file.Additions, file.Deletions))
	}
	return strings.Join(lines, "\n"), nil
}

func appendDescriptionBlock(description, block string) string {
	description = strings.TrimSpace(description)
	if description == "" {
		return block
	}
	return description + "\n\n" + block
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

func parseCreatedIssueKey(raw json.RawMessage) (string, bool) {
	var created struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(raw, &created); err != nil || created.Key == "" {
		return "", false
	}
	return created.Key, true
}
