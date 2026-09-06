## ADDED Requirements

### Requirement: Anonymous client session lifecycle
The Go API SHALL provide `POST /api/v1/sessions`, `GET /api/v1/sessions`, and `DELETE /api/v1/sessions/{session_id}` for the caller identified by `X-Client-ID`. The API MUST derive the owner key through the domain owner-key function and MUST return only non-deleted sessions belonging to that owner, ordered by most recently updated first.

#### Scenario: Create a session for an anonymous client
- **WHEN** a request with a valid `X-Client-ID` calls `POST /api/v1/sessions`
- **THEN** the API returns `201` with a new active session owned by that client, titled `新会话`, using model profile `default`, and including creation and update timestamps

#### Scenario: List only the caller's sessions
- **WHEN** an anonymous client calls `GET /api/v1/sessions` after sessions belonging to multiple clients exist
- **THEN** the API returns `200` with only that client's non-deleted sessions ordered by descending `updated_at`

#### Scenario: Delete an owned session
- **WHEN** an anonymous client calls `DELETE /api/v1/sessions/{session_id}` for one of its active sessions
- **THEN** the API soft-deletes the session and returns `204`

#### Scenario: Hide an unowned or absent session
- **WHEN** an anonymous client requests deletion of a session that is absent, deleted, or owned by another client
- **THEN** the API returns `404` with error code `SESSION_NOT_FOUND` and does not reveal whether the session exists for another owner

### Requirement: Ordered message history with ownership isolation
The Go API SHALL provide `GET /api/v1/sessions/{session_id}/messages` and MUST first validate session ownership. It MUST return messages in ascending session sequence order using the public message representation defined by the API contract.

#### Scenario: Read an owned session's history
- **WHEN** an anonymous client requests messages for one of its active sessions containing persisted messages
- **THEN** the API returns `200` with an `items` array ordered by ascending message `seq`

#### Scenario: Reject history access outside the owner boundary
- **WHEN** an anonymous client requests messages for a session that is absent, deleted, or owned by another client
- **THEN** the API returns `404` with error code `SESSION_NOT_FOUND`

### Requirement: Synchronous text message completion
The Go API SHALL provide `POST /api/v1/sessions/{session_id}/messages` to synchronously persist one user text message and one completed assistant text message. The request MUST require non-blank `content` and non-blank `client_message_id`, enforce the contract limits of 32,000 Unicode characters and 128 characters respectively, and validate session ownership before any message is persisted.

#### Scenario: Send a valid message and receive persisted pair
- **WHEN** an anonymous client sends valid content and a new `client_message_id` to one of its active sessions
- **THEN** the API persists a completed user message, invokes the configured AI completer with bounded conversation context, persists its completed assistant result, and returns `200` containing `request_id`, `user_message`, and `assistant_message`

#### Scenario: Validate message request input
- **WHEN** a client sends malformed JSON, blank content, content longer than 32,000 Unicode characters, blank `client_message_id`, or a `client_message_id` longer than 128 characters
- **THEN** the API returns `400` with error code `INVALID_ARGUMENT` and persists no new message

#### Scenario: Reject a message for an unowned or absent session
- **WHEN** a client sends a valid message request to a session that is absent, deleted, or owned by another client
- **THEN** the API returns `404` with error code `SESSION_NOT_FOUND` and persists no new message

#### Scenario: Detect a duplicate client message identifier
- **WHEN** a client sends a message whose `client_message_id` already identifies a user message in the same session
- **THEN** the API returns `409` with error code `DUPLICATE_REQUEST`, persists no additional message, and does not invoke the AI completer again

### Requirement: Deterministic Fake AI completion boundary
The Go chat application SHALL depend on an AI-completion abstraction rather than a concrete HTTP handler dependency. For this phase, production wiring MUST use a deterministic in-process Fake implementation and MUST NOT invoke the Python gRPC service from the send-message request path.

#### Scenario: Complete a valid request without Python AI availability
- **WHEN** a client sends a valid new message while the Python AI service is unavailable
- **THEN** the Go API completes the request using its in-process Fake AI implementation and returns the persisted assistant response without making a gRPC call

### Requirement: Bounded conversation context
Before requesting an assistant completion, the Go chat application SHALL construct context from completed persisted text messages in ascending sequence order. It MUST remove the oldest eligible messages until the configured context budget is met and MUST retain the current user message in the resulting context.

#### Scenario: Trim oldest history while retaining the current message
- **WHEN** completed session history plus the current user message exceeds the configured context budget
- **THEN** the application excludes the oldest eligible messages first and passes a context that still contains the current user message to the AI completer

### Requirement: Session metadata after first successful message
The Go chat application SHALL refresh an active session's last-message and update timestamps after a successful message pair is persisted. If this is the session's first successful user message, it MUST replace the initial `新会话` title with the trimmed first 32 Unicode characters of that user message, adding an ellipsis when truncation occurs; subsequent messages MUST NOT replace the title.

#### Scenario: Name a session from its first message
- **WHEN** the first valid message in a new session completes successfully
- **THEN** the session title becomes the deterministic first-message title and its last-message and update timestamps are refreshed

#### Scenario: Preserve an established session title
- **WHEN** a later valid message completes successfully in a session that already has a last-message timestamp
- **THEN** the session refreshes its timestamps without changing its existing title

### Requirement: Consistent request identity and error envelope
Every Go chat API response that includes a JSON error SHALL use `{request_id, error: {code, message}}`, and the response header `X-Request-ID` SHALL contain the same request identity generated or accepted by existing middleware. Missing or invalid anonymous-client identity MUST be reported as `400 INVALID_ARGUMENT`; unexpected application failures MUST be reported as `500 INTERNAL_ERROR` without exposing internal details.

#### Scenario: Return a traceable validation error
- **WHEN** a chat API request has a missing or invalid `X-Client-ID`
- **THEN** the API returns `400` with `INVALID_ARGUMENT`, an error envelope containing `request_id`, and a matching `X-Request-ID` response header

#### Scenario: Contain an unexpected internal failure
- **WHEN** an unexpected persistence or application failure occurs before a response is written
- **THEN** the API returns `500` with error code `INTERNAL_ERROR` and does not expose the underlying error detail
