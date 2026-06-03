package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	openai "github.com/sashabaranov/go-openai"
)

const systemPrompt = `You are a Jira assistant. You help the user search, read, create, comment on,
and transition Jira issues by calling the provided tools.

Rules:
- Prefer calling tools over guessing. Never invent issue keys, accountIds, or transition IDs.
- For status changes: call list_transitions first to learn the valid transition_id, then transition_issue.
- For assigning a user: call search_users to resolve a name/email to an accountId.
- When the user says "me" / "my issues", call myself once to get their accountId, then use
  ` + "`assignee = currentUser()`" + ` in JQL.
- Keep replies short. Show issue keys, summaries, and statuses in compact tables/lists.
- Confirm with the user before destructive or large-batch changes.`

const maxSteps = 12

func main() {
	loadDotEnv(".env")

	jc, err := NewJiraClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v. Copy .env.example to .env and fill it in.\n", err)
		os.Exit(1)
	}

	baseURL := envOr("LLM_BASE_URL", "http://localhost:11434/v1")
	apiKey := firstNonEmpty(os.Getenv("LLM_API_KEY"), os.Getenv("OPENAI_API_KEY"), "ollama")
	model := envOr("LLM_MODEL", "llama3.1")

	cfg := openai.DefaultConfig(apiKey)
	cfg.BaseURL = baseURL
	client := openai.NewClientWithConfig(cfg)

	messages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
	}
	fmt.Printf("Jira agent (model: %s) — type 'exit' to quit.\n\n", model)

	in := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("you> ")
		line, err := in.ReadString('\n')
		if err != nil {
			fmt.Println()
			return
		}
		user := strings.TrimSpace(line)
		if user == "" {
			continue
		}
		switch strings.ToLower(user) {
		case "exit", "quit", ":q":
			return
		}
		messages = append(messages, openai.ChatCompletionMessage{
			Role: openai.ChatMessageRoleUser, Content: user,
		})
		messages = runTurn(client, model, messages, jc)
	}
}

func runTurn(client *openai.Client, model string, messages []openai.ChatCompletionMessage, jc *JiraClient) []openai.ChatCompletionMessage {
	ctx := context.Background()
	for step := 0; step < maxSteps; step++ {
		resp, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
			Model:      model,
			Messages:   messages,
			Tools:      ToolSchemas,
			ToolChoice: "auto",
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "LLM error: %v\n", err)
			return messages
		}
		msg := resp.Choices[0].Message
		messages = append(messages, openai.ChatCompletionMessage{
			Role:      openai.ChatMessageRoleAssistant,
			Content:   msg.Content,
			ToolCalls: msg.ToolCalls,
		})

		if len(msg.ToolCalls) == 0 {
			if msg.Content != "" {
				fmt.Println(msg.Content)
			}
			return messages
		}

		for _, tc := range msg.ToolCalls {
			args := tc.Function.Arguments
			if args == "" {
				args = "{}"
			}
			fmt.Printf("\033[2m→ %s(%s)\033[0m\n", tc.Function.Name, args)
			result := CallTool(jc, tc.Function.Name, args)
			messages = append(messages, openai.ChatCompletionMessage{
				Role:       openai.ChatMessageRoleTool,
				ToolCallID: tc.ID,
				Name:       tc.Function.Name,
				Content:    result,
			})
		}
	}
	fmt.Println("Step limit reached.")
	return messages
}

// ---------- env helpers ----------

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

// loadDotEnv is a minimal .env loader: KEY=VALUE lines, no quoting tricks,
// existing environment wins. Silently ignored if the file is missing.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		k := strings.TrimSpace(line[:eq])
		v := strings.TrimSpace(line[eq+1:])
		v = strings.Trim(v, `"'`)
		if _, exists := os.LookupEnv(k); !exists {
			os.Setenv(k, v)
		}
	}
}
