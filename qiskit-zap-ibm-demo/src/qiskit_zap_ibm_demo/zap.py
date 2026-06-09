from __future__ import annotations

import time
from collections.abc import Sequence
from typing import Any
from urllib.parse import urlparse

import httpx


def wait_for_zap(zap_api: str, timeout_seconds: int = 60) -> str:
    deadline = time.monotonic() + timeout_seconds
    version_url = f"{zap_api.rstrip('/')}/JSON/core/view/version/"
    last_error: Exception | None = None

    while time.monotonic() < deadline:
        try:
            response = httpx.get(version_url, timeout=3.0)
            response.raise_for_status()
            return str(response.json().get("version", "unknown"))
        except (httpx.HTTPError, ValueError) as exc:
            last_error = exc
            time.sleep(1)

    raise RuntimeError(f"ZAP API did not become ready at {zap_api}: {last_error}")


def start_new_session(zap_api: str) -> None:
    session_url = f"{zap_api.rstrip('/')}/JSON/core/action/newSession/"
    params = {"name": f"qiskit-zap-demo-{int(time.time())}", "overwrite": "true"}
    response = httpx.get(session_url, params=params, timeout=10.0)
    response.raise_for_status()


def fetch_zap_messages(zap_api: str, limit: int = 500) -> list[dict[str, Any]]:
    messages_url = f"{zap_api.rstrip('/')}/JSON/core/view/messages/"
    response = httpx.get(messages_url, params={"start": 0, "count": limit}, timeout=20.0)
    response.raise_for_status()
    payload = response.json()
    messages = payload.get("messages", [])
    if not isinstance(messages, list):
        return []
    return [message for message in messages if isinstance(message, dict)]


def filter_messages_for_targets(messages: list[dict[str, Any]], targets: Sequence[str]) -> list[dict[str, Any]]:
    hosts = [urlparse(target).netloc or target for target in targets if target]
    if not hosts:
        return messages

    selected: list[dict[str, Any]] = []
    for message in messages:
        haystack = "\n".join(
            str(message.get(field, ""))
            for field in ("url", "requestHeader", "responseHeader", "requestBody", "responseBody")
        )
        if any(host in haystack for host in hosts):
            selected.append(message)
    return selected
