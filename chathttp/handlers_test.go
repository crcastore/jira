package chathttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ccastorena/jira-agent/chat"
)

type fakeService struct {
	resolveTo string
	turn      chat.Turn
	err       error

	runSession string
	runPrompt  string
	runModel   string
	resetID    string
	usage      int
	maxTokens  int
}

func (f *fakeService) ResolveModel(_ context.Context, requested string) string {
	if f.resolveTo != "" {
		return f.resolveTo
	}
	return requested
}

func (f *fakeService) RunTurn(_ context.Context, sessionID, prompt, requestedModel string) (chat.Turn, error) {
	f.runSession = sessionID
	f.runPrompt = prompt
	f.runModel = requestedModel
	return f.turn, f.err
}

func (f *fakeService) ResetSession(sessionID string) { f.resetID = sessionID }

func (f *fakeService) CurrentTokenUsage(string) int { return f.usage }

func (f *fakeService) SetMaxContextTokens(maxTokens int) { f.maxTokens = maxTokens }

func fixedSession(id string) SessionIDFunc {
	return func(*http.Request, http.ResponseWriter) (string, error) { return id, nil }
}

func TestChatHandlerRendersTurn(t *testing.T) {
	svc := &fakeService{
		resolveTo: "resolved-model",
		turn:      chat.Turn{Reply: "hello back", Events: []chat.ToolEvent{{Name: "lookup", Args: `{}`, Result: "ok"}}},
	}
	h := ChatHandler{Service: svc, SessionID: fixedSession("sid")}
	req := httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader("prompt=hello&model=m"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{"hx-chat-bubble user", "hello", "hello back", "lookup"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q: %s", want, body)
		}
	}
	if svc.runSession != "sid" || svc.runPrompt != "hello" || svc.runModel != "resolved-model" {
		t.Fatalf("unexpected run args: session=%q prompt=%q model=%q", svc.runSession, svc.runPrompt, svc.runModel)
	}
}

func TestChatHandlerUsesCustomErrorMessage(t *testing.T) {
	svc := &fakeService{resolveTo: "m", err: errors.New("boom")}
	h := ChatHandler{
		Service:   svc,
		SessionID: fixedSession("sid"),
		ErrorMessage: func(err error, model string) string {
			return model + ": " + err.Error()
		},
	}
	req := httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader("prompt=hello&model=m"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "m: boom") {
		t.Fatalf("expected custom error message, got: %s", rr.Body.String())
	}
}

func TestChatHandlerRequiresPrompt(t *testing.T) {
	h := ChatHandler{Service: &fakeService{}, SessionID: fixedSession("sid")}
	req := httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader("prompt=+"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d want 400", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "prompt required") {
		t.Fatalf("body: %q", rr.Body.String())
	}
}

func TestResetHandler(t *testing.T) {
	svc := &fakeService{}
	h := ResetHandler{Service: svc, SessionID: fixedSession("sid")}
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/reset", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rr.Code)
	}
	if svc.resetID != "sid" {
		t.Fatalf("reset session: got %q want sid", svc.resetID)
	}
	if !strings.Contains(rr.Body.String(), `"status":"ok"`) {
		t.Fatalf("unexpected body: %s", rr.Body.String())
	}
}

func TestTokenLimitHandlerGetAndPost(t *testing.T) {
	svc := &fakeService{usage: 123}
	limit := 4000
	h := TokenLimitHandler{
		Service:      svc,
		SessionID:    fixedSession("sid"),
		CurrentLimit: func() int { return limit },
		SetLimit:     func(maxTokens int) { limit = maxTokens },
	}

	getRR := httptest.NewRecorder()
	h.ServeHTTP(getRR, httptest.NewRequest(http.MethodGet, "/api/token-limit", nil))
	if getRR.Code != http.StatusOK {
		t.Fatalf("GET status: got %d want 200", getRR.Code)
	}
	if !strings.Contains(getRR.Body.String(), `"current_tokens":123`) || !strings.Contains(getRR.Body.String(), `"max_tokens":4000`) {
		t.Fatalf("unexpected GET body: %s", getRR.Body.String())
	}

	postRR := httptest.NewRecorder()
	h.ServeHTTP(postRR, httptest.NewRequest(http.MethodPost, "/api/token-limit", strings.NewReader(`{"max_tokens":5000}`)))
	if postRR.Code != http.StatusOK {
		t.Fatalf("POST status: got %d want 200 body %s", postRR.Code, postRR.Body.String())
	}
	if svc.maxTokens != 5000 || limit != 5000 {
		t.Fatalf("token limit not updated: svc=%d limit=%d", svc.maxTokens, limit)
	}
	if !strings.Contains(postRR.Body.String(), `"max_tokens":5000`) {
		t.Fatalf("unexpected POST body: %s", postRR.Body.String())
	}
}

func TestTokenLimitHandlerRejectsInvalidLimit(t *testing.T) {
	h := TokenLimitHandler{Service: &fakeService{}, SessionID: fixedSession("sid")}
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/token-limit", strings.NewReader(`{"max_tokens":0}`)))

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d want 400", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "max_tokens must be positive") {
		t.Fatalf("unexpected body: %s", rr.Body.String())
	}
}
