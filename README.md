# URL Chat (Rust)

A small Rust web chat served through a URL. It keeps only the chat experience and does not use the old Jira or GitHub integrations.

The server exposes:

- `GET /` for the chat page
- `POST /api/chat` for chat turns
- `POST /api/reset` to clear the current browser session
- `GET /healthz` for a plain health check

## Run

```bash
cp .env.example .env
./run.sh
```

Then open <http://localhost:8080>.

You can also run it directly with Cargo:

```bash
cargo run
```

## Configuration

The app reads `.env` automatically.

```env
LLM_BASE_URL=http://localhost:11434/v1
LLM_MODEL=llama3.1:8b
LLM_API_KEY=ollama
WEB_ADDR=:8080
```

`LLM_BASE_URL` must point at an OpenAI-compatible chat completions API. The default works with Ollama's OpenAI-compatible endpoint.

For hosted providers, set their base URL, model, and API key:

```env
LLM_BASE_URL=https://api.openai.com/v1
LLM_MODEL=gpt-4o-mini
LLM_API_KEY=sk-...
```

## Ollama

```bash
ollama serve
ollama pull llama3.1:8b
./run.sh
```

## Build

```bash
cargo check
cargo build
```

## Project Layout

| Path | Purpose |
| --- | --- |
| `Cargo.toml` | Rust package and dependencies |
| `src/main.rs` | Web server, chat API, session memory, and HTML UI |
| `.env.example` | LLM and web server configuration |
| `run.sh` | Convenience runner for the Rust app |

The old Go Jira/GitHub code is still present in the repository, but this Rust app does not call it or depend on it.