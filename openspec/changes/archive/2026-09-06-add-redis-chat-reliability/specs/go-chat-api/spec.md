## MODIFIED Requirements

### Requirement: Synchronous text message completion
The Go API SHALL provide `POST /api/v1/sessions/{session_id}/messages` to synchronously persist one user text message and one completed assistant text message. The request MUST require non-blank `content` and non-blank `client_message_id`, enforce the contract limits of 32,000 Unicode characters and 128 characters respectively, and validate session ownership before any message is persisted. A completed retry with the same owner, session ID, `client_message_id`, normalized content, and model profile MUST return the original persisted pair rather than a conflict or a second AI invocation.

#### Scenario: Send a valid message and receive persisted pair
- **WHEN** an anonymous client sends valid content and a new `client_message_id` to one of its active sessions
- **THEN** the API persists a completed user message, invokes the configured AI completer with bounded conversation context, persists its completed assistant result, and returns `200` containing `request_id`, `user_message`, and `assistant_message`

#### Scenario: Validate message request input
- **WHEN** a client sends malformed JSON, blank content, content longer than 32,000 Unicode characters, blank `client_message_id`, or a `client_message_id` longer than 128 characters
- **THEN** the API returns `400` with error code `INVALID_ARGUMENT` and persists no new message

#### Scenario: Reject a message for an unowned or absent session
- **WHEN** a client sends a valid message request to a session that is absent, deleted, or owned by another client
- **THEN** the API returns `404` with error code `SESSION_NOT_FOUND` and persists no new message

#### Scenario: Replay a completed duplicate client message identifier
- **WHEN** a client sends a message whose `client_message_id` already identifies a completed send for the same owner and session with identical normalized input inside the idempotency retention period
- **THEN** the API returns `200` with the originally persisted user and assistant messages, persists no additional message, and does not invoke the AI completer again

#### Scenario: Reject an in-progress or mismatched duplicate identifier
- **WHEN** a client sends a message whose `client_message_id` identifies an in-progress request or has different normalized input in the same owner and session
- **THEN** the API returns `409` with error code `DUPLICATE_REQUEST`, persists no additional message, and does not invoke the AI completer again

### Requirement: Consistent request identity and error envelope
Every Go chat API response that includes a JSON error SHALL use `{request_id, error: {code, message}}`, and the response header `X-Request-ID` SHALL contain the same request identity generated or accepted by existing middleware. Missing or invalid anonymous-client identity MUST be reported as `400 INVALID_ARGUMENT`; an in-flight distinct send for the same session MUST be reported as `409 SESSION_BUSY`; rate limiting MUST be reported as `429 RATE_LIMITED`; required Redis coordination failure MUST be reported as `503 REDIS_UNAVAILABLE`; AI-service unavailability or provider failure MUST be reported as `502 AI_UNAVAILABLE` or `AI_PROVIDER_ERROR`; AI timeout MUST be reported as `504 AI_TIMEOUT`; unexpected application failures MUST be reported as `500 INTERNAL_ERROR` without exposing internal details.

#### Scenario: Return a traceable validation error
- **WHEN** a chat API request has a missing or invalid `X-Client-ID`
- **THEN** the API returns `400` with `INVALID_ARGUMENT`, an error envelope containing `request_id`, and a matching `X-Request-ID` response header

#### Scenario: Return a traceable session-busy error
- **WHEN** a valid distinct send request cannot acquire its session's active generation lock
- **THEN** the API returns `409` with `SESSION_BUSY`, an error envelope containing `request_id`, and a matching `X-Request-ID` response header

#### Scenario: Return a traceable rate-limit error
- **WHEN** a valid newly claimed or resumed send exceeds the anonymous owner's current fixed-window quota
- **THEN** the API returns `429` with `RATE_LIMITED`, an error envelope containing `request_id`, and a matching `X-Request-ID` response header

#### Scenario: Return a traceable Redis-unavailable error
- **WHEN** a valid send cannot enforce required Redis coordination
- **THEN** the API returns `503` with `REDIS_UNAVAILABLE`, an error envelope containing `request_id`, and a matching `X-Request-ID` response header

#### Scenario: Return a traceable AI timeout
- **WHEN** the configured Python AI completion exceeds its deadline
- **THEN** the API returns `504` with `AI_TIMEOUT`, an error envelope containing `request_id`, and a matching `X-Request-ID` response header

#### Scenario: Contain an unexpected internal failure
- **WHEN** an unexpected persistence or application failure occurs before a response is written
- **THEN** the API returns `500` with error code `INTERNAL_ERROR` and does not expose the underlying error detail
