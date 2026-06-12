# Jira + GitHub Agent (Go)

A Go CLI and HTMX web app for working with Jira Cloud and a focused GitHub issue workflow through an OpenAI-compatible tool-calling model.

The current model-visible tool set is intentionally small: GitHub identity, repository discovery, issue listing, issue lookup, pull request / merge request (MR) changed-file inspection, issue creation, issue closing, and issue comments. The removed Jira, pull request write, search, and workflow schemas are archived in [REMOVED_TOOLS.md](REMOVED_TOOLS.md) and can be restored in [cmd/jira-agent/tools.go](cmd/jira-agent/tools.go).

## Build & Run

```bash
go mod tidy
go run ./cmd/jira-agent

# or
go build -o jira-agent ./cmd/jira-agent
./jira-agent
```

## Web UI

```bash
go run ./cmd/jira-agent serve
```

Then open <http://localhost:8080>.

The page includes:

- A chat box backed by the shared agent engine
- A model picker populated from Ollama's `/api/tags` when available
- Available GitHub repositories when `GITHUB_TOKEN` is configured
- Your open Jira issues from `assignee = currentUser() AND statusCategory != Done`

## Configuration

Copy `.env.example` to `.env` and fill it in, or export the same variables in your shell.

```env
JIRA_BASE_URL=https://your-domain.atlassian.net
JIRA_EMAIL=you@example.com
JIRA_API_TOKEN=...

# Required for the exposed gh_* chat tools.
GITHUB_TOKEN=ghp_...
# GITHUB_API_URL=https://github.example.com/api/v3   # GHES only

# Defaults to local Ollama.
LLM_BASE_URL=http://localhost:11434/v1
LLM_API_KEY=ollama
LLM_MODEL=llama3.1:8b
WEB_LLM_TIMEOUT_SEC=180

# Optional web settings.
WEB_ADDR=:8080
WEB_MAX_CONTEXT_TOKENS=4000
```

Get a Jira API token at <https://id.atlassian.com/manage-profile/security/api-tokens>.

### Ollama

```bash
ollama serve
ollama pull llama3.1:8b
```

Use a model that supports tool calling. Local model quality varies; `llama3.1:8b` and `qwen2.5` are good starting points.

## Example Prompts

- `who am I on GitHub?`
- `list my repos sorted by recently pushed`
- `show open issues in owner/repo`
- `show issue #17 in owner/repo`
- `show files changed in MR #12 in owner/repo`
- Create a Jira issue from the web form, choose a GitHub repository, then choose an available **PR / MR** to append PR details and changed files to the issue description. Issue types are loaded from the selected Jira project. Add names in **Subtask names** to create one child subtask per name. See [JIRA_CREATE_EXTRACTION.md](JIRA_CREATE_EXTRACTION.md) for how to reuse this flow elsewhere.
- `create an issue in owner/repo titled "Fix flaky login test" with label bug`
- `comment on issue #17 in owner/repo saying "looking into this today"`
- `close issue #17 in owner/repo`

## Exposed Tools

| Tool | Purpose |
| --- | --- |
| `gh_me` | Authenticated GitHub user |
| `gh_list_my_repos` | List repositories for the authenticated user |
| `gh_get_repo` | Read repository metadata |
| `gh_list_issues` | List repository issues, including PRs returned by the issues API |
| `gh_get_issue` | Read one issue or PR by number |
| `gh_list_pr_files` | List files changed in a pull request / merge request (MR) |
| `gh_create_issue` | Open a GitHub issue |
| `gh_close_issue` | Close a GitHub issue |
| `gh_comment_issue` | Comment on an issue or PR |

## Project Layout

| Path | Purpose |
| --- | --- |
| [cmd/jira-agent/main.go](cmd/jira-agent/main.go) | CLI entry point and system prompt |
| [cmd/jira-agent/agent.go](cmd/jira-agent/agent.go) | Shared env, LLM, and engine wiring |
| [cmd/jira-agent/tools.go](cmd/jira-agent/tools.go) | Model-visible tool schemas and dispatcher |
| [cmd/jira-agent/web_server.go](cmd/jira-agent/web_server.go) | HTTP route wiring |
| [cmd/jira-agent/web_templates.go](cmd/jira-agent/web_templates.go) | Dashboard shell and side-panel templates |
| [cmd/jira-agent/jira.go](cmd/jira-agent/jira.go) | Jira Cloud REST v3 client |
| [cmd/jira-agent/github.go](cmd/jira-agent/github.go) | GitHub REST v3 client |
| [agentcore/](agentcore/) | Transport-agnostic chat service and model catalog |
| [chat/](chat/) | Tool-calling chat engine and session store |
| [chathttp/](chathttp/) | Reusable HTMX chat, reset, and token-limit handlers |
| [chatui/](chatui/) | Drop-in HTMX chat widget |
| [githubpr/](githubpr/) | Reusable GitHub repository + PR/MR picker and changed-file enrichment for Jira issue forms |
| [jiraissueui/](jiraissueui/) | Drop-in HTMX Jira issue create button, dialog, form, and parser. See [JIRA_ISSUE_UI.md](JIRA_ISSUE_UI.md) and [JIRA_CREATE_EXTRACTION.md](JIRA_CREATE_EXTRACTION.md). |

## Test

```bash
go test ./...
```

## Notes

- Credentials live in `.env`, which is gitignored.
- The app uses Jira REST API v3 for Jira Cloud. Jira credentials are still needed for the current dashboard panels.
- `CallTool` still contains the removed dispatch paths for easy restoration, but only schemas in `ToolSchemas` are advertised to the model.
