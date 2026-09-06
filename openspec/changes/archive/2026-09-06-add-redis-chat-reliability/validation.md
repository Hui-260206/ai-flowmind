## Validation

Completed on 2026-09-06:

- `go vet ./...` — passed.
- `make test-go` — passed.
- `go test -tags=integration ./internal/redis ./internal/repository` — package passed. The Redis lifecycle tests were skipped because the locally running Redis requires credentials that were not injected into the test command; MySQL repository integration tests were skipped because local MySQL credentials were not injected. Both integration suites are intentionally written to skip in that situation instead of using or exposing secrets.
- `openspec validate add-redis-chat-reliability --strict` — passed.

The deterministic unit/HTTP tests cover replay, failed-message resume, session busy, owner rate limiting, fallback after idempotency-state loss, fail-closed Redis behavior, and history reads during an induced Redis-coordination outage.
