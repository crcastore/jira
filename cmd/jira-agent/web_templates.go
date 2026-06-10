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
      --hxc-border: var(--border);
      --hxc-muted: var(--muted);
      --hxc-ink: var(--ink);
      --hxc-bubble-user: var(--bubble-user);
      --hxc-bubble-assistant: var(--bubble-assistant);
      --hxc-accent: linear-gradient(135deg, var(--accent), var(--accent-2));
      --hxc-accent-dot: var(--accent-2);
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      font-family: "SF Pro Text", "Segoe UI", sans-serif;
      color: var(--ink);
      background: radial-gradient(circle at 15% 0%, #e0f2fe 0%, transparent 40%), var(--bg);
    }
    .top-shell {
      max-width: 1280px;
      margin: 18px auto 0;
      padding: 0 16px;
    }
    .nav-tabs {
      display: flex;
      gap: 8px;
      align-items: center;
    }
    .nav-tab {
      display: inline-flex;
      align-items: center;
      min-height: 36px;
      padding: 8px 12px;
      border: 1px solid var(--border);
      border-radius: 8px;
      background: #fff;
      color: var(--ink);
      font-size: 13px;
      font-weight: 700;
      text-decoration: none;
    }
    .nav-tab.active {
      border-color: transparent;
      background: linear-gradient(135deg, var(--accent), var(--accent-2));
      color: #fff;
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
    .card-toolbar {
      display: flex;
      justify-content: flex-end;
      margin-bottom: 10px;
    }
    .card-toolbar button,
    .issue-form button {
      padding: 8px 12px;
    }
    .issue-form {
      display: grid;
      gap: 10px;
    }
    .field-grid {
      display: grid;
      grid-template-columns: 1fr 1fr;
      gap: 10px;
    }
    .field {
      display: grid;
      gap: 4px;
      font-size: 12px;
      color: var(--muted);
    }
    .field input,
    .field select,
    .field textarea {
      width: 100%;
      border: 1px solid var(--border);
      border-radius: 8px;
      padding: 8px 10px;
      color: var(--ink);
      font: inherit;
      font-size: 13px;
      background: #fff;
    }
    .field textarea {
      min-height: 86px;
      resize: vertical;
    }
    .form-result { min-height: 18px; }
    .form-working { display: none; }
    .form-working.htmx-request { display: block; }
    .notice { color: #166534; font-size: 12px; }
    .notice a { color: #166534; font-weight: 700; text-decoration: none; }
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
    .token-controls {
      display: flex;
      gap: 8px;
      align-items: center;
      padding: 0 12px 8px;
      border-top: 1px solid var(--border);
      font-size: 12px;
    }
    .token-controls input {
      width: 70px;
      padding: 4px 6px;
      border: 1px solid var(--border);
      border-radius: 6px;
      font-size: 12px;
    }
    .token-controls button {
      padding: 4px 8px;
      background: var(--accent);
      color: white;
      border: none;
      border-radius: 6px;
      cursor: pointer;
      font-size: 11px;
    }
    .token-controls button:hover { background: var(--accent-2); }
    @media (max-width: 980px) {
      .layout { grid-template-columns: 1fr; }
      .chat { min-height: 60vh; }
      .card-body { max-height: none; }
      .field-grid { grid-template-columns: 1fr; }
    }
  </style>
  {{.ChatStyles}}
</head>
<body>
  <div class="top-shell">
    <nav class="nav-tabs" aria-label="Primary">
      <a class="nav-tab active" href="/">Dashboard</a>
      <a class="nav-tab" href="/jira/create">Create Jira Issue</a>
    </nav>
  </div>
  <div class="layout">
    <section class="panel chat">
      <header class="chat-head">
        <h1>Jira + GitHub Agent</h1>
        <p>Model: {{.Model}} | GitHub: {{if .GitHubReady}}enabled{{else}}disabled{{end}}</p>
      </header>

      {{.ChatWidget}}
      <div class="token-controls">
        <span>Tokens:</span>
        <span id="token-display">--</span>
        <input type="number" id="token-input" value="{{.MaxContextTokens}}" min="100" max="32000">
        <button type="button" id="token-btn">Set Limit</button>
      </div>
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
          <a class="nav-tab" href="/jira/create">Create</a>
        </div>
        <div class="card-body">
          <div class="card-toolbar"><button hx-get="/partials/jira-issues" hx-target="#jira-body" hx-swap="innerHTML">Refresh</button></div>
          <div id="jira-body" hx-get="/partials/jira-issues" hx-trigger="load, every 90s" hx-swap="innerHTML">
            <div class="tiny">Loading issues...</div>
          </div>
        </div>
      </section>
    </aside>
  </div>

  <script>
    (function() {
      var tokenDisplay = document.getElementById('token-display');
      var tokenInput = document.getElementById('token-input');
      var tokenBtn = document.getElementById('token-btn');

      function updateTokenDisplay() {
        fetch('/api/token-limit')
          .then(function(r) { return r.json(); })
          .then(function(data) {
            tokenDisplay.textContent = data.current_tokens + ' / ' + data.max_tokens;
            tokenInput.value = data.max_tokens;
          })
          .catch(function(e) { console.log('Token fetch failed:', e); });
      }

      updateTokenDisplay();

      document.body.addEventListener('htmx:afterSwap', function(event) {
        if (event.target && event.target.id === 'chat-log') {
          updateTokenDisplay();
        }
      });
      document.body.addEventListener('hx-chat:reset', updateTokenDisplay);

      tokenBtn.addEventListener('click', function() {
        var maxTokens = parseInt(tokenInput.value);
        if (isNaN(maxTokens) || maxTokens <= 0) {
          alert('Invalid token limit');
          return;
        }
        fetch('/api/token-limit', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({max_tokens: maxTokens})
        })
          .then(function(r) { return r.json(); })
          .then(function(data) {
            if (data.status === 'ok') {
              tokenInput.value = data.max_tokens;
              updateTokenDisplay();
            }
          })
          .catch(function(e) { alert('Failed to update token limit: ' + e); });
      });

    })();
  </script>
</body>
</html>`))

var createIssuePageTmpl = template.Must(template.New("create-issue-page").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Create Jira Issue</title>
  <style>
    :root {
      --bg: #f6f7f4;
      --panel: #ffffff;
      --ink: #1f2937;
      --muted: #6b7280;
      --border: #d1d5db;
      --accent: #115e59;
      --accent-2: #0f766e;
      --error: #9f1239;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      font-family: "SF Pro Text", "Segoe UI", sans-serif;
      color: var(--ink);
      background: radial-gradient(circle at 15% 0%, #e0f2fe 0%, transparent 40%), var(--bg);
    }
    .top-shell,
    .create-layout {
      max-width: 880px;
      margin: 18px auto 0;
      padding: 0 16px;
    }
    .create-layout { margin-top: 14px; padding-bottom: 32px; }
    .nav-tabs {
      display: flex;
      gap: 8px;
      align-items: center;
    }
    .nav-tab {
      display: inline-flex;
      align-items: center;
      min-height: 36px;
      padding: 8px 12px;
      border: 1px solid var(--border);
      border-radius: 8px;
      background: #fff;
      color: var(--ink);
      font-size: 13px;
      font-weight: 700;
      text-decoration: none;
    }
    .nav-tab.active {
      border-color: transparent;
      background: linear-gradient(135deg, var(--accent), var(--accent-2));
      color: #fff;
    }
    .panel {
      background: var(--panel);
      border: 1px solid var(--border);
      border-radius: 14px;
      box-shadow: 0 8px 24px rgba(0,0,0,0.05);
    }
    .page-head {
      padding: 16px;
      border-bottom: 1px solid var(--border);
    }
    .page-head h1 {
      margin: 0;
      font-size: 22px;
    }
    .page-body { padding: 16px; }
    .issue-form {
      display: grid;
      gap: 12px;
    }
    .field-grid {
      display: grid;
      grid-template-columns: 1fr 1fr;
      gap: 12px;
    }
    .field {
      display: grid;
      gap: 5px;
      font-size: 12px;
      color: var(--muted);
    }
    .field input,
    .field select,
    .field textarea {
      width: 100%;
      border: 1px solid var(--border);
      border-radius: 8px;
      padding: 9px 10px;
      color: var(--ink);
      font: inherit;
      font-size: 14px;
      background: #fff;
    }
    .field textarea {
      min-height: 140px;
      resize: vertical;
    }
    button {
      justify-self: start;
      border: 0;
      border-radius: 10px;
      background: linear-gradient(135deg, var(--accent), var(--accent-2));
      color: #fff;
      font-weight: 700;
      padding: 10px 14px;
      cursor: pointer;
    }
    .warn { color: var(--error); font-size: 13px; }
    .notice { color: #166534; font-size: 13px; }
    .notice a { color: #166534; font-weight: 700; text-decoration: none; }
    @media (max-width: 720px) {
      .field-grid { grid-template-columns: 1fr; }
    }
  </style>
</head>
<body>
  <div class="top-shell">
    <nav class="nav-tabs" aria-label="Primary">
      <a class="nav-tab" href="/">Dashboard</a>
      <a class="nav-tab active" href="/jira/create">Create Jira Issue</a>
    </nav>
  </div>
  <main class="create-layout">
    <section class="panel">
      <header class="page-head"><h1>Create Jira Issue</h1></header>
      <div class="page-body">
        {{if .Result.Err}}<div class="warn">{{.Result.Err}}</div>{{end}}
        {{if .Result.Key}}<div class="notice">Created <a href="{{.Result.URL}}" target="_blank" rel="noreferrer">{{.Result.Key}}</a></div>{{end}}
        {{if .ProjectsErr}}<div class="warn">Could not load Jira projects: {{.ProjectsErr}}</div>{{end}}
        {{if .AssigneesErr}}<div class="warn">Could not load Jira assignees: {{.AssigneesErr}}</div>{{end}}
        <form class="issue-form" action="/jira/create" method="post">
          <div class="field-grid">
            <label class="field">Project{{if .Projects}}<select name="project_key" required>{{range .Projects}}<option value="{{.Key}}"{{if eq .Key $.SelectedProject}} selected{{end}}>{{.Key}} - {{.Name}}</option>{{end}}</select>{{else}}<input name="project_key" type="text" autocomplete="off" required>{{end}}</label>
            <label class="field">Issue type<select name="issue_type"><option>Task</option><option>Bug</option><option>Story</option><option>Epic</option></select></label>
          </div>
          <label class="field">Summary<input name="summary" type="text" autocomplete="off" required></label>
          <label class="field">Description<textarea name="description"></textarea></label>
          <div class="field-grid">
            <label class="field">Priority<select name="priority"><option value="">None</option><option>Highest</option><option>High</option><option>Medium</option><option>Low</option><option>Lowest</option></select></label>
            <label class="field">Labels<input name="labels" type="text" autocomplete="off"></label>
          </div>
          <div class="field-grid">
            <label class="field">Assignee<select name="assignee_account_id"><option value="">Unassigned</option>{{range .Assignees}}<option value="{{.AccountID}}">{{.DisplayName}}</option>{{end}}</select></label>
            <label class="field">Reporter<select name="reporter_account_id"><option value="">Default</option>{{range .Assignees}}<option value="{{.AccountID}}">{{.DisplayName}}</option>{{end}}</select></label>
          </div>
          <button type="submit">Create Issue</button>
        </form>
      </div>
    </section>
  </main>
</body>
</html>`))

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
{{else if not .Err}}
<div class="tiny">No Jira issues returned.</div>
{{end}}
`))
