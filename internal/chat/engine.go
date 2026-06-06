// Package chat contains the LLM conversation engine. It is deliberately free of
// any HTTP, CLI, or transport concerns so the same agent loop can be reused by
// the web server, the terminal client, or tests with fake dependencies.
package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

// ErrNoChoices is returned when the LLM responds without any choices.
var ErrNoChoices = errors.New("chat: llm returned no choices")

// LLM is the subset of the OpenAI-compatible client the engine needs. The
// concrete *openai.Client satisfies this interface, and tests can provide a
// scripted fake.
type LLM interface {
	CreateChatCompletion(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error)
}

// ToolBox exposes the tool schemas advertised to the model and executes a tool
// call by name. Implementations decide how to route calls (Jira, GitHub, etc).
type ToolBox interface {
	Schemas() []openai.Tool
	Call(name, args string) string
}

// ToolEvent records a single tool invocation made during a turn.
type ToolEvent struct {
	Name   string
	Args   string
	Result string
}

// Turn is the outcome of running the agent loop for one user prompt.
type Turn struct {
	Reply  string
	Events []ToolEvent
}

// Engine drives the tool-calling conversation loop. The zero value is not
// usable; set at least LLM and Tools.
type Engine struct {
	LLM          LLM
	Tools        ToolBox
	DefaultModel string
	MaxSteps     int
	Timeout      time.Duration

	// Optional lifecycle hooks, primarily for the CLI to drive a spinner and
	// echo tool calls. All are safe to leave nil.
	OnStepStart func()
	OnStepEnd   func()
	OnToolCall  func(name, args string)

	// ResolveToolName, when set, maps a (possibly hallucinated) tool name to a
	// canonical one before it is executed. It is applied to text-emitted tool
	// calls so aliases like "gh_list_repos" can resolve to "gh_list_my_repos".
	ResolveToolName func(name string) string
}

const defaultMaxSteps = 12

// Run executes the agent loop for a single prompt. It treats history as
// read-only, returning the updated message slice so the caller owns
// persistence. The returned Turn captures the final assistant reply plus any
// tool events that occurred along the way.
func (e *Engine) Run(ctx context.Context, history []openai.ChatCompletionMessage, prompt, model string) (Turn, []openai.ChatCompletionMessage, error) {
	if strings.TrimSpace(model) == "" {
		model = e.DefaultModel
	}
	maxSteps := e.MaxSteps
	if maxSteps <= 0 {
		maxSteps = defaultMaxSteps
	}

	messages := make([]openai.ChatCompletionMessage, len(history), len(history)+1)
	copy(messages, history)
	messages = append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: prompt,
	})

	turn := Turn{}

	for step := 0; step < maxSteps; step++ {
		resp, err := e.complete(ctx, model, messages)
		if err != nil {
			return turn, messages, err
		}
		if len(resp.Choices) == 0 {
			return turn, messages, ErrNoChoices
		}

		msg := resp.Choices[0].Message
		messages = append(messages, openai.ChatCompletionMessage{
			Role:      openai.ChatMessageRoleAssistant,
			Content:   msg.Content,
			ToolCalls: msg.ToolCalls,
		})
		turn.Reply = strings.TrimSpace(msg.Content)

		if len(msg.ToolCalls) == 0 {
			// Some models (notably smaller/tuned local ones) ignore the native
			// tools API and emit the tool call as plain text content. Recover
			// that case instead of surfacing raw JSON to the user.
			if name, args, ok := e.parseTextToolCall(msg.Content); ok {
				callID := fmt.Sprintf("text-call-%d", step)
				// Rewrite the assistant message into a proper tool call so the
				// conversation stays well-formed for the following step.
				messages[len(messages)-1] = openai.ChatCompletionMessage{
					Role: openai.ChatMessageRoleAssistant,
					ToolCalls: []openai.ToolCall{{
						ID:       callID,
						Type:     openai.ToolTypeFunction,
						Function: openai.FunctionCall{Name: name, Arguments: args},
					}},
				}
				if e.OnToolCall != nil {
					e.OnToolCall(name, args)
				}
				result := e.Tools.Call(name, args)
				turn.Events = append(turn.Events, ToolEvent{Name: name, Args: args, Result: result})
				messages = append(messages, openai.ChatCompletionMessage{
					Role:       openai.ChatMessageRoleTool,
					ToolCallID: callID,
					Name:       name,
					Content:    result,
				})
				turn.Reply = ""
				continue
			}
			if turn.Reply == "" {
				turn.Reply = "Done."
			}
			return turn, messages, nil
		}

		for _, tc := range msg.ToolCalls {
			args := tc.Function.Arguments
			if strings.TrimSpace(args) == "" {
				args = "{}"
			}
			name := e.resolveName(tc.Function.Name)
			if e.OnToolCall != nil {
				e.OnToolCall(name, args)
			}
			result := e.Tools.Call(name, args)
			turn.Events = append(turn.Events, ToolEvent{
				Name:   name,
				Args:   args,
				Result: result,
			})
			messages = append(messages, openai.ChatCompletionMessage{
				Role:       openai.ChatMessageRoleTool,
				ToolCallID: tc.ID,
				Name:       name,
				Content:    result,
			})
		}
	}

	if turn.Reply == "" {
		turn.Reply = "Step limit reached."
	}
	return turn, messages, nil
}

// complete performs one LLM call, applying the per-step timeout and lifecycle
// hooks.
func (e *Engine) complete(ctx context.Context, model string, messages []openai.ChatCompletionMessage) (openai.ChatCompletionResponse, error) {
	if e.OnStepStart != nil {
		e.OnStepStart()
	}
	if e.OnStepEnd != nil {
		defer e.OnStepEnd()
	}

	stepCtx := ctx
	cancel := context.CancelFunc(func() {})
	if e.Timeout > 0 {
		stepCtx, cancel = context.WithTimeout(ctx, e.Timeout)
	}
	defer cancel()

	return e.LLM.CreateChatCompletion(stepCtx, openai.ChatCompletionRequest{
		Model:      model,
		Messages:   messages,
		Tools:      e.Tools.Schemas(),
		ToolChoice: "auto",
	})
}

// resolveName applies the optional alias resolver.
func (e *Engine) resolveName(name string) string {
	if e.ResolveToolName != nil {
		if c := e.ResolveToolName(name); c != "" {
			return c
		}
	}
	return name
}

// knownToolNames returns the set of tool names the toolbox advertises.
func (e *Engine) knownToolNames() map[string]bool {
	names := make(map[string]bool)
	for _, t := range e.Tools.Schemas() {
		if t.Function != nil {
			names[t.Function.Name] = true
		}
	}
	return names
}

// parseTextToolCall recovers a tool call that a model emitted as plain text
// content instead of via the native tools API. It only succeeds when the
// content contains a JSON object with a "name" that (after alias resolution)
// matches a known tool, guarding against treating ordinary replies as calls.
func (e *Engine) parseTextToolCall(content string) (name, args string, ok bool) {
	s := strings.TrimSpace(content)
	start := strings.IndexByte(s, '{')
	end := strings.LastIndexByte(s, '}')
	if start < 0 || end <= start {
		return "", "", false
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(s[start:end+1]), &obj); err != nil {
		return "", "", false
	}

	rawName, hasName := obj["name"]
	if !hasName {
		return "", "", false
	}
	if err := json.Unmarshal(rawName, &name); err != nil {
		return "", "", false
	}
	name = e.resolveName(strings.TrimSpace(name))
	if !e.knownToolNames()[name] {
		return "", "", false
	}

	args = "{}"
	for _, key := range []string{"parameters", "arguments", "args"} {
		raw, found := obj[key]
		if !found {
			continue
		}
		trimmed := strings.TrimSpace(string(raw))
		switch {
		case trimmed == "" || trimmed == "null":
			// keep default {}
		case strings.HasPrefix(trimmed, "\""):
			// Arguments encoded as a JSON string of JSON.
			var inner string
			if err := json.Unmarshal(raw, &inner); err == nil && strings.TrimSpace(inner) != "" {
				args = inner
			}
		default:
			args = trimmed
		}
		break
	}
	return name, args, true
}
