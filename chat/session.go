package chat

import (
	"sync"

	openai "github.com/sashabaranov/go-openai"
)

// SessionStore keeps per-session conversation history in memory. It is safe for
// concurrent use. New sessions are seeded with the configured system prompt.
// If MaxContextTokens > 0, history is trimmed to stay within that limit.
type SessionStore struct {
	mu               sync.Mutex
	systemPrompt     string
	sessions         map[string][]openai.ChatCompletionMessage
	ollamaURL        string
	model            string
	maxContextTokens int
}

// NewSessionStore returns a store that seeds every new session with the given
// system prompt. maxContextTokens limits history size (0 = unlimited).
// ollamaURL should be like "http://localhost:11434"
func NewSessionStore(systemPrompt string) *SessionStore {
	return &SessionStore{
		systemPrompt: systemPrompt,
		sessions:     make(map[string][]openai.ChatCompletionMessage),
	}
}

// WithOllamaTokenLimit configures token-based history trimming.
// ollamaURL is the Ollama server URL (e.g., "http://localhost:11434")
// model is the model name to use for tokenization (e.g., "mistral")
// maxTokens is the maximum context tokens to keep (e.g., 4000)
func (s *SessionStore) WithOllamaTokenLimit(ollamaURL, model string, maxTokens int) *SessionStore {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ollamaURL = ollamaURL
	s.model = model
	s.maxContextTokens = maxTokens
	return s
}

// SetMaxContextTokens updates the token limit at runtime.
func (s *SessionStore) SetMaxContextTokens(maxTokens int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.maxContextTokens = maxTokens
}

// CurrentTokenUsage returns the current token count for a session.
func (s *SessionStore) CurrentTokenUsage(id string) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	msgs, ok := s.sessions[id]
	if !ok {
		return s.countTokens(s.systemPrompt)
	}
	return s.countHistoryTokens(msgs)
}

// countTokens estimates the token count for text.
// Uses ~4 characters per token, which is the standard approximation for
// most modern LLM tokenizers (BPE-based: GPT, Llama, Mistral, etc.).
func (s *SessionStore) countTokens(text string) int {
	if text == "" {
		return 0
	}
	// Standard approximation: 1 token ≈ 4 characters of English text
	// Add 1 for the message structure overhead
	return (len(text) / 4) + 1
}

// countHistoryTokens returns total tokens for all messages.
func (s *SessionStore) countHistoryTokens(msgs []openai.ChatCompletionMessage) int {
	total := 0
	for _, msg := range msgs {
		total += s.countTokens(msg.Content)
	}
	return total
}

// Get returns a copy of the message history for id, creating and seeding it on
// first access. History is trimmed to maxContextTokens if configured.
// The returned slice is a copy so callers can safely append.
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

	// Trim history if token limit is configured
	if s.maxContextTokens > 0 {
		trimmed := s.trimToTokenLimit(msgs)
		s.sessions[id] = trimmed
		return append([]openai.ChatCompletionMessage(nil), trimmed...)
	}

	return append([]openai.ChatCompletionMessage(nil), msgs...)
}

// trimToTokenLimit drops oldest messages (preserving system prompt) until under limit.
func (s *SessionStore) trimToTokenLimit(msgs []openai.ChatCompletionMessage) []openai.ChatCompletionMessage {
	if len(msgs) <= 1 || s.maxContextTokens <= 0 {
		return msgs
	}

	start := len(msgs)
	for i := len(msgs) - 1; i >= 1; i-- {
		candidate := append([]openai.ChatCompletionMessage{msgs[0]}, msgs[i:]...)
		if s.countHistoryTokens(candidate) > s.maxContextTokens {
			break
		}
		start = i
	}

	result := []openai.ChatCompletionMessage{msgs[0]}
	if start < len(msgs) {
		result = append(result, msgs[start:]...)
	}
	return result
}

// Set replaces the stored history for id with a copy of msgs.
func (s *SessionStore) Set(id string, msgs []openai.ChatCompletionMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[id] = append([]openai.ChatCompletionMessage(nil), msgs...)
}

// Reset clears the conversation history for id, keeping only the system prompt.
func (s *SessionStore) Reset(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[id] = []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: s.systemPrompt},
	}
}
