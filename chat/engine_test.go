package chat

import (
	"context"
	"errors"
	"testing"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

// scriptedLLM returns a queued response on each call. If the queue is empty it
// returns the lastResp repeatedly, which is handy for max-step tests.
type scriptedLLM struct {
	responses []openai.ChatCompletionResponse
	lastResp  openai.ChatCompletionResponse
	err       error
	calls     int
}

func (s *scriptedLLM) CreateChatCompletion(_ context.Context, _ openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	s.calls++
	if s.err != nil {
		return openai.ChatCompletionResponse{}, s.err
	}
	if len(s.responses) == 0 {
		return s.lastResp, nil
	}
	resp := s.responses[0]
	s.responses = s.responses[1:]
	return resp, nil
}

// fakeTools records calls and returns canned results.
type fakeTools struct {
	calls   []string
	results map[string]string
}

func (f *fakeTools) Schemas() []openai.Tool {
	return []openai.Tool{{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{Name: "noop"}}}
}

func (f *fakeTools) Call(name, args string) string {
	f.calls = append(f.calls, name+":"+args)
	if r, ok := f.results[name]; ok {
		return r
	}
	return "ok"
}

func assistantText(content string) openai.ChatCompletionResponse {
	return openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{{
			Message: openai.ChatCompletionMessage{Role: openai.ChatMessageRoleAssistant, Content: content},
		}},
	}
}

func assistantToolCall(id, name, args string) openai.ChatCompletionResponse {
	return openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{{
			Message: openai.ChatCompletionMessage{
				Role: openai.ChatMessageRoleAssistant,
				ToolCalls: []openai.ToolCall{{
					ID:       id,
					Type:     openai.ToolTypeFunction,
					Function: openai.FunctionCall{Name: name, Arguments: args},
				}},
			},
		}},
	}
}

func newEngine(llm LLM, tools ToolBox) *Engine {
	return &Engine{LLM: llm, Tools: tools, DefaultModel: "test-model", MaxSteps: 5}
}

func TestRunPlainReply(t *testing.T) {
	llm := &scriptedLLM{responses: []openai.ChatCompletionResponse{assistantText("hello there")}}
	tools := &fakeTools{}
	e := newEngine(llm, tools)

	turn, history, err := e.Run(context.Background(), nil, "hi", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if turn.Reply != "hello there" {
		t.Fatalf("reply: got %q want %q", turn.Reply, "hello there")
	}
	if len(turn.Events) != 0 {
		t.Fatalf("expected no tool events, got %d", len(turn.Events))
	}
	if len(tools.calls) != 0 {
		t.Fatalf("tools should not be called, got %v", tools.calls)
	}
	// user + assistant
	if len(history) != 2 {
		t.Fatalf("history length: got %d want 2", len(history))
	}
	if history[0].Role != openai.ChatMessageRoleUser || history[0].Content != "hi" {
		t.Fatalf("first message should be the user prompt, got %+v", history[0])
	}
}

func TestRunDefaultsModelWhenEmpty(t *testing.T) {
	llm := &scriptedLLM{responses: []openai.ChatCompletionResponse{assistantText("ok")}}
	e := newEngine(llm, &fakeTools{})
	if _, _, err := e.Run(context.Background(), nil, "hi", "   "); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if llm.calls != 1 {
		t.Fatalf("expected exactly one LLM call, got %d", llm.calls)
	}
}

func TestRunExecutesToolThenReplies(t *testing.T) {
	llm := &scriptedLLM{responses: []openai.ChatCompletionResponse{
		assistantToolCall("call-1", "search_issues", `{"jql":"x"}`),
		assistantText("found 3 issues"),
	}}
	tools := &fakeTools{results: map[string]string{"search_issues": "3 results"}}
	e := newEngine(llm, tools)

	turn, history, err := e.Run(context.Background(), nil, "find my issues", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if turn.Reply != "found 3 issues" {
		t.Fatalf("reply: got %q", turn.Reply)
	}
	if len(turn.Events) != 1 {
		t.Fatalf("expected 1 tool event, got %d", len(turn.Events))
	}
	ev := turn.Events[0]
	if ev.Name != "search_issues" || ev.Args != `{"jql":"x"}` || ev.Result != "3 results" {
		t.Fatalf("unexpected tool event: %+v", ev)
	}
	if len(tools.calls) != 1 || tools.calls[0] != `search_issues:{"jql":"x"}` {
		t.Fatalf("unexpected tool calls: %v", tools.calls)
	}
	// user, assistant(toolcall), tool result, assistant(final)
	if len(history) != 4 {
		t.Fatalf("history length: got %d want 4", len(history))
	}
	if history[2].Role != openai.ChatMessageRoleTool || history[2].ToolCallID != "call-1" {
		t.Fatalf("third message should be the tool result, got %+v", history[2])
	}
}

func TestRunEmptyReplyBecomesDone(t *testing.T) {
	llm := &scriptedLLM{responses: []openai.ChatCompletionResponse{assistantText("")}}
	e := newEngine(llm, &fakeTools{})
	turn, _, err := e.Run(context.Background(), nil, "hi", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if turn.Reply != "Done." {
		t.Fatalf("reply: got %q want %q", turn.Reply, "Done.")
	}
}

func TestRunHitsStepLimit(t *testing.T) {
	// Always asks for a tool, never finishes.
	llm := &scriptedLLM{lastResp: assistantToolCall("c", "noop", "{}")}
	tools := &fakeTools{}
	e := &Engine{LLM: llm, Tools: tools, DefaultModel: "m", MaxSteps: 3}

	turn, _, err := e.Run(context.Background(), nil, "loop", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if turn.Reply != "Step limit reached." {
		t.Fatalf("reply: got %q", turn.Reply)
	}
	if llm.calls != 3 {
		t.Fatalf("expected 3 LLM calls (max steps), got %d", llm.calls)
	}
	if len(turn.Events) != 3 {
		t.Fatalf("expected 3 tool events, got %d", len(turn.Events))
	}
}

func TestRunPropagatesLLMError(t *testing.T) {
	wantErr := errors.New("boom")
	llm := &scriptedLLM{err: wantErr}
	e := newEngine(llm, &fakeTools{})
	_, _, err := e.Run(context.Background(), nil, "hi", "")
	if !errors.Is(err, wantErr) {
		t.Fatalf("error: got %v want %v", err, wantErr)
	}
}

func TestRunNoChoices(t *testing.T) {
	llm := &scriptedLLM{responses: []openai.ChatCompletionResponse{{}}}
	e := newEngine(llm, &fakeTools{})
	_, _, err := e.Run(context.Background(), nil, "hi", "")
	if !errors.Is(err, ErrNoChoices) {
		t.Fatalf("error: got %v want %v", err, ErrNoChoices)
	}
}

func TestRunInvokesLifecycleHooks(t *testing.T) {
	llm := &scriptedLLM{responses: []openai.ChatCompletionResponse{
		assistantToolCall("c", "noop", "{}"),
		assistantText("done"),
	}}
	var starts, ends int
	var toolNames []string
	e := newEngine(llm, &fakeTools{})
	e.OnStepStart = func() { starts++ }
	e.OnStepEnd = func() { ends++ }
	e.OnToolCall = func(name, _ string) { toolNames = append(toolNames, name) }

	if _, _, err := e.Run(context.Background(), nil, "hi", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if starts != 2 || ends != 2 {
		t.Fatalf("step hooks: starts=%d ends=%d want 2/2", starts, ends)
	}
	if len(toolNames) != 1 || toolNames[0] != "noop" {
		t.Fatalf("tool hook names: %v", toolNames)
	}
}

func TestRunRecoversTextEmittedToolCall(t *testing.T) {
	// The model emits a tool call as plain text content instead of using the
	// native tools API, then returns a normal answer on the next step.
	llm := &scriptedLLM{responses: []openai.ChatCompletionResponse{
		assistantText(`{"name": "noop", "parameters": {}}`),
		assistantText("here are your repos"),
	}}
	tools := &fakeTools{}
	e := newEngine(llm, tools)

	turn, history, err := e.Run(context.Background(), nil, "what repos do you see", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if turn.Reply != "here are your repos" {
		t.Fatalf("reply: got %q", turn.Reply)
	}
	if len(turn.Events) != 1 || turn.Events[0].Name != "noop" {
		t.Fatalf("expected 1 noop tool event, got %+v", turn.Events)
	}
	if len(tools.calls) != 1 {
		t.Fatalf("tool should have been executed once, got %v", tools.calls)
	}
	// The raw JSON content must have been rewritten into a real tool call.
	if history[1].Content != "" || len(history[1].ToolCalls) != 1 {
		t.Fatalf("assistant message not rewritten into a tool call: %+v", history[1])
	}
}

func TestRunResolvesAliasInTextToolCall(t *testing.T) {
	llm := &scriptedLLM{responses: []openai.ChatCompletionResponse{
		assistantText(`{"name": "gh_list_repos", "parameters": {}}`),
		assistantText("done"),
	}}
	tools := &fakeTools{}
	e := newEngine(llm, tools)
	e.ResolveToolName = func(name string) string {
		if name == "gh_list_repos" {
			return "noop" // canonical known tool in this test
		}
		return name
	}

	turn, _, err := e.Run(context.Background(), nil, "repos?", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(turn.Events) != 1 || turn.Events[0].Name != "noop" {
		t.Fatalf("alias not resolved before execution: %+v", turn.Events)
	}
}

func TestRunIgnoresNonToolJSONContent(t *testing.T) {
	// Content that happens to be JSON but references no known tool must be
	// treated as a normal reply, not executed.
	llm := &scriptedLLM{responses: []openai.ChatCompletionResponse{
		assistantText(`{"name": "definitely_not_a_tool", "parameters": {}}`),
	}}
	tools := &fakeTools{}
	e := newEngine(llm, tools)

	turn, _, err := e.Run(context.Background(), nil, "hi", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tools.calls) != 0 {
		t.Fatalf("no tool should run, got %v", tools.calls)
	}
	if turn.Reply == "" {
		t.Fatalf("expected the JSON to be returned as the reply")
	}
}

func TestRunResolvesAliasInNativeToolCall(t *testing.T) {
	llm := &scriptedLLM{responses: []openai.ChatCompletionResponse{
		assistantToolCall("c", "aliased_name", "{}"),
		assistantText("done"),
	}}
	tools := &fakeTools{}
	e := newEngine(llm, tools)
	e.ResolveToolName = func(name string) string {
		if name == "aliased_name" {
			return "noop"
		}
		return name
	}

	turn, _, err := e.Run(context.Background(), nil, "go", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(turn.Events) != 1 || turn.Events[0].Name != "noop" {
		t.Fatalf("native alias not resolved: %+v", turn.Events)
	}
}

func TestRunDoesNotMutateInputHistory(t *testing.T) {
	llm := &scriptedLLM{responses: []openai.ChatCompletionResponse{assistantText("ok")}}
	e := newEngine(llm, &fakeTools{})
	history := []openai.ChatCompletionMessage{{Role: openai.ChatMessageRoleSystem, Content: "sys"}}

	_, _, err := e.Run(context.Background(), history, "hi", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("input history was mutated: len=%d", len(history))
	}
}

func TestRunRespectsTimeoutConfig(t *testing.T) {
	// A slow LLM with a tiny per-step timeout should surface a deadline error.
	llm := slowLLM{delay: 50 * time.Millisecond}
	e := &Engine{LLM: llm, Tools: &fakeTools{}, DefaultModel: "m", Timeout: time.Millisecond}
	_, _, err := e.Run(context.Background(), nil, "hi", "")
	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
}

type slowLLM struct{ delay time.Duration }

func (s slowLLM) CreateChatCompletion(ctx context.Context, _ openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	select {
	case <-time.After(s.delay):
		return assistantText("late"), nil
	case <-ctx.Done():
		return openai.ChatCompletionResponse{}, ctx.Err()
	}
}

func TestSessionStoreSeedsAndPersists(t *testing.T) {
	store := NewSessionStore("system prompt")

	first := store.Get("abc")
	if len(first) != 1 || first[0].Role != openai.ChatMessageRoleSystem || first[0].Content != "system prompt" {
		t.Fatalf("new session not seeded correctly: %+v", first)
	}

	// Mutating the returned copy must not affect stored state.
	first = append(first, openai.ChatCompletionMessage{Role: openai.ChatMessageRoleUser, Content: "leak?"})
	if again := store.Get("abc"); len(again) != 1 {
		t.Fatalf("Get returned a non-copy; stored history mutated to len=%d", len(again))
	}

	store.Set("abc", first)
	if got := store.Get("abc"); len(got) != 2 {
		t.Fatalf("Set did not persist: len=%d want 2", len(got))
	}
}

func TestSessionStoreTrimsToNewestMessages(t *testing.T) {
	store := NewSessionStore("sys")
	store.SetMaxContextTokens(5)
	store.Set("abc", []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: "sys"},
		{Role: openai.ChatMessageRoleUser, Content: "aaaa"},
		{Role: openai.ChatMessageRoleAssistant, Content: "bbbb"},
		{Role: openai.ChatMessageRoleUser, Content: "cccc"},
	})

	got := store.Get("abc")
	if len(got) != 3 {
		t.Fatalf("trimmed history length: got %d want 3 (%+v)", len(got), got)
	}
	if got[0].Content != "sys" || got[1].Content != "bbbb" || got[2].Content != "cccc" {
		t.Fatalf("trimmed history kept wrong messages: %+v", got)
	}
}
