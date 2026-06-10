from __future__ import annotations

import json
import os
from pathlib import Path
from typing import Any

import typer
from dotenv import load_dotenv
from rich.console import Console
from rich.table import Table

from qiskit_zap_ibm_demo.circuit import build_bell_circuit, circuit_to_qasm
from qiskit_zap_ibm_demo.redact import redact_json, redact_json_text, redact_text
from qiskit_zap_ibm_demo.zap import fetch_zap_messages, filter_messages_for_targets, start_new_session, wait_for_zap

app = typer.Typer(no_args_is_help=True)
console = Console()

TRANSCRIPT_FILE = "http-transcript.md"
ZAP_MESSAGES_FILE = "zap-messages.json"
IBM_CAPTURE_HOSTS = ("ibm.com",)
DEFAULT_ZAP_URL = "http://zap:8090"
DEFAULT_TOKEN = "fake-ibm-quantum-token"
DEFAULT_CHANNEL = "ibm_quantum_platform"
DEFAULT_ARTIFACT_DIR = "artifacts"
OPERATION_DESCRIPTION = "QiskitRuntimeService(...), service.backends(), then SamplerV2.run(...) if auth succeeds"


@app.callback()
def main() -> None:
    """Capture IBM Qiskit Runtime invalid-auth traffic through OWASP ZAP."""


@app.command()
def run(
    zap_proxy: str | None = typer.Option(None, help="OWASP ZAP proxy URL."),
    zap_api: str | None = typer.Option(None, help="OWASP ZAP API URL."),
    token: str | None = typer.Option(None, help="Fake IBM Quantum token. Must start with fake-."),
    channel: str | None = typer.Option(None, help="Qiskit IBM Runtime channel."),
    instance: str | None = typer.Option(None, help="Optional IBM Runtime instance or CRN."),
    artifact_dir: Path | None = typer.Option(None, help="Directory for generated ZAP artifacts."),
    verify_tls: bool = typer.Option(
        False,
        "--verify-tls/--no-verify-tls",
        help="Verify upstream TLS certificates. Keep disabled for local ZAP HTTPS inspection.",
    ),
    clear_zap_session: bool = typer.Option(True, help="Start a fresh ZAP session before calling IBM."),
) -> None:
    load_dotenv()

    zap_proxy = zap_proxy or os.getenv("ZAP_PROXY", DEFAULT_ZAP_URL)
    zap_api = zap_api or os.getenv("ZAP_API", DEFAULT_ZAP_URL)
    token = token or os.getenv("INVALID_IBM_QUANTUM_TOKEN", DEFAULT_TOKEN)
    channel = channel or os.getenv("IBM_QUANTUM_CHANNEL", DEFAULT_CHANNEL)
    instance = instance or os.getenv("IBM_QUANTUM_INSTANCE") or None
    artifact_dir = artifact_dir or Path(os.getenv("ARTIFACT_DIR", DEFAULT_ARTIFACT_DIR))

    if not token.startswith("fake-"):
        raise typer.BadParameter("This demo only sends fake-* invalid tokens to IBM Quantum.")

    console.print("[bold]Waiting for OWASP ZAP...[/bold]")
    zap_version = wait_for_zap(zap_api)
    if clear_zap_session:
        start_new_session(zap_api)
    console.print(f"ZAP ready: {zap_version}")

    circuit = build_bell_circuit()
    attempt = build_attempt_details(
        channel=channel,
        instance=instance,
        token=token,
        zap_proxy=zap_proxy,
        verify_tls=verify_tls,
        circuit_qasm=circuit_to_qasm(circuit),
    )

    console.print("[bold]Calling IBM Runtime with fake credentials through ZAP...[/bold]")
    probe_result = probe_ibm_runtime_invalid_auth(
        circuit=circuit,
        channel=channel,
        token=token,
        instance=instance,
        zap_proxy=zap_proxy,
        verify_tls=verify_tls,
    )

    messages = filter_messages_for_targets(fetch_zap_messages(zap_api), IBM_CAPTURE_HOSTS)
    write_artifacts(artifact_dir, messages, attempt, probe_result)
    print_summary(probe_result, messages, artifact_dir)

    if probe_result.get("credentials_accepted") is True:
        raise RuntimeError("IBM Runtime unexpectedly accepted the fake credentials")


def build_attempt_details(
    channel: str,
    instance: str | None,
    token: str,
    zap_proxy: str,
    verify_tls: bool,
    circuit_qasm: str,
) -> dict[str, Any]:
    return {
        "package": "qiskit-ibm-runtime",
        "operation": OPERATION_DESCRIPTION,
        "channel": channel,
        "instance": instance,
        "token": token,
        "proxy": zap_proxy,
        "verify_tls": verify_tls,
        "circuit": {
            "name": "bell_pair",
            "format": "openqasm2",
            "qasm": circuit_qasm,
        },
    }


def probe_ibm_runtime_invalid_auth(
    circuit: Any,
    channel: str,
    token: str,
    instance: str | None,
    zap_proxy: str,
    verify_tls: bool,
) -> dict[str, Any]:
    from qiskit_ibm_runtime import QiskitRuntimeService, SamplerV2

    proxies = {"urls": {"http": zap_proxy, "https": zap_proxy}}

    try:
        service = QiskitRuntimeService(
            channel=channel,
            token=token,
            instance=instance,
            proxies=proxies,
            verify=verify_tls,
        )
        backends = service.backends()
        if not backends:
            return {
                "credentials_accepted": True,
                "stage": "backend_discovery",
                "exception_type": None,
                "exception_message": "No IBM Runtime backends were returned.",
                "backend_count": 0,
            }

        backend = backends[0]
        job = SamplerV2(mode=backend).run([circuit], shots=1)
        result = job.result()
    except Exception as exc:
        return {
            "credentials_accepted": False,
            "stage": "authentication_or_runtime_call",
            "exception_type": exc.__class__.__name__,
            "exception_message": redact_text(str(exc)),
        }

    return {
        "credentials_accepted": True,
        "stage": "sampler_submission",
        "exception_type": None,
        "exception_message": "",
        "job_id": getattr(job, "job_id", lambda: None)(),
        "result_type": result.__class__.__name__,
    }


def write_artifacts(
    artifact_dir: Path,
    messages: list[dict[str, Any]],
    attempt: dict[str, Any],
    probe_result: dict[str, Any],
) -> None:
    artifact_dir.mkdir(parents=True, exist_ok=True)
    redacted_messages = redact_json(messages)
    redacted_attempt = redact_json(attempt)
    redacted_result = redact_json(probe_result)

    (artifact_dir / ZAP_MESSAGES_FILE).write_text(
        format_json(redacted_messages),
        encoding="utf-8",
    )
    (artifact_dir / TRANSCRIPT_FILE).write_text(
        build_markdown_transcript(redacted_messages, redacted_attempt, redacted_result),
        encoding="utf-8",
    )


def build_markdown_transcript(
    messages: list[dict[str, Any]],
    attempt: dict[str, Any],
    probe_result: dict[str, Any],
) -> str:
    circuit = attempt.get("circuit", {})
    call_details = {key: value for key, value in attempt.items() if key != "circuit"}
    lines = [
        "# OWASP ZAP IBM Runtime Invalid Auth Transcript",
        "",
        "## Demo Summary",
        "",
        "- Target: `IBM Quantum Runtime`",
        f"- Credentials accepted: `{probe_result.get('credentials_accepted', 'unknown')}`",
        f"- Stage: `{probe_result.get('stage', 'unknown')}`",
        f"- Exception type: `{probe_result.get('exception_type', 'unknown')}`",
        f"- Captured messages: `{len(messages)}`",
        "",
        "## Attempted Runtime Call",
        "",
    ]
    append_json_block(lines, call_details)
    lines.extend([
        "## Local Qiskit Circuit",
        "",
    ])
    append_json_block(lines, circuit)
    lines.extend([
        "## Returned Error",
        "",
    ])
    append_json_block(lines, probe_result)
    lines.extend([
        "## ZAP Messages",
        "",
    ])

    append_zap_message_markdown(lines, messages)

    return "\n".join(lines)


def append_zap_message_markdown(lines: list[str], messages: list[dict[str, Any]]) -> None:
    for index, message in enumerate(messages, start=1):
        method = message.get("method", "HTTP")
        url = message.get("url", "unknown-url")
        status_code = message.get("statusCode", "unknown")
        lines.extend(
            [
                f"### Message {index}: {method} {url}",
                "",
                f"Status code: `{status_code}`",
                "",
            ]
        )
        append_markdown_block(
            lines,
            "Request headers:",
            "http",
            redact_text(str(message.get("requestHeader", "")).strip()),
        )
        append_markdown_block(
            lines,
            "Request body:",
            "json",
            redact_json_text(str(message.get("requestBody", "")).strip() or "{}"),
        )
        append_markdown_block(
            lines,
            "Response headers:",
            "http",
            redact_text(str(message.get("responseHeader", "")).strip()),
        )
        append_markdown_block(
            lines,
            "Response body:",
            "json",
            redact_json_text(str(message.get("responseBody", "")).strip() or "{}"),
        )


def append_json_block(lines: list[str], value: Any) -> None:
    append_markdown_block(lines, "", "json", format_json(value))


def append_markdown_block(lines: list[str], title: str, language: str, value: str) -> None:
    if title:
        lines.extend([title, ""])
    lines.extend([f"```{language}", value, "```", ""])


def format_json(value: Any) -> str:
    return json.dumps(value, indent=2, sort_keys=True)


def print_summary(
    probe_result: dict[str, Any],
    messages: list[dict[str, Any]],
    artifact_dir: Path,
) -> None:
    table = Table(title="Qiskit IBM Runtime ZAP Invalid Auth")
    table.add_column("Field")
    table.add_column("Value")
    table.add_row("Credentials accepted", str(probe_result.get("credentials_accepted")))
    table.add_row("Stage", str(probe_result.get("stage")))
    table.add_row("Error type", str(probe_result.get("exception_type")))
    table.add_row("Error", shorten(str(probe_result.get("exception_message", ""))))
    table.add_row("Captured messages", str(len(messages)))
    table.add_row("Transcript", str(artifact_dir / TRANSCRIPT_FILE))
    table.add_row("Raw ZAP JSON", str(artifact_dir / ZAP_MESSAGES_FILE))
    console.print(table)


def shorten(value: str, max_length: int = 140) -> str:
    return value if len(value) <= max_length else f"{value[: max_length - 3]}..."


if __name__ == "__main__":
    app()