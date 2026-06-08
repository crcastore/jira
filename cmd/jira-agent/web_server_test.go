package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/ccastorena/jira-agent/chat"
)

type fakeChatService struct {
	defaultModel string
	models       []string
	resolveTo    string
	turn         chat.Turn
	err          error
}

func (f fakeChatService) DefaultModel() string { return f.defaultModel }

func (f fakeChatService) AvailableModels(context.Context) ([]string, error) {
	return append([]string(nil), f.models...), nil
}

func (f fakeChatService) ResolveModel(_ context.Context, requested string) string {
	if f.resolveTo != "" {
		return f.resolveTo
	}
	return requested
}

func (f fakeChatService) RunTurn(context.Context, string, string, string) (chat.Turn, error) {
	return f.turn, f.err
}

func (f fakeChatService) SetMaxContextTokens(int) {}

func (f fakeChatService) CurrentTokenUsage(string) int { return 0 }

func (f fakeChatService) ResetSession(string) {}

func TestHandleIndexRendersPage(t *testing.T) {
	app := &webApp{chat: fakeChatService{defaultModel: "llama", models: []string{"llama", "qwen"}}, llmTimeout: 5 * time.Second}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	app.handleIndex(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Jira + GitHub Agent") || !strings.Contains(body, "hx-chat") {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestHandleChatPromptRequired(t *testing.T) {
	app := &webApp{chat: fakeChatService{defaultModel: "m"}, llmTimeout: 5 * time.Second}
	req := httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(url.Values{"model": {"m"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	app.chatHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d want 400", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "prompt required") {
		t.Fatalf("body: got %q", rr.Body.String())
	}
}

func TestHandleChatRendersFriendlyTimeout(t *testing.T) {
	app := &webApp{
		chat:       fakeChatService{defaultModel: "m", resolveTo: "m", err: context.DeadlineExceeded},
		llmTimeout: 3 * time.Second,
	}
	req := httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(url.Values{"prompt": {"hello"}, "model": {"m"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	app.chatHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "did not respond within") {
		t.Fatalf("expected timeout guidance, got: %s", rr.Body.String())
	}
}

func TestFriendlyLLMErrorDefault(t *testing.T) {
	app := &webApp{llmTimeout: 2 * time.Second}
	msg := app.friendlyLLMError(errors.New("boom"), "x")
	if msg != "Error: boom" {
		t.Fatalf("unexpected message: %q", msg)
	}
}
