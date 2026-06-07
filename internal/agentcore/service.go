package agentcore

import (
	"context"
	"sort"
	"strings"

	"github.com/ccastorena/jira-agent/internal/chat"
)

// ModelCatalog provides discoverable model names for a runtime.
type ModelCatalog interface {
	List(ctx context.Context) ([]string, error)
}

// StaticModelCatalog returns a fixed set of models.
type StaticModelCatalog struct {
	Models []string
}

// List returns the configured model names.
func (c StaticModelCatalog) List(_ context.Context) ([]string, error) {
	out := append([]string(nil), c.Models...)
	sort.Strings(out)
	return out, nil
}

// ChatService exposes a transport-agnostic chat API that can be used by the
// CLI, web handlers, or other callers embedding the agent.
type ChatService interface {
	DefaultModel() string
	AvailableModels(ctx context.Context) ([]string, error)
	ResolveModel(ctx context.Context, requested string) string
	RunTurn(ctx context.Context, sessionID, prompt, requestedModel string) (chat.Turn, error)
}

// AgentChatService is the default ChatService implementation backed by a
// chat.Engine and in-memory session store.
type AgentChatService struct {
	engine       *chat.Engine
	sessions     *chat.SessionStore
	defaultModel string
	catalog      ModelCatalog
}

// NewAgentChatService creates a new reusable chat service.
func NewAgentChatService(engine *chat.Engine, systemPrompt, defaultModel string, catalog ModelCatalog) *AgentChatService {
	return &AgentChatService{
		engine:       engine,
		sessions:     chat.NewSessionStore(systemPrompt),
		defaultModel: defaultModel,
		catalog:      catalog,
	}
}

// DefaultModel returns the configured fallback model name.
func (s *AgentChatService) DefaultModel() string {
	return s.defaultModel
}

// AvailableModels returns catalog models when configured, otherwise just the
// default model.
func (s *AgentChatService) AvailableModels(ctx context.Context) ([]string, error) {
	if s.catalog == nil {
		if strings.TrimSpace(s.defaultModel) == "" {
			return nil, nil
		}
		return []string{s.defaultModel}, nil
	}
	models, err := s.catalog.List(ctx)
	if err != nil {
		return nil, err
	}
	if len(models) == 0 && strings.TrimSpace(s.defaultModel) != "" {
		return []string{s.defaultModel}, nil
	}
	return models, nil
}

// ResolveModel chooses the requested model only when present in the model
// catalog. Otherwise it falls back to the default model.
func (s *AgentChatService) ResolveModel(ctx context.Context, requested string) string {
	model := strings.TrimSpace(requested)
	if model == "" {
		return s.defaultModel
	}
	if s.catalog == nil {
		return model
	}
	models, err := s.catalog.List(ctx)
	if err != nil || len(models) == 0 {
		return s.defaultModel
	}
	for _, candidate := range models {
		if candidate == model {
			return model
		}
	}
	return s.defaultModel
}

// RunTurn executes one user prompt and persists updated conversation state for
// the given session ID.
func (s *AgentChatService) RunTurn(ctx context.Context, sessionID, prompt, requestedModel string) (chat.Turn, error) {
	history := s.sessions.Get(sessionID)
	model := s.ResolveModel(ctx, requestedModel)
	turn, next, err := s.engine.Run(ctx, history, prompt, model)
	s.sessions.Set(sessionID, next)
	return turn, err
}
