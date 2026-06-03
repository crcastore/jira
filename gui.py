"""Streamlit chat GUI for the Jira agent.

Run:  streamlit run gui.py
"""
from __future__ import annotations

import json
import os

import httpx
import streamlit as st
from dotenv import load_dotenv
from openai import OpenAI

from jira_client import JiraClient
from tools import TOOL_SCHEMAS, build_dispatcher, call_tool

load_dotenv()

SYSTEM_PROMPT = """You are a Jira assistant. You help the user search, read, create, comment on,
and transition Jira issues by calling the provided tools.

Rules:
- Prefer calling tools over guessing. Never invent issue keys, accountIds, or transition IDs.
- For status changes: call list_transitions first, then transition_issue.
- For assigning a user: call search_users to resolve a name/email to an accountId.
- When the user says "me" / "my issues", call myself once to get their accountId, then use
  `assignee = currentUser()` in JQL.
- Keep replies short. Show issue keys, summaries, and statuses in compact tables/lists.
- Confirm before destructive or large-batch changes."""

OPENAI_MODELS = ["gpt-4o-mini", "gpt-4o", "gpt-4.1-mini", "gpt-4.1"]
MAX_STEPS = 12


# ---------- model discovery ----------
@st.cache_data(ttl=30)
def list_ollama_models(base: str = "http://localhost:11434") -> list[str]:
    try:
        r = httpx.get(f"{base}/api/tags", timeout=3.0)
        r.raise_for_status()
        return sorted(m["name"] for m in r.json().get("models", []))
    except Exception:
        return []


# ---------- LLM client ----------
def make_client(backend: str) -> tuple[OpenAI, str]:
    if backend == "Ollama":
        return OpenAI(base_url="http://localhost:11434/v1", api_key="ollama"), "ollama"
    api_key = os.environ.get("OPENAI_API_KEY") or os.environ.get("LLM_API_KEY")
    if not api_key:
        st.error("OPENAI_API_KEY is not set in your shell or .env.")
        st.stop()
    return OpenAI(api_key=api_key), "openai"


# ---------- sidebar ----------
st.set_page_config(page_title="Jira Agent", page_icon="🧰", layout="wide")
st.title("Jira Agent")

with st.sidebar:
    st.header("Model")
    backend = st.radio("Backend", ["OpenAI", "Ollama"], horizontal=True)
    if backend == "Ollama":
        models = list_ollama_models()
        if not models:
            st.warning("No Ollama models found. Is `ollama serve` running?")
            model = st.text_input("Model", value="llama3.1:8b")
        else:
            model = st.selectbox("Model", models, index=0)
    else:
        model = st.selectbox("Model", OPENAI_MODELS, index=0)

    st.divider()
    if st.button("Clear chat", use_container_width=True):
        st.session_state.pop("messages", None)
        st.rerun()


# ---------- session state ----------
if "messages" not in st.session_state:
    st.session_state.messages = [{"role": "system", "content": SYSTEM_PROMPT}]

try:
    jira = JiraClient()
except KeyError as e:
    st.error(f"Missing env var: {e}. Fill in `.env` first.")
    st.stop()
dispatcher = build_dispatcher(jira)


# ---------- render history ----------
for m in st.session_state.messages:
    if m["role"] == "system":
        continue
    if m["role"] == "user":
        with st.chat_message("user"):
            st.markdown(m["content"])
    elif m["role"] == "assistant":
        with st.chat_message("assistant"):
            if m.get("content"):
                st.markdown(m["content"])
            for tc in m.get("tool_calls", []) or []:
                with st.expander(f"🔧 {tc['function']['name']}", expanded=False):
                    st.code(tc["function"]["arguments"] or "{}", language="json")
    elif m["role"] == "tool":
        with st.expander(f"↳ result of {m.get('name','tool')}", expanded=False):
            try:
                st.json(json.loads(m["content"]))
            except Exception:
                st.code(m["content"])


# ---------- chat input ----------
prompt = st.chat_input("Ask the agent something... e.g. 'list my SCRUM issues that aren't done'")
if prompt:
    st.session_state.messages.append({"role": "user", "content": prompt})
    with st.chat_message("user"):
        st.markdown(prompt)

    client, _ = make_client(backend)

    with st.chat_message("assistant"):
        status = st.status("Thinking...", expanded=False)
        try:
            for step in range(MAX_STEPS):
                resp = client.chat.completions.create(
                    model=model,
                    messages=st.session_state.messages,
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
                            "function": {
                                "name": tc.function.name,
                                "arguments": tc.function.arguments,
                            },
                        }
                        for tc in msg.tool_calls
                    ]
                st.session_state.messages.append(assistant_msg)

                if not msg.tool_calls:
                    status.update(label="Done", state="complete")
                    if msg.content:
                        st.markdown(msg.content)
                    break

                for tc in msg.tool_calls:
                    name = tc.function.name
                    args = tc.function.arguments or "{}"
                    status.update(label=f"Calling {name}...")
                    with st.expander(f"🔧 {name}", expanded=False):
                        st.code(args, language="json")
                    result = call_tool(dispatcher, name, args)
                    st.session_state.messages.append(
                        {"role": "tool", "tool_call_id": tc.id, "name": name, "content": result}
                    )
                    with st.expander(f"↳ result of {name}", expanded=False):
                        try:
                            st.json(json.loads(result))
                        except Exception:
                            st.code(result)
            else:
                status.update(label="Step limit reached", state="error")
        except Exception as e:  # noqa: BLE001
            status.update(label="Error", state="error")
            st.error(str(e))
