# Usage Guide

A natural-language agent for **Jira Cloud** dashboard data and a focused
**GitHub issue** chat workflow. It runs as either a terminal CLI or an HTMX web
app, and both share the same chat engine. This guide covers setup, running,
configuration, and how to reuse the modular chat packages in your own HTMX apps.

---

## 1. Prerequisites

- **Go 1.22+**
- A **Jira Cloud** site + API token
- A **GitHub** token to enable the exposed `gh_*` chat tools
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
| `GITHUB_TOKEN` | yes for chat | Enables the exposed `gh_*` chat tools and GitHub panel |
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
go run ./cmd/jira-agent
# or build a binary
go build -o jira-agent ./cmd/jira-agent && ./jira-agent
```

You'll get a `you>` prompt. Type requests in plain English; type `exit`,
`quit`, or `:q` to leave. Tool calls are echoed live (e.g. `→ gh_list_issues({...})`)
with a spinner while the model thinks.

### Web mode (Go + HTMX)

```bash
go run ./cmd/jira-agent serve
# or
./jira-agent serve
```

Open <http://localhost:8080>. The page provides:

- A **chat box** backed by the focused GitHub issue agent
- A **model picker** dropdown (populated from Ollama's `/api/tags`, filtered to
  tool-capable models)
- **Available GitHub repositories**
- **Your open Jira issues** (`assignee = currentUser() AND statusCategory != Done`)

---

## 4. Example prompts

The currently exposed chat tools are intentionally limited to GitHub identity,
repository discovery, issue workflows, and changed-file inspection for pull
requests / merge requests (MRs). Removed Jira, pull request write, search, and
workflow schemas are archived in [REMOVED_TOOLS.md](REMOVED_TOOLS.md).

- `who am I on GitHub?`
- `list my repos sorted by recently pushed`
- `show open issues in owner/repo`
- `show issue #17 in owner/repo`
- `show files changed in MR #12 in owner/repo`
- `create an issue in owner/repo titled "Fix flaky login test" with label bug`
- `comment on issue #17 in owner/repo saying "looking into this today"`
- `close issue #17 in owner/repo`

---

## 5. Project layout

The chat logic is decoupled from the transport layers so it can be tested and
reused independently.

| Path | Purpose |
| --- | --- |
| [cmd/jira-agent/main.go](cmd/jira-agent/main.go) | CLI entry point + interactive loop |
| [cmd/jira-agent/agent.go](cmd/jira-agent/agent.go) | Shared wiring: env config, engine + tool-box construction |
| [cmd/jira-agent/web_server.go](cmd/jira-agent/web_server.go) | HTTP handlers and dashboard route wiring |
| [cmd/jira-agent/web_templates.go](cmd/jira-agent/web_templates.go) | Dashboard shell and side-panel templates |
| [cmd/jira-agent/tools.go](cmd/jira-agent/tools.go) | OpenAI tool schemas + dispatcher |
| [cmd/jira-agent/jira.go](cmd/jira-agent/jira.go) | Jira Cloud REST v3 client |
| [cmd/jira-agent/github.go](cmd/jira-agent/github.go) | GitHub REST v3 client |
| [agentcore/](agentcore/) | Transport-agnostic chat service + model catalog |
| [chat/](chat/) | Transport-free chat engine + session store |
| [chathttp/](chathttp/) | Reusable HTMX chat/reset/token HTTP handlers |
| [chatui/](chatui/) | Drop-in HTMX chat widget |

### `chat` — the chat engine

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

### `chatui` — the drop-in HTMX widget

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
  Endpoint:      "/chat",                 // your hx-post handler
  Greeting:      "Ask me anything.",
  Placeholder:   "Type a message...",
  Model:         "llama3.1:8b",
  Models:        []string{"llama3.1:8b", "qwen2.5"}, // optional dropdown
  ResetEndpoint: "/api/reset",            // optional reset button
})
```

If you only need rendering, your `/chat` handler can return one exchange to
append to the log:

```go
func chatHandler(w http.ResponseWriter, r *http.Request) {
    // ... run the engine ...
  _ = ui.RenderChunk(w, chatui.FromChatTurn(prompt, turn))
}
```

The widget includes its own init script that auto-scrolls the log and clears the
input after each HTMX swap. If `ResetEndpoint` is set, it also renders the reset
button, POSTs to that endpoint, restores the greeting bubble, and dispatches an
`hx-chat:reset` browser event for host-page controls.

### `chathttp` — reusable HTMX handlers

`chathttp` packages the common `/chat`, `/reset`, and token-limit endpoints. A
host app brings its own chat service and session lookup:

```go
ui := chatui.New()

sessionID := func(r *http.Request, w http.ResponseWriter) (string, error) {
  // Use your app's cookie, auth session, JWT claims, etc.
  return "session-id", nil
}

mux.Handle("/chat", chathttp.ChatHandler{
  Service:   chatService,
  UI:        ui,
  SessionID: sessionID,
})

mux.Handle("/api/reset", chathttp.ResetHandler{
  Service:   chatService,
  SessionID: sessionID,
})

maxTokens := 4000
mux.Handle("/api/token-limit", chathttp.TokenLimitHandler{
  Service:      chatService,
  SessionID:    sessionID,
  CurrentLimit: func() int { return maxTokens },
  SetLimit:     func(n int) { maxTokens = n },
})
```

The full Jira/GitHub web app uses these handlers in [cmd/jira-agent/web_server.go](cmd/jira-agent/web_server.go), so the reusable path is exercised by the production app.

---

## 6. Testing

```bash
go test ./...
```

The `chat`, `chatui`, and `chathttp` packages are unit-tested without network
access (scripted fake LLM + fake tool box, template render assertions, and HTTP
handler fakes).

---

## 7. Notes

- Uses Jira REST API **v3** (Cloud). Self-hosted Data Center needs v2 + different
  auth; swap it in [cmd/jira-agent/jira.go](cmd/jira-agent/jira.go).
- Tool calls include the exposed schema set, so the chosen model **must**
  support tool calling. The web model picker filters to models advertising both
  `completion` and `tools` capabilities.
- Set `WEB_LLM_TIMEOUT_SEC=0` if a slow local model keeps hitting the per-step
  timeout.
