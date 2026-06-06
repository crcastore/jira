// Package chatui renders a self-contained, drop-in HTMX chat widget. The widget
// is parameterized by the HTMX endpoint it posts to and the DOM id of its
// message log, so it can be embedded into any HTMX page (not just this app's
// dashboard). All styles are scoped under the ".hx-chat" class to avoid
// clashing with a host page's CSS.
package chatui

import (
	"bytes"
	"html/template"
	"io"
)

// Event is a single tool invocation surfaced in the transcript.
type Event struct {
	Name   string
	Args   string
	Result string
}

// Turn is one user/assistant exchange to append to the chat log.
type Turn struct {
	Prompt    string
	Assistant string
	Events    []Event
}

// WidgetData configures a rendered chat widget.
type WidgetData struct {
	// Endpoint is the HTMX hx-post target that returns a chat chunk. Required.
	Endpoint string
	// LogID is the DOM id of the message log container. Defaults to "chat-log".
	LogID string
	// Greeting is the first assistant bubble shown before any exchange.
	Greeting string
	// Placeholder is the input placeholder text.
	Placeholder string
	// Model is the currently selected model; also used as the sole option when
	// Models is empty.
	Model string
	// Models, when non-empty, renders a model picker <select>.
	Models []string
	// ModelsErr, when set, renders a small note under the form.
	ModelsErr string
}

func (d *WidgetData) applyDefaults() {
	if d.LogID == "" {
		d.LogID = "chat-log"
	}
	if d.Greeting == "" {
		d.Greeting = "Ask me anything."
	}
	if d.Placeholder == "" {
		d.Placeholder = "Type a message..."
	}
}

// Component renders the chat widget and per-turn chunks.
type Component struct {
	tmpl *template.Template
}

// New returns a ready-to-use Component.
func New() *Component {
	return &Component{tmpl: template.Must(template.New("chatui").Parse(widgetTmpl + chunkTmpl))}
}

// RenderWidget writes the chat widget markup to w.
func (c *Component) RenderWidget(w io.Writer, d WidgetData) error {
	d.applyDefaults()
	return c.tmpl.ExecuteTemplate(w, "chat-widget", d)
}

// Widget returns the chat widget markup as template.HTML for embedding into a
// host page template (e.g. {{.ChatWidget}}).
func (c *Component) Widget(d WidgetData) (template.HTML, error) {
	var buf bytes.Buffer
	if err := c.RenderWidget(&buf, d); err != nil {
		return "", err
	}
	return template.HTML(buf.String()), nil
}

// RenderChunk writes a single user/assistant exchange to w, suitable for an
// hx-swap="beforeend" response.
func (c *Component) RenderChunk(w io.Writer, t Turn) error {
	return c.tmpl.ExecuteTemplate(w, "chat-chunk", t)
}

// Chunk returns a single exchange as template.HTML.
func (c *Component) Chunk(t Turn) (template.HTML, error) {
	var buf bytes.Buffer
	if err := c.RenderChunk(&buf, t); err != nil {
		return "", err
	}
	return template.HTML(buf.String()), nil
}

// StyleTag returns the component's scoped CSS wrapped in a <style> tag, ready to
// drop into a host page's <head>.
func StyleTag() template.HTML {
	return template.HTML("<style>" + componentCSS + "</style>")
}

// CSS returns the raw scoped CSS for callers that manage their own <style>
// tags or bundle stylesheets.
func CSS() template.CSS {
	return template.CSS(componentCSS)
}

const widgetTmpl = `{{define "chat-widget"}}
<div class="hx-chat">
  <div class="hx-chat-log" id="{{.LogID}}">
    <div class="hx-chat-bubble assistant">{{.Greeting}}</div>
  </div>
  <form class="hx-chat-form" hx-post="{{.Endpoint}}" hx-target="#{{.LogID}}" hx-swap="beforeend">
    {{if .Models}}
    <select name="model" aria-label="Model">
      {{$sel := .Model}}
      {{range .Models}}<option value="{{.}}"{{if eq . $sel}} selected{{end}}>{{.}}</option>{{end}}
    </select>
    {{else if .Model}}
    <input type="hidden" name="model" value="{{.Model}}">
    {{end}}
    <input class="hx-chat-input" name="prompt" type="text" placeholder="{{.Placeholder}}" autocomplete="off" required>
    <button type="submit">Send</button>
  </form>
  {{if .ModelsErr}}<div class="hx-chat-note">Model list unavailable: {{.ModelsErr}}</div>{{end}}
  <script>
  (function(){
    var log = document.getElementById({{.LogID}});
    if(!log || log.dataset.hxchatInit){ return; }
    log.dataset.hxchatInit = "1";
    var widget = log.closest(".hx-chat");
    var form = widget ? widget.querySelector(".hx-chat-form") : null;
    var input = form ? form.querySelector('input[name="prompt"]') : null;
    document.body.addEventListener("htmx:afterSwap", function(e){
      if(e.target && e.target.id === {{.LogID}}){
        log.scrollTop = log.scrollHeight;
        if(input){ input.value = ""; input.focus(); }
      }
    });
  })();
  </script>
</div>
{{end}}`

const chunkTmpl = `{{define "chat-chunk"}}
<div class="hx-chat-bubble user">{{.Prompt}}</div>
<div class="hx-chat-bubble assistant">
  {{.Assistant}}
  {{if .Events}}
  <div class="hx-chat-tools">Tools used: {{len .Events}}</div>
  {{range .Events}}
  <details class="hx-chat-tool">
    <summary>{{.Name}}</summary>
    <div><strong>args</strong></div>
    <pre>{{.Args}}</pre>
    <div><strong>result</strong></div>
    <pre>{{.Result}}</pre>
  </details>
  {{end}}
  {{end}}
</div>
{{end}}`

// componentCSS is scoped under .hx-chat so it can drop into any page. Colors
// fall back to sensible defaults but honor host CSS variables when present.
const componentCSS = `
.hx-chat {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
}
.hx-chat-log {
  padding: 14px;
  overflow-y: auto;
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.hx-chat-bubble {
  border-radius: 12px;
  padding: 10px 12px;
  max-width: 100%;
  white-space: pre-wrap;
  line-height: 1.45;
  border: 1px solid var(--hxc-border, #d1d5db);
}
.hx-chat-bubble.user {
  background: var(--hxc-bubble-user, #d1fae5);
  align-self: flex-end;
}
.hx-chat-bubble.assistant {
  background: var(--hxc-bubble-assistant, #fff7ed);
}
.hx-chat-tools {
  margin-top: 8px;
  border-top: 1px dashed var(--hxc-border, #d1d5db);
  padding-top: 8px;
  font-size: 12px;
  color: var(--hxc-muted, #6b7280);
}
.hx-chat-tool {
  margin-top: 6px;
  border: 1px solid var(--hxc-border, #d1d5db);
  border-radius: 10px;
  padding: 8px;
  background: #fafafa;
}
.hx-chat-tool pre {
  white-space: pre-wrap;
  word-break: break-word;
  margin: 6px 0 0;
  font-size: 11px;
  color: #111827;
}
.hx-chat-form {
  display: grid;
  grid-template-columns: auto 1fr auto;
  gap: 10px;
  padding: 12px;
  border-top: 1px solid var(--hxc-border, #d1d5db);
}
.hx-chat-form select,
.hx-chat-form .hx-chat-input {
  border: 1px solid var(--hxc-border, #d1d5db);
  border-radius: 10px;
  padding: 10px 12px;
  font-size: 14px;
  color: var(--hxc-ink, #1f2937);
  background: #fff;
}
.hx-chat-form select { min-width: 180px; }
.hx-chat-form .hx-chat-input { width: 100%; }
.hx-chat-form button {
  border: 0;
  border-radius: 10px;
  background: var(--hxc-accent, linear-gradient(135deg, #115e59, #0f766e));
  color: #fff;
  font-weight: 600;
  padding: 10px 14px;
  cursor: pointer;
}
.hx-chat-note {
  font-size: 12px;
  color: var(--hxc-muted, #6b7280);
  padding: 0 12px 12px;
}
@media (max-width: 680px) {
  .hx-chat-form { grid-template-columns: 1fr; }
}
`
