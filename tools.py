"""LLM tool definitions + dispatcher. Each tool maps to a JiraClient method."""
from __future__ import annotations

import json
from typing import Any, Callable

from jira_client import JiraClient


TOOL_SCHEMAS: list[dict] = [
    {
        "type": "function",
        "function": {
            "name": "search_issues",
            "description": "Search Jira issues using JQL. Returns a compact list of matching issues.",
            "parameters": {
                "type": "object",
                "properties": {
                    "jql": {"type": "string", "description": "JQL query, e.g. 'assignee = currentUser() AND status != Done'"},
                    "max_results": {"type": "integer", "default": 25},
                },
                "required": ["jql"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "get_issue",
            "description": "Fetch full details for one issue by its key (e.g. ABC-123).",
            "parameters": {
                "type": "object",
                "properties": {"key": {"type": "string"}},
                "required": ["key"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "create_issue",
            "description": "Create a new Jira issue in the given project.",
            "parameters": {
                "type": "object",
                "properties": {
                    "project_key": {"type": "string"},
                    "summary": {"type": "string"},
                    "issue_type": {"type": "string", "default": "Task"},
                    "description": {"type": "string"},
                    "assignee_account_id": {"type": "string"},
                    "priority": {"type": "string", "description": "e.g. Highest, High, Medium, Low, Lowest"},
                    "labels": {"type": "array", "items": {"type": "string"}},
                },
                "required": ["project_key", "summary"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "add_comment",
            "description": "Add a plain-text comment to an issue.",
            "parameters": {
                "type": "object",
                "properties": {"key": {"type": "string"}, "body": {"type": "string"}},
                "required": ["key", "body"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "list_transitions",
            "description": "List available workflow transitions for an issue (use before transition_issue).",
            "parameters": {
                "type": "object",
                "properties": {"key": {"type": "string"}},
                "required": ["key"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "transition_issue",
            "description": "Move an issue to a new status using a transition ID from list_transitions.",
            "parameters": {
                "type": "object",
                "properties": {"key": {"type": "string"}, "transition_id": {"type": "string"}},
                "required": ["key", "transition_id"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "update_issue_fields",
            "description": "Update arbitrary fields on an issue. `fields` must be a JSON object matching Jira's edit API.",
            "parameters": {
                "type": "object",
                "properties": {
                    "key": {"type": "string"},
                    "fields": {"type": "object", "additionalProperties": True},
                },
                "required": ["key", "fields"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "search_users",
            "description": "Find users by name/email; returns accountIds usable as assignee.",
            "parameters": {
                "type": "object",
                "properties": {"query": {"type": "string"}, "max_results": {"type": "integer", "default": 10}},
                "required": ["query"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "list_projects",
            "description": "List Jira projects available to the current user.",
            "parameters": {"type": "object", "properties": {}},
        },
    },
    {
        "type": "function",
        "function": {
            "name": "myself",
            "description": "Return the authenticated user's profile (accountId, email, displayName).",
            "parameters": {"type": "object", "properties": {}},
        },
    },
]


def build_dispatcher(jira: JiraClient) -> dict[str, Callable[..., Any]]:
    return {
        "search_issues": lambda jql, max_results=25: _trim_search(jira.search(jql, max_results=max_results)),
        "get_issue": lambda key: _trim_issue(jira.get_issue(key)),
        "create_issue": jira.create_issue,
        "add_comment": jira.add_comment,
        "list_transitions": jira.list_transitions,
        "transition_issue": jira.transition_issue,
        "update_issue_fields": lambda key, fields: jira.update_issue(key, fields),
        "search_users": jira.search_users,
        "list_projects": jira.list_projects,
        "myself": jira.myself,
    }


def call_tool(dispatcher: dict[str, Callable[..., Any]], name: str, arguments_json: str) -> str:
    try:
        args = json.loads(arguments_json or "{}")
    except json.JSONDecodeError as e:
        return json.dumps({"error": f"invalid JSON arguments: {e}"})
    fn = dispatcher.get(name)
    if fn is None:
        return json.dumps({"error": f"unknown tool '{name}'"})
    try:
        result = fn(**args)
    except TypeError as e:
        return json.dumps({"error": f"bad arguments for {name}: {e}"})
    except Exception as e:  # noqa: BLE001 - surface to LLM
        return json.dumps({"error": str(e)})
    return json.dumps(result, default=str)[:15000]


def _trim_search(payload: dict) -> dict:
    issues = []
    for it in payload.get("issues", []):
        f = it.get("fields", {})
        issues.append(
            {
                "key": it.get("key"),
                "summary": f.get("summary"),
                "status": (f.get("status") or {}).get("name"),
                "assignee": (f.get("assignee") or {}).get("displayName"),
                "priority": (f.get("priority") or {}).get("name"),
                "type": (f.get("issuetype") or {}).get("name"),
                "updated": f.get("updated"),
            }
        )
    return {"count": len(issues), "next_page_token": payload.get("nextPageToken"), "issues": issues}


def _trim_issue(issue: dict) -> dict:
    f = issue.get("fields", {})
    return {
        "key": issue.get("key"),
        "summary": f.get("summary"),
        "status": (f.get("status") or {}).get("name"),
        "assignee": (f.get("assignee") or {}).get("displayName"),
        "reporter": (f.get("reporter") or {}).get("displayName"),
        "priority": (f.get("priority") or {}).get("name"),
        "type": (f.get("issuetype") or {}).get("name"),
        "labels": f.get("labels"),
        "created": f.get("created"),
        "updated": f.get("updated"),
        "description": f.get("description"),
    }
