package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

type webApp struct {
	client     *openai.Client
	model      string
	baseURL    string
	llmTimeout time.Duration
	jc         *JiraClient
	gc         *GitHubClient
	sessions   map[string][]openai.ChatCompletionMessage
	mu         sync.Mutex
}

type repoItem struct {
	FullName string
	URL      string
	Updated  string
	Private  bool
}

type jiraIssueItem struct {
	Key      string
	Summary  string
	Status   string
	Assignee string
	Updated  string
}

type toolEvent struct {
	Name   string
	Args   string
	Result string
}

func serveWeb() {
	loadDotEnv(".env")

	jc, err := NewJiraClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v. Copy .env.example to .env and fill it in.\n", err)
		os.Exit(1)
	}

	gc, gerr := NewGitHubClient()
	if gerr != nil {
		fmt.Fprintf(os.Stderr, "GitHub panel disabled: %v\n", gerr)
	}

	baseURL := envOr("LLM_BASE_URL", "http://localhost:11434/v1")
	apiKey := firstNonEmpty(os.Getenv("LLM_API_KEY"), os.Getenv("OPENAI_API_KEY"), "ollama")
	model := envOr("LLM_MODEL", "llama3.1:8b")
	llmTimeoutSec := envOrInt("WEB_LLM_TIMEOUT_SEC", 600)
	addr := envOr("WEB_ADDR", ":8080")

	cfg := openai.DefaultConfig(apiKey)
	cfg.BaseURL = baseURL

	app := &webApp{
		client:     openai.NewClientWithConfig(cfg),
		model:      model,
		baseURL:    baseURL,
		llmTimeout: time.Duration(llmTimeoutSec) * time.Second,
		jc:         jc,
		gc:         gc,
		sessions:   map[string][]openai.ChatCompletionMessage{},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", app.handleIndex)
	mux.HandleFunc("/chat", app.handleChat)
	mux.HandleFunc("/partials/repos", app.handleRepos)
	mux.HandleFunc("/partials/jira-issues", app.handleJiraIssues)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
	})

	fmt.Printf("Web UI running on http://localhost%s\n", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		fmt.Fprintf(os.Stderr, "web server error: %v\n", err)
		os.Exit(1)
	}
}

func (a *webApp) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	_, _ = a.ensureSession(r, w)
	models, modelsErr := a.fetchOllamaModels()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pageTmpl.Execute(w, map[string]any{
		"Model":       a.model,
		"Models":      models,
		"ModelsErr":   errString(modelsErr),
		"GitHubReady": a.gc != nil,
	})
}

func (a *webApp) handleRepos(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	repos, err := a.fetchRepos()
	_ = reposTmpl.Execute(w, map[string]any{"Repos": repos, "Err": errString(err)})
}

func (a *webApp) handleJiraIssues(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	issues, err := a.fetchJiraIssues()
	_ = issuesTmpl.Execute(w, map[string]any{"Issues": issues, "Err": errString(err)})
}

func (a *webApp) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sid, _ := a.ensureSession(r, w)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	prompt := strings.TrimSpace(r.FormValue("prompt"))
	model := strings.TrimSpace(r.FormValue("model"))
	if model == "" {
		model = a.model
	}
	models, _ := a.fetchOllamaModels()
	if len(models) > 0 && !containsString(models, model) {
		model = a.model
	}
	if prompt == "" {
		http.Error(w, "prompt required", http.StatusBadRequest)
		return
	}

	assistant, events, err := a.runAgentTurn(sid, prompt, model)
	if err != nil {
		assistant = a.friendlyLLMError(err, model)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = chatChunkTmpl.Execute(w, map[string]any{
		"Prompt":    prompt,
		"Assistant": assistant,
		"Events":    events,
	})
}

// friendlyLLMError turns a raw LLM transport error into actionable guidance.
// A context deadline almost always means the local model was slower than the
// configured per-request timeout (WEB_LLM_TIMEOUT_SEC), which is common on
// laptops running larger models with the full tool schema.
func (a *webApp) friendlyLLMError(err error, model string) string {
	if errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "context deadline exceeded") {
		return fmt.Sprintf(
			"The model %q did not respond within %s. Local models can be slow to evaluate the full tool list. "+
				"Try a smaller/faster model, keep it warm (OLLAMA_KEEP_ALIVE), or raise WEB_LLM_TIMEOUT_SEC in your .env, then restart the server.",
			model, a.llmTimeout)
	}
	return "Error: " + err.Error()
}

func (a *webApp) runAgentTurn(sessionID, prompt, model string) (string, []toolEvent, error) {
	messages := a.getSessionMessages(sessionID)
	messages = append(messages, openai.ChatCompletionMessage{Role: openai.ChatMessageRoleUser, Content: prompt})
	events := make([]toolEvent, 0)
	assistantText := ""
	if strings.TrimSpace(model) == "" {
		model = a.model
	}

	for step := 0; step < maxSteps; step++ {
		ctx := context.Background()
		cancel := func() {}
		if a.llmTimeout > 0 {
			ctx, cancel = context.WithTimeout(ctx, a.llmTimeout)
		}
		resp, err := a.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
			Model:      model,
			Messages:   messages,
			Tools:      ToolSchemas,
			ToolChoice: "auto",
		})
		cancel()
		if err != nil {
			a.setSessionMessages(sessionID, messages)
			return assistantText, events, err
		}
		msg := resp.Choices[0].Message
		messages = append(messages, openai.ChatCompletionMessage{
			Role:      openai.ChatMessageRoleAssistant,
			Content:   msg.Content,
			ToolCalls: msg.ToolCalls,
		})
		assistantText = strings.TrimSpace(msg.Content)

		if len(msg.ToolCalls) == 0 {
			if assistantText == "" {
				assistantText = "Done."
			}
			a.setSessionMessages(sessionID, messages)
			return assistantText, events, nil
		}

		for _, tc := range msg.ToolCalls {
			args := tc.Function.Arguments
			if strings.TrimSpace(args) == "" {
				args = "{}"
			}
			result := CallTool(a.jc, a.gc, tc.Function.Name, args)
			events = append(events, toolEvent{Name: tc.Function.Name, Args: args, Result: result})
			messages = append(messages, openai.ChatCompletionMessage{
				Role:       openai.ChatMessageRoleTool,
				ToolCallID: tc.ID,
				Name:       tc.Function.Name,
				Content:    result,
			})
		}
	}

	a.setSessionMessages(sessionID, messages)
	if assistantText == "" {
		assistantText = "Step limit reached."
	}
	return assistantText, events, nil
}

func (a *webApp) fetchRepos() ([]repoItem, error) {
	if a.gc == nil {
		return nil, fmt.Errorf("GITHUB_TOKEN not set")
	}
	raw, err := a.gc.ListMyRepos("", "updated", 150)
	if err != nil {
		return nil, err
	}
	var repos []struct {
		FullName string `json:"full_name"`
		HTMLURL  string `json:"html_url"`
		Updated  string `json:"updated_at"`
		Private  bool   `json:"private"`
	}
	if err := json.Unmarshal(raw, &repos); err != nil {
		return nil, err
	}
	items := make([]repoItem, 0, len(repos))
	for _, r := range repos {
		items = append(items, repoItem{
			FullName: r.FullName,
			URL:      r.HTMLURL,
			Updated:  trimISODate(r.Updated),
			Private:  r.Private,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Updated > items[j].Updated })
	if len(items) > 40 {
		items = items[:40]
	}
	return items, nil
}

func (a *webApp) fetchJiraIssues() ([]jiraIssueItem, error) {
	raw, err := a.jc.Search(
		"assignee = currentUser() AND statusCategory != Done ORDER BY updated DESC",
		[]string{"summary", "status", "assignee", "updated"},
		40,
	)
	if err != nil {
		return nil, err
	}

	var payload struct {
		Issues []struct {
			Key    string `json:"key"`
			Fields struct {
				Summary string `json:"summary"`
				Updated string `json:"updated"`
				Status  struct {
					Name string `json:"name"`
				} `json:"status"`
				Assignee struct {
					DisplayName string `json:"displayName"`
				} `json:"assignee"`
			} `json:"fields"`
		} `json:"issues"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	items := make([]jiraIssueItem, 0, len(payload.Issues))
	for _, it := range payload.Issues {
		assignee := it.Fields.Assignee.DisplayName
		if assignee == "" {
			assignee = "Unassigned"
		}
		items = append(items, jiraIssueItem{
			Key:      it.Key,
			Summary:  it.Fields.Summary,
			Status:   it.Fields.Status.Name,
			Assignee: assignee,
			Updated:  trimISODate(it.Fields.Updated),
		})
	}
	return items, nil
}

func (a *webApp) getSessionMessages(sessionID string) []openai.ChatCompletionMessage {
	a.mu.Lock()
	defer a.mu.Unlock()
	if msgs, ok := a.sessions[sessionID]; ok {
		return append([]openai.ChatCompletionMessage(nil), msgs...)
	}
	msgs := []openai.ChatCompletionMessage{{Role: openai.ChatMessageRoleSystem, Content: systemPrompt}}
	a.sessions[sessionID] = msgs
	return append([]openai.ChatCompletionMessage(nil), msgs...)
}

func (a *webApp) setSessionMessages(sessionID string, msgs []openai.ChatCompletionMessage) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sessions[sessionID] = append([]openai.ChatCompletionMessage(nil), msgs...)
}

func (a *webApp) ensureSession(r *http.Request, w http.ResponseWriter) (string, error) {
	if c, err := r.Cookie("jira_agent_sid"); err == nil && c.Value != "" {
		return c.Value, nil
	}
	id, err := randomHex(16)
	if err != nil {
		return "", err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "jira_agent_sid",
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	return id, nil
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func trimISODate(s string) string {
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func envOrInt(k string, def int) int {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	if n < 0 {
		return def
	}
	return n
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func (a *webApp) fetchOllamaModels() ([]string, error) {
	base := strings.TrimRight(a.baseURL, "/")
	base = strings.TrimSuffix(base, "/v1")
	if base == "" {
		return nil, fmt.Errorf("LLM_BASE_URL is empty")
	}

	req, err := http.NewRequest(http.MethodGet, base+"/api/tags", nil)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("ollama tags request failed: %s", resp.Status)
	}

	var payload struct {
		Models []struct {
			Name         string   `json:"name"`
			Capabilities []string `json:"capabilities"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	names := make([]string, 0, len(payload.Models))
	for _, m := range payload.Models {
		if m.Name == "" {
			continue
		}
		if len(m.Capabilities) > 0 {
			hasCompletion := false
			hasTools := false
			for _, cap := range m.Capabilities {
				if cap == "completion" {
					hasCompletion = true
				}
				if cap == "tools" {
					hasTools = true
				}
			}
			if !hasCompletion || !hasTools {
				continue
			}
		}
		names = append(names, m.Name)
	}
	sort.Strings(names)
	return names, nil
}

var pageTmpl = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Jira + GitHub Agent</title>
  <script src="https://unpkg.com/htmx.org@1.9.12"></script>
  <style>
    :root {
      --bg: #f6f7f4;
      --panel: #ffffff;
      --ink: #1f2937;
      --muted: #6b7280;
      --border: #d1d5db;
      --accent: #115e59;
      --accent-2: #0f766e;
      --bubble-user: #d1fae5;
      --bubble-assistant: #fff7ed;
      --error: #9f1239;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      font-family: "SF Pro Text", "Segoe UI", sans-serif;
      color: var(--ink);
      background: radial-gradient(circle at 15% 0%, #e0f2fe 0%, transparent 40%), var(--bg);
    }
    .layout {
      display: grid;
      grid-template-columns: 2fr 1fr;
      gap: 16px;
      max-width: 1280px;
      margin: 20px auto;
      padding: 0 16px 24px;
    }
    .panel {
      background: var(--panel);
      border: 1px solid var(--border);
      border-radius: 14px;
      box-shadow: 0 8px 24px rgba(0,0,0,0.05);
    }
    .chat {
      display: flex;
      flex-direction: column;
      min-height: 80vh;
    }
    .chat-head {
      padding: 14px 16px;
      border-bottom: 1px solid var(--border);
    }
    .chat-head h1 {
      margin: 0;
      font-size: 20px;
    }
    .chat-head p {
      margin: 6px 0 0;
      color: var(--muted);
      font-size: 13px;
    }
    #chat-log {
      padding: 14px;
      overflow-y: auto;
      flex: 1;
      display: flex;
      flex-direction: column;
      gap: 10px;
    }
    .bubble {
      border-radius: 12px;
      padding: 10px 12px;
      max-width: 100%;
      white-space: pre-wrap;
      line-height: 1.45;
      border: 1px solid var(--border);
    }
    .user { background: var(--bubble-user); align-self: flex-end; }
    .assistant { background: var(--bubble-assistant); }
    .tool-log {
      margin-top: 8px;
      border-top: 1px dashed var(--border);
      padding-top: 8px;
      font-size: 12px;
      color: var(--muted);
    }
    details {
      margin-top: 6px;
      border: 1px solid var(--border);
      border-radius: 10px;
      padding: 8px;
      background: #fafafa;
    }
    pre {
      white-space: pre-wrap;
      word-break: break-word;
      margin: 6px 0 0;
      font-size: 11px;
      color: #111827;
    }
    form {
      display: grid;
			grid-template-columns: auto 1fr auto;
      gap: 10px;
      padding: 12px;
      border-top: 1px solid var(--border);
    }
		select {
			border: 1px solid var(--border);
			border-radius: 10px;
			padding: 10px 12px;
			font-size: 14px;
			background: #fff;
			color: var(--ink);
			min-width: 180px;
		}
    input[type="text"] {
      width: 100%;
      border: 1px solid var(--border);
      border-radius: 10px;
      padding: 10px 12px;
      font-size: 14px;
    }
    button {
      border: 0;
      border-radius: 10px;
      background: linear-gradient(135deg, var(--accent), var(--accent-2));
      color: #fff;
      font-weight: 600;
      padding: 10px 14px;
      cursor: pointer;
    }
    .side {
      display: flex;
      flex-direction: column;
      gap: 12px;
    }
    .card h2 {
      margin: 0;
      font-size: 15px;
    }
    .card-head {
      display: flex;
      justify-content: space-between;
      align-items: center;
      padding: 12px;
      border-bottom: 1px solid var(--border);
    }
    .card-body {
      padding: 12px;
      max-height: 34vh;
      overflow: auto;
    }
    .list {
      display: grid;
      gap: 8px;
    }
    .item {
      border: 1px solid var(--border);
      border-radius: 10px;
      padding: 8px;
      background: #fcfcfc;
    }
    .item a { color: #0c4a6e; text-decoration: none; }
    .meta { color: var(--muted); font-size: 12px; margin-top: 4px; }
    .warn { color: var(--error); font-size: 12px; }
    .tiny { font-size: 12px; color: var(--muted); }
    .chat-working {
      display: none;
      align-items: center;
      gap: 8px;
      margin: 0 14px 4px;
      padding: 8px 12px;
      align-self: flex-start;
      border-radius: 12px;
      border: 1px solid var(--border);
      background: var(--bubble-assistant);
      font-size: 13px;
      color: var(--muted);
    }
    .chat-working.htmx-request { display: inline-flex; }
    .typing { display: inline-flex; align-items: center; gap: 4px; }
    .typing span {
      width: 6px;
      height: 6px;
      border-radius: 50%;
      background: var(--accent-2);
      opacity: 0.4;
      animation: typing-bounce 1.2s infinite ease-in-out;
    }
    .typing span:nth-child(2) { animation-delay: 0.2s; }
    .typing span:nth-child(3) { animation-delay: 0.4s; }
    @keyframes typing-bounce {
      0%, 80%, 100% { transform: translateY(0); opacity: 0.4; }
      40% { transform: translateY(-4px); opacity: 1; }
    }
    #chat-form.htmx-request button { opacity: 0.6; cursor: progress; }
    @media (max-width: 980px) {
      .layout { grid-template-columns: 1fr; }
      .chat { min-height: 60vh; }
      .card-body { max-height: none; }
    }
  </style>
</head>
<body>
  <div class="layout">
    <section class="panel chat">
      <header class="chat-head">
        <h1>Jira + GitHub Agent</h1>
        <p>Model: {{.Model}} | GitHub: {{if .GitHubReady}}enabled{{else}}disabled{{end}}</p>
      </header>

      <div id="chat-log">
        <div class="bubble assistant">Ask anything about your Jira tickets or GitHub work.</div>
      </div>

      <div id="chat-working" class="chat-working" aria-live="polite">
        <span class="typing"><span></span><span></span><span></span></span>
        <span>Working…</span>
      </div>

      <form id="chat-form" hx-post="/chat" hx-target="#chat-log" hx-swap="beforeend"
            hx-indicator="#chat-working" hx-disabled-elt="find button">
				<select name="model" aria-label="Model" id="model-select">
					{{if .Models}}
						{{range .Models}}
							<option value="{{.}}" {{if eq . $.Model}}selected{{end}}>{{.}}</option>
						{{end}}
					{{else}}
						<option value="{{.Model}}" selected>{{.Model}}</option>
					{{end}}
				</select>
        <input name="prompt" type="text" placeholder="Ask the agent something..." autocomplete="off" required>
        <button type="submit">Send</button>
      </form>
			{{if .ModelsErr}}<div class="tiny" style="padding: 0 12px 12px;">Model list unavailable: {{.ModelsErr}}</div>{{end}}
    </section>

    <aside class="side">
      <section class="panel card">
        <div class="card-head">
          <h2>Available GitHub Repos</h2>
          <button hx-get="/partials/repos" hx-target="#repos-body" hx-swap="innerHTML">Refresh</button>
        </div>
        <div class="card-body" id="repos-body" hx-get="/partials/repos" hx-trigger="load, every 90s" hx-swap="innerHTML">
          <div class="tiny">Loading repos...</div>
        </div>
      </section>

      <section class="panel card">
        <div class="card-head">
          <h2>My Open Jira Issues</h2>
          <button hx-get="/partials/jira-issues" hx-target="#jira-body" hx-swap="innerHTML">Refresh</button>
        </div>
        <div class="card-body" id="jira-body" hx-get="/partials/jira-issues" hx-trigger="load, every 90s" hx-swap="innerHTML">
          <div class="tiny">Loading issues...</div>
        </div>
      </section>
    </aside>
  </div>

  <script>
    (function() {
      var form = document.getElementById('chat-form');
      var input = form.querySelector('input[name="prompt"]');
      var log = document.getElementById('chat-log');

      document.body.addEventListener('htmx:afterSwap', function(event) {
        if (event.target && event.target.id === 'chat-log') {
          log.scrollTop = log.scrollHeight;
          input.value = '';
          input.focus();
        }
      });
    })();
  </script>
</body>
</html>`))

var chatChunkTmpl = template.Must(template.New("chatChunk").Parse(`
<div class="bubble user">{{.Prompt}}</div>
<div class="bubble assistant">
  {{.Assistant}}
  {{if .Events}}
  <div class="tool-log">Tools used: {{len .Events}}</div>
  {{range .Events}}
  <details>
    <summary>{{.Name}}</summary>
    <div><strong>args</strong></div>
    <pre>{{.Args}}</pre>
    <div><strong>result</strong></div>
    <pre>{{.Result}}</pre>
  </details>
  {{end}}
  {{end}}
</div>
`))

var reposTmpl = template.Must(template.New("repos").Parse(`
{{if .Err}}<div class="warn">{{.Err}}</div>{{end}}
{{if .Repos}}
<div class="list">
  {{range .Repos}}
  <div class="item">
    <a href="{{.URL}}" target="_blank" rel="noreferrer">{{.FullName}}</a>
    <div class="meta">updated {{.Updated}} {{if .Private}}| private{{end}}</div>
  </div>
  {{end}}
</div>
{{else}}
<div class="tiny">No repositories returned.</div>
{{end}}
`))

var issuesTmpl = template.Must(template.New("issues").Parse(`
{{if .Err}}<div class="warn">{{.Err}}</div>{{end}}
{{if .Issues}}
<div class="list">
  {{range .Issues}}
  <div class="item">
    <div><strong>{{.Key}}</strong> - {{.Summary}}</div>
    <div class="meta">{{.Status}} | {{.Assignee}} | updated {{.Updated}}</div>
  </div>
  {{end}}
</div>
{{else}}
<div class="tiny">No Jira issues returned.</div>
{{end}}
`))
