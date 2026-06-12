package githubpr

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ccastorena/jira-agent/jiraissueui"
)

type fakeClient struct {
	repos json.RawMessage
	pulls json.RawMessage
	pull  json.RawMessage
	files json.RawMessage

	listReposVisibility string
	listReposSort       string
	listReposMax        int
	listPullsOwner      string
	listPullsRepo       string
	listPullsState      string
	listPullsPerPage    int
	getPullOwner        string
	getPullRepo         string
	getPullNumber       int
	listFilesOwner      string
	listFilesRepo       string
	listFilesNumber     int
	listFilesPerPage    int
}

func (f *fakeClient) ListMyRepos(visibility, sort string, maxTotal int) (json.RawMessage, error) {
	f.listReposVisibility = visibility
	f.listReposSort = sort
	f.listReposMax = maxTotal
	return f.repos, nil
}

func (f *fakeClient) ListPulls(owner, repo, state string, perPage int) (json.RawMessage, error) {
	f.listPullsOwner = owner
	f.listPullsRepo = repo
	f.listPullsState = state
	f.listPullsPerPage = perPage
	return f.pulls, nil
}

func (f *fakeClient) GetPull(owner, repo string, number int) (json.RawMessage, error) {
	f.getPullOwner = owner
	f.getPullRepo = repo
	f.getPullNumber = number
	return f.pull, nil
}

func (f *fakeClient) ListPullFiles(owner, repo string, number, perPage int) (json.RawMessage, error) {
	f.listFilesOwner = owner
	f.listFilesRepo = repo
	f.listFilesNumber = number
	f.listFilesPerPage = perPage
	return f.files, nil
}

func TestPickerRepositoriesMapsAndSortsRepoOptions(t *testing.T) {
	client := &fakeClient{repos: json.RawMessage(`[
		{"full_name":"zeta/app"},
		{"full_name":""},
		{"full_name":"alpha/api"}
	]`)}

	repos, err := NewPicker(client).Repositories()
	if err != nil {
		t.Fatalf("Repositories returned error: %v", err)
	}

	if client.listReposVisibility != "all" || client.listReposSort != "pushed" || client.listReposMax != 150 {
		t.Fatalf("ListMyRepos args = visibility=%q sort=%q max=%d", client.listReposVisibility, client.listReposSort, client.listReposMax)
	}
	if len(repos) != 2 || repos[0].FullName != "alpha/api" || repos[1].FullName != "zeta/app" {
		t.Fatalf("repositories = %#v", repos)
	}
}

func TestPickerPullRequestsMapsAllStateOptions(t *testing.T) {
	client := &fakeClient{pulls: json.RawMessage(`[
		{"number":12,"title":"Add login fix","state":"open","head":{"ref":"fix-login"},"base":{"ref":"main"}},
		{"number":7,"title":"Clean up docs","state":"closed","merged_at":"2026-06-10T12:00:00Z","head":{"ref":"docs"},"base":{"ref":"main"}},
		{"number":0,"title":"skip me"}
	]`)}

	options, err := NewPicker(client).PullRequests("octo/hello")
	if err != nil {
		t.Fatalf("PullRequests returned error: %v", err)
	}

	if client.listPullsOwner != "octo" || client.listPullsRepo != "hello" || client.listPullsState != "all" || client.listPullsPerPage != 100 {
		t.Fatalf("ListPulls args = owner=%q repo=%q state=%q per_page=%d", client.listPullsOwner, client.listPullsRepo, client.listPullsState, client.listPullsPerPage)
	}
	labels := optionLabels(options)
	if labels["octo/hello#12"] != "#12 Add login fix (fix-login -> main)" {
		t.Fatalf("#12 option label = %q", labels["octo/hello#12"])
	}
	if labels["octo/hello#7"] != "#7 Clean up docs [merged] (docs -> main)" {
		t.Fatalf("#7 option label = %q", labels["octo/hello#7"])
	}
}

func TestParseReference(t *testing.T) {
	cases := map[string]Reference{
		"https://github.com/octo/hello/pull/12": {Owner: "octo", Repo: "hello", Number: 12},
		"github.com/octo/hello/pull/12":         {Owner: "octo", Repo: "hello", Number: 12},
		"octo/hello#12":                         {Owner: "octo", Repo: "hello", Number: 12},
		"octo/hello!12":                         {Owner: "octo", Repo: "hello", Number: 12},
		"octo/hello/pull/12":                    {Owner: "octo", Repo: "hello", Number: 12},
		"octo/hello/12":                         {Owner: "octo", Repo: "hello", Number: 12},
	}
	for input, want := range cases {
		got, err := ParseReference(input)
		if err != nil {
			t.Fatalf("ParseReference(%q) returned error: %v", input, err)
		}
		if got != want {
			t.Fatalf("ParseReference(%q) = %+v, want %+v", input, got, want)
		}
	}
	for _, input := range []string{"", "octo/hello", "octo/hello#nope", "https://github.com/octo/hello/issues/12"} {
		if got, err := ParseReference(input); err == nil {
			t.Fatalf("ParseReference(%q) = %+v, want error", input, got)
		}
	}
}

func TestPickerEnrichIssueAppendsPullRequestDetailsAndChangedFiles(t *testing.T) {
	client := &fakeClient{
		pull: json.RawMessage(`{
			"number": 12,
			"title": "Add login fix",
			"state": "open",
			"html_url": "https://github.com/octo/hello/pull/12",
			"user": {"login": "octocat"},
			"head": {"ref": "fix-login"},
			"base": {"ref": "main"}
		}`),
		files: json.RawMessage(`[
			{"filename":"cmd/main.go","status":"modified","additions":12,"deletions":3},
			{"filename":"README.md","status":"added","additions":5,"deletions":0}
		]`),
	}
	form := jiraissueui.IssueForm{
		Description: "Button fails",
		PullRequest: "octo/hello#12",
	}

	enriched, err := NewPicker(client).EnrichIssue(form)
	if err != nil {
		t.Fatalf("EnrichIssue returned error: %v", err)
	}

	if client.getPullOwner != "octo" || client.getPullRepo != "hello" || client.getPullNumber != 12 {
		t.Fatalf("GetPull args = owner=%q repo=%q number=%d", client.getPullOwner, client.getPullRepo, client.getPullNumber)
	}
	if client.listFilesOwner != "octo" || client.listFilesRepo != "hello" || client.listFilesNumber != 12 || client.listFilesPerPage != 100 {
		t.Fatalf("ListPullFiles args = owner=%q repo=%q number=%d per_page=%d", client.listFilesOwner, client.listFilesRepo, client.listFilesNumber, client.listFilesPerPage)
	}
	if enriched.PullRequestRepo != "octo/hello" {
		t.Fatalf("PullRequestRepo = %q", enriched.PullRequestRepo)
	}
	for _, want := range []string{
		"Button fails",
		"Related pull request",
		"- PR: https://github.com/octo/hello/pull/12",
		"- Repository: octo/hello",
		"- Number: #12",
		"- Title: Add login fix",
		"- Branches: fix-login -> main",
		"Changed files",
		"- cmd/main.go (modified, +12/-3)",
		"- README.md (added, +5/-0)",
	} {
		if !strings.Contains(enriched.Description, want) {
			t.Fatalf("description missing %q:\n%s", want, enriched.Description)
		}
	}
}

func optionLabels(options []jiraissueui.PullRequestOption) map[string]string {
	labels := make(map[string]string, len(options))
	for _, option := range options {
		labels[option.Value] = option.Label
	}
	return labels
}
