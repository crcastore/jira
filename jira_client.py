"""Thin Jira Cloud REST API v3 client (basic auth with email + API token)."""
from __future__ import annotations

import os
from typing import Any

import httpx


class JiraClient:
    def __init__(
        self,
        base_url: str | None = None,
        email: str | None = None,
        token: str | None = None,
        timeout: float = 30.0,
    ) -> None:
        base = (base_url or os.environ["JIRA_BASE_URL"]).rstrip("/")
        self._http = httpx.Client(
            base_url=base,
            auth=(email or os.environ["JIRA_EMAIL"], token or os.environ["JIRA_API_TOKEN"]),
            headers={"Accept": "application/json", "Content-Type": "application/json"},
            timeout=timeout,
        )

    def _request(self, method: str, path: str, **kw: Any) -> Any:
        r = self._http.request(method, path, **kw)
        if r.status_code >= 400:
            raise RuntimeError(f"Jira {method} {path} -> {r.status_code}: {r.text}")
        if r.status_code == 204 or not r.content:
            return {}
        return r.json()

    # ---------- Issues ----------
    def search(self, jql: str, fields: list[str] | None = None, max_results: int = 25) -> dict:
        body = {
            "jql": jql,
            "fields": fields or ["summary", "status", "assignee", "priority", "issuetype", "updated"],
            "maxResults": max_results,
        }
        # New enhanced-search endpoint (replaces deprecated /rest/api/3/search).
        return self._request("POST", "/rest/api/3/search/jql", json=body)

    def get_issue(self, key: str) -> dict:
        return self._request("GET", f"/rest/api/3/issue/{key}")

    def create_issue(
        self,
        project_key: str,
        summary: str,
        issue_type: str = "Task",
        description: str | None = None,
        assignee_account_id: str | None = None,
        priority: str | None = None,
        labels: list[str] | None = None,
    ) -> dict:
        fields: dict[str, Any] = {
            "project": {"key": project_key},
            "summary": summary,
            "issuetype": {"name": issue_type},
        }
        if description:
            fields["description"] = _adf(description)
        if assignee_account_id:
            fields["assignee"] = {"accountId": assignee_account_id}
        if priority:
            fields["priority"] = {"name": priority}
        if labels:
            fields["labels"] = labels
        return self._request("POST", "/rest/api/3/issue", json={"fields": fields})

    def update_issue(self, key: str, fields: dict[str, Any]) -> dict:
        return self._request("PUT", f"/rest/api/3/issue/{key}", json={"fields": fields})

    def add_comment(self, key: str, body: str) -> dict:
        return self._request("POST", f"/rest/api/3/issue/{key}/comment", json={"body": _adf(body)})

    def list_transitions(self, key: str) -> dict:
        return self._request("GET", f"/rest/api/3/issue/{key}/transitions")

    def transition_issue(self, key: str, transition_id: str) -> dict:
        return self._request(
            "POST",
            f"/rest/api/3/issue/{key}/transitions",
            json={"transition": {"id": transition_id}},
        )

    # ---------- Lookups ----------
    def myself(self) -> dict:
        return self._request("GET", "/rest/api/3/myself")

    def search_users(self, query: str, max_results: int = 10) -> list[dict]:
        return self._request("GET", "/rest/api/3/user/search", params={"query": query, "maxResults": max_results})

    def list_projects(self) -> list[dict]:
        return self._request("GET", "/rest/api/3/project/search").get("values", [])


def _adf(text: str) -> dict:
    """Wrap plain text in Atlassian Document Format (required by API v3)."""
    return {
        "type": "doc",
        "version": 1,
        "content": [{"type": "paragraph", "content": [{"type": "text", "text": text}]}],
    }
