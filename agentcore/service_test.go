package agentcore

import (
	"context"
	"errors"
	"testing"

	openai "github.com/sashabaranov/go-openai"

	"github.com/ccastorena/jira-agent/chat"
)

type fakeCatalog struct {
	models []string
	err    error
}

func (f fakeCatalog) List(_ context.Context) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return append([]string(nil), f.models...), nil
}

type fakeToolBox struct{}

func (f fakeToolBox) Schemas() []openai.Tool {
	return []openai.Tool{{
		Type: openai.ToolTypeFunction,
		Function: &openai.FunctionDefinition{
			Name: "noop",
		},
	}}
}

func (f fakeToolBox) Call(name, args string) string {
	return "ok"
}

type scriptedLLM struct {
	responses []openai.ChatCompletionResponse
	seenLens  []int
}

func (s *scriptedLLM) CreateChatCompletion(_ context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	s.seenLens = append(s.seenLens, len(req.Messages))
	if len(s.responses) == 0 {
		return openai.ChatCompletionResponse{}, errors.New("no response queued")
	}
	resp := s.responses[0]
	s.responses = s.responses[1:]
	return resp, nil
}

func assistantText(content string) openai.ChatCompletionResponse {
	return openai.ChatCompletionResponse{Choices: []openai.ChatCompletionChoice{{
		Message: openai.ChatCompletionMessage{Role: openai.ChatMessageRoleAssistant, Content: content},
	}}}
}

func TestStaticModelCatalogSorts(t *testing.T) {
	models, err := (StaticModelCatalog{Models: []string{"z", "a", "m"}}).List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"a", "m", "z"}
	for i := range want {
		if models[i] != want[i] {
			t.Fatalf("got %v want %v", models, want)
		}
	}
}

func TestAgentChatServiceResolveModel(t *testing.T) {
	svc := NewAgentChatService(
		&chat.Engine{LLM: &scriptedLLM{responses: []openai.ChatCompletionResponse{assistantText("ok")}}, Tools: fakeToolBox{}, DefaultModel: "base", MaxSteps: 3},
		"sys",
		"base",
		fakeCatalog{models: []string{"alpha", "beta"}},
	)

	if got := svc.ResolveModel(context.Background(), ""); got != "base" {
		t.Fatalf("empty model: got %q want %q", got, "base")
	}
	if got := svc.ResolveModel(context.Background(), "beta"); got != "beta" {
		t.Fatalf("known model: got %q want %q", got, "beta")
	}
	if got := svc.ResolveModel(context.Background(), "missing"); got != "base" {
		t.Fatalf("unknown model: got %q want %q", got, "base")
	}
}

func TestAgentChatServiceRunTurnPersistsHistory(t *testing.T) {
	llm := &scriptedLLM{responses: []openai.ChatCompletionResponse{
		assistantText("first"),
		assistantText("second"),
	}}
	engine := &chat.Engine{LLM: llm, Tools: fakeToolBox{}, DefaultModel: "base", MaxSteps: 3}
	svc := NewAgentChatService(engine, "sys", "base", nil)

	turn1, err := svc.RunTurn(context.Background(), "sid", "hello", "")
	if err != nil {
		t.Fatalf("turn1 error: %v", err)
	}
	if turn1.Reply != "first" {
		t.Fatalf("turn1 reply: got %q", turn1.Reply)
	}

	turn2, err := svc.RunTurn(context.Background(), "sid", "again", "")
	if err != nil {
		t.Fatalf("turn2 error: %v", err)
	}
	if turn2.Reply != "second" {
		t.Fatalf("turn2 reply: got %q", turn2.Reply)
	}

	if len(llm.seenLens) != 2 {
		t.Fatalf("expected 2 llm calls, got %d", len(llm.seenLens))
	}
	if llm.seenLens[0] != 2 {
		t.Fatalf("first request messages: got %d want 2", llm.seenLens[0])
	}
	if llm.seenLens[1] != 4 {
		t.Fatalf("second request messages: got %d want 4", llm.seenLens[1])
	}
}

func TestAgentChatServiceRunTurnDoesNotPersistFailedHistory(t *testing.T) {
	llm := &scriptedLLM{}
	engine := &chat.Engine{LLM: llm, Tools: fakeToolBox{}, DefaultModel: "base", MaxSteps: 3}
	svc := NewAgentChatService(engine, "sys", "base", nil)

	if _, err := svc.RunTurn(context.Background(), "sid", "failed", ""); err == nil {
		t.Fatal("expected first turn to fail")
	}

	llm.responses = []openai.ChatCompletionResponse{assistantText("recovered")}
	turn, err := svc.RunTurn(context.Background(), "sid", "retry", "")
	if err != nil {
		t.Fatalf("retry error: %v", err)
	}
	if turn.Reply != "recovered" {
		t.Fatalf("retry reply: got %q", turn.Reply)
	}
	if len(llm.seenLens) != 2 {
		t.Fatalf("expected 2 LLM attempts, got %d", len(llm.seenLens))
	}
	if llm.seenLens[1] != 2 {
		t.Fatalf("failed prompt persisted into retry: second request messages=%d want 2", llm.seenLens[1])
	}
}

func TestAgentChatServiceAvailableModelsFallbackOnEmptyCatalog(t *testing.T) {
	svc := NewAgentChatService(
		&chat.Engine{LLM: &scriptedLLM{responses: []openai.ChatCompletionResponse{assistantText("ok")}}, Tools: fakeToolBox{}, DefaultModel: "base", MaxSteps: 3},
		"sys",
		"base",
		fakeCatalog{models: nil},
	)
	models, err := svc.AvailableModels(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 1 || models[0] != "base" {
		t.Fatalf("fallback models: got %v", models)
	}
}
