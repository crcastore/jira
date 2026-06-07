# Jira + GitHub Agent (Go)

CLI agent that talks to **Jira Cloud** and **GitHub** in natural language. The
LLM is any OpenAI-compatible endpoint, so you can plug in:

- **Ollama** (local, free) — e.g. `llama3.1`, `qwen2.5` (needs a tool-calling model)
- **OpenAI** (`gpt-4o-mini`, etc.)
- **Groq**, **OpenRouter**, **Together**, or anything else exposing `/v1/chat/completions` with tool calling

## Build & run

```bash
go mod tidy
go run ./cmd/jira-agent
# or
go build -o jira-agent ./cmd/jira-agent && ./jira-agent
```

## Web UI (Go + HTMX)

```bash
go run ./cmd/jira-agent serve
```

Then open `http://localhost:8080`.

What the page shows:
- Chat box powered by the same Jira/GitHub tool-calling agent
- Available GitHub repositories (from `gh_list_my_repos` equivalent data)
- Your open Jira issues (`assignee = currentUser() AND statusCategory != Done`)

## Configuration

Copy `.env.example` to `.env` and fill it in (or export real env vars):

```env
JIRA_BASE_URL=https://your-domain.atlassian.net
JIRA_EMAIL=you@example.com
JIRA_API_TOKEN=...

# Optional — enables the gh_* tools. Classic token with `repo` scope,
# or fine-grained with Issues + Pull requests read/write.
GITHUB_TOKEN=ghp_...
# GITHUB_API_URL=https://github.example.com/api/v3   # GHES only

# Defaults to local Ollama:
LLM_BASE_URL=http://localhost:11434/v1
LLM_API_KEY=ollama
LLM_MODEL=llama3.1:8b
WEB_LLM_TIMEOUT_SEC=180
# set to 0 to disable timeout for web chat requests

# Or OpenAI:
# LLM_BASE_URL=https://api.openai.com/v1
# LLM_API_KEY=sk-...
# LLM_MODEL=gpt-4o-mini
# WEB_LLM_TIMEOUT_SEC=180
```

Get a Jira API token: <https://id.atlassian.com/manage-profile/security/api-tokens>

### Using Ollama (default)

```bash
ollama serve
Jira:
- `what issues are assigned to me and not done?`
- `show me ABC-123`
- `comment on ABC-123 saying "merged, ready for QA"`
- `move ABC-123 to In Review`
- `create a Bug in project ABC titled "Login button misaligned on Safari"`
- `assign ABC-123 to Maria Lopez`

GitHub:
- `list my open PRs across all repos`
- `show me PR #42 in owner/repo and the files it touches`
- `open a PR from feature/foo into main on owner/repo titled "Add foo"`
- `comment on issue #17 in owner/repo: "looking into this today"`
- `approve PR #42 in owner/repo with body "LGTM"`
- `close issue #5 in owner/repoe and not done?`
- `show me ABC-123`
- `comment on ABC-123 saying "merged, ready for QA"`
- `move ABC-123 to In Review`
- `create a Bug in proje.

**Jira**

| Tool | Purpose |
| --- | --- |
| `search_issues` | JQL search |
| `get_issue` | Read one issue |
| `create_issue` | New issue in a project |
| `add_comment` | Comment on an issue |
| `list_transitions` / `transition_issue` | Change status |
| `update_issue_fields` | Generic field edit |
| `search_users` | Resolve a name/email to an accountId |
| `list_projects`, `myself` | Discovery |

**GitHub** (enabled when `GITHUB_TOKEN` is set)

| Tool | Purpose |
| --- | --- |
| `gh_me` | Authenticated GitHub user |
| `gh_list_my_repos` / `gh_get_repo` / `gh_search_repos` | Repository discovery |
| `gh_list_issues` / `gh_get_issue` | Read issues |
| `gh_create_issue` / `gh_update_issue` / `gh_close_issue` | Write issues |
| `gh_comment_issue` | Comment on an issue or PR |
| `gh_search_issues` | Global issue/PR search syntax |
| `gh_list_pulls` / `gh_get_pull` / `gh_list_pr_files` | Read PRs |
| `gh_create_pull` / `gh_merge_pull` / `gh_review_pull` | Write PRs |

## Files

| File | Purpose |
| --- | --- |
| [main.go](main.go) | CLI loop, LLM wiring, tool-call dispatch |
| [jira.go](jira.go) | Jira Cloud REST v3 client |
| [github.go](github.go) | GitHub REST v3 client

| File | Purpose |
| --- | --- |
| [main.go](main.go) | CLI loop, LLM wiring, tool-call dispatch |
| [jira.go](jira.go) | Jira Cloud REST v3 client (basic auth) |
| [tools.go](tools.go) | OpenAI tool schemas + dispatcher |

## Notes

- Uses Jira REST API **v3** (Cloud). Self-hosted Data Center needs v2 + different auth — easy to swap in [jira.go](jira.go).
- Local Ollama models vary in tool-calling quality. `llama3.1:8b` and `qwen2.5` work well; smaller models often hallucinate arguments.
- Credentials live in `.env` (gitignored). The agent never sends them to the LLM.
