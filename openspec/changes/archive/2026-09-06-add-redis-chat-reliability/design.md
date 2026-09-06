## Context

The Go API already uses Redis for its readiness check, but the synchronous send-message workflow only relies on MySQL's `(session_id, client_message_id)` uniqueness check. Consequently, a completed request whose HTTP response is lost cannot be replayed, and two distinct sends may each write a user message and build AI context while the other generation is still in progress. MySQL remains the durable record of sessions and messages; Redis is available through `go-redis/v9` and is appropriate only for short-lived coordination.

The public contract already reserves `409 DUPLICATE_REQUEST` / `SESSION_BUSY` and `429 RATE_LIMITED`. This change makes those reliability behaviors concrete before mobile integration. It must preserve the existing REST success shape, request-ID envelope, Python gRPC boundary, and no-MQ MVP boundary.

## Goals / Non-Goals

**Goals:**

- Ensure one `client_message_id` is processed at most once per owner/session while its Redis idempotency record exists, and replay the completed persisted pair on retry.
- Ensure only one distinct message-generation workflow is active for a session at a time.
- Bound anonymous-device sends to 20 new or resumed attempts per fixed one-minute window.
- Make Redis failure explicit for send requests rather than silently dropping idempotency, locking, or rate-limit guarantees.
- Keep all message content and durable history in MySQL.

**Non-Goals:**

- RabbitMQ, Outbox publication or consumers, asynchronous title generation, streaming, or mobile changes.
- A permanent user-to-assistant reply relation. A small MySQL send-operation record persists recovery identifiers and state so a Redis response-loss window cannot strand a completed or resumable request.
- Distributed lock renewal, automatic AI retries, cross-region Redis replication, or a general-purpose rate-limiting platform.
- Guaranteeing replay after an idempotency record expires or Redis loses its volatile data; MySQL's existing unique constraint remains the final no-duplicate-write defense in that case.

## Decisions

### 1. Coordinate by owner, session, and client message ID

The idempotency key SHALL be `idempotency:{owner_key}:{session_id}:{client_message_id}`. Including `session_id` matches the existing MySQL uniqueness scope and prevents a client that reuses an ID in two different sessions from receiving a result for the wrong session. Redis key components will use a safe deterministic encoding so separators or Redis glob syntax in a client-generated ID cannot alter key structure.

The value is a small JSON record containing a random operation token, lifecycle state, a canonical request fingerprint, and, once available, user and assistant message IDs. It never stores message content. The fingerprint covers the normalized content and model profile, so reuse of an ID with different input returns `409 DUPLICATE_REQUEST` rather than replaying an unrelated request.

Alternatives considered:

- `owner_key + client_message_id` only: rejected because it conflicts with MySQL's per-session uniqueness scope.
- Caching the full response in Redis: rejected because MySQL is the fact source and response content could be large or stale.
- New database tables or a reply-relation migration: deferred to keep the MVP change focused; the record retains exact message IDs for its 48-hour lifetime.

### 2. Use a three-state idempotency state machine with token-checked transitions

The coordinator creates or claims a record atomically with Redis Lua scripts. It has these states:

```text
new request ──claim──▶ processing ──success──▶ completed
                              │
                              └─ failure after user persistence ──▶ failed
                                                                   │
                                                         same fingerprint retry
                                                                   └─▶ processing
```

- `processing`: another caller with the same key receives `409 DUPLICATE_REQUEST` and the model is not called again.
- `completed`: the service reads the recorded user/assistant IDs from MySQL, validates they belong to the owned session, and returns the normal `200` send-message payload. A missing or inconsistent pair is treated as a controlled internal reliability failure, not as permission to invoke the model again.
- `failed`: records the already-persisted user-message ID. A same-fingerprint retry claims it atomically and resumes generation from that message; it does not insert a second user message.

Each transition, deletion, and failed-state restoration is conditioned on the operation token. This prevents an older request from clearing or overwriting state acquired by a newer request. New records that cannot proceed because of rate limiting or session-lock contention are deleted; a claimed failed record is restored to `failed` with its original user-message ID. The TTL is refreshed on each state transition and fixed at 48 hours.

Redis commands have an unavoidable response-loss ambiguity: a database write can succeed immediately before Redis becomes unreachable, or a Redis Lua script can succeed while its reply is lost. Therefore `chat_send_operations` persists the owner/session/client-message scope, fingerprint, lifecycle state, and exact user/assistant message IDs. An assistant message also carries its send-operation ID. On a retry, the service acquires the session lock, reclaims an interrupted Redis record, then reconciles the durable operation: it resumes from a stored user message, discovers an assistant already written before an interrupted operation update, completes session metadata, and finally repopulates Redis. Redis remains the short-lived coordination and fast-replay layer; MySQL is the recovery authority.

After Redis TTL expiry or data loss, the durable operation record safely replays the exact completed pair or resumes its recorded user message. It never guesses a user/assistant pairing from sequence order.

### 3. Lock the full session generation workflow with a leased token lock

After an idempotency claim and before persisting or resuming a message, the service attempts `SET lock:conversation:{session_id} <random-token> NX PX <ttl>`. It releases with a compare-and-delete Lua script; a plain `DEL` is forbidden because an expired lock could have been acquired by a different request.

The lock covers user-message persistence or recovery, context loading, Python gRPC completion, assistant persistence, session metadata update, and idempotency finalization. Lock acquisition failure returns `409 SESSION_BUSY`, and the corresponding idempotency claim is safely rolled back as described above.

The initial lock TTL is 30 seconds. It exceeds the existing 15-second HTTP write timeout and 14-second gRPC deadline, leaving room for persistence and cleanup; the handler context cancellation still ends the active workflow. This MVP uses a deliberately bounded synchronous operation rather than lock renewal. A future longer-running or streaming workflow must add token-checked renewal before extending its timeout budget.

### 4. Apply rate limiting only to newly accepted or resumed work

The rate-limit key is `rate_limit:owner:{owner_key}:{UTC-minute}`. An atomic Lua operation increments the counter and assigns its expiry on first use. The maximum is 20 requests in the one-minute fixed window.

The service first checks or claims idempotency. A completed replay and a duplicate already in `processing` consume no quota; only a newly claimed operation or a failed-operation resume is rate-limited. If the limit rejects that operation, its claim is rolled back so a later request is not incorrectly held in `processing`. This ordering avoids penalizing a client for retrying after a lost successful response.

A fixed UTC-minute window is selected for the MVP because it is explainable and inexpensive. Sliding windows and token buckets are better for smoother traffic, but introduce extra state and are not needed for the initial anonymous-device guardrail.

### 5. Fail closed for send operations when Redis coordination is unavailable

Any Redis read, script, lock, rate-limit, or release failure that prevents the send workflow from enforcing its guarantee maps to `503 REDIS_UNAVAILABLE`. The API logs the operation stage and request ID but never a message body or Redis credentials. The send handler does not fall back to an unlocked workflow.

Read-only session/history endpoints and session deletion remain MySQL-only and continue to work while Redis is down. `/readyz` already marks Redis as required, so operators can detect the outage before traffic is routed. This is preferred over fail-open because duplicate AI charges and interleaved context are worse than an explicit retryable error.

### 6. Keep the application service independent of go-redis

Introduce a narrow chat-facing reliability port with operations for idempotency claim/transitions, session lock acquire/release, and rate-limit check. The concrete Redis adapter lives below the application layer and is wired in `cmd/api/main.go`; deterministic in-memory fakes exercise the state machine in unit tests. The existing Go Redis client remains the only external library integration.

This preserves the current separation between chat use cases, HTTP, persistence repositories, and gRPC completion. Letting `chat.Service` issue `go-redis` commands directly would make lifecycle behavior harder to test and couple business sequencing to a protocol client.

## Risks / Trade-offs

- [Redis expires or loses an idempotency record] → the durable send-operation record lets the API replay a completed pair or resume the exact persisted user message; it never guesses a pair from sequence order.
- [Lock lease expires before a pathological workflow finishes] → a 30-second lease exceeds all current synchronous timeout budgets; enforce timeout ordering and revisit with token-checked renewal before increasing those budgets.
- [An HTTP client cancels after the provider has completed] → context cancellation stops further work where possible; any persisted user message is marked `failed` only when the workflow owns the idempotency token and can be resumed with the same ID.
- [Redis outage blocks sends] → return retryable `503`, retain MySQL history access, expose the existing readiness failure, and use structured logs to identify the failed coordination stage.
- [Fixed-window boundary permits a brief burst around a minute change] → accepted MVP trade-off; migrate to a token bucket only if actual traffic requires smoother enforcement.
- [Expired replay record with a completed pair cannot be returned automatically] → the mobile client can refresh history; exact replay is intentionally bounded to the 48-hour idempotency window.

## Migration Plan

1. Add configuration defaults and validate that the lock TTL exceeds the current synchronous timeout budget.
2. Implement the Redis reliability adapter and unit-test its atomic scripts against a Redis test instance or deterministic adapter contract.
3. Inject the reliability port into the chat service, retain the existing MySQL uniqueness check as the final fallback, and add HTTP mappings for `SESSION_BUSY`, `RATE_LIMITED`, and `REDIS_UNAVAILABLE`.
4. Deploy with the existing Redis dependency, then verify a completed retry, same-session concurrent sends, 21 sends in one minute, and an induced Redis outage.
5. Roll back by removing the reliability dependency from the composition root and restoring current duplicate-conflict behavior. Existing Redis coordination keys expire naturally; no MySQL migration or data rollback is required.

## Open Questions

- None for the MVP. The 48-hour idempotency TTL, 30-second lock TTL, and 20 requests per UTC minute are intentionally fixed starting values and will be exposed as validated configuration for later operational tuning.
