package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ccastorena/jira-agent/agentcore"
)

const systemPrompt = `You are an engineering assistant for a focused GitHub issue workflow. You help the user
discover repositories, read issues and issue-like pull request records, inspect files changed in pull requests / merge requests (MRs), create issues, close issues,
and comment on issues or pull requests by calling the provided tools.

The currently exposed tools are: gh_me, gh_list_my_repos, gh_get_repo, gh_list_issues, gh_get_issue,
gh_list_pr_files, gh_create_issue, gh_close_issue, and gh_comment_issue.

Rules:
- Prefer calling tools over guessing. Never invent GitHub owners, repos, issue numbers, pull request
  numbers, assignees, labels, or branch names.
- When the user says "me" or asks for their own GitHub identity, call gh_me.
- When the user asks which files changed in a PR, pull request, MR, or merge request, use gh_list_pr_files.
- For GitHub, owner+repo are required for most tools. If the user only gives a repo name,
  ask for the owner unless it's obvious from context.
- If the user asks for Jira work, pull request creation/review/merge, global issue search, workflow
  runs, or other operations outside the exposed tool set, say that operation is not enabled in the
  current tool set.
- Keep replies short. Show keys/numbers, titles, and statuses in compact tables/lists.
- When a tool returns a list, show every item it returned — do not summarize, truncate, or pick a few. If the count is large, present a compact table with all rows.
- Confirm with the user before destructive or large-batch changes, including closing issues.`

const maxSteps = 12

func main() {
	if len(os.Args) > 1 {
		switch strings.ToLower(os.Args[1]) {
		case "serve", "web":
			serveWeb()
			return
		}
	}

	runCLI()
}

func runCLI() {
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

	llmCfg := loadLLMConfig()
	engine := newEngine(llmCfg, jc, gc)
	onStepStart, onStepEnd := startSpinner("thinking")
	engine.OnStepStart = onStepStart
	engine.OnStepEnd = onStepEnd
	engine.OnToolCall = func(name, args string) {
		fmt.Printf("\033[2m→ %s(%s)\033[0m\n", name, args)
	}
	service := agentcore.NewAgentChatService(engine, systemPrompt, llmCfg.model, nil)

	ghStatus := "off"
	if gc != nil {
		ghStatus = "on"
	}
	fmt.Printf("GitHub issue agent (model: %s, github: %s) — type 'exit' to quit.\n\n", llmCfg.model, ghStatus)

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

		turn, err := service.RunTurn(context.Background(), "cli", user, "")
		if err != nil {
			fmt.Fprintf(os.Stderr, "LLM error: %v\n", err)
			continue
		}
		if turn.Reply != "" {
			fmt.Println(turn.Reply)
		}
	}
}

// startSpinner prints an animated indicator with elapsed seconds until the
// returned stop function is called. Output goes to stderr so it doesn't get
// mixed into captured stdout.
func startSpinner(label string) (onStart func(), onEnd func()) {
	if !isTerminal(os.Stderr) {
		return func() {}, func() {}
	}
	running := false
	var done chan struct{}

	startFn := func() {
		if running {
			return
		}
		running = true
		done = make(chan struct{})
		localDone := done
		go func() {
			frames := []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}
			start := time.Now()
			i := 0
			t := time.NewTicker(120 * time.Millisecond)
			defer t.Stop()
			for {
				select {
				case <-localDone:
					fmt.Fprint(os.Stderr, "\r\033[K")
					return
				case <-t.C:
					fmt.Fprintf(os.Stderr, "\r\033[2m%c %s… %ds\033[0m", frames[i%len(frames)], label, int(time.Since(start).Seconds()))
					i++
				}
			}
		}()
	}

	endFn := func() {
		if !running {
			return
		}
		running = false
		close(done)
	}

	return startFn, endFn
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
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
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		v = strings.Trim(v, `"'`)
		os.Setenv(k, v)
	}
}
