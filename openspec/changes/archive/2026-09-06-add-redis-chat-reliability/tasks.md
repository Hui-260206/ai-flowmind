## 1. Reliability configuration and error contract

- [x] 1.1 Add validated Redis reliability configuration for a 48-hour idempotency TTL, 30-second session-lock TTL, and 20-request UTC-minute owner limit; reject a lock TTL that cannot cover the configured synchronous timeout budget.
- [x] 1.2 Add domain errors and HTTP/OpenAPI mappings for `SESSION_BUSY` (409), `RATE_LIMITED` (429), and `REDIS_UNAVAILABLE` (503) while preserving the request-ID error envelope.
- [x] 1.3 Add configuration and HTTP error-mapping tests for valid defaults, invalid duration/limit relationships, and all new response codes.

## 2. Redis reliability adapter

- [x] 2.1 Define a narrow chat-facing reliability port and data types for idempotency claims, fingerprints, persisted message IDs, operation tokens, session locks, and rate-limit decisions.
- [x] 2.2 Implement safe deterministic Redis key construction and atomic Lua-backed idempotency transitions for create, processing duplicate, completed replay, failed resume, rollback, and token-checked completion/failure updates.
- [x] 2.3 Implement token-owned Redis session lock acquisition and compare-and-delete release with the configured lease TTL.
- [x] 2.4 Implement the atomic per-owner, UTC-minute fixed-window rate counter, including first-increment expiry and the 20-request limit.
- [x] 2.5 Add adapter tests for every idempotency state transition, mismatched fingerprints, token ownership, lock replacement safety, rate-window expiry, and Redis command failures.

## 3. MySQL recovery lookups and application workflow

- [x] 3.1 Extend the message repository with ownership-safe lookup of an exact persisted message by ID and add repository tests for found, absent, and wrong-session results.
- [x] 3.2 Inject the reliability port into the chat service and sequence validation, idempotency claim/replay, rate-limit check, session-lock lifecycle, and existing persistence/completion workflow.
- [x] 3.3 Persist the user-message ID into the idempotency record before AI completion; on downstream failure transition to resumable `failed`, and on success atomically record both message IDs as `completed`.
- [x] 3.4 Preserve the MySQL unique-key duplicate check as the safe fallback when a Redis idempotency record is absent or expired, without calling the AI completer again.
- [x] 3.5 Add service-level deterministic tests for completed replay, in-progress duplicate rejection, failed-send resume without a second user row, same-session busy rejection, expired-record fallback, and Redis unavailable fail-closed behavior.

## 4. Production wiring and end-to-end contract verification

- [x] 4.1 Wire the Redis reliability adapter and its validated configuration through the Go production composition root; keep session/history reads independent of the new send-path coordination port.
- [x] 4.2 Update the API contract and local environment examples with the new error responses and non-sensitive Redis reliability settings.
- [x] 4.3 Add HTTP/integration tests covering lost-response retry replay, two simultaneous distinct sends to one session, twenty-first request rate limiting, and MySQL history reads during an induced Redis outage.
- [x] 4.4 Run `make test-go` and relevant Redis/MySQL integration checks; record results and any local-service prerequisites in the change validation notes.

## 5. Recovery hardening from review

- [x] 5.1 Add a durable MySQL send-operation recovery record and migration, including exact user/assistant identifiers and a message-to-operation recovery link.
- [x] 5.2 Reconcile interrupted Redis claims under the session lock so Redis attach/complete response-loss failures resume or replay without a duplicate model call.
- [x] 5.3 Treat idempotency rollback and session-lock release failures as controlled Redis failures; add deterministic recovery tests for Redis attach, Redis completion, and session-metadata update failures.
