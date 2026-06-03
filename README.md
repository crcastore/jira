# Jira Agent (Go)

CLI agent that talks to **Jira Cloud** in natural language. The LLM is any
OpenAI-compatible endpoint, so you can plug in:

- **Ollama** (local, free) — e.g. `llama3.1`, `qwen2.5` (needs a tool-calling model)
- **OpenAI** (`gpt-4o-mini`, etc.)
- **Groq**, **OpenRouter**, **Together**, or anything else exposing `/v1/chat/completions` with tool calling

## Build & run

```bash
go mod tidy
go run .
# or
go build -o jira-agent && ./jira-agent
```

## Configuration

Copy `.env.example` to `.env` and fill it in (or export real env vars):

```env
JIRA_BASE_URL=https://your-domain.atlassian.net
JIRA_EMAIL=you@example.com
JIRA_API_TOKEN=...

# Defaults to local Ollama:
LLM_BASE_URL=http://localhost:11434/v1
LLM_API_KEY=ollama
LLM_MODEL=llama3.1

# Or OpenAI:
# LLM_BASE_URL=https://api.openai.com/v1
# LLM_API_KEY=sk-...
# LLM_MODEL=gpt-4o-mini
```

Get a Jira API token: <https://id.atlassian.com/manage-profile/security/api-tokens>

### Using Ollama (default)

```bash
ollama serve
ollama pull llama3.1
```

## Example prompts

- `what issues are assigned to me and not done?`
- `show me ABC-123`
- `comment on ABC-123 saying "merged, ready for QA"`
- `move ABC-123 to In Review`
- `create a Bug in project ABC titled "Login button misaligned on Safari"`
- `assign ABC-123 to Maria Lopez`

## Tools exposed to the LLM

See [tools.go](tools.go):

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

## Files

| File | Purpose |
| --- | --- |
| [main.go](main.go) | CLI loop, LLM wiring, tool-call dispatch |
| [jira.go](jira.go) | Jira Cloud REST v3 client (basic auth) |
| [tools.go](tools.go) | OpenAI tool schemas + dispatcher |

## Notes

- Uses Jira REST API **v3** (Cloud). Self-hosted Data Center needs v2 + different auth — easy to swap in [jira.go](jira.go).
- Local Ollama models vary in tool-calling quality. `llama3.1:8b` and `qwen2.5` work well; smaller models often hallucinate arguments.
- Credentials live in `.env` (gitignored). The agent never sends them to the LLM.
