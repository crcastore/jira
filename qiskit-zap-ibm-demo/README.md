# Qiskit IBM Runtime ZAP Invalid Credentials Demo

This project captures the HTTP traffic produced when the IBM Qiskit Runtime Python package is called with intentionally invalid credentials through an OWASP ZAP proxy.

It does not bypass IBM Quantum authentication, payment, quotas, or access controls. It only sends a fake token, expects IBM to reject it, and writes a redacted transcript of what ZAP observed.

## What Runs

- `zap`: OWASP ZAP in daemon/proxy mode on port `8090`.
- `client`: a Python CLI that builds a Bell circuit with Qiskit, calls `qiskit-ibm-runtime` with a fake token through ZAP, captures the IBM auth/API error traffic, and writes request/response artifacts.

Because the credentials are fake, the IBM Runtime client should fail before any runtime job is created or sent to quantum hardware. The captured traffic is the authentication/client-discovery failure path returned by IBM.

## Requirements

- Docker Desktop with Docker Compose v2.
- Internet access from Docker containers.
- Python 3.11+ only if you want to run the client outside Docker.

## Quick Start

```bash
cd qiskit-zap-ibm-demo
docker compose --profile demo up --build --abort-on-container-exit client
```

After the client exits, inspect:

- [artifacts/http-transcript.md](artifacts/http-transcript.md)
- [artifacts/zap-messages.json](artifacts/zap-messages.json)

The transcript redacts bearer tokens and fake token values before writing to disk.

To stop services and clean up containers:

```bash
docker compose down --remove-orphans
```

## Run Pieces Manually

Start ZAP:

```bash
docker compose up -d zap
```

Run the invalid-credentials IBM Runtime client through the proxy:

```bash
docker compose --profile demo run --rm client
```

Ask ZAP for the captured messages directly:

```bash
curl 'http://localhost:8090/JSON/core/view/messages/?start=0&count=50'
```

## Local Python Development

```bash
python3 -m venv .venv
. .venv/bin/activate
python -m pip install --upgrade pip
python -m pip install -e .
python -m unittest discover -s tests -v
```

The Docker path is recommended for the full ZAP capture because the container network is already wired so the client can reach ZAP as `http://zap:8090`.

## Configuration

Copy [.env.example](.env.example) to `.env` if you want to change defaults.

Important variables:

- `INVALID_IBM_QUANTUM_TOKEN`: defaults to `fake-ibm-quantum-token` and must start with `fake-`.
- `IBM_QUANTUM_CHANNEL`: defaults to `ibm_quantum_platform`.
- `IBM_QUANTUM_INSTANCE`: optional IBM Runtime instance or CRN. Usually left blank for this invalid-auth probe.
- `ZAP_PROXY`: defaults to `http://zap:8090` in Docker.
- `ZAP_API`: defaults to `http://zap:8090` in Docker.

## Project Layout

```text
qiskit-zap-ibm-demo/
  docker-compose.yml
  Dockerfile
  pyproject.toml
  src/qiskit_zap_ibm_demo/
    client.py
    qiskit_payload.py
    redact.py
    zap.py
  tests/
    test_payload.py
    test_redact.py
    test_zap.py
  artifacts/
    .gitkeep
```