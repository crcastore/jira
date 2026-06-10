# How The ZAP Capture Works

This project runs a Python program inside Docker, routes its IBM-bound HTTP/S traffic through OWASP ZAP, then writes a redacted transcript of what ZAP captured.

The important idea is that ZAP is not reading Python objects directly. ZAP is acting as a proxy between the Python client and IBM. The Python code makes a normal IBM Runtime SDK call, the SDK sends HTTP/S traffic through the proxy, and ZAP records the request and response.

## One-Screen Flow

```text
./run.sh or make demo
-> docker compose starts ZAP
-> docker compose runs the Python client container
-> Python waits for ZAP to be ready
-> Python builds a local Qiskit Bell circuit
-> Python configures qiskit-ibm-runtime to use ZAP as its proxy
-> service.backends() triggers IBM authentication/backend discovery
-> IBM IAM rejects the fake API key
-> ZAP records the outbound request and inbound response
-> Python fetches messages from the ZAP API
-> Python redacts token-like values
-> Python writes artifacts/http-transcript.md and artifacts/zap-messages.json
```

## What Starts ZAP

ZAP is started by Docker Compose in `docker-compose.yml`.

```yaml
services:
  zap:
    image: ghcr.io/zaproxy/zaproxy:stable
    command:
      - zap.sh
      - -daemon
      - -host
      - 0.0.0.0
      - -port
      - "8090"
    ports:
      - "8090:8090"
```

Inside the Docker Compose network, the client reaches ZAP at:

```text
http://zap:8090
```

From your Mac, the same ZAP container is exposed at:

```text
http://localhost:8090
```

The Compose file also disables the ZAP API key and allows API access from the Compose network:

```yaml
- -config
- api.disablekey=true
- -config
- api.addrs.addr.name=.*
- -config
- api.addrs.addr.regex=true
```

That is why the Python client can call the ZAP API to reset the session and fetch captured messages.

## What Starts The Python Client

The `client` service in `docker-compose.yml` builds this repository into a Docker image and runs:

```yaml
command: qiskit-zap-demo run
```

The `qiskit-zap-demo` command is defined in `pyproject.toml`:

```toml
[project.scripts]
qiskit-zap-demo = "qiskit_zap_ibm_demo.client:app"
```

So this command enters the Typer app in:

```text
src/qiskit_zap_ibm_demo/client.py
```

The Docker image itself comes from `Dockerfile`, which is based on:

```dockerfile
FROM python:3.12-slim
```

So the Python program runs inside the `client` container, not directly on your Mac.

## How `run.sh` Fits In

`run.sh` is just a convenience wrapper around Docker Compose.

It does four things:

1. Checks that Docker and Docker Compose are available.
2. Checks that Docker Desktop is running.
3. Runs the Compose demo.
4. Cleans up containers when the script exits.

The important command in `run.sh` is:

```sh
$COMPOSE up --build --abort-on-container-exit --exit-code-from client client
```

That starts the dependency service `zap`, runs the `client` service, and exits with the client's exit code.

## What The Python `run()` Command Does

The main command is `run()` in `src/qiskit_zap_ibm_demo/client.py`.

It loads configuration from environment variables passed by Docker Compose:

```yaml
INVALID_IBM_QUANTUM_TOKEN: ${INVALID_IBM_QUANTUM_TOKEN:-fake-ibm-quantum-token}
IBM_QUANTUM_CHANNEL: ${IBM_QUANTUM_CHANNEL:-ibm_quantum_platform}
IBM_QUANTUM_INSTANCE: ${IBM_QUANTUM_INSTANCE:-}
ZAP_PROXY: ${ZAP_PROXY:-http://zap:8090}
ZAP_API: ${ZAP_API:-http://zap:8090}
ARTIFACT_DIR: /app/artifacts
```

The default token intentionally starts with `fake-`. The code refuses to run unless the token starts with `fake-`, because this demo is intended to show a rejected authentication request, not submit real IBM work.

Then `run()` does this:

```text
wait_for_zap(zap_api)
start_new_session(zap_api)
build_bell_circuit()
build_attempt_details(...)
probe_ibm_runtime_invalid_auth(...)
fetch_zap_messages(zap_api)
filter_messages_for_targets(..., ("ibm.com",))
write_artifacts(...)
print_summary(...)
```

## What `build_attempt_details()` Does

`build_attempt_details()` does not send anything to IBM.

It only creates local metadata that gets written into the transcript. It describes what the program is about to try:

```python
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
```

That dictionary is later redacted and printed under `Attempted Runtime Call` in `artifacts/http-transcript.md`.

## The Line That Sends Traffic To IBM

The actual IBM Runtime attempt happens in `probe_ibm_runtime_invalid_auth()` in `client.py`.

The SDK client is created here:

```python
service = QiskitRuntimeService(
    channel=channel,
    token=token,
    instance=instance,
    proxies=proxies,
    verify=verify_tls,
)
```

The first line that triggers the important IBM authentication/backend-discovery traffic is:

```python
backends = service.backends()
```

In the current file, that is line 140.

That call causes `qiskit-ibm-runtime` and IBM's Python SDK internals to contact IBM IAM. In the captured run, ZAP saw this outbound request:

```http
POST https://iam.cloud.ibm.com/identity/token HTTP/1.1
```

The body contains an IAM API-key token exchange request. The artifact redacts the fake key:

```text
grant_type=urn%3Aibm%3Aparams%3Aoauth%3Agrant-type%3Aapikey&apikey=<redacted>&response_type=cloud_iam
```

IBM rejects the fake key and returns an error like:

```json
{
  "errorCode": "BXNIM0415E",
  "errorMessage": "Provided API key could not be found."
}
```

The Python code catches that exception and records it in the generated transcript.

The `QiskitRuntimeService(...)` arguments are local SDK configuration. They are not all sent to IBM as one object. The API key is later used in IBM's IAM token exchange request, but the proxy configuration is only used by the local Python client to decide how to route the HTTP/S connection.

## Why The Sampler Job Usually Does Not Run

After backend discovery, the code is prepared to submit a tiny Qiskit sampler job:

```python
backend = backends[0]
job = SamplerV2(mode=backend).run([circuit], shots=1)
result = job.result()
```

With the default fake token, execution does not get that far. IBM rejects the API key during authentication/backend discovery, so there is no backend list and no submitted quantum job.

That is expected. The demo is successful when the fake-token authentication request is captured and IBM rejects it.

## How Python Is Routed Through ZAP

The key proxy configuration is in `probe_ibm_runtime_invalid_auth()`:

```python
proxies = {"urls": {"http": zap_proxy, "https": zap_proxy}}
```

That gets passed into `QiskitRuntimeService(...)`:

```python
proxies=proxies,
verify=verify_tls,
```

That proxy object is passed to the local IBM Runtime SDK client. It is not sent to IBM as a field named `proxies`. From IBM's point of view, it receives a normal HTTPS request. From the Python client's point of view, the request is routed through ZAP first.

IBM's SDK supports proxy settings because many enterprise environments require outbound HTTP/S traffic to go through a corporate proxy for firewall, audit, routing, or inspection reasons. This project uses the same normal proxy feature, but points it at OWASP ZAP.

By default, `verify_tls` is `False`. That matters because ZAP is inspecting HTTPS traffic. The client talks to ZAP as its HTTPS proxy, ZAP opens its own HTTPS connection to IBM, and ZAP records the decrypted request and response.

Conceptually:

```text
Python qiskit-ibm-runtime client
-> proxy connection to ZAP at http://zap:8090
-> ZAP records the HTTP/S request
-> ZAP forwards to https://iam.cloud.ibm.com
-> IBM sends the response
-> ZAP records the response
-> ZAP passes the response back to Python
```

This is proxy-based HTTP/S inspection. It is not passive packet capture.

## How Captured Messages Are Read Back

ZAP records traffic internally while the client runs.

After the IBM SDK call fails, the Python code asks ZAP for the captured messages:

```python
messages = filter_messages_for_targets(fetch_zap_messages(zap_api), IBM_CAPTURE_HOSTS)
```

`fetch_zap_messages()` lives in `src/qiskit_zap_ibm_demo/zap.py`. It calls the ZAP API endpoint:

```text
/JSON/core/view/messages/
```

`filter_messages_for_targets()` then keeps messages that mention the configured target host. For this demo, the host filter is:

```python
IBM_CAPTURE_HOSTS = ("ibm.com",)
```

That keeps IBM-related traffic and avoids writing unrelated proxy noise into the artifacts.

If authentication had succeeded and the Python code had made more IBM Runtime calls through the same proxied SDK client, those additional HTTP/S exchanges would also be captured by ZAP. They would appear as additional message objects before filtering and, if they matched `ibm.com`, in the written artifact.

ZAP only captures traffic routed through ZAP. It will not capture traffic from another process, non-HTTP traffic, a library that ignores the proxy settings, or messages filtered out before artifact writing.

## What `zap-messages.json` Contains

`artifacts/zap-messages.json` is a redacted ZAP export. It is not only the IBM response, and it is not only what Python sent. Each object is one ZAP-captured HTTP/S message with request fields, response fields, and ZAP metadata.

The important fields are:

```text
requestHeader   what the Python client sent out
requestBody     the outbound request body
responseHeader  what came back from IBM or the upstream server
responseBody    the inbound response body
```

The surrounding fields are ZAP bookkeeping:

```text
id
rtt
timestamp
tags
type
note
cookieParams
```

In the current artifact, message 1 is the meaningful IBM IAM exchange. Its request is:

```http
POST https://iam.cloud.ibm.com/identity/token HTTP/1.1
```

Its response starts with:

```http
HTTP/1.1 400 Bad Request
ibm-cloud-service-name: iam-identity
Content-Type: application/json
```

Its response body is the IBM IAM error saying the fake API key could not be found. Messages 2 and 3 are empty ZAP-recorded GET entries with `HTTP/1.0 0`; they are not meaningful IBM API responses.

## How The Artifacts Are Written

The final artifact step happens in `write_artifacts()` in `client.py`.

It writes two files:

```text
artifacts/http-transcript.md
artifacts/zap-messages.json
```

Before writing, it redacts the captured messages, the local attempt metadata, and the result object:

```python
redacted_messages = redact_json(messages)
redacted_attempt = redact_json(attempt)
redacted_result = redact_json(probe_result)
```

The redaction code lives in `src/qiskit_zap_ibm_demo/redact.py`. It redacts keys and text patterns related to tokens, API keys, authorization headers, passwords, and secrets.

`artifacts/zap-messages.json` is the structured redacted ZAP export.

`artifacts/http-transcript.md` is the readable Markdown version produced by `build_markdown_transcript()`.

## Which File To Read First

Read these in this order:

1. `run.sh` if you want to understand how the local command starts the containers.
2. `docker-compose.yml` if you want to understand where ZAP and the client run.
3. `src/qiskit_zap_ibm_demo/client.py` if you want to understand the Python control flow.
4. `src/qiskit_zap_ibm_demo/zap.py` if you want to understand the ZAP API calls.
5. `src/qiskit_zap_ibm_demo/redact.py` if you want to understand artifact redaction.
6. `artifacts/http-transcript.md` if you want to inspect the latest captured run.

## How This Could Become An Arbitrary Python Capture Tool

Right now the project is wired to one built-in Qiskit Runtime demo. To make it run an arbitrary Python file, the reusable pieces would stay the same:

```text
start/wait for ZAP
start a clean ZAP session
run some Python code through the proxy
fetch ZAP messages
filter messages
redact messages
write artifacts
```

The part that would change is `probe_ibm_runtime_invalid_auth()`. Instead of calling `QiskitRuntimeService(...)` directly, a generic version would run a user-provided Python file with proxy environment variables set:

```sh
HTTP_PROXY=http://zap:8090
HTTPS_PROXY=http://zap:8090
http_proxy=http://zap:8090
https_proxy=http://zap:8090
```

Then it would fetch and write the same ZAP artifacts after that script exits.

The main caveats are:

- The target Python library must respect proxy settings, or it must expose its own proxy configuration.
- HTTPS capture requires trusting ZAP's certificate or disabling TLS verification for the test run.
- Redaction is best-effort and should be reviewed before sharing captures.
- Running with real credentials would capture sensitive traffic, so the current project intentionally defaults to fake credentials.
