package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

type webApp struct {
	chat       ChatService
	llmTimeout time.Duration
	jc         *JiraClient
	gc         *GitHubClient
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

	cfg := loadLLMConfig()
	llmTimeoutSec := envOrInt("WEB_LLM_TIMEOUT_SEC", 600)
	addr := envOr("WEB_ADDR", ":8080")

	engine := newEngine(cfg, jc, gc)
	catalog := NewOllamaModelCatalog(cfg.baseURL, http.DefaultClient)
	chatSvc := NewAgentChatService(engine, systemPrompt, cfg.model, catalog)

	app := &webApp{
		chat:       chatSvc,
		llmTimeout: time.Duration(llmTimeoutSec) * time.Second,
		jc:         jc,
		gc:         gc,
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
	models, modelsErr := a.chat.AvailableModels(r.Context())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pageTmpl.Execute(w, map[string]any{
		"Model":       a.chat.DefaultModel(),
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
	requestedModel := strings.TrimSpace(r.FormValue("model"))
	resolvedModel := a.chat.ResolveModel(r.Context(), requestedModel)
	if prompt == "" {
		http.Error(w, "prompt required", http.StatusBadRequest)
		return
	}

	turn, err := a.chat.RunTurn(r.Context(), sid, prompt, resolvedModel)
	assistant := turn.Reply
	events := turn.Events
	if err != nil {
		assistant = a.friendlyLLMError(err, resolvedModel)
		events = nil
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
