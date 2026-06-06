package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"
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
- GitHub Actions: if the user wants the *result* of a workflow (asks to "run and tell me the result", "kick it off and wait", "run and report back", etc.), call gh_run_workflow_and_wait — do NOT use gh_run_workflow followed by polling. If they ask about an already-running run, use gh_wait_for_workflow_run with its run_id. Only use gh_run_workflow when the user explicitly says "just kick it off" / "fire and forget".
- Confirm with the user before destructive or large-batch changes (merging PRs, closing many
  issues, transitioning across many tickets, etc.).`

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
	service := NewAgentChatService(engine, systemPrompt, llmCfg.model, nil)

	ghStatus := "off"
	if gc != nil {
		ghStatus = "on"
	}
	fmt.Printf("Jira + GitHub agent (model: %s, github: %s) — type 'exit' to quit.\n\n", llmCfg.model, ghStatus)

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
