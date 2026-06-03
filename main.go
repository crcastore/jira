package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	openai "github.com/sashabaranov/go-openai"
)

const systemPrompt = `You are an engineering assistant for Jira and GitHub. You help the user search,
read, create, comment on, and transition issues and pull requests by calling the provided tools.

Jira tools are unprefixed (search_issues, get_issue, ...). GitHub tools are prefixed with gh_.

Rules:
- Prefer calling tools over guessing. Never invent issue keys, accountIds, transition IDs,
  GitHub owners/repos/PR numbers, or branch names.
- Jira status changes: call list_transitions first, then transition_issue.
- Jira user assignment: call search_users to resolve a name/email to an accountId.
- When the user says "me" / "my issues" in Jira context, call myself once and use
  ` + "`assignee = currentUser()`" + ` in JQL. In GitHub context use ` + "`author:@me`" + ` or
  ` + "`assignee:@me`" + ` in search queries, or call gh_me.
- For GitHub, owner+repo are required for most tools. If the user only gives a repo name,
  ask for the owner unless it's obvious from context.
- Keep replies short. Show keys/numbers, titles, and statuses in compact tables/lists.
- When a tool returns a list, show every item it returned — do not summarize, truncate, or pick a few. If the count is large, present a compact table with all rows.
- Confirm with the user before destructive or large-batch changes (merging PRs, closing many
  issues, transitioning across many tickets, etc.).`

const maxSteps = 12

func main() {
	loadDotEnv(".env")

	jc, err := NewJiraClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v. Copy .env.example to .env and fill it in.\n", err)
		os.Exit(1)
	}

	gc, gerr := NewGitHubClient()
	if gerr != nil {
		fmt.Fprintf(os.Stderr, "GitHub disabled: %v\n", gerr)
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
	ghStatus := "off"
	if gc != nil {
		ghStatus = "on"
	}
	fmt.Printf("Jira + GitHub agent (model: %s, github: %s) — type 'exit' to quit.\n\n", model, ghStatus)

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
		messages = runTurn(client, model, messages, jc, gc)
	}
}

func runTurn(client *openai.Client, model string, messages []openai.ChatCompletionMessage, jc *JiraClient, gc *GitHubClient) []openai.ChatCompletionMessage {
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
			result := CallTool(jc, gc, tc.Function.Name, args)
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

// loadDotEnv is a minimal .env loader: KEY=VALUE lines, no quoting tricks.
// Values from .env override the existing process environment so a stale shell
// export doesn't shadow what's in the file. Silently ignored if the file is missing.
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
		os.Setenv(k, v)
	}
}
