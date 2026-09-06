## 1. Application boundaries and composition

- [x] 1.1 Define the chat application service's input/output types, domain errors, ID/time collaborators, and replaceable AI-completion interface; add focused unit tests for the public contract.
- [x] 1.2 Implement the deterministic in-process Fake AI completer and unit-test its completed assistant result without starting Python or gRPC.
- [x] 1.3 Construct GORM session/message repositories, the chat application service, and HTTP dependencies in `cmd/api/main.go` without changing health/readiness behavior.

## 2. Session and history endpoints

- [x] 2.1 Add anonymous client-ID parsing and HTTP error-to-envelope mapping, preserving the existing request ID response header and body behavior.
- [x] 2.2 Implement create and list session use cases plus `POST` / `GET /api/v1/sessions` handlers and handler/service tests for ownership and ordering.
- [x] 2.3 Implement delete session plus `DELETE /api/v1/sessions/{session_id}` and tests for soft deletion and concealed cross-owner access.
- [x] 2.4 Implement ordered history use case plus `GET /api/v1/sessions/{session_id}/messages` and tests for message DTO mapping, `seq` ordering, and ownership isolation.

## 3. Send-message workflow

- [x] 3.1 Implement request validation for JSON shape, non-blank content and client message ID, Unicode character limits, and model-profile boundary; add table-driven tests with no persistence side effects on invalid input.
- [x] 3.2 Implement the database-level duplicate `client_message_id` check and conflict mapping; test that a duplicate neither writes another message nor calls the AI completer.
- [x] 3.3 Implement completed user-message persistence and context construction with ascending sequence order, oldest-first trimming, and guaranteed retention of the current message; add unit tests for normal and over-budget contexts.
- [x] 3.4 Implement Fake completion result persistence, API response mapping, and failure containment; test that a successful request returns two persisted messages and an unexpected completer error uses the standard internal-error envelope.
- [x] 3.5 Implement first-message deterministic session-title generation and session timestamp refresh; test title truncation and preservation of an established title.
- [x] 3.6 Add `POST /api/v1/sessions/{session_id}/messages` route and HTTP contract tests for success, validation, missing/unowned sessions, duplicate requests, request ID propagation, and no Python gRPC dependency.

## 4. Verification and documentation

- [x] 4.1 Run Go formatting and the focused unit/HTTP test suite; resolve failures without changing unrelated code.
- [x] 4.2 With local MySQL available, run the existing migration path and execute the documented curl flow: create session, list sessions, send message, read ordered history, and delete session.
- [x] 4.3 Update the phase-3 completion checklist and service API documentation only where implementation behavior now differs from the current roadmap or setup guidance.
