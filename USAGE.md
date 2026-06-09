# Usage

This project now runs as a Rust URL chat. It does not need Jira or GitHub credentials.

## 1. Configure

Copy the example environment file:

```bash
cp .env.example .env
```

Defaults are set for Ollama:

```env
LLM_BASE_URL=http://localhost:11434/v1
LLM_MODEL=llama3.1:8b
LLM_API_KEY=ollama
WEB_ADDR=:8080
```

`WEB_ADDR` controls where the Rust server listens. `:8080` means the chat is available at <http://localhost:8080>.

## 2. Start A Model

For Ollama:

```bash
ollama serve
ollama pull llama3.1:8b
```

For another OpenAI-compatible provider, set `LLM_BASE_URL`, `LLM_MODEL`, and `LLM_API_KEY` in `.env`.

## 3. Run The Chat

```bash
./run.sh
```

Then open <http://localhost:8080>.

## 4. Build Or Check

```bash
./run.sh check
./run.sh build
```

Equivalent Cargo commands:

```bash
cargo check
cargo build
```

## 5. What It Does

The Rust app serves a single chat page and stores conversation history in memory per browser session cookie.

Each message is sent to:

```text
{LLM_BASE_URL}/chat/completions
```

with an OpenAI-compatible request body. The assistant reply is returned to the browser and appended to the chat log.