# What This Project Does

This project runs a controlled network-capture demo for the IBM Qiskit Runtime Python package.

It uses intentionally invalid IBM Quantum credentials, routes the IBM-bound HTTPS traffic through OWASP ZAP, records what ZAP sees going out and coming back, redacts sensitive token values, and writes the capture to files under `artifacts/`.

The project no longer uses a mock IBM service. It talks to the real IBM IAM authentication endpoint, but only with a fake API key.

## Short Version

When you run the project, it does this:

```text
Start OWASP ZAP
-> build a Bell circuit with Qiskit
-> configure qiskit-ibm-runtime with a fake IBM API key
-> send the IBM Runtime authentication attempt through ZAP
-> IBM IAM rejects the fake key
-> ZAP records the outbound request and inbound response
-> the client writes redacted artifacts
```

The expected successful result is not a completed quantum job. The expected successful result is an IBM authentication error captured by ZAP.

## What It Uses

The main pieces are:

- Docker Compose: starts ZAP and the Python client container.
- OWASP ZAP: acts as the HTTP/S inspection proxy.
- Qiskit: builds a local Bell circuit.
- `qiskit-ibm-runtime`: creates the IBM Runtime service client and attempts IBM authentication.
- IBM IAM: receives the fake API key token exchange request and rejects it.

## What It Does Not Do

This project does not bypass IBM authentication.

It does not submit work to a real IBM Quantum backend when using the default fake token.

It does not avoid IBM account permissions, quotas, costs, or access controls.

It does not use the old local mock IBM API anymore.

## How To Run It

From the project root:

```bash
docker compose --profile demo up --build --abort-on-container-exit client
```

After the run finishes, clean up the remaining ZAP container:

```bash
docker compose down --remove-orphans
```

The expected terminal summary looks like this:

```text
Credentials accepted: False
Stage: authentication_or_runtime_call
Error type: InvalidAccountError
Captured messages: 3
```

That means the end-to-end demo worked: the IBM client sent an authentication request through ZAP, IBM rejected the fake key, and ZAP captured the exchange.

## Runtime Flow In Detail

The `client` container runs this command:

```bash
qiskit-zap-demo run
```

That command lives in [src/qiskit_zap_ibm_demo/client.py](src/qiskit_zap_ibm_demo/client.py).

The code first loads configuration from environment variables:

```text
INVALID_IBM_QUANTUM_TOKEN=fake-ibm-quantum-token
IBM_QUANTUM_CHANNEL=ibm_quantum_platform
ZAP_PROXY=http://zap:8090
ZAP_API=http://zap:8090
ARTIFACT_DIR=/app/artifacts
```

Then it waits for OWASP ZAP to be ready and starts a fresh ZAP session.

Next, it builds a local Bell circuit using Qiskit:

```text
h q[0]
cx q[0],q[1]
measure q[0] -> c[0]
measure q[1] -> c[1]
```

The circuit is included in the Markdown transcript so you can see what the local Qiskit side created. With fake credentials, that circuit does not reach IBM as a submitted job because authentication fails first.

## How The IBM Call Is Made

The code creates a `QiskitRuntimeService` from `qiskit-ibm-runtime`:

```python
service = QiskitRuntimeService(
    channel="ibm_quantum_platform",
    token="fake-ibm-quantum-token",
    proxies={"urls": {"http": "http://zap:8090", "https": "http://zap:8090"}},
    verify=False,
)
```

Then it calls:

```python
service.backends()
```

That causes the IBM SDK to authenticate with IBM IAM. Since the token is fake, IBM IAM rejects it before backend discovery can succeed.

The code is prepared to call `SamplerV2.run(...)` only if authentication and backend discovery succeed. With the default fake key, execution never reaches that job-submission step.

## How ZAP Sees HTTPS Traffic

The IBM request is HTTPS, so it is encrypted on the network.

ZAP sees the decrypted content because the client is configured to use ZAP as an HTTPS proxy and TLS verification is disabled for this fake-token demo.

Conceptually, the traffic path is:

```text
qiskit-ibm-runtime client
-> HTTPS through ZAP proxy
-> ZAP inspects the decrypted HTTP request
-> ZAP opens its own HTTPS connection to IBM IAM
-> IBM IAM sends the response
-> ZAP records the decrypted HTTP response
-> ZAP passes the response back to the client
```

This is proxy-based TLS inspection. It is not passive packet sniffing before encryption inside the Python process.

## What Goes Out To IBM

The meaningful outbound request captured by ZAP is an IBM IAM token exchange:

```http
POST https://iam.cloud.ibm.com/identity/token HTTP/1.1
host: iam.cloud.ibm.com
User-Agent: ibm-python-sdk-core/iam-authenticator-3.24.4 os.name=Linux os.version=6.12.76-linuxkit python.version=3.12.13
Accept: application/json
Connection: keep-alive
Content-Type: application/x-www-form-urlencoded
```

The request body is redacted before being written:

```text
grant_type=urn%3Aibm%3Aparams%3Aoauth%3Agrant-type%3Aapikey&apikey=<redacted>&response_type=cloud_iam
```

Meaning: the IBM SDK is asking IBM IAM to turn the configured API key into an IAM access token.

## What Comes Back From IBM

IBM IAM rejects the fake API key and returns:

```http
HTTP/1.1 400 Bad Request
ibm-cloud-service-name: iam-identity
Content-Type: application/json
```

The response body says:

```json
{
  "errorCode": "BXNIM0415E",
  "errorMessage": "Provided API key could not be found."
}
```

The Python client reports that as:

```text
InvalidAccountError: Unable to retrieve instances. Please check that you are using a valid API token.
```

## Why It Stops Before Job Submission

The fake API key fails during IBM IAM authentication.

The runtime chain is:

```text
QiskitRuntimeService(...)
-> service.backends()
-> IBM IAM token exchange
-> 400 Bad Request
-> InvalidAccountError
-> stop
```

Because authentication fails, the client never receives a valid IAM access token, never gets an IBM backend list, and never submits the Bell circuit as a runtime job.

## Generated Files

Each end-to-end run writes two generated files under `artifacts/`.

### `artifacts/http-transcript.md`

This is the human-readable transcript. Open this file first.

It contains:

- a summary of the run,
- the attempted IBM Runtime call,
- the local Qiskit circuit,
- the Python error raised by the IBM SDK,
- each ZAP-captured request and response.

### `artifacts/zap-messages.json`

This is the structured ZAP export after redaction.

It contains the same captured messages as JSON objects with fields like:

```text
requestHeader
requestBody
responseHeader
responseBody
rtt
timestamp
```

Use this file if you want to inspect or process the captured traffic programmatically.

## The Two Extra ZAP Entries

In the latest run, ZAP captured three messages total.

The meaningful message is the `POST https://iam.cloud.ibm.com/identity/token` request and its `400 Bad Request` response.

The other two messages are empty ZAP-recorded GET entries:

```http
GET https://iam.cloud.ibm.com HTTP/1.1
GET https://iam.cloud.ibm.com/identity HTTP/1.1
```

They have empty bodies and `HTTP/1.0 0` response headers in the ZAP export. They are not separate successful IBM API calls. The important request/response pair is the IAM token exchange.

## Redaction

The project redacts token-like values before writing artifacts.

The default fake token is:

```text
fake-ibm-quantum-token
```

The artifact output shows it as:

```text
apikey=<redacted>
```

The redaction code lives in [src/qiskit_zap_ibm_demo/redact.py](src/qiskit_zap_ibm_demo/redact.py).

## Important Source Files

- [docker-compose.yml](docker-compose.yml): defines the `zap` and `client` services.
- [Dockerfile](Dockerfile): builds the Python runtime image.
- [pyproject.toml](pyproject.toml): declares dependencies, including `qiskit` and `qiskit-ibm-runtime`.
- [src/qiskit_zap_ibm_demo/client.py](src/qiskit_zap_ibm_demo/client.py): main CLI, IBM Runtime call, ZAP capture, artifact writing.
- [src/qiskit_zap_ibm_demo/qiskit_payload.py](src/qiskit_zap_ibm_demo/qiskit_payload.py): local Qiskit Bell circuit builder.
- [src/qiskit_zap_ibm_demo/zap.py](src/qiskit_zap_ibm_demo/zap.py): ZAP API helpers.
- [src/qiskit_zap_ibm_demo/redact.py](src/qiskit_zap_ibm_demo/redact.py): artifact redaction helpers.

## Bottom Line

This project demonstrates this specific behavior:

```text
Use qiskit-ibm-runtime with a fake IBM API key
-> route the IBM-bound HTTPS authentication request through OWASP ZAP
-> let ZAP inspect and record the decrypted request and response
-> show IBM's real authentication failure response
-> redact token material
-> save a Markdown transcript and JSON message export
```

The project is working when IBM returns an invalid-credentials error and ZAP records the request/response. With fake credentials, a real quantum job submission is not expected.