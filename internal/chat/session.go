package chat

import (
	"sync"

	openai "github.com/sashabaranov/go-openai"
)

// SessionStore keeps per-session conversation history in memory. It is safe for
// concurrent use. New sessions are seeded with the configured system prompt.
type SessionStore struct {
	mu           sync.Mutex
	systemPrompt string
	sessions     map[string][]openai.ChatCompletionMessage
}

// NewSessionStore returns a store that seeds every new session with the given
// system prompt.
func NewSessionStore(systemPrompt string) *SessionStore {
	return &SessionStore{
		systemPrompt: systemPrompt,
		sessions:     make(map[string][]openai.ChatCompletionMessage),
	}
}

// Get returns a copy of the message history for id, creating and seeding it on
// first access. The returned slice is a copy so callers can safely append.
func (s *SessionStore) Get(id string) []openai.ChatCompletionMessage {
	s.mu.Lock()
	defer s.mu.Unlock()

	msgs, ok := s.sessions[id]
	if !ok {
		msgs = []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: s.systemPrompt},
		}
		s.sessions[id] = msgs
	}
	return append([]openai.ChatCompletionMessage(nil), msgs...)
}

// Set replaces the stored history for id with a copy of msgs.
func (s *SessionStore) Set(id string, msgs []openai.ChatCompletionMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[id] = append([]openai.ChatCompletionMessage(nil), msgs...)
}
