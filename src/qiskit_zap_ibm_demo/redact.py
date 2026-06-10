from __future__ import annotations

import json
import re
from collections.abc import Mapping, Sequence
from typing import Any

REDACTED = "<redacted>"
SENSITIVE_KEY_RE = re.compile(r"(authorization|token|api[_-]?key|password|secret)", re.IGNORECASE)
BEARER_RE = re.compile(r"Bearer\s+[-._~+/=A-Za-z0-9]+", re.IGNORECASE)
FAKE_TOKEN_RE = re.compile(r"\bfake[-._~+/=A-Za-z0-9]*", re.IGNORECASE)


def redact_text(value: str) -> str:
    return FAKE_TOKEN_RE.sub(REDACTED, BEARER_RE.sub(f"Bearer {REDACTED}", value))


def redact_json(value: Any) -> Any:
    if isinstance(value, Mapping):
        redacted: dict[str, Any] = {}
        for key, item in value.items():
            key_text = str(key)
            redacted[key_text] = REDACTED if SENSITIVE_KEY_RE.search(key_text) else redact_json(item)
        return redacted

    if isinstance(value, str):
        return redact_text(value)

    if isinstance(value, Sequence) and not isinstance(value, bytes | bytearray):
        return [redact_json(item) for item in value]

    return value


def redact_json_text(value: str) -> str:
    try:
        parsed = json.loads(value)
    except json.JSONDecodeError:
        return redact_text(value)
    return json.dumps(redact_json(parsed), indent=2, sort_keys=True)
