// Package githubpr adapts GitHub pull request data for the Jira issue create UI.
//
// It is intentionally independent from this app's webApp type. To reuse the
// repository + PR/MR picker elsewhere, provide any client that implements the
// Client interface and pass the returned jiraissueui options into the form.
package githubpr

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/ccastorena/jira-agent/jiraissueui"
)

// Client is the small GitHub API surface needed by the repository/PR picker and
// Jira description enrichment. cmd/jira-agent.GitHubClient satisfies it.
type Client interface {
	ListMyRepos(visibility, sort string, maxTotal int) (json.RawMessage, error)
	ListPulls(owner, repo, state string, perPage int) (json.RawMessage, error)
	GetPull(owner, repo string, number int) (json.RawMessage, error)
	ListPullFiles(owner, repo string, number, perPage int) (json.RawMessage, error)
}

// Picker loads repository and PR/MR options and enriches submitted Jira issue
// forms with the selected PR/MR metadata and changed files.
type Picker struct {
	Client Client
}

func NewPicker(client Client) Picker {
	return Picker{Client: client}
}

// Reference identifies a pull request / merge request.
type Reference struct {
	Owner  string
	Repo   string
	Number int
}

func (r Reference) FullName() string {
	return r.Owner + "/" + r.Repo
}

func (r Reference) Value() string {
	return fmt.Sprintf("%s#%d", r.FullName(), r.Number)
}

// PullRequest is the subset of GitHub pull metadata appended to Jira issues.
type PullRequest struct {
	Number  int
	Title   string
	State   string
	HTMLURL string
	Author  string
	HeadRef string
	BaseRef string
}

// ChangedFile is the subset of GitHub changed-file metadata appended to Jira issues.
type ChangedFile struct {
	Filename  string
	Status    string
	Additions int
	Deletions int
}

// Details contains a selected PR/MR and its changed files.
type Details struct {
	Reference Reference
	Pull      PullRequest
	Files     []ChangedFile
}

// Repositories returns GitHub repositories in the shape expected by jiraissueui.
func (p Picker) Repositories() ([]jiraissueui.PullRequestRepo, error) {
	if p.Client == nil {
		return nil, nil
	}
	raw, err := p.Client.ListMyRepos("all", "pushed", 150)
	if err != nil {
		return nil, err
	}
	var payload []struct {
		FullName string `json:"full_name"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	items := make([]jiraissueui.PullRequestRepo, 0, len(payload))
	for _, repo := range payload {
		if repo.FullName == "" {
			continue
		}
		items = append(items, jiraissueui.PullRequestRepo{FullName: repo.FullName})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].FullName < items[j].FullName })
	return items, nil
}

// PullRequests returns all-state PR/MR options for a selected repository.
func (p Picker) PullRequests(fullName string) ([]jiraissueui.PullRequestOption, error) {
	fullName = strings.TrimSpace(fullName)
	if fullName == "" {
		return nil, nil
	}
	if p.Client == nil {
		return nil, fmt.Errorf("GITHUB_TOKEN not set")
	}
	owner, repo, ok := SplitOwnerRepo(fullName)
	if !ok {
		return nil, fmt.Errorf("invalid repository %q", fullName)
	}
	raw, err := p.Client.ListPulls(owner, repo, "all", 100)
	if err != nil {
		return nil, err
	}
	var payload []pullOptionPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	items := make([]jiraissueui.PullRequestOption, 0, len(payload))
	for _, pr := range payload {
		if pr.Number <= 0 {
			continue
		}
		items = append(items, jiraissueui.PullRequestOption{
			Value: fmt.Sprintf("%s#%d", fullName, pr.Number),
			Label: pr.OptionLabel(),
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Value > items[j].Value })
	return items, nil
}

// Details loads the selected PR/MR plus its changed files.
func (p Picker) Details(rawReference string) (Details, error) {
	if p.Client == nil {
		return Details{}, fmt.Errorf("GitHub is not configured (set GITHUB_TOKEN) to attach PR details")
	}
	ref, err := ParseReference(rawReference)
	if err != nil {
		return Details{}, err
	}
	pull, err := p.Client.GetPull(ref.Owner, ref.Repo, ref.Number)
	if err != nil {
		return Details{}, err
	}
	files, err := p.Client.ListPullFiles(ref.Owner, ref.Repo, ref.Number, 100)
	if err != nil {
		return Details{}, err
	}
	return detailsFromRaw(ref, pull, files)
}

// EnrichIssue appends selected PR/MR metadata and changed files to a Jira issue form.
func (p Picker) EnrichIssue(form jiraissueui.IssueForm) (jiraissueui.IssueForm, error) {
	if strings.TrimSpace(form.PullRequest) == "" {
		return form, nil
	}
	details, err := p.Details(form.PullRequest)
	if err != nil {
		return form, err
	}
	if form.PullRequestRepo == "" {
		form.PullRequestRepo = details.Reference.FullName()
	}
	form.Description = appendDescriptionBlock(form.Description, details.DescriptionBlock())
	return form, nil
}

func ParseReference(raw string) (Reference, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return Reference{}, fmt.Errorf("PR / MR reference is required")
	}
	if u, err := url.Parse(value); err == nil && u.Scheme != "" && u.Host != "" {
		if ref, ok := parsePath(strings.Trim(u.Path, "/")); ok {
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
		owner, repo, ok := SplitOwnerRepo(ownerRepo)
		if !ok {
			break
		}
		number, ok := positiveInt(numberText)
		if ok {
			return Reference{Owner: owner, Repo: repo, Number: number}, nil
		}
	}

	if ref, ok := parsePath(strings.Trim(normalized, "/")); ok {
		return ref, nil
	}
	return Reference{}, fmt.Errorf("invalid PR / MR reference %q; use owner/repo#123 or a GitHub pull request URL", raw)
}

func SplitOwnerRepo(value string) (string, string, bool) {
	parts := strings.Split(strings.Trim(value, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func (d Details) DescriptionBlock() string {
	number := d.Pull.Number
	if number == 0 {
		number = d.Reference.Number
	}
	pullURL := d.Pull.HTMLURL
	if pullURL == "" {
		pullURL = d.Reference.Value()
	}

	lines := []string{
		"Related pull request",
		fmt.Sprintf("- PR: %s", pullURL),
		fmt.Sprintf("- Repository: %s", d.Reference.FullName()),
		fmt.Sprintf("- Number: #%d", number),
	}
	if d.Pull.Title != "" {
		lines = append(lines, "- Title: "+d.Pull.Title)
	}
	if d.Pull.State != "" {
		lines = append(lines, "- State: "+d.Pull.State)
	}
	if d.Pull.Author != "" {
		lines = append(lines, "- Author: "+d.Pull.Author)
	}
	if d.Pull.HeadRef != "" || d.Pull.BaseRef != "" {
		lines = append(lines, fmt.Sprintf("- Branches: %s -> %s", d.Pull.HeadRef, d.Pull.BaseRef))
	}
	lines = append(lines, "", "Changed files")
	if len(d.Files) == 0 {
		lines = append(lines, "- No changed files returned.")
	}
	for _, file := range d.Files {
		status := file.Status
		if status == "" {
			status = "changed"
		}
		lines = append(lines, fmt.Sprintf("- %s (%s, +%d/-%d)", file.Filename, status, file.Additions, file.Deletions))
	}
	return strings.Join(lines, "\n")
}

type pullOptionPayload struct {
	Number   int    `json:"number"`
	Title    string `json:"title"`
	State    string `json:"state"`
	MergedAt string `json:"merged_at"`
	Head     struct {
		Ref string `json:"ref"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
}

func (pr pullOptionPayload) OptionLabel() string {
	label := fmt.Sprintf("#%d", pr.Number)
	if pr.Title != "" {
		label += " " + pr.Title
	}
	if pr.MergedAt != "" {
		label += " [merged]"
	} else if pr.State != "" && pr.State != "open" {
		label += " [" + pr.State + "]"
	}
	if pr.Head.Ref != "" || pr.Base.Ref != "" {
		label += fmt.Sprintf(" (%s -> %s)", pr.Head.Ref, pr.Base.Ref)
	}
	return label
}

func detailsFromRaw(ref Reference, rawPull, rawFiles json.RawMessage) (Details, error) {
	var pullPayload struct {
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
	if err := json.Unmarshal(rawPull, &pullPayload); err != nil {
		return Details{}, err
	}

	var filePayload []ChangedFile
	if err := json.Unmarshal(rawFiles, &filePayload); err != nil {
		return Details{}, err
	}

	return Details{
		Reference: ref,
		Pull: PullRequest{
			Number:  pullPayload.Number,
			Title:   pullPayload.Title,
			State:   pullPayload.State,
			HTMLURL: pullPayload.HTMLURL,
			Author:  pullPayload.User.Login,
			HeadRef: pullPayload.Head.Ref,
			BaseRef: pullPayload.Base.Ref,
		},
		Files: filePayload,
	}, nil
}

func parsePath(path string) (Reference, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 4 && (parts[2] == "pull" || parts[2] == "pulls" || parts[2] == "merge_requests") {
		number, ok := positiveInt(parts[3])
		return Reference{Owner: parts[0], Repo: parts[1], Number: number}, ok && parts[0] != "" && parts[1] != ""
	}
	if len(parts) == 3 {
		number, ok := positiveInt(parts[2])
		return Reference{Owner: parts[0], Repo: parts[1], Number: number}, ok && parts[0] != "" && parts[1] != ""
	}
	return Reference{}, false
}

func positiveInt(value string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	return n, err == nil && n > 0
}

func appendDescriptionBlock(description, block string) string {
	description = strings.TrimSpace(description)
	if description == "" {
		return block
	}
	return description + "\n\n" + block
}
