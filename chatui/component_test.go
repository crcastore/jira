package chatui

import (
	"strings"
	"testing"

	corechat "github.com/ccastorena/jira-agent/chat"
)

func TestWidgetRendersFormAndEndpoint(t *testing.T) {
	c := New()
	html, err := c.Widget(WidgetData{
		Endpoint: "/chat",
		LogID:    "chat-log",
		Greeting: "hello",
		Model:    "llama3.1:8b",
		Models:   []string{"llama3.1:8b", "qwen2.5"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := string(html)

	for _, want := range []string{
		`hx-post="/chat"`,
		`hx-target="#chat-log"`,
		`id="chat-log"`,
		`<select name="model"`,
		`<option value="llama3.1:8b" selected>llama3.1:8b</option>`,
		`<option value="qwen2.5">qwen2.5</option>`,
		`name="prompt"`,
		`>hello<`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("widget missing %q\n---\n%s", want, got)
		}
	}
}

func TestWidgetAppliesDefaults(t *testing.T) {
	c := New()
	html, err := c.Widget(WidgetData{Endpoint: "/c"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := string(html)
	if !strings.Contains(got, `id="chat-log"`) {
		t.Errorf("default LogID not applied: %s", got)
	}
	if !strings.Contains(got, `hx-target="#chat-log"`) {
		t.Errorf("default target not applied: %s", got)
	}
}

func TestWidgetCustomLogID(t *testing.T) {
	c := New()
	html, _ := c.Widget(WidgetData{Endpoint: "/c", LogID: "support-log"})
	got := string(html)
	if !strings.Contains(got, `id="support-log"`) || !strings.Contains(got, `hx-target="#support-log"`) {
		t.Errorf("custom LogID not honored: %s", got)
	}
	// The init script must reference the same id (JS-quoted by html/template).
	if !strings.Contains(got, `getElementById("support-log")`) {
		t.Errorf("init script does not target custom log id: %s", got)
	}
}

func TestWidgetWithoutModelsUsesHiddenInput(t *testing.T) {
	c := New()
	html, _ := c.Widget(WidgetData{Endpoint: "/c", Model: "solo"})
	got := string(html)
	if strings.Contains(got, "<select") {
		t.Errorf("did not expect a select with a single model: %s", got)
	}
	if !strings.Contains(got, `<input type="hidden" name="model" value="solo">`) {
		t.Errorf("expected hidden model input: %s", got)
	}
}

func TestWidgetWithResetEndpointRendersResetButton(t *testing.T) {
	c := New()
	html, err := c.Widget(WidgetData{Endpoint: "/chat", ResetEndpoint: "/api/reset"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := string(html)
	for _, want := range []string{
		`class="hx-chat-reset"`,
		`data-endpoint="/api/reset"`,
		`hx-chat:reset`,
		`>Reset</button>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("reset widget missing %q\n---\n%s", want, got)
		}
	}
}

func TestChunkRendersBubblesAndTools(t *testing.T) {
	c := New()
	html, err := c.Chunk(Turn{
		Prompt:    "find issues",
		Assistant: "here you go",
		Events:    []Event{{Name: "search_issues", Args: `{"jql":"x"}`, Result: "3 results"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := string(html)
	for _, want := range []string{
		`hx-chat-bubble user`,
		`find issues`,
		`hx-chat-bubble assistant`,
		`here you go`,
		`Tools used: 1`,
		`<summary>search_issues</summary>`,
		`3 results`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("chunk missing %q\n---\n%s", want, got)
		}
	}
}

func TestChunkEscapesUserInput(t *testing.T) {
	c := New()
	html, _ := c.Chunk(Turn{Prompt: `<script>alert(1)</script>`, Assistant: "ok"})
	got := string(html)
	if strings.Contains(got, "<script>alert(1)</script>") {
		t.Errorf("user input was not escaped: %s", got)
	}
	if !strings.Contains(got, "&lt;script&gt;") {
		t.Errorf("expected HTML-escaped prompt: %s", got)
	}
}

func TestFromChatTurnAdaptsCoreTurn(t *testing.T) {
	turn := FromChatTurn("hello", corechat.Turn{
		Reply:  "done",
		Events: []corechat.ToolEvent{{Name: "search", Args: `{}`, Result: "ok"}},
	})
	if turn.Prompt != "hello" || turn.Assistant != "done" {
		t.Fatalf("unexpected turn text: %+v", turn)
	}
	if len(turn.Events) != 1 || turn.Events[0].Name != "search" || turn.Events[0].Result != "ok" {
		t.Fatalf("unexpected events: %+v", turn.Events)
	}
}

func TestStyleTagIsScoped(t *testing.T) {
	style := string(StyleTag())
	if !strings.HasPrefix(style, "<style>") || !strings.HasSuffix(style, "</style>") {
		t.Errorf("StyleTag should be wrapped in a style tag: %s", style[:20])
	}
	if !strings.Contains(style, ".hx-chat") {
		t.Errorf("styles should be scoped under .hx-chat")
	}
}

func TestWidgetShowsWorkingIndicator(t *testing.T) {
	c := New()
	html, err := c.Widget(WidgetData{Endpoint: "/chat"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := string(html)
	for _, want := range []string{
		`hx-indicator="#chat-log-working"`,
		`hx-disabled-elt="find input, find select, find button"`,
		`class="hx-chat-working htmx-indicator" id="chat-log-working" role="status"`,
		`hx-chat-typing`,
		`Working`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("widget missing working indicator markup %q\n---\n%s", want, got)
		}
	}
}

func TestWorkingIndicatorIsHiddenByDefault(t *testing.T) {
	// The indicator must be hidden until htmx adds the htmx-request class.
	css := string(CSS())
	if !strings.Contains(css, ".hx-chat-working {") || !strings.Contains(css, "display: none;") {
		t.Errorf("expected .hx-chat-working to default to display:none")
	}
	if !strings.Contains(css, ".hx-chat-working.htmx-request {") {
		t.Errorf("expected a rule to reveal the indicator during a request")
	}
}

func TestWidgetHandlesRequestFailures(t *testing.T) {
	c := New()
	html, err := c.Widget(WidgetData{Endpoint: "/chat"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := string(html)
	for _, want := range []string{
		`function clearBusy()`,
		`working.classList.remove("htmx-request")`,
		`htmx:responseError`,
		`Chat request failed`,
		`htmx:sendError`,
		`Could not reach the chat server.`,
		`hx-chat-error`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("widget missing failure handling %q\n---\n%s", want, got)
		}
	}

	css := string(CSS())
	if !strings.Contains(css, ".hx-chat-bubble.hx-chat-error {") {
		t.Errorf("expected error bubble styling")
	}
}
