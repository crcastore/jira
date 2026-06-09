# Qiskit IBM Runtime ZAP Capture

This project shows what the IBM Qiskit Runtime Python package sends over the network when it is configured with an intentionally fake IBM API key and routed through OWASP ZAP.

The expected result is an IBM authentication error captured by ZAP. No quantum job is submitted with the default fake credentials.

## Run It

```bash
make demo
```

Equivalent Docker command:

```bash
docker compose up --build --abort-on-container-exit client
```

After the run, open:

- [artifacts/http-transcript.md](artifacts/http-transcript.md) for the readable ZAP transcript.
- [artifacts/zap-messages.json](artifacts/zap-messages.json) for the redacted raw ZAP export.
- [PROJECT_EXPLANATION.md](PROJECT_EXPLANATION.md) for the full explanation of how the project works.

Clean up ZAP afterward:

```bash
make down
```

## Expected Output

```text
Credentials accepted: False
Stage: authentication_or_runtime_call
Error type: InvalidAccountError
Captured messages: 3
```

That means `qiskit-ibm-runtime` reached IBM IAM through ZAP, IBM rejected the fake key, and the request/response was captured and redacted.

## Useful Commands

```bash
make demo   # run the full ZAP capture
make test   # run the unit tests in Docker
make up     # start only ZAP
make run    # run only the client against an already-running ZAP
make down   # stop containers and remove the Compose network
```

## Configuration

Copy [.env.example](.env.example) to `.env` if you want to override defaults.

Important values:

- `INVALID_IBM_QUANTUM_TOKEN`: defaults to `fake-ibm-quantum-token` and must start with `fake-`.
- `IBM_QUANTUM_CHANNEL`: defaults to `ibm_quantum_platform`.
- `IBM_QUANTUM_INSTANCE`: optional IBM Runtime instance or CRN.
- `ZAP_PROXY`: defaults to `http://zap:8090` in Docker.
- `ZAP_API`: defaults to `http://zap:8090` in Docker.

## Requirements

- Docker Desktop with Docker Compose v2.
- Internet access from Docker containers.