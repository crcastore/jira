use axum::{
    extract::State,
    http::{header, HeaderMap, HeaderValue, StatusCode},
    response::{Html, IntoResponse, Response},
    routing::{get, post},
    Json, Router,
};
use reqwest::Client;
use serde::{Deserialize, Serialize};
use std::{collections::HashMap, env, net::SocketAddr, sync::Arc};
use tokio::{net::TcpListener, sync::Mutex};
use uuid::Uuid;

const DEFAULT_ADDR: &str = "0.0.0.0:8080";
const DEFAULT_PUBLIC_ADDR: &str = "localhost:8080";
const DEFAULT_LLM_BASE_URL: &str = "http://localhost:11434/v1";
const DEFAULT_LLM_API_KEY: &str = "ollama";
const DEFAULT_LLM_MODEL: &str = "llama3.1:8b";
const SESSION_COOKIE: &str = "url_chat_session";
const SYSTEM_PROMPT: &str =
    "You are a helpful chat assistant. Answer directly and do not use Jira or GitHub tools.";

#[derive(Clone)]
struct AppState {
    config: Config,
    http: Client,
    sessions: Arc<Mutex<HashMap<String, Vec<ChatMessage>>>>,
}

#[derive(Clone)]
struct Config {
    listen_addr: String,
    public_addr: String,
    llm_base_url: String,
    llm_api_key: String,
    llm_model: String,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
struct ChatMessage {
    role: String,
    content: String,
}

#[derive(Debug, Deserialize)]
struct ChatRequest {
    message: String,
    model: Option<String>,
}

#[derive(Debug, Serialize)]
struct ChatResponse {
    reply: String,
    model: String,
}

#[derive(Debug, Serialize)]
struct OpenAIChatRequest {
    model: String,
    messages: Vec<ChatMessage>,
}

#[derive(Debug, Deserialize)]
struct OpenAIChatResponse {
    choices: Vec<OpenAIChoice>,
}

#[derive(Debug, Deserialize)]
struct OpenAIChoice {
    message: ChatMessage,
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    dotenvy::dotenv().ok();

    let config = Config::from_env();
    let state = AppState {
        config: config.clone(),
        http: Client::new(),
        sessions: Arc::new(Mutex::new(HashMap::new())),
    };

    let app = Router::new()
        .route("/", get(index))
        .route("/api/chat", post(chat))
        .route("/api/reset", post(reset))
        .route("/healthz", get(|| async { "ok" }))
        .with_state(state);

    let listener = TcpListener::bind(&config.listen_addr).await?;
    println!("Chat running at http://{}", config.public_addr);
    axum::serve(listener, app).await?;

    Ok(())
}

impl Config {
    fn from_env() -> Self {
        let raw_addr = env_or("WEB_ADDR", DEFAULT_ADDR);
        let listen_addr = normalize_listen_addr(&raw_addr);
        let public_addr = public_addr_from_listen_addr(&listen_addr)
            .unwrap_or_else(|| DEFAULT_PUBLIC_ADDR.to_string());

        Self {
            listen_addr,
            public_addr,
            llm_base_url: env_or("LLM_BASE_URL", DEFAULT_LLM_BASE_URL),
            llm_api_key: env_or("LLM_API_KEY", DEFAULT_LLM_API_KEY),
            llm_model: env_or("LLM_MODEL", DEFAULT_LLM_MODEL),
        }
    }

    fn chat_url(&self) -> String {
        format!(
            "{}/chat/completions",
            self.llm_base_url.trim_end_matches('/')
        )
    }
}

async fn index(State(state): State<AppState>, headers: HeaderMap) -> Response {
    let session_id = session_id_from_headers(&headers).unwrap_or_else(new_session_id);
    let mut response = Html(index_html(&state.config.llm_model)).into_response();
    set_session_cookie(response.headers_mut(), &session_id);
    response
}

async fn chat(
    State(state): State<AppState>,
    headers: HeaderMap,
    Json(request): Json<ChatRequest>,
) -> Response {
    let user_message = request.message.trim();
    if user_message.is_empty() {
        return api_error(StatusCode::BAD_REQUEST, "message is required");
    }

    let session_id = session_id_from_headers(&headers).unwrap_or_else(new_session_id);
    let model = request
        .model
        .as_deref()
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .unwrap_or(&state.config.llm_model)
        .to_string();

    let mut messages = {
        let mut sessions = state.sessions.lock().await;
        sessions
            .entry(session_id.clone())
            .or_insert_with(|| {
                vec![ChatMessage {
                    role: "system".to_string(),
                    content: SYSTEM_PROMPT.to_string(),
                }]
            })
            .clone()
    };

    messages.push(ChatMessage {
        role: "user".to_string(),
        content: user_message.to_string(),
    });

    let llm_response = match call_llm(&state, &model, &messages).await {
        Ok(reply) => reply,
        Err(error) => {
            return api_error(
                StatusCode::BAD_GATEWAY,
                &format!("LLM request failed: {error}"),
            )
        }
    };

    messages.push(ChatMessage {
        role: "assistant".to_string(),
        content: llm_response.clone(),
    });

    {
        let mut sessions = state.sessions.lock().await;
        sessions.insert(session_id.clone(), trim_history(messages));
    }

    let mut response = Json(ChatResponse {
        reply: llm_response,
        model,
    })
    .into_response();
    set_session_cookie(response.headers_mut(), &session_id);
    response
}

async fn reset(State(state): State<AppState>, headers: HeaderMap) -> Response {
    let session_id = session_id_from_headers(&headers).unwrap_or_else(new_session_id);
    {
        let mut sessions = state.sessions.lock().await;
        sessions.remove(&session_id);
    }
    let mut response = Json(ChatResponse {
        reply: "Chat reset.".to_string(),
        model: state.config.llm_model.clone(),
    })
    .into_response();
    set_session_cookie(response.headers_mut(), &session_id);
    response
}

async fn call_llm(
    state: &AppState,
    model: &str,
    messages: &[ChatMessage],
) -> Result<String, String> {
    let payload = OpenAIChatRequest {
        model: model.to_string(),
        messages: messages.to_vec(),
    };

    let mut request = state.http.post(state.config.chat_url()).json(&payload);
    if !state.config.llm_api_key.trim().is_empty() {
        request = request.bearer_auth(&state.config.llm_api_key);
    }

    let response = request.send().await.map_err(|error| error.to_string())?;
    let status = response.status();
    if !status.is_success() {
        let body = response.text().await.unwrap_or_default();
        return Err(format!("{status}: {body}"));
    }

    let completion = response
        .json::<OpenAIChatResponse>()
        .await
        .map_err(|error| error.to_string())?;

    completion
        .choices
        .into_iter()
        .next()
        .map(|choice| choice.message.content.trim().to_string())
        .filter(|reply| !reply.is_empty())
        .ok_or_else(|| "LLM returned no reply".to_string())
}

fn trim_history(messages: Vec<ChatMessage>) -> Vec<ChatMessage> {
    const MAX_MESSAGES: usize = 41;
    if messages.len() <= MAX_MESSAGES {
        return messages;
    }

    let mut trimmed = Vec::with_capacity(MAX_MESSAGES);
    if let Some(system) = messages.first().filter(|message| message.role == "system") {
        trimmed.push(system.clone());
    }

    let keep = MAX_MESSAGES.saturating_sub(trimmed.len());
    let start = messages.len().saturating_sub(keep);
    trimmed.extend(messages.into_iter().skip(start));
    trimmed
}

fn session_id_from_headers(headers: &HeaderMap) -> Option<String> {
    let cookie = headers.get(header::COOKIE)?.to_str().ok()?;
    cookie.split(';').find_map(|part| {
        let (name, value) = part.trim().split_once('=')?;
        (name == SESSION_COOKIE && !value.trim().is_empty()).then(|| value.trim().to_string())
    })
}

fn set_session_cookie(headers: &mut HeaderMap, session_id: &str) {
    let cookie = format!("{SESSION_COOKIE}={session_id}; Path=/; HttpOnly; SameSite=Lax");
    if let Ok(value) = HeaderValue::from_str(&cookie) {
        headers.insert(header::SET_COOKIE, value);
    }
}

fn new_session_id() -> String {
    Uuid::new_v4().to_string()
}

fn api_error(status: StatusCode, message: &str) -> Response {
    #[derive(Serialize)]
    struct ErrorBody<'a> {
        error: &'a str,
    }

    (status, Json(ErrorBody { error: message })).into_response()
}

fn env_or(name: &str, default: &str) -> String {
    env::var(name)
        .ok()
        .map(|value| value.trim().to_string())
        .filter(|value| !value.is_empty())
        .unwrap_or_else(|| default.to_string())
}

fn normalize_listen_addr(value: &str) -> String {
    if value.starts_with(':') {
        format!("0.0.0.0{value}")
    } else {
        value.to_string()
    }
}

fn public_addr_from_listen_addr(value: &str) -> Option<String> {
    let addr: SocketAddr = value.parse().ok()?;
    Some(format!("localhost:{}", addr.port()))
}

fn index_html(model: &str) -> String {
    format!(
        r#"<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>URL Chat</title>
  <style>
    :root {{
      color-scheme: light dark;
      --bg: #f6f5f2;
      --panel: #ffffff;
      --text: #202124;
      --muted: #676b73;
      --border: #d9d7d1;
      --accent: #0f766e;
      --accent-strong: #0b5f59;
      --assistant: #eef5f3;
      --user: #fff4d8;
      font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
    }}

    @media (prefers-color-scheme: dark) {{
      :root {{
        --bg: #171717;
        --panel: #222222;
        --text: #f3f1ed;
        --muted: #b7b1a8;
        --border: #3a3835;
        --accent: #2dd4bf;
        --accent-strong: #5eead4;
        --assistant: #1f3633;
        --user: #3a311b;
      }}
    }}

    * {{ box-sizing: border-box; }}
    body {{
      margin: 0;
      min-height: 100vh;
      background: var(--bg);
      color: var(--text);
    }}
    main {{
      width: min(920px, calc(100vw - 32px));
      min-height: 100vh;
      margin: 0 auto;
      display: grid;
      grid-template-rows: auto 1fr auto;
      gap: 16px;
      padding: 20px 0;
    }}
    header {{
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 16px;
      border-bottom: 1px solid var(--border);
      padding-bottom: 14px;
    }}
    h1 {{
      margin: 0;
      font-size: 1.25rem;
      font-weight: 700;
      letter-spacing: 0;
    }}
    .model {{
      color: var(--muted);
      font-size: 0.9rem;
      overflow-wrap: anywhere;
      text-align: right;
    }}
    #log {{
      min-height: 0;
      overflow-y: auto;
      display: flex;
      flex-direction: column;
      gap: 12px;
      padding: 8px 2px;
    }}
    .message {{
      max-width: min(760px, 92%);
      padding: 12px 14px;
      border: 1px solid var(--border);
      border-radius: 8px;
      line-height: 1.45;
      white-space: pre-wrap;
      overflow-wrap: anywhere;
    }}
    .assistant {{
      align-self: flex-start;
      background: var(--assistant);
    }}
    .user {{
      align-self: flex-end;
      background: var(--user);
    }}
    form {{
      display: grid;
      grid-template-columns: 1fr auto auto;
      gap: 10px;
      border-top: 1px solid var(--border);
      padding-top: 14px;
    }}
    textarea {{
      width: 100%;
      min-height: 46px;
      max-height: 180px;
      resize: vertical;
      border: 1px solid var(--border);
      border-radius: 8px;
      padding: 12px;
      background: var(--panel);
      color: var(--text);
      font: inherit;
    }}
    button {{
      min-height: 46px;
      border: 1px solid var(--accent);
      border-radius: 8px;
      padding: 0 16px;
      background: var(--accent);
      color: #ffffff;
      font: inherit;
      font-weight: 700;
      cursor: pointer;
    }}
    button.secondary {{
      border-color: var(--border);
      background: var(--panel);
      color: var(--text);
    }}
    button:disabled {{
      cursor: wait;
      opacity: 0.7;
    }}
    @media (max-width: 640px) {{
      main {{ width: min(100vw - 20px, 920px); padding: 12px 0; }}
      header {{ align-items: flex-start; flex-direction: column; }}
      .model {{ text-align: left; }}
      form {{ grid-template-columns: 1fr; }}
    }}
  </style>
</head>
<body>
  <main>
    <header>
      <h1>URL Chat</h1>
      <div class="model">Model: <span id="model">{model}</span></div>
    </header>

    <section id="log" aria-live="polite">
      <div class="message assistant">Ask me anything.</div>
    </section>

    <form id="chat-form">
      <textarea id="message" name="message" placeholder="Type a message..." autocomplete="off" required></textarea>
      <button id="send" type="submit">Send</button>
      <button class="secondary" id="reset" type="button">Reset</button>
    </form>
  </main>

  <script>
    const form = document.querySelector('#chat-form');
    const input = document.querySelector('#message');
    const log = document.querySelector('#log');
    const send = document.querySelector('#send');
    const reset = document.querySelector('#reset');

    function addMessage(role, text) {{
      const bubble = document.createElement('div');
      bubble.className = `message ${{role}}`;
      bubble.textContent = text;
      log.appendChild(bubble);
      log.scrollTop = log.scrollHeight;
      return bubble;
    }}

    async function postJson(url, body) {{
      const response = await fetch(url, {{
        method: 'POST',
        headers: {{ 'Content-Type': 'application/json' }},
        body: JSON.stringify(body),
      }});
      const data = await response.json().catch(() => ({{ error: 'Invalid server response' }}));
      if (!response.ok) {{
        throw new Error(data.error || `Request failed: ${{response.status}}`);
      }}
      return data;
    }}

    form.addEventListener('submit', async (event) => {{
      event.preventDefault();
      const message = input.value.trim();
      if (!message) return;

      addMessage('user', message);
      input.value = '';
      send.disabled = true;
      const pending = addMessage('assistant', 'Thinking...');

      try {{
        const data = await postJson('/api/chat', {{ message }});
        pending.textContent = data.reply;
        document.querySelector('#model').textContent = data.model;
      }} catch (error) {{
        pending.textContent = error.message;
      }} finally {{
        send.disabled = false;
        input.focus();
      }}
    }});

    reset.addEventListener('click', async () => {{
      reset.disabled = true;
      try {{
        await postJson('/api/reset', {{}});
        log.innerHTML = '';
        addMessage('assistant', 'Ask me anything.');
      }} catch (error) {{
        addMessage('assistant', error.message);
      }} finally {{
        reset.disabled = false;
        input.focus();
      }}
    }});

    input.addEventListener('keydown', (event) => {{
      if (event.key === 'Enter' && !event.shiftKey) {{
        event.preventDefault();
        form.requestSubmit();
      }}
    }});
  </script>
</body>
</html>"#,
        model = escape_html(model)
    )
}

fn escape_html(value: &str) -> String {
    value
        .replace('&', "&amp;")
        .replace('<', "&lt;")
        .replace('>', "&gt;")
        .replace('"', "&quot;")
        .replace('\'', "&#39;")
}
