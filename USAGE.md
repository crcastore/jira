# Usage Guide

A natural-language agent for **Jira Cloud** and **GitHub**. It runs as either a
terminal CLI or an HTMX web app, and both share the same chat engine. This guide
covers setup, running, configuration, and how to reuse the modular chat packages
in your own HTMX apps.

---

## 1. Prerequisites

- **Go 1.22+**
- A **Jira Cloud** site + API token
- (Optional) a **GitHub** token to enable the `gh_*` tools
- An **OpenAI-compatible LLM endpoint** that supports tool calling:
  - Local **Ollama** (e.g. `llama3.1:8b`, `qwen2.5`), or
  - **OpenAI** / **Groq** / **OpenRouter** / **Together**, etc.

---

## 2. Configure

Copy the example env file and fill it in:

```bash
cp .env.example .env
```

Key settings (see [.env.example](.env.example) for the full list):

| Variable | Required | Description |
| --- | --- | --- |
| `JIRA_BASE_URL` | yes | e.g. `https://your-domain.atlassian.net` |
| `JIRA_EMAIL` | yes | Atlassian account email |
| `JIRA_API_TOKEN` | yes | [Create a token](https://id.atlassian.com/manage-profile/security/api-tokens) |
| `GITHUB_TOKEN` | no | Enables the `gh_*` tools; Jira-only if unset |
| `GITHUB_API_URL` | no | Only for GitHub Enterprise Server |
| `LLM_BASE_URL` | no | Defaults to `http://localhost:11434/v1` (Ollama) |
| `LLM_API_KEY` | no | Defaults to `ollama`; use your real key for hosted LLMs |
| `LLM_MODEL` | no | Defaults to `llama3.1:8b`; must support tool calling |
| `WEB_LLM_TIMEOUT_SEC` | no | Per-step LLM timeout in seconds (`0` disables it) |
| `WEB_ADDR` | no | Web server listen address; defaults to `:8080` |

> Credentials live in `.env` (gitignored) and are never sent to the LLM.

### Using Ollama (default)

```bash
ollama serve
ollama pull llama3.1:8b
```

Pick any tool-capable model; smaller models tend to hallucinate tool arguments.

---

## 3. Run

### CLI mode

```bash
go run .
# or build a binary
go build -o jira-agent && ./jira-agent
```

You'll get a `you>` prompt. Type requests in plain English; type `exit`,
`quit`, or `:q` to leave. Tool calls are echoed live (e.g. `→ search_issues({...})`)
with a spinner while the model thinks.

### Web mode (Go + HTMX)

```bash
go run . serve
# or
./jira-agent serve
```

Open <http://localhost:8080>. The page provides:

- A **chat box** backed by the same Jira/GitHub agent
- A **model picker** dropdown (populated from Ollama's `/api/tags`, filtered to
  tool-capable models)
- **Available GitHub repositories**
- **Your open Jira issues** (`assignee = currentUser() AND statusCategory != Done`)

---

## 4. Example prompts

**Jira**

- `what issues are assigned to me and not done?`
- `show me ABC-123`
- `comment on ABC-123 saying "merged, ready for QA"`
- `move ABC-123 to In Review`
- `create a Bug in project ABC titled "Login button misaligned on Safari"`
- `assign ABC-123 to Maria Lopez`

**GitHub** (requires `GITHUB_TOKEN`)

- `list my open PRs across all repos`
- `show me PR #42 in owner/repo and the files it touches`
- `open a PR from feature/foo into main on owner/repo titled "Add foo"`
- `approve PR #42 in owner/repo with body "LGTM"`
- `close issue #5 in owner/repo`

---

## 5. Project layout

The chat logic is decoupled from the transport layers so it can be tested and
reused independently.

| Path | Purpose |
| --- | --- |
| [main.go](main.go) | CLI entry point + interactive loop |
| [agent.go](agent.go) | Shared wiring: env config, engine + tool-box construction |
| [web.go](web.go) | HTTP handlers and the dashboard page template |
| [tools.go](tools.go) | OpenAI tool schemas + dispatcher |
| [jira.go](jira.go) | Jira Cloud REST v3 client |
| [github.go](github.go) | GitHub REST v3 client |
| [internal/chat/](internal/chat/) | Transport-free chat engine + session store |
| [internal/chatui/](internal/chatui/) | Drop-in HTMX chat widget |

### `internal/chat` — the chat engine

Pure conversation logic with no HTTP, CLI, or network coupling. It depends on two
small interfaces so it can be unit-tested with fakes:

```go
type LLM interface {
    CreateChatCompletion(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error)
}

type ToolBox interface {
    Schemas() []openai.Tool
    Call(name, args string) string
}
```

`Engine.Run` executes the tool-calling loop for one prompt and returns the final
reply, the tool events, and the updated message history (the caller owns
persistence):

```go
turn, history, err := engine.Run(ctx, history, prompt, model)
```

`SessionStore` provides thread-safe, per-session history seeded with the system
prompt. Optional `OnStepStart` / `OnStepEnd` / `OnToolCall` hooks let the CLI
drive a spinner and echo tool calls.

### `internal/chatui` — the drop-in HTMX widget

A self-contained chat component whose CSS is scoped under `.hx-chat`, so it won't
clash with a host page's styles. It is parameterized by the HTMX endpoint and the
log's DOM id, so it can be embedded in **any** HTMX page:

```go
ui := chatui.New()

// In your page <head>:
//   {{ .ChatStyles }}   <- chatui.StyleTag()
//
// Where you want the chat:
widget, _ := ui.Widget(chatui.WidgetData{
    Endpoint:    "/chat",                 // your hx-post handler
    Greeting:    "Ask me anything.",
    Placeholder: "Type a message...",
    Model:       "llama3.1:8b",
    Models:      []string{"llama3.1:8b", "qwen2.5"}, // optional dropdown
})
```

Your `/chat` handler returns one exchange to append to the log:

```go
func chatHandler(w http.ResponseWriter, r *http.Request) {
    // ... run the engine ...
    _ = ui.RenderChunk(w, chatui.Turn{
        Prompt:    prompt,
        Assistant: turn.Reply,
        Events:    events, // []chatui.Event{{Name, Args, Result}}
    })
}
```

The widget includes its own init script that auto-scrolls the log and clears the
input after each HTMX swap.

---

## 6. Testing

```bash
go test ./...
```

The `internal/chat` and `internal/chatui` packages are fully unit-tested without
any network access (scripted fake LLM + fake tool box, and template render
assertions).

---

## 7. Notes

- Uses Jira REST API **v3** (Cloud). Self-hosted Data Center needs v2 + different
  auth — swap it in [jira.go](jira.go).
- Tool calls always include the full schema set, so the chosen model **must**
  support tool calling. The web model picker filters to models advertising both
  `completion` and `tools` capabilities.
- Set `WEB_LLM_TIMEOUT_SEC=0` if a slow local model keeps hitting the per-step
  timeout.
