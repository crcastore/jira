// Package chathttp provides reusable HTTP handlers for HTMX chat widgets.
package chathttp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/ccastorena/jira-agent/chat"
	"github.com/ccastorena/jira-agent/chatui"
)

// SessionIDFunc returns the conversation session id for the current request.
// Implementations can use cookies, auth claims, framework sessions, or any
// other host-app convention.
type SessionIDFunc func(r *http.Request, w http.ResponseWriter) (string, error)

// ErrorMessageFunc maps a chat error and resolved model into user-facing text.
type ErrorMessageFunc func(err error, model string) string

// ChatService is the subset of agentcore.ChatService needed by the /chat HTMX
// endpoint.
type ChatService interface {
	ResolveModel(ctx context.Context, requested string) string
	RunTurn(ctx context.Context, sessionID, prompt, requestedModel string) (chat.Turn, error)
}

// ResetService is the subset of agentcore.ChatService needed by reset handlers.
type ResetService interface {
	ResetSession(sessionID string)
}

// TokenService is the subset of agentcore.ChatService needed by token limit
// handlers.
type TokenService interface {
	CurrentTokenUsage(sessionID string) int
	SetMaxContextTokens(maxTokens int)
}

// ChatHandler handles form POSTs from chatui.Widget and writes one rendered
// chat chunk suitable for hx-swap="beforeend".
type ChatHandler struct {
	Service      ChatService
	UI           *chatui.Component
	SessionID    SessionIDFunc
	ErrorMessage ErrorMessageFunc
	PromptField  string
	ModelField   string
}

func (h ChatHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.Service == nil {
		http.Error(w, "chat service unavailable", http.StatusInternalServerError)
		return
	}
	sessionID, ok := h.lookupSession(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	prompt := strings.TrimSpace(r.FormValue(h.promptField()))
	if prompt == "" {
		http.Error(w, "prompt required", http.StatusBadRequest)
		return
	}
	requestedModel := strings.TrimSpace(r.FormValue(h.modelField()))
	resolvedModel := h.Service.ResolveModel(r.Context(), requestedModel)

	turn, err := h.Service.RunTurn(r.Context(), sessionID, prompt, resolvedModel)
	if err != nil {
		turn = chat.Turn{Reply: h.errorMessage(err, resolvedModel)}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.component().RenderChunk(w, chatui.FromChatTurn(prompt, turn))
}

func (h ChatHandler) lookupSession(w http.ResponseWriter, r *http.Request) (string, bool) {
	return lookupSession(w, r, h.SessionID)
}

func (h ChatHandler) component() *chatui.Component {
	if h.UI != nil {
		return h.UI
	}
	return chatui.New()
}

func (h ChatHandler) promptField() string {
	if h.PromptField != "" {
		return h.PromptField
	}
	return "prompt"
}

func (h ChatHandler) modelField() string {
	if h.ModelField != "" {
		return h.ModelField
	}
	return "model"
}

func (h ChatHandler) errorMessage(err error, model string) string {
	if h.ErrorMessage != nil {
		return h.ErrorMessage(err, model)
	}
	return "Error: " + err.Error()
}

// ResetHandler clears the current session's conversation history.
type ResetHandler struct {
	Service   ResetService
	SessionID SessionIDFunc
}

func (h ResetHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.Service == nil {
		http.Error(w, "chat service unavailable", http.StatusInternalServerError)
		return
	}
	sessionID, ok := lookupSession(w, r, h.SessionID)
	if !ok {
		return
	}
	h.Service.ResetSession(sessionID)
	writeJSON(w, map[string]string{"status": "ok"})
}

// TokenLimitHandler handles GET and POST requests for current token usage and
// runtime context-limit updates.
type TokenLimitHandler struct {
	Service      TokenService
	SessionID    SessionIDFunc
	CurrentLimit func() int
	SetLimit     func(maxTokens int)
}

func (h TokenLimitHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.Service == nil {
		http.Error(w, "chat service unavailable", http.StatusInternalServerError)
		return
	}
	sessionID, ok := lookupSession(w, r, h.SessionID)
	if !ok {
		return
	}

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, map[string]any{
			"current_tokens": h.Service.CurrentTokenUsage(sessionID),
			"max_tokens":     h.currentLimit(),
		})
	case http.MethodPost:
		var req struct {
			MaxTokens int `json:"max_tokens"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		if req.MaxTokens <= 0 {
			writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "max_tokens must be positive"})
			return
		}
		h.Service.SetMaxContextTokens(req.MaxTokens)
		if h.SetLimit != nil {
			h.SetLimit(req.MaxTokens)
		}
		writeJSON(w, map[string]any{
			"status":     "ok",
			"max_tokens": h.currentLimit(),
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h TokenLimitHandler) currentLimit() int {
	if h.CurrentLimit != nil {
		return h.CurrentLimit()
	}
	return 0
}

func lookupSession(w http.ResponseWriter, r *http.Request, fn SessionIDFunc) (string, bool) {
	if fn == nil {
		http.Error(w, "session id source unavailable", http.StatusInternalServerError)
		return "", false
	}
	sessionID, err := fn(r, w)
	if err != nil {
		http.Error(w, "session unavailable", http.StatusInternalServerError)
		return "", false
	}
	if strings.TrimSpace(sessionID) == "" {
		http.Error(w, "session unavailable", http.StatusInternalServerError)
		return "", false
	}
	return sessionID, true
}

func writeJSON(w http.ResponseWriter, v any) {
	writeJSONStatus(w, http.StatusOK, v)
}

func writeJSONStatus(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
