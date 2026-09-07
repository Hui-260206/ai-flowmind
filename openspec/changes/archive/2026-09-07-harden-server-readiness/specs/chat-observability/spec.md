## ADDED Requirements

### Requirement: Prometheus-compatible chat metrics endpoint
The Go API SHALL expose a read-only `GET /metrics` endpoint that serves Prometheus-compatible text exposition from the application's metrics registry. The endpoint MUST remain available for diagnosis independently of the readiness status of MySQL, Redis, or Python gRPC, and it MUST NOT alter the existing chat API response contracts.

#### Scenario: Scrape metrics while a dependency is unready
- **WHEN** a dependency causes `/readyz` to report not-ready
- **THEN** `GET /metrics` remains scrapeable and returns the registered metric families in Prometheus text format

### Requirement: Synchronous chat workflow metrics
The Go API SHALL expose the following metrics for synchronous message sends: `chat_request_total`, `chat_request_failed_total`, `chat_request_latency`, `ai_grpc_latency`, `ai_timeout_total`, `message_persist_failed_total`, and `redis_lock_failed_total`. Counters MUST increase only at their defined semantic boundary, and latency metrics MUST record non-negative durations in a consistent unit.

#### Scenario: Record a successful chat workflow
- **WHEN** a new send persists a user message, receives a completed AI response, and persists the assistant message
- **THEN** `chat_request_total` and `chat_request_latency` record the successful send and `ai_grpc_latency` records the corresponding gRPC call without incrementing failure-only counters

#### Scenario: Record a timed-out AI workflow
- **WHEN** a send reaches the Go AI RPC deadline
- **THEN** `chat_request_total`, `chat_request_failed_total`, `chat_request_latency`, `ai_grpc_latency`, and `ai_timeout_total` record the final timeout outcome, and the response retains the established `504 AI_TIMEOUT` contract

#### Scenario: Record Redis lock and persistence failures
- **WHEN** a send fails while acquiring or releasing required Redis session coordination, or when message persistence fails
- **THEN** the corresponding `redis_lock_failed_total` or `message_persist_failed_total` counter increments in addition to the workflow's failed-send metrics

### Requirement: Bounded metric labels and log correlation
The metrics implementation SHALL use only bounded operational labels, such as operation name, outcome/error code, and HTTP status class. It MUST NOT use request IDs, client/owner IDs, session IDs, message IDs, message content, model output, provider URLs, or raw parameterized request paths as metric labels. Structured Go and Python logs SHALL continue to use the propagated `request_id` for per-request correlation.

#### Scenario: Inspect emitted metric labels after a chat request
- **WHEN** an operator scrapes metrics after successful and failed sends using distinct request and session identifiers
- **THEN** metric series distinguish only the documented bounded labels and contain none of those request-specific or content-bearing values
