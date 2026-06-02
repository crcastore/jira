# Jira Agent (Python, BYO-LLM / Ollama)

A small CLI agent that interacts with **Jira Cloud** using natural language.
The LLM is any OpenAI-compatible endpoint, so you can plug in:

- **Ollama** (local, free) — e.g. `llama3.1`, `qwen2.5`, `mistral-nemo` (needs a model that supports tool calling)
- **OpenAI** (`gpt-4o-mini`, etc.)
- **Groq**, **OpenRouter**, **Together**, or anything else that exposes `/v1/chat/completions` with tool-calling

## Setup

```bash
python3 -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt
cp .env.example .env
# edit .env with your Jira creds + LLM choice
```

Get a Jira API token: <https://id.atlassian.com/manage-profile/security/api-tokens>

### Using Ollama (default)

```bash
# in another terminal
ollama serve
ollama pull llama3.1
```

Leave `.env` at the defaults and you're ready.

### Using OpenAI

```env
LLM_BASE_URL=https://api.openai.com/v1
LLM_MODEL=gpt-4o-mini
LLM_API_KEY=sk-...
```

## Run

```bash
python agent.py
```

Example prompts:

- `what issues are assigned to me and not done?`
- `show me ABC-123`
- `comment on ABC-123 saying "merged, ready for QA"`
- `move ABC-123 to In Review`
- `create a Bug in project ABC titled "Login button misaligned on Safari"`
- `assign ABC-123 to Maria Lopez`

## What it can do

Tools exposed to the LLM (see [tools.py](tools.py)):

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

## Notes

- Uses Jira REST API **v3** (Cloud). Self-hosted Data Center would need v2 + different auth — easy to swap in [jira_client.py](jira_client.py).
- Local Ollama models vary a lot in tool-calling quality. `llama3.1:8b` and `qwen2.5` work well; smaller models often hallucinate arguments.
- Credentials live in `.env` (gitignored). The agent never sends them to the LLM.
