## MODIFIED Requirements

### Requirement: Deterministic Fake AI completion boundary
The Go chat application SHALL depend on an AI-completion abstraction rather than a concrete HTTP handler dependency. Production wiring MUST use a gRPC-backed implementation that invokes the internal Python AI service; deterministic in-process Fake implementations MUST remain available for unit tests and local deterministic development.

#### Scenario: Complete a valid request through Python gRPC
- **WHEN** a client sends a valid new message while the Python AI service is available
- **THEN** the Go API invokes the gRPC-backed AI implementation, persists the returned assistant response, and returns the persisted result

#### Scenario: Python AI dependency is unavailable
- **WHEN** a client sends a valid new message while the Python AI service is unavailable
- **THEN** the Go API does not use an in-process fallback, returns a controlled AI-unavailable error, and its readiness endpoint reports the Python dependency as not ready

### Requirement: Consistent request identity and error envelope
Every Go chat API response that includes a JSON error SHALL use `{request_id, error: {code, message}}`, and the response header `X-Request-ID` SHALL contain the same request identity generated or accepted by existing middleware. Missing or invalid anonymous-client identity MUST be reported as `400 INVALID_ARGUMENT`; AI-service unavailability or provider failure MUST be reported as `502 AI_UNAVAILABLE` or `AI_PROVIDER_ERROR`; AI timeout MUST be reported as `504 AI_TIMEOUT`; unexpected application failures MUST be reported as `500 INTERNAL_ERROR` without exposing internal details.

#### Scenario: Return a traceable validation error
- **WHEN** a chat API request has a missing or invalid `X-Client-ID`
- **THEN** the API returns `400` with `INVALID_ARGUMENT`, an error envelope containing `request_id`, and a matching `X-Request-ID` response header

#### Scenario: Return a traceable AI timeout
- **WHEN** the configured Python AI completion exceeds its deadline
- **THEN** the API returns `504` with `AI_TIMEOUT`, an error envelope containing `request_id`, and a matching `X-Request-ID` response header

#### Scenario: Contain an unexpected internal failure
- **WHEN** an unexpected persistence or application failure occurs before a response is written
- **THEN** the API returns `500` with error code `INTERNAL_ERROR` and does not expose the underlying error detail
