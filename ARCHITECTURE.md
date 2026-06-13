# Jira Agent Architecture

This document explains how the chat engine, tools, model selection, and history/session handling work together.

## High-Level Flow

1. A user sends a prompt from CLI or Web.
2. The app resolves which model to use.
3. The app loads conversation history for that session.
4. The engine runs an LLM step loop.
5. If the model requests tools, the app executes them and feeds results back to the model.
6. When the model returns a final reply, updated history is stored.
7. The reply is returned to CLI or rendered in Web UI.

## Main Components

1. Entry points
   - [cmd/jira-agent/main.go](cmd/jira-agent/main.go)
   - [cmd/jira-agent/web_server.go](cmd/jira-agent/web_server.go)

2. Chat service and model catalog
   - [agentcore/service.go](agentcore/service.go)
   - [agentcore/models.go](agentcore/models.go)

3. Core LLM engine and session store
   - [chat/engine.go](chat/engine.go)
   - [chat/session.go](chat/session.go)

4. Reusable HTMX transport and rendering
   - [chathttp/handlers.go](chathttp/handlers.go)
   - [chatui/component.go](chatui/component.go)

5. Tool schemas and dispatch
   - [cmd/jira-agent/tools.go](cmd/jira-agent/tools.go)
   - [cmd/jira-agent/agent.go](cmd/jira-agent/agent.go)

6. External integrations
   - Jira client: [cmd/jira-agent/jira.go](cmd/jira-agent/jira.go)
   - GitHub client: [cmd/jira-agent/github.go](cmd/jira-agent/github.go)
   - Jira create helpers: [jiracreate/](jiracreate/)

## Model Selection

Model selection is handled in [agentcore/service.go](agentcore/service.go).

1. Default model comes from environment-backed config in app wiring.
2. If the user provides a model name, the service validates it against available models.
3. If invalid or unavailable, the service falls back to the default model.
4. In Web mode, available models are discovered via Ollama tags in [agentcore/models.go](agentcore/models.go).

## History and Sessions

History is not stored inside the engine.

1. The service loads history from [chat/session.go](chat/session.go).
2. The service calls Engine Run with history plus the new prompt.
3. The engine returns updated message history.
4. The service writes updated history back to the session store.

Session behavior by transport:

1. CLI uses a fixed session id for the local run.
2. Web uses a cookie-backed session id so each browser session keeps its own history.

## Tool Calling Lifecycle

Tool lifecycle is implemented across [chat/engine.go](chat/engine.go) and [cmd/jira-agent/tools.go](cmd/jira-agent/tools.go).

1. The engine sends messages plus tool schemas to the LLM.
2. If the LLM emits tool calls, each call is dispatched by tool name.
3. Dispatch executes Jira or GitHub client methods.
4. Tool results are appended as tool messages.
5. The engine calls the LLM again so the model can use those tool outputs.
6. Loop continues until a final assistant reply or step limit.

## Engine Loop Details

In [chat/engine.go](chat/engine.go):

1. Max steps protect against infinite loops.
2. Per-step timeout protects against slow model calls.
3. Lifecycle hooks can be used by CLI for spinner and tool echo.
4. The engine supports a fallback path for models that output tool calls as text instead of native tool-call fields.

## Web UI Boundaries

Web concerns are separated from chat orchestration.

1. HTTP handlers and route wiring: [cmd/jira-agent/web_server.go](cmd/jira-agent/web_server.go)
2. Data mapping for side panels: [cmd/jira-agent/web_data.go](cmd/jira-agent/web_data.go)
3. Session/env helper functions: [cmd/jira-agent/web_helpers.go](cmd/jira-agent/web_helpers.go)
4. Dashboard shell and side-panel templates: [cmd/jira-agent/web_templates.go](cmd/jira-agent/web_templates.go)
5. Chat/reset/token handlers: [chathttp/handlers.go](chathttp/handlers.go)
6. Chat widget and chat chunks: [chatui/component.go](chatui/component.go)

The Web layer delegates chat work to the agentcore service, mounts reusable chathttp handlers, and renders chat through the reusable HTMX chatui component.

## Why This Structure

1. Reusable chat packages live at public import paths so other Go apps can import them.
2. Transport layers remain thin and focused.
3. Chat engine remains independent of Web and CLI.
4. Tool definitions and dispatch are centralized in one place.
5. Session handling is explicit and testable.