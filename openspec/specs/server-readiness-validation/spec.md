## Purpose

Define strict real-dependency validation and the private full-stack Compose topology required before mobile clients integrate with the server.

## Requirements

### Requirement: Strict dependency-backed readiness gate
The services project SHALL provide a documented command that validates server readiness against real MySQL and Redis and host-run Python and Go service processes. The command MUST load the supported local service configuration, require usable MySQL and Redis credentials, force the Python AI service to use its deterministic Fake Provider, and fail with a non-zero result when configuration, dependencies, process startup, or an assertion is unavailable or fails. It MUST NOT report success solely because a dependency-backed check was skipped.

#### Scenario: Run readiness verification with configured dependencies
- **WHEN** an operator runs the documented readiness command with reachable MySQL and Redis and valid local configuration
- **THEN** it starts the Python Fake Provider and Go API, waits for Go readiness, executes the required end-to-end checks, cleans up its child processes and test-scoped data, and exits successfully only after all checks pass

#### Scenario: Reject missing readiness prerequisites
- **WHEN** an operator runs the readiness command without required configuration or with unreachable MySQL or Redis
- **THEN** it exits non-zero with an actionable prerequisite or dependency failure and does not classify skipped tests as a successful readiness result

### Requirement: Production-shaped REST conversation verification
The readiness gate SHALL exercise the mobile-facing REST API through the real Go composition root, MySQL repositories, Redis reliability adapter, Go gRPC client, and Python gRPC server using the Fake Provider. It MUST verify session creation, sending a valid message, ordered history retrieval, `request_id` propagation, and a stable completed assistant response without calling an external model provider.

#### Scenario: Complete a real-dependency conversation
- **WHEN** the readiness gate creates a session and sends a valid message with valid anonymous client and request IDs
- **THEN** the API returns the normal success response, persists one completed user message and one completed assistant message in sequence, returns them in ordered history, and the Python Fake Provider receives the propagated request identity

### Requirement: Durable and isolated REST behavior verification
The readiness gate SHALL verify the public API's existing idempotency, anonymous-ownership, and persistence guarantees against real dependencies. It MUST prove that a same-fingerprint retry with the same `client_message_id` replays the original pair without another generated message, that another owner cannot access the session or its history, and that a Go API restart preserves the original owner's readable history.

#### Scenario: Replay an idempotent real-dependency send
- **WHEN** the readiness gate submits the same valid send request twice with the same owner, session ID, client message ID, normalized content, and model profile
- **THEN** both responses succeed with the same persisted message identities and history contains exactly one user/assistant pair

#### Scenario: Conceal a session from another anonymous owner
- **WHEN** the readiness gate requests a known session or its messages using a distinct valid anonymous client ID
- **THEN** the API returns the established not-found response and exposes no session or message content

#### Scenario: Preserve history across Go restart
- **WHEN** the readiness gate successfully persists a conversation, restarts only the Go API process, and requests the original session history as its owner
- **THEN** the API becomes ready again and returns the same ordered persisted message pair

### Requirement: Controlled dependency-failure and cancellation verification
The readiness gate SHALL verify the existing API contract for critical dependency failures without using an external provider. It MUST prove that unavailable Python AI produces `502 AI_UNAVAILABLE`, an AI deadline produces `504 AI_TIMEOUT`, unavailable Redis produces `503 REDIS_UNAVAILABLE` for a new send while MySQL-backed history remains readable, and an operator-controlled MySQL outage makes readiness fail and repository-backed REST requests fail until recovery. It MUST also prove that a disconnected HTTP client cannot cause an assistant message to be persisted after cancellation.

#### Scenario: Handle unavailable Python AI service
- **WHEN** the readiness gate stops or makes unavailable the Python gRPC service before a new valid send
- **THEN** the API returns `502` with `AI_UNAVAILABLE` and the Go readiness response reports the Python gRPC dependency as failing

#### Scenario: Handle AI deadline expiration
- **WHEN** the readiness gate uses a controlled Python Fake Provider response that exceeds the configured Go AI RPC deadline
- **THEN** the API returns `504` with `AI_TIMEOUT` and does not continue waiting for a completed assistant response

#### Scenario: Handle unavailable Redis while retaining history reads
- **WHEN** the readiness gate makes Redis unavailable after a conversation is persisted and then submits a new valid send
- **THEN** the send returns `503` with `REDIS_UNAVAILABLE` while the existing owner can still retrieve the persisted session history

#### Scenario: Handle unavailable MySQL and restore readiness
- **WHEN** the readiness gate invokes its explicitly configured MySQL stop command after a conversation has been persisted
- **THEN** `/readyz` returns not-ready and a repository-backed REST request fails; after the configured start command, the harness waits for an authenticated MySQL connection and `/readyz` to become ready again

#### Scenario: Cancel a slow HTTP send
- **WHEN** a client disconnects while a deliberately delayed Fake Provider is handling a valid send
- **THEN** the HTTP request context cancels its gRPC work, the durable user message may remain, and no assistant message for that operation is persisted

### Requirement: Shared protected-Redis test configuration
All dependency-backed Redis verification SHALL use the same supported address, database, and password configuration contract as the Go API. Test code and harnesses MUST namespace Redis keys and remove only keys created for their own run.

#### Scenario: Verify against password-protected Redis
- **WHEN** Redis requires the configured password
- **THEN** the Redis integration checks and readiness gate authenticate successfully and execute their reliability assertions without disabling authentication

### Requirement: Complete local Compose topology
The services project SHALL provide reproducible Go API and Python AI images and a Compose topology containing Go API, Python AI, MySQL, and Redis. MySQL and Redis MUST retain health checks and persistent volumes, Go MUST wait for their healthy state, and only the Go HTTP listener MAY be published to the host. Python gRPC, MySQL, and Redis MUST remain private to the Compose backend network.

#### Scenario: Start full local service stack
- **WHEN** an operator configures a private deployment environment file and runs the documented Compose startup command
- **THEN** Compose builds the Go/Python images, starts all four services, applies Go's embedded migrations after MySQL is healthy, and makes Go `/readyz` reachable through the configured host HTTP port
