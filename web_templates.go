package main

import "html/template"

var pageTmpl = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Jira + GitHub Agent</title>
  <script src="https://unpkg.com/htmx.org@1.9.12"></script>
  <style>
    :root {
      --bg: #f6f7f4;
      --panel: #ffffff;
      --ink: #1f2937;
      --muted: #6b7280;
      --border: #d1d5db;
      --accent: #115e59;
      --accent-2: #0f766e;
      --bubble-user: #d1fae5;
      --bubble-assistant: #fff7ed;
      --error: #9f1239;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      font-family: "SF Pro Text", "Segoe UI", sans-serif;
      color: var(--ink);
      background: radial-gradient(circle at 15% 0%, #e0f2fe 0%, transparent 40%), var(--bg);
    }
    .layout {
      display: grid;
      grid-template-columns: 2fr 1fr;
      gap: 16px;
      max-width: 1280px;
      margin: 20px auto;
      padding: 0 16px 24px;
    }
    .panel {
      background: var(--panel);
      border: 1px solid var(--border);
      border-radius: 14px;
      box-shadow: 0 8px 24px rgba(0,0,0,0.05);
    }
    .chat {
      display: flex;
      flex-direction: column;
      min-height: 80vh;
    }
    .chat-head {
      padding: 14px 16px;
      border-bottom: 1px solid var(--border);
    }
    .chat-head h1 {
      margin: 0;
      font-size: 20px;
    }
    .chat-head p {
      margin: 6px 0 0;
      color: var(--muted);
      font-size: 13px;
    }
    #chat-log {
      padding: 14px;
      overflow-y: auto;
      flex: 1;
      display: flex;
      flex-direction: column;
      gap: 10px;
    }
    .bubble {
      border-radius: 12px;
      padding: 10px 12px;
      max-width: 100%;
      white-space: pre-wrap;
      line-height: 1.45;
      border: 1px solid var(--border);
    }
    .user { background: var(--bubble-user); align-self: flex-end; }
    .assistant { background: var(--bubble-assistant); }
    .tool-log {
      margin-top: 8px;
      border-top: 1px dashed var(--border);
      padding-top: 8px;
      font-size: 12px;
      color: var(--muted);
    }
    details {
      margin-top: 6px;
      border: 1px solid var(--border);
      border-radius: 10px;
      padding: 8px;
      background: #fafafa;
    }
    pre {
      white-space: pre-wrap;
      word-break: break-word;
      margin: 6px 0 0;
      font-size: 11px;
      color: #111827;
    }
    form {
      display: grid;
      grid-template-columns: auto 1fr auto;
      gap: 10px;
      padding: 12px;
      border-top: 1px solid var(--border);
    }
    select {
      border: 1px solid var(--border);
      border-radius: 10px;
      padding: 10px 12px;
      font-size: 14px;
      background: #fff;
      color: var(--ink);
      min-width: 180px;
    }
    input[type="text"] {
      width: 100%;
      border: 1px solid var(--border);
      border-radius: 10px;
      padding: 10px 12px;
      font-size: 14px;
    }
    button {
      border: 0;
      border-radius: 10px;
      background: linear-gradient(135deg, var(--accent), var(--accent-2));
      color: #fff;
      font-weight: 600;
      padding: 10px 14px;
      cursor: pointer;
    }
    .side {
      display: flex;
      flex-direction: column;
      gap: 12px;
    }
    .card h2 {
      margin: 0;
      font-size: 15px;
    }
    .card-head {
      display: flex;
      justify-content: space-between;
      align-items: center;
      padding: 12px;
      border-bottom: 1px solid var(--border);
    }
    .card-body {
      padding: 12px;
      max-height: 34vh;
      overflow: auto;
    }
    .list {
      display: grid;
      gap: 8px;
    }
    .item {
      border: 1px solid var(--border);
      border-radius: 10px;
      padding: 8px;
      background: #fcfcfc;
    }
    .item a { color: #0c4a6e; text-decoration: none; }
    .meta { color: var(--muted); font-size: 12px; margin-top: 4px; }
    .warn { color: var(--error); font-size: 12px; }
    .tiny { font-size: 12px; color: var(--muted); }
    .chat-working {
      display: none;
      align-items: center;
      gap: 8px;
      margin: 0 14px 4px;
      padding: 8px 12px;
      align-self: flex-start;
      border-radius: 12px;
      border: 1px solid var(--border);
      background: var(--bubble-assistant);
      font-size: 13px;
      color: var(--muted);
    }
    .chat-working.htmx-request { display: inline-flex; }
    .typing { display: inline-flex; align-items: center; gap: 4px; }
    .typing span {
      width: 6px;
      height: 6px;
      border-radius: 50%;
      background: var(--accent-2);
      opacity: 0.4;
      animation: typing-bounce 1.2s infinite ease-in-out;
    }
    .typing span:nth-child(2) { animation-delay: 0.2s; }
    .typing span:nth-child(3) { animation-delay: 0.4s; }
    @keyframes typing-bounce {
      0%, 80%, 100% { transform: translateY(0); opacity: 0.4; }
      40% { transform: translateY(-4px); opacity: 1; }
    }
    #chat-form.htmx-request button { opacity: 0.6; cursor: progress; }
    @media (max-width: 980px) {
      .layout { grid-template-columns: 1fr; }
      .chat { min-height: 60vh; }
      .card-body { max-height: none; }
    }
  </style>
</head>
<body>
  <div class="layout">
    <section class="panel chat">
      <header class="chat-head">
        <h1>Jira + GitHub Agent</h1>
        <p>Model: {{.Model}} | GitHub: {{if .GitHubReady}}enabled{{else}}disabled{{end}}</p>
      </header>

      <div id="chat-log">
        <div class="bubble assistant">Ask anything about your Jira tickets or GitHub work.</div>
      </div>

      <div id="chat-working" class="chat-working" aria-live="polite">
        <span class="typing"><span></span><span></span><span></span></span>
        <span>Working...</span>
      </div>

      <form id="chat-form" hx-post="/chat" hx-target="#chat-log" hx-swap="beforeend"
            hx-indicator="#chat-working" hx-disabled-elt="find button">
        <select name="model" aria-label="Model" id="model-select">
          {{if .Models}}
            {{range .Models}}
              <option value="{{.}}" {{if eq . $.Model}}selected{{end}}>{{.}}</option>
            {{end}}
          {{else}}
            <option value="{{.Model}}" selected>{{.Model}}</option>
          {{end}}
        </select>
        <input name="prompt" type="text" placeholder="Ask the agent something..." autocomplete="off" required>
        <button type="submit">Send</button>
      </form>
      {{if .ModelsErr}}<div class="tiny" style="padding: 0 12px 12px;">Model list unavailable: {{.ModelsErr}}</div>{{end}}
    </section>

    <aside class="side">
      <section class="panel card">
        <div class="card-head">
          <h2>Available GitHub Repos</h2>
          <button hx-get="/partials/repos" hx-target="#repos-body" hx-swap="innerHTML">Refresh</button>
        </div>
        <div class="card-body" id="repos-body" hx-get="/partials/repos" hx-trigger="load, every 90s" hx-swap="innerHTML">
          <div class="tiny">Loading repos...</div>
        </div>
      </section>

      <section class="panel card">
        <div class="card-head">
          <h2>My Open Jira Issues</h2>
          <button hx-get="/partials/jira-issues" hx-target="#jira-body" hx-swap="innerHTML">Refresh</button>
        </div>
        <div class="card-body" id="jira-body" hx-get="/partials/jira-issues" hx-trigger="load, every 90s" hx-swap="innerHTML">
          <div class="tiny">Loading issues...</div>
        </div>
      </section>
    </aside>
  </div>

  <script>
    (function() {
      var form = document.getElementById('chat-form');
      var input = form.querySelector('input[name="prompt"]');
      var log = document.getElementById('chat-log');

      document.body.addEventListener('htmx:afterSwap', function(event) {
        if (event.target && event.target.id === 'chat-log') {
          log.scrollTop = log.scrollHeight;
          input.value = '';
          input.focus();
        }
      });
    })();
  </script>
</body>
</html>`))

var chatChunkTmpl = template.Must(template.New("chatChunk").Parse(`
<div class="bubble user">{{.Prompt}}</div>
<div class="bubble assistant">
  {{.Assistant}}
  {{if .Events}}
  <div class="tool-log">Tools used: {{len .Events}}</div>
  {{range .Events}}
  <details>
    <summary>{{.Name}}</summary>
    <div><strong>args</strong></div>
    <pre>{{.Args}}</pre>
    <div><strong>result</strong></div>
    <pre>{{.Result}}</pre>
  </details>
  {{end}}
  {{end}}
</div>
`))

var reposTmpl = template.Must(template.New("repos").Parse(`
{{if .Err}}<div class="warn">{{.Err}}</div>{{end}}
{{if .Repos}}
<div class="list">
  {{range .Repos}}
  <div class="item">
    <a href="{{.URL}}" target="_blank" rel="noreferrer">{{.FullName}}</a>
    <div class="meta">updated {{.Updated}} {{if .Private}}| private{{end}}</div>
  </div>
  {{end}}
</div>
{{else}}
<div class="tiny">No repositories returned.</div>
{{end}}
`))

var issuesTmpl = template.Must(template.New("issues").Parse(`
{{if .Err}}<div class="warn">{{.Err}}</div>{{end}}
{{if .Issues}}
<div class="list">
  {{range .Issues}}
  <div class="item">
    <div><strong>{{.Key}}</strong> - {{.Summary}}</div>
    <div class="meta">{{.Status}} | {{.Assignee}} | updated {{.Updated}}</div>
  </div>
  {{end}}
</div>
{{else}}
<div class="tiny">No Jira issues returned.</div>
{{end}}
`))
