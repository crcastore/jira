# OWASP ZAP IBM Runtime Invalid Auth Transcript

## Demo Summary

- Target: `IBM Quantum Runtime`
- Credentials accepted: `False`
- Stage: `authentication_or_runtime_call`
- Exception type: `InvalidAccountError`
- Captured messages: `3`

## Attempted Runtime Call

```json
{
  "channel": "ibm_quantum_platform",
  "instance": null,
  "operation": "QiskitRuntimeService(...), service.backends(), then SamplerV2.run(...) if auth succeeds",
  "package": "qiskit-ibm-runtime",
  "proxy": "http://zap:8090",
  "token": "<redacted>",
  "verify_tls": false
}
```

## Local Qiskit Circuit

```json
{
  "format": "openqasm2",
  "name": "bell_pair",
  "qasm": "OPENQASM 2.0;\ninclude \"qelib1.inc\";\nqreg q[2];\ncreg c[2];\nh q[0];\ncx q[0],q[1];\nmeasure q[0] -> c[0];\nmeasure q[1] -> c[1];"
}
```

## Returned Error

```json
{
  "credentials_accepted": false,
  "exception_message": "'Unable to retrieve instances. Please check that you are using a valid API token.'",
  "exception_type": "InvalidAccountError",
  "stage": "authentication_or_runtime_call"
}
```

## ZAP Messages

### Message 1: HTTP unknown-url

Status code: `unknown`

Request headers:

```http
POST https://iam.cloud.ibm.com/identity/token HTTP/1.1
host: iam.cloud.ibm.com
User-Agent: ibm-python-sdk-core/iam-authenticator-3.24.4 os.name=Linux os.version=6.12.76-linuxkit python.version=3.12.13
Accept: application/json
Connection: keep-alive
Content-Type: application/x-www-form-urlencoded
Content-Length: 113
```

Request body:

```json
grant_type=urn%3Aibm%3Aparams%3Aoauth%3Agrant-type%3Aapikey&apikey=<redacted>&response_type=cloud_iam
```

Response headers:

```http
HTTP/1.1 400 Bad Request
x-content-type-options: nosniff
ibm-cloud-service-name: iam-identity
transaction-id: YnA4N2o-238821a41f6f40c48ef98917a695fe7c
x-request-id: ae78d3a6-b624-43a8-b9aa-28431843fdd7
x-correlation-id: YnA4N2o-238821a41f6f40c48ef98917a695fe7c
Cache-Control: no-cache, no-store, must-revalidate
Expires: 0
Pragma: no-cache
Content-Type: application/json
Content-Language: en-US
Content-Length: 627
strict-transport-security: max-age=31536000; includeSubDomains
Date: Wed, 10 Jun 2026 10:25:03 GMT
Connection: close
Akamai-GRN: 0.25b5655f.1781087102.14e30781
x-proxy-upstream-service-time: 53
```

Response body:

```json
{
  "context": {
    "clusterName": "iam-id-prod-eu-de-fra02",
    "elapsedTime": "45",
    "endTime": "10.06.2026 10:25:03:062 UTC",
    "host": "iamid-11-5-5390-a968904-68bd454f74-bp87j",
    "instanceId": "iamid-11-5-5390-a968904-68bd454f74-bp87j",
    "locale": "en_US",
    "requestId": "YnA4N2o-238821a41f6f40c48ef98917a695fe7c",
    "requestType": "incoming.Identity_Token",
    "startTime": "10.06.2026 10:25:03:017 UTC",
    "threadId": "150e",
    "url": "https://iam.cloud.ibm.com",
    "userAgent": "ibm-python-sdk-core/iam-authenticator-3.24.4 os.name=Linux os.version=6.12.76-linuxkit python.version=3.12.13"
  },
  "errorCode": "BXNIM0415E",
  "errorMessage": "Provided API key could not be found."
}
```

### Message 2: HTTP unknown-url

Status code: `unknown`

Request headers:

```http
GET https://iam.cloud.ibm.com HTTP/1.1
host: iam.cloud.ibm.com
User-Agent: ibm-python-sdk-core/iam-authenticator-3.24.4 os.name=Linux os.version=6.12.76-linuxkit python.version=3.12.13
Accept: application/json
Connection: keep-alive
```

Request body:

```json
{}
```

Response headers:

```http
HTTP/1.0 0
```

Response body:

```json
{}
```

### Message 3: HTTP unknown-url

Status code: `unknown`

Request headers:

```http
GET https://iam.cloud.ibm.com/identity HTTP/1.1
host: iam.cloud.ibm.com
User-Agent: ibm-python-sdk-core/iam-authenticator-3.24.4 os.name=Linux os.version=6.12.76-linuxkit python.version=3.12.13
Accept: application/json
Connection: keep-alive
```

Request body:

```json
{}
```

Response headers:

```http
HTTP/1.0 0
```

Response body:

```json
{}
```
