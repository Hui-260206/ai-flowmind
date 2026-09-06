## Why

The current chat endpoint prevents duplicate user-message rows through MySQL, but a network retry receives `409` instead of the original successful result; it also permits overlapping AI generations in one session. Before mobile clients integrate, the API needs bounded, observable protection against retries, concurrent sends, and accidental request bursts without making Redis the source of chat history.

## What Changes

- Add Redis-backed idempotency coordination for `POST /api/v1/sessions/{session_id}/messages`, including `processing`, `completed`, and `failed` states and replay of a previously completed message pair.
- Serialize one session's full generation workflow with a TTL-backed Redis lock and return `409 SESSION_BUSY` for a concurrently submitted, distinct request.
- Enforce a fixed-window anonymous-device send limit of 20 requests per minute and return `429 RATE_LIMITED` when exceeded.
- Define controlled behavior when Redis is unavailable: message-send requests fail with `503 REDIS_UNAVAILABLE`; persisted MySQL sessions and history remain readable.
- Add Redis reliability configuration, structured lifecycle/error logs, and focused unit/integration tests for retry, concurrency, timeout, and unavailable-cache paths.
- **BREAKING**: A repeated `client_message_id` for a completed request changes from `409 DUPLICATE_REQUEST` to a `200` replay of the original successful response. A second distinct in-flight request for the same session receives `409 SESSION_BUSY`.
- Keep RabbitMQ, Outbox publishing/consumption, streaming, and mobile changes out of scope; stage 7 remains deferred beyond the MVP.

## Capabilities

### New Capabilities

- `redis-chat-reliability`: Redis-coordinated idempotency, session serialization, anonymous-device rate limiting, and Redis-outage behavior for synchronous chat sends.

### Modified Capabilities

- `go-chat-api`: Changes synchronous send-message duplicate, concurrency, rate-limit, and dependency-error response behavior.

## Impact

- Affected Go API areas: Redis client wrapper, configuration, chat application service, HTTP error mapping, production composition root, and Go tests.
- Affected public API: `POST /api/v1/sessions/{session_id}/messages` gains `409 SESSION_BUSY`, `429 RATE_LIMITED`, and `503 REDIS_UNAVAILABLE` semantics; completed retries return the existing successful response shape.
- Affected dependencies: reuse the existing `github.com/redis/go-redis/v9` client; no RabbitMQ client, broker, Python AI-service, schema migration, or mobile change is introduced.
- MySQL remains the durable source of sessions and messages. Redis stores only expiring coordination state and message identifiers, never chat content as an authoritative record.
