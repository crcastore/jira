"""Jira CLI agent. Uses any OpenAI-compatible chat endpoint (incl. Ollama)."""
from __future__ import annotations

import os
import sys

from dotenv import load_dotenv
from openai import OpenAI
from rich.console import Console
from rich.markdown import Markdown

from jira_client import JiraClient
from tools import TOOL_SCHEMAS, build_dispatcher, call_tool

SYSTEM_PROMPT = """You are a Jira assistant. You help the user search, read, create, comment on,
and transition Jira issues by calling the provided tools.

Rules:
- Prefer calling tools over guessing. Never invent issue keys, accountIds, or transition IDs.
- For status changes: call list_transitions first to learn the valid transition_id, then transition_issue.
- For assigning a user: call search_users to resolve a name/email to an accountId.
- When the user says "me" / "my issues", call myself once to get their accountId, then use
  `assignee = currentUser()` in JQL.
- Keep replies short. Show issue keys, summaries, and statuses in compact tables/lists.
- Confirm with the user before destructive or large-batch changes."""

MAX_STEPS = 12


def run() -> None:
    load_dotenv()
    console = Console()

    jira = JiraClient()
    dispatcher = build_dispatcher(jira)

    client = OpenAI(
        base_url=os.environ.get("LLM_BASE_URL", "http://localhost:11434/v1"),
        api_key=os.environ.get("LLM_API_KEY") or os.environ.get("OPENAI_API_KEY") or "ollama",
    )
    model = os.environ.get("LLM_MODEL", "llama3.1")

    messages: list[dict] = [{"role": "system", "content": SYSTEM_PROMPT}]
    console.print(f"[bold green]Jira agent[/bold green] (model: {model}) — type 'exit' to quit.\n")

    while True:
        try:
            user = console.input("[bold cyan]you> [/bold cyan]").strip()
        except (EOFError, KeyboardInterrupt):
            console.print()
            return
        if not user:
            continue
        if user.lower() in {"exit", "quit", ":q"}:
            return

        messages.append({"role": "user", "content": user})
        _run_turn(client, model, messages, dispatcher, console)


def _run_turn(client, model, messages, dispatcher, console) -> None:
    for _ in range(MAX_STEPS):
        resp = client.chat.completions.create(
            model=model,
            messages=messages,
            tools=TOOL_SCHEMAS,
            tool_choice="auto",
        )
        msg = resp.choices[0].message

        assistant_msg: dict = {"role": "assistant", "content": msg.content or ""}
        if msg.tool_calls:
            assistant_msg["tool_calls"] = [
                {
                    "id": tc.id,
                    "type": "function",
                    "function": {"name": tc.function.name, "arguments": tc.function.arguments},
                }
                for tc in msg.tool_calls
            ]
        messages.append(assistant_msg)

        if not msg.tool_calls:
            if msg.content:
                console.print(Markdown(msg.content))
            return

        for tc in msg.tool_calls:
            name = tc.function.name
            args = tc.function.arguments or "{}"
            console.print(f"[dim]→ {name}({args})[/dim]")
            result = call_tool(dispatcher, name, args)
            messages.append({"role": "tool", "tool_call_id": tc.id, "name": name, "content": result})

    console.print("[yellow]Step limit reached.[/yellow]")


if __name__ == "__main__":
    try:
        run()
    except KeyError as e:
        print(f"Missing env var: {e}. Copy .env.example to .env and fill it in.", file=sys.stderr)
        sys.exit(1)
