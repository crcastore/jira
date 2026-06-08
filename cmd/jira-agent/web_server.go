package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ccastorena/jira-agent/agentcore"
	"github.com/ccastorena/jira-agent/chathttp"
	"github.com/ccastorena/jira-agent/chatui"
)

type webApp struct {
	chat             agentcore.ChatService
	chatUI           *chatui.Component
	llmTimeout       time.Duration
	jc               *JiraClient
	gc               *GitHubClient
	maxContextTokens int
}

const defaultWebMaxContextTokens = 4000

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

	cfg := loadLLMConfig()
	llmTimeoutSec := envOrInt("WEB_LLM_TIMEOUT_SEC", 600)
	addr := envOr("WEB_ADDR", ":8080")

	engine := newEngine(cfg, jc, gc)
	catalog := agentcore.NewOllamaModelCatalog(cfg.baseURL, http.DefaultClient)
	chatSvc := agentcore.NewAgentChatService(engine, systemPrompt, cfg.model, catalog)

	// Configure token limiting (can be adjusted via API).
	maxTokens := envOrInt("WEB_MAX_CONTEXT_TOKENS", defaultWebMaxContextTokens)
	chatSvc.WithTokenLimit(cfg.baseURL, cfg.model, maxTokens)

	app := &webApp{
		chat:             chatSvc,
		chatUI:           chatui.New(),
		llmTimeout:       time.Duration(llmTimeoutSec) * time.Second,
		jc:               jc,
		gc:               gc,
		maxContextTokens: maxTokens,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", app.handleIndex)
	mux.Handle("/chat", app.chatHandler())
	mux.Handle("/api/token-limit", app.tokenLimitHandler())
	mux.Handle("/api/reset", app.resetHandler())
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
	models, modelsErr := a.chat.AvailableModels(r.Context())
	ui := a.chatComponent()
	widget, err := ui.Widget(chatui.WidgetData{
		Endpoint:      "/chat",
		LogID:         "chat-log",
		Greeting:      "Ask anything about your Jira tickets or GitHub work.",
		Placeholder:   "Ask the agent something...",
		Model:         a.chat.DefaultModel(),
		Models:        models,
		ModelsErr:     errString(modelsErr),
		ResetEndpoint: "/api/reset",
	})
	if err != nil {
		http.Error(w, "chat widget unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pageTmpl.Execute(w, map[string]any{
		"Model":            a.chat.DefaultModel(),
		"GitHubReady":      a.gc != nil,
		"ChatStyles":       chatui.StyleTag(),
		"ChatWidget":       widget,
		"MaxContextTokens": a.currentMaxContextTokens(),
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

func (a *webApp) chatComponent() *chatui.Component {
	if a.chatUI != nil {
		return a.chatUI
	}
	return chatui.New()
}

func (a *webApp) chatHandler() http.Handler {
	return chathttp.ChatHandler{
		Service:      a.chat,
		UI:           a.chatComponent(),
		SessionID:    a.ensureSession,
		ErrorMessage: a.friendlyLLMError,
	}
}

func (a *webApp) resetHandler() http.Handler {
	return chathttp.ResetHandler{
		Service:   a.chat,
		SessionID: a.ensureSession,
	}
}

func (a *webApp) tokenLimitHandler() http.Handler {
	return chathttp.TokenLimitHandler{
		Service:      a.chat,
		SessionID:    a.ensureSession,
		CurrentLimit: func() int { return a.currentMaxContextTokens() },
		SetLimit:     func(maxTokens int) { a.maxContextTokens = maxTokens },
	}
}

func (a *webApp) currentMaxContextTokens() int {
	if a.maxContextTokens > 0 {
		return a.maxContextTokens
	}
	return defaultWebMaxContextTokens
}

// friendlyLLMError turns a raw LLM transport error into actionable guidance.
// A context deadline almost always means the local model was slower than the
// configured per-request timeout (WEB_LLM_TIMEOUT_SEC), which is common on
// laptops running larger local models.
func (a *webApp) friendlyLLMError(err error, model string) string {
	if errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "context deadline exceeded") {
		return fmt.Sprintf(
			"The model %q did not respond within %s. Local models can be slow to evaluate tool calls. "+
				"Try a smaller/faster model, keep it warm (OLLAMA_KEEP_ALIVE), or raise WEB_LLM_TIMEOUT_SEC in your .env, then restart the server.",
			model, a.llmTimeout)
	}
	return "Error: " + err.Error()
}
