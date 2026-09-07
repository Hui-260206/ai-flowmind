## 1. Observability foundation

- [x] 1.1 Add the Prometheus Go client and create a Go-owned metrics registry with the seven specified chat metric families and only bounded labels.
- [x] 1.2 Register a read-only Prometheus-compatible `GET /metrics` route independently of readiness and preserve all existing API routes and envelopes.
- [x] 1.3 Instrument final send outcomes and end-to-end latency in the HTTP/chat boundary without double-counting idempotent or failed paths.
- [x] 1.4 Instrument Go gRPC duration/timeouts, message persistence failures, and Redis session coordination failures at their concrete workflow boundaries.
- [x] 1.5 Add focused metrics tests for success, timeout, Redis/persistence failure, endpoint availability during failed readiness, and prohibition of high-cardinality identifiers/content in metric labels.

## 2. Real-dependency verification plumbing

- [x] 2.1 Align Redis integration-test configuration with the application's address, database, and password settings; namespace and clean up only test-owned keys.
- [x] 2.2 Add strict readiness configuration validation that requires a safe test database target and reachable credentialed MySQL/Redis instead of treating unmet prerequisites as a pass.
- [x] 2.3 Create host-process fixture helpers that launch the Python gRPC service with `AI_PROVIDER=fake`, launch the real Go composition root on configurable loopback ports, wait with bounded deadlines, and always terminate child processes.
- [x] 2.4 Add a documented Make target that loads `services/.env`, invokes the strict readiness harness, and returns non-zero for missing configuration, unavailable dependencies, startup failure, or failed assertions.

## 3. Server-readiness end-to-end coverage

- [x] 3.1 Implement the real HTTP flow check for Go readiness, session creation, message send, ordered history, stable Fake Provider response, and request-ID propagation.
- [x] 3.2 Implement real-dependency assertions for same-ID idempotent replay and cross-owner session/history concealment.
- [x] 3.3 Implement a Go-only restart check that proves previously persisted history remains available to its original owner.
- [x] 3.4 Implement controlled Python-unavailable and AI-deadline checks that assert the established `502 AI_UNAVAILABLE` and `504 AI_TIMEOUT` response contracts.
- [x] 3.5 Implement a controlled Redis-unavailable check that asserts `503 REDIS_UNAVAILABLE` for a new send while existing MySQL-backed history remains readable.
- [x] 3.6 Ensure the harness uses unique test ownership/data, cleans up only its own MySQL records and Redis keys, and produces actionable process logs when it fails.

## 4. Documentation and verification

- [x] 4.1 Document strict readiness prerequisites, environment safety guardrails, command usage, expected Fake Provider behavior, failure diagnosis, and the Compose-infrastructure-only boundary in `services/README.md`.
- [x] 4.2 Document the `/metrics` endpoint, metric semantics, permitted labels, and the distinction between metrics and `request_id` log correlation.
- [x] 4.3 Run and record `make test-go`, `make test-ai`, linting, metric tests, and the strict readiness target against configured MySQL/Redis; resolve failures before marking the change complete.（2026-09-07：`make test-go`、`make test-ai`、`make lint-ai`、Go tagged integration tests 与 `make test-server-readiness` 均通过；严格验证使用 `flowmind_test` 和独立 Redis `127.0.0.1:6380/DB 15`。）

## 5. Remaining stage-8 readiness and local topology

- [x] 5.1 Add a controlled MySQL-unavailable readiness check using explicit operator-supplied stop/start commands, fail `/readyz`, assert a repository-backed REST request fails, and wait for authenticated recovery.
- [x] 5.2 Add an HTTP-client-cancellation E2E check against a slow Fake Provider and assert that no assistant message is persisted after disconnect.
- [x] 5.3 Add reproducible Go and Python Dockerfiles, a build-safe dockerignore, and Compose services that wire Go, Python, MySQL, and Redis through an internal network with only Go HTTP exposed.
- [x] 5.4 Document Compose configuration/start/stop, readiness fault-injection prerequisites, and verification limitations when Docker is unavailable.
