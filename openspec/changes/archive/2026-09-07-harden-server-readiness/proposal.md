## Why

The chat service has solid unit and HTTP-contract coverage, but mobile integration still lacks repeatable evidence that the real Go, MySQL, Redis, and Python gRPC chain behaves correctly under normal operation, restart, and dependency failures. It also emits correlated logs without exposing the baseline metrics needed to detect regressions and investigate failures.

## What Changes

- Add a strict, documented server-readiness verification path that runs against real MySQL, Redis, and the Python AI service using its deterministic Fake Provider.
- Add repeatable end-to-end checks for the mobile-facing REST conversation lifecycle, durable-history recovery, idempotent replay, ownership isolation, and controlled dependency failures.
- Keep fast unit tests separate from dependency-backed checks; readiness verification must fail rather than silently skip when its required services or credentials are unavailable.
- Expose a minimal Prometheus-compatible metrics endpoint for chat outcomes, latency, AI gRPC latency/timeouts, message-persistence failures, and Redis lock failures.
- Add controlled MySQL-outage and HTTP-client-cancellation readiness checks, plus a complete local Docker Compose topology for Go API, Python AI, MySQL, and Redis.

## Capabilities

### New Capabilities

- `server-readiness-validation`: Strict, repeatable real-dependency and end-to-end verification for the mobile-facing server API.
- `chat-observability`: Low-cardinality Prometheus-compatible metrics for the synchronous chat workflow and its critical dependencies.

### Modified Capabilities

<!-- None. Existing chat API behavior and error contracts remain unchanged. -->

## Impact

- Affects Go test/verification tooling, the service Makefile, test fixtures, and service operations documentation.
- Adds a Go metrics dependency and a read-only HTTP `/metrics` endpoint; no mobile API request or response shape changes.
- Exercises the existing MySQL repositories, Redis reliability adapter, Go HTTP API, Go gRPC client, and Python gRPC server with the Fake Provider.
- Requires an environment that supplies MySQL and Redis credentials for strict readiness verification; Docker is required only for the optional Compose runtime path.
