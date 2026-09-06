## ADDED Requirements

### Requirement: Redis-scoped idempotency for synchronous message sends
The Go API SHALL coordinate each valid send request with an expiring Redis idempotency record scoped to the authenticated anonymous owner, session ID, and `client_message_id`. The record MUST use `processing`, `completed`, and `failed` states, retain a canonical fingerprint of normalized content and model profile, and retain only message identifiers rather than message content. The record TTL MUST be 48 hours and its state transitions MUST be atomic and conditioned on an operation token.

The API SHALL persist a MySQL send-operation recovery record scoped to the same owner, session, and client message ID before message persistence. It MUST retain the fingerprint, lifecycle state, and exact persisted message IDs, and MUST allow a retry to reconcile an interrupted Redis transition without issuing a duplicate model request.

#### Scenario: Replay a completed message send
- **WHEN** the same owner sends the same session ID, `client_message_id`, normalized content, and model profile again while its idempotency record is `completed`
- **THEN** the API MUST load the recorded user and assistant messages from MySQL, verify that both belong to the owned session, and return `200` using the normal send-message response shape without calling the AI completer or creating messages

#### Scenario: Reject a duplicate currently in progress
- **WHEN** a request arrives with the same idempotency scope and fingerprint while the existing record is `processing`
- **THEN** the API MUST return `409` with error code `DUPLICATE_REQUEST`, create no messages, and not invoke the AI completer

#### Scenario: Resume after an AI completion failure
- **WHEN** a request has persisted its user message but its AI completion or assistant-message persistence fails and the caller retries with the same idempotency scope and fingerprint
- **THEN** the API MUST resume the generation from the recorded user-message ID without persisting a second user message

#### Scenario: Reject reuse of an identifier with different input
- **WHEN** a request reuses an idempotency scope whose fingerprint differs in normalized content or model profile from the stored record
- **THEN** the API MUST return `409` with error code `DUPLICATE_REQUEST` and MUST NOT replay, persist, or generate a message

#### Scenario: Preserve data safety after an expired or lost record
- **WHEN** Redis has no idempotency record but MySQL already contains a user message with the same session ID and `client_message_id`
- **THEN** the API MUST use the durable send-operation recovery record to replay a completed pair or resume the persisted user message, and MUST NOT create another user message or invoke a duplicate model completion

#### Scenario: Recover after a Redis response-loss window
- **WHEN** user or assistant persistence has completed but the corresponding Redis state transition reports an error or its reply is lost
- **THEN** a same-fingerprint retry MUST reconcile the durable send-operation record, return or finish the original message pair, and MUST NOT write another user message or invoke the AI completer a second time

### Requirement: Per-session serialized generation
The Go API SHALL acquire an expiring Redis lock scoped to the session ID before it persists or resumes a user message and SHALL hold it through context construction, AI completion, assistant persistence, session metadata update, and idempotency finalization. The lock MUST use a unique ownership token, a 30-second TTL, and token-checked release.

#### Scenario: Reject a distinct concurrent send for one session
- **WHEN** one send workflow holds the session lock and another request with a different `client_message_id` targets that session
- **THEN** the second request MUST return `409` with error code `SESSION_BUSY`, MUST NOT persist a message, and MUST NOT invoke the AI completer

#### Scenario: Release a completed workflow's lock safely
- **WHEN** a send workflow completes or exits with an error while it still owns the session lock
- **THEN** the API MUST release only the lock with its matching ownership token and a later send for the session MUST be able to acquire a new lock

#### Scenario: Do not delete a replacement lock
- **WHEN** a workflow's lock lease has expired and another workflow has acquired the same lock key before the first workflow attempts cleanup
- **THEN** the first workflow's cleanup MUST NOT delete the later workflow's lock

### Requirement: Anonymous-device fixed-window rate limit
The Go API SHALL limit each anonymous owner to 20 newly claimed or resumed send-message workflows in one UTC-minute fixed window using expiring Redis state. Completed idempotency replays and duplicates already in processing MUST NOT consume quota.

#### Scenario: Reject the twenty-first accepted attempt
- **WHEN** an anonymous owner attempts a twenty-first newly claimed or resumed send workflow within one UTC-minute window
- **THEN** the API MUST return `429` with error code `RATE_LIMITED`, MUST NOT persist a message, and MUST NOT invoke the AI completer

#### Scenario: Allow a completed retry without charging quota
- **WHEN** an owner retries a completed request with the same idempotency scope and fingerprint after reaching the per-minute limit
- **THEN** the API MUST replay the completed result and MUST NOT return `RATE_LIMITED`

#### Scenario: Start a fresh fixed window
- **WHEN** an owner sends a newly claimed request after the prior UTC-minute rate-limit key has expired
- **THEN** the API MUST evaluate the request against a fresh counter for the new minute

### Requirement: Controlled Redis coordination failure
The Go API SHALL treat Redis as a mandatory dependency for `POST /api/v1/sessions/{session_id}/messages` once this capability is enabled. It MUST map a Redis coordination failure to a traceable `503 REDIS_UNAVAILABLE` response and MUST NOT silently execute an unlocked or unmetered send workflow.

#### Scenario: Fail a new send while Redis is unavailable
- **WHEN** a valid send request cannot read or update Redis idempotency, lock, or rate-limit state
- **THEN** the API MUST return `503` with error code `REDIS_UNAVAILABLE`, include the request ID in the standard error envelope, and MUST NOT invoke the AI completer

#### Scenario: Preserve MySQL history access during a Redis outage
- **WHEN** Redis is unavailable and an owner requests an existing session list or message history
- **THEN** the MySQL-backed read endpoint MUST retain its existing behavior and MUST NOT require Redis coordination
