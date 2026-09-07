## Context

The Go API already has deterministic unit and HTTP-contract tests, tagged MySQL/Redis integration tests, a Python gRPC AI service, readiness probes, and JSON logs correlated by `request_id`. The tagged dependency tests currently skip when infrastructure or credentials are missing, while no single command boots the host-process Go/Python chain and proves the mobile-facing REST flow against real MySQL and Redis. No metric registry or `/metrics` endpoint exists.

The mobile client needs a release gate that proves the production-shaped synchronous path without sending traffic to HY3. The deterministic Python Fake Provider is the suitable integration-test provider. Host-process readiness remains the fault-injection path, while Compose provides a portable complete local startup path.

## Goals / Non-Goals

**Goals:**

- Establish a strict, repeatable readiness command that requires configured MySQL and Redis and starts the real Python Fake Provider and Go API.
- Exercise the REST lifecycle through the real Go HTTP server, repositories, Redis reliability adapter, and Python gRPC boundary.
- Make normal and failure-path evidence machine-checkable, including restart persistence, idempotency, ownership isolation, and prerequisite failures.
- Expose bounded-cardinality metrics for the synchronous chat workflow without changing its REST contract.
- Retain `request_id` as the log-correlation mechanism and document how it complements metrics.
- Exercise an operator-configured MySQL outage/recovery and HTTP-client cancellation.
- Provide Dockerfiles and Compose services for the Go API and Python AI service.

**Non-Goals:**

- Calling HY3 or another external provider during automated readiness verification.
- Adding distributed tracing, dashboards, alerting rules, authentication, streaming, RabbitMQ, or a generic telemetry platform.
- Replacing existing fast unit tests or altering existing chat API success/error responses.

## Decisions

### 1. Separate strict readiness verification from optional developer integration tests

Keep focused tagged repository/Redis tests useful for local development, but add a separate `test-server-readiness` Make target and host-process test harness. The target SHALL load `services/.env`, validate mandatory MySQL and Redis configuration, force `AI_PROVIDER=fake`, and fail when prerequisites, startup, or assertions fail. It SHALL not treat a skipped dependency test as readiness success.

The harness will use unique test identifiers and clean up only records it created. It starts Python before Go, waits for Go `/readyz`, and shuts down both child processes on success, failure, or interruption. Its checks use the public REST API rather than package-private methods so it also validates wiring, migrations, request headers, and gRPC propagation.

Alternative considered: change every existing tagged test from `Skip` to `Fatal`. Rejected because those tests are also valuable as opt-in local checks and need not make ordinary developer workflows fail. A dedicated strict gate expresses the stronger contract cleanly.

### 2. Use the Python Fake Provider and real MySQL/Redis for E2E

The readiness chain will run Python's actual gRPC server with the configured Fake Provider, alongside Go's actual composition root and real infrastructure connections. It will verify create/list/send/history, same-ID replay, cross-owner concealment, and history after a Go restart. It will also assert the established controlled responses for unavailable Python, Redis, and a bounded AI deadline using test-controlled child-process lifecycle/configuration.

Alternative considered: use in-memory repositories and a mocked gRPC client. Rejected because this duplicates existing unit coverage and cannot prove migrations, Redis scripts/credentials, network wiring, or process restart durability.

### 3. Make Redis integration configuration match the application configuration

Dependency-backed Redis tests and the readiness harness will use the same address, database, and password configuration contract as the production client. A protected Redis must therefore be testable without weakening Compose security. Tests will namespace their keys and remove only those keys after completion.

Alternative considered: start a separate password-free Redis solely for tests. Rejected because it fails to validate the configuration mode used by the service and introduces another lifecycle dependency.

### 4. Provide one Go-owned, Prometheus-compatible `/metrics` endpoint

The Go API will own a registry and expose a read-only, unauthenticated `GET /metrics` endpoint in Prometheus text format. Instrumentation lives at workflow boundaries: HTTP request completion, gRPC completion duration/error class, timeout occurrence, assistant/user persistence errors, and Redis lock acquisition/release failures. The endpoint remains available independently of `/readyz` so an unhealthy dependency does not prevent diagnosis.

Metrics will use only bounded labels such as operation, result/error code, and HTTP status class. They MUST NOT use `request_id`, owner/client IDs, session/message IDs, raw paths containing IDs, message content, model output, or provider URLs as labels. `request_id` remains in structured logs for per-request investigation.

Alternative considered: OpenTelemetry-only instrumentation or metrics emitted by Python. Rejected for this phase: the mobile-facing SLO boundary is Go, and a small Prometheus client adds a directly scrapeable, testable operational surface without requiring a collector.

### 5. Treat counters as semantic outcomes and histograms as bounded latency evidence

`chat_request_total` and `chat_request_failed_total` count completed send attempts according to their final HTTP-visible result. Replayed sends are successes but distinguishable through a bounded `outcome` label. `chat_request_latency` measures the whole send handler workflow. `ai_grpc_latency` measures each call from the Go completer; `ai_timeout_total` increments only for deadline expiry. `message_persist_failed_total` and `redis_lock_failed_total` increment at their concrete failure boundary. Metrics should not count endpoint reads as chat requests.

Alternative considered: capture every internal state transition. Rejected because it creates noisy, hard-to-interpret metrics and risks double-counting failure paths.

### 6. Make fault injection explicit and verify cancellation end to end

The readiness environment provides explicit commands to stop and start its dedicated MySQL instance. The harness confirms that `/readyz` becomes unavailable and that a repository-backed REST request fails while MySQL is down, restores the service in cleanup even after an assertion failure, and waits for authenticated MySQL and Go readiness before continuing.

For cancellation, a deliberately slow Fake Provider is paired with a short-lived HTTP client. The harness proves the request context cancels the Go-to-Python gRPC call: the user message may be durable because it precedes generation, but no assistant message can be appended after the client disconnects.

### 7. Use Compose as a four-service local topology

Compose builds Go and Python images from the `services/` context. Python generates ignored protobuf stubs during its image build. MySQL and Redis retain persistent named volumes and health checks; Go waits for those checks and reaches Python over the private backend network. Only Go HTTP maps to the host.

## Risks / Trade-offs

- [Host-process readiness tests can be flaky due to ports or leaked child processes] → allocate explicit configurable loopback ports, wait with bounded deadlines, prefix logs, and use deferred process-group cleanup.
- [Real-infrastructure tests can pollute developer data] → require an explicitly configured test database or a test-name suffix/guard, generate unique owner/session IDs, and clean up only data/keys created by the harness.
- [Failure injection can be nondeterministic] → control the Go/Python child processes and per-test timeouts rather than relying on arbitrary network disruption.
- [Prometheus label misuse can create unbounded series] → centralize metric creation, enumerate permitted labels in tests, and prohibit identifier/content labels in the spec.
- [A new metrics dependency modestly increases binary size] → use the official Prometheus Go client only for the required registry and HTTP handler.

## Migration Plan

1. Add metrics registry/instrumentation and its HTTP route while preserving existing routes and error envelopes.
2. Add strict configuration validation, dependency fixture helpers, and the host-process readiness harness using Fake Provider only.
3. Document prerequisites and commands, then add the readiness target to the server verification workflow.
4. Add controlled MySQL-loss and client-disconnect assertions to the readiness gate.
5. Build the Go/Python images and add them to the existing Compose network, retaining one externally mapped Go HTTP port.
6. Run fast Go/Python suites and the strict readiness gate with configured MySQL/Redis; validate Compose when Docker is available.
7. Rollback consists of removing the new target/endpoint/instrumentation/images; existing chat storage and public API data remain unchanged because this change adds no schema migration.

## Open Questions

- Which CI runner will provide MySQL and Redis credentials for the strict readiness gate? The implementation will keep the command host-portable until CI provisioning is selected.
- Whether `/metrics` should stay on the main HTTP listener or move to a dedicated internal listener when deployment hardening begins. This proposal keeps the main listener to avoid premature deployment topology work.
