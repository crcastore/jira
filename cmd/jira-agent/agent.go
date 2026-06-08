package main

import (
	"os"
	"time"

	openai "github.com/sashabaranov/go-openai"

	"github.com/ccastorena/jira-agent/chat"
)

// agentToolBox adapts the main package's Jira/GitHub tools to the chat.ToolBox
// interface so the engine stays decoupled from how tools are implemented.
type agentToolBox struct {
	jc *JiraClient
	gc *GitHubClient
}

func (b agentToolBox) Schemas() []openai.Tool { return ToolSchemas }

func (b agentToolBox) Call(name, args string) string {
	return CallTool(b.jc, b.gc, name, args)
}

// llmConfig holds the resolved LLM connection settings.
type llmConfig struct {
	baseURL string
	apiKey  string
	model   string
	timeout time.Duration
}

// loadLLMConfig reads LLM settings from the environment with sensible defaults.
func loadLLMConfig() llmConfig {
	return llmConfig{
		baseURL: envOr("LLM_BASE_URL", "http://localhost:11434/v1"),
		apiKey:  firstNonEmpty(os.Getenv("LLM_API_KEY"), os.Getenv("OPENAI_API_KEY"), "ollama"),
		model:   envOr("LLM_MODEL", "llama3.1:8b"),
		timeout: time.Duration(envOrInt("WEB_LLM_TIMEOUT_SEC", 180)) * time.Second,
	}
}

// newEngine builds a chat.Engine wired to the OpenAI-compatible endpoint and
// the Jira/GitHub toolbox. Both the web server and the CLI use this.
func newEngine(cfg llmConfig, jc *JiraClient, gc *GitHubClient) *chat.Engine {
	oc := openai.DefaultConfig(cfg.apiKey)
	oc.BaseURL = cfg.baseURL
	return &chat.Engine{
		LLM:             openai.NewClientWithConfig(oc),
		Tools:           agentToolBox{jc: jc, gc: gc},
		DefaultModel:    cfg.model,
		MaxSteps:        maxSteps,
		Timeout:         cfg.timeout,
		ResolveToolName: canonicalToolName,
	}
}
