# IBM Runtime Traffic Capture Through OWASP ZAP

This project currently demonstrates what the IBM Qiskit Runtime Python package sends over the network when it is configured with intentionally invalid IBM Quantum credentials and routed through OWASP ZAP.

The important result: ZAP is working. It captured an outbound IBM IAM authentication request and the inbound IBM error response. The flow stops at authentication because the fake API key is rejected before any IBM Quantum job can be submitted.

## Current Flow

```text
Docker Compose starts OWASP ZAP
-> Docker Compose starts the Python client
-> Qiskit builds a local Bell circuit
-> qiskit-ibm-runtime creates a QiskitRuntimeService with a fake token
-> qiskit-ibm-runtime tries to authenticate with IBM IAM
-> the HTTPS request is routed through ZAP
-> IBM IAM rejects the fake API key
-> the client fetches ZAP's captured messages
-> captured messages are redacted and written to artifacts/
```

The client does not use the old mock IBM service anymore. The mock API was removed. The only external target in the current run is IBM IAM.

## Key Files

- [docker-compose.yml](docker-compose.yml): starts ZAP and the profiled client container.
- [src/qiskit_zap_ibm_demo/client.py](src/qiskit_zap_ibm_demo/client.py): configures the IBM Runtime client, routes it through ZAP, collects ZAP messages, and writes artifacts.
- [src/qiskit_zap_ibm_demo/qiskit_payload.py](src/qiskit_zap_ibm_demo/qiskit_payload.py): builds the local Bell circuit with Qiskit.
- [src/qiskit_zap_ibm_demo/zap.py](src/qiskit_zap_ibm_demo/zap.py): waits for ZAP, starts a clean session, fetches messages, and filters IBM traffic.
- [src/qiskit_zap_ibm_demo/redact.py](src/qiskit_zap_ibm_demo/redact.py): redacts bearer/API-token-like values before writing artifacts.
- [artifacts/http-transcript.md](artifacts/http-transcript.md): readable transcript from the most recent run.
- [artifacts/zap-messages.json](artifacts/zap-messages.json): redacted raw ZAP message export from the most recent run.

## Compose Setup

The `zap` service runs OWASP ZAP in daemon/proxy mode on port `8090`.

The `client` service is wired with these relevant environment variables:

```yaml
INVALID_IBM_QUANTUM_TOKEN: fake-ibm-quantum-token
IBM_QUANTUM_CHANNEL: ibm_quantum_platform
IBM_QUANTUM_INSTANCE: ""
ZAP_PROXY: http://zap:8090
ZAP_API: http://zap:8090
ARTIFACT_DIR: /app/artifacts
```

Inside the client code, the IBM Runtime service receives this proxy configuration:

```python
proxies = {"urls": {"http": zap_proxy, "https": zap_proxy}}
```

The client also runs with TLS verification disabled for this controlled fake-token demo:

```text
verify_tls: false
```

That lets ZAP inspect the HTTPS traffic. Without that, the IBM Runtime client would reject ZAP's interception certificate unless ZAP's CA certificate were explicitly trusted by the client container.

## What ZAP Captured

The latest end-to-end run captured 3 ZAP messages.

Only one of them is a full, meaningful IBM request/response exchange. The other two are empty ZAP-recorded GET entries for IBM IAM host paths with `HTTP/1.0 0` response headers.

## Main Outbound Request

The real outbound request was an IBM IAM token request:

```http
POST https://iam.cloud.ibm.com/identity/token HTTP/1.1
host: iam.cloud.ibm.com
User-Agent: ibm-python-sdk-core/iam-authenticator-3.24.4 os.name=Linux os.version=6.12.76-linuxkit python.version=3.12.13
Accept: application/json
Connection: keep-alive
Content-Type: application/x-www-form-urlencoded
Content-Length: 113
```

Request body, redacted:

```text
grant_type=urn%3Aibm%3Aparams%3Aoauth%3Agrant-type%3Aapikey&apikey=<redacted>&response_type=cloud_iam
```

Meaning: the IBM SDK is trying to exchange the configured IBM API key for an IBM Cloud IAM token. Since the key is fake, IBM rejects it.

## Main Inbound Response

IBM returned:

```http
HTTP/1.1 400 Bad Request
ibm-cloud-service-name: iam-identity
Content-Type: application/json
Content-Language: en-US
strict-transport-security: max-age=31536000; includeSubDomains
```

Response body:

```json
{
  "errorCode": "BXNIM0415E",
  "errorMessage": "Provided API key could not be found."
}
```

The Python client surfaced this as:

```text
InvalidAccountError: Unable to retrieve instances. Please check that you are using a valid API token.
```

## The Two Extra ZAP Messages

ZAP also recorded these two entries:

```http
GET https://iam.cloud.ibm.com HTTP/1.1
GET https://iam.cloud.ibm.com/identity HTTP/1.1
```

Both entries had empty request and response bodies, and their response headers were:

```http
HTTP/1.0 0
```

Those are not the important authentication exchange. The meaningful captured traffic is the `POST /identity/token` request and the `400 Bad Request` response from IBM IAM.

## Encryption And ZAP Inspection

The traffic to IBM is HTTPS, so it is encrypted on the network.

In this demo, the client deliberately routes HTTPS through ZAP and disables TLS certificate verification. That allows ZAP to act as the TLS inspection proxy for this fake-token test run.

Conceptually:

```text
qiskit-ibm-runtime client
-> HTTPS request through ZAP proxy
-> ZAP inspects decrypted HTTP content
-> ZAP opens its own upstream HTTPS connection to IBM IAM
-> IBM IAM responds
-> ZAP records the decrypted response
-> client receives the response and raises InvalidAccountError
```

So yes: ZAP is seeing what goes out and what comes back. It is not seeing a later quantum job submission because the fake token fails before the runtime client can discover backends or submit a job.

## Why No Quantum Job Is Submitted

The code is prepared to call `SamplerV2.run(...)` only if authentication and backend discovery succeed.

With the fake token, the call fails earlier:

```text
QiskitRuntimeService(...)
-> service.backends()
-> IBM IAM token exchange
-> 400 Bad Request
-> InvalidAccountError
-> stop
```

Because authentication fails, there is no valid IBM IAM access token, no backend list, and no job submission request.

## Redaction

Artifacts are redacted before being written.

The fake token is stored in Compose as:

```text
fake-ibm-quantum-token
```

But the artifact request body shows:

```text
apikey=<redacted>
```

The latest artifact check found no raw fake token in [artifacts/http-transcript.md](artifacts/http-transcript.md) or [artifacts/zap-messages.json](artifacts/zap-messages.json).

## How To Reproduce

Run:

```bash
docker compose --profile demo up --build --abort-on-container-exit client
```

Then inspect:

```text
artifacts/http-transcript.md
artifacts/zap-messages.json
```

Clean up afterward:

```bash
docker compose down --remove-orphans
```

## Bottom Line

The project is doing what it is currently designed to do:

```text
Use qiskit-ibm-runtime with fake IBM credentials
-> route the IBM-bound HTTPS auth request through OWASP ZAP
-> capture the outbound request and inbound IBM error response
-> redact token material
-> write the results to artifacts/
```

The expected result with fake credentials is an IBM IAM authentication failure, not a quantum job submission.