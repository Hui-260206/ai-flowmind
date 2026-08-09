.PHONY: proto-check proto-generate compose-config dev-go test-go

# Stage 1.1 records the toolchain entry points. Implementations are added in
# the corresponding later stages after the Go/Python services are initialized.
proto-check:
	buf lint proto

proto-generate:
	buf generate proto

compose-config:
	docker compose --env-file .env.docker.example -f docker-compose.yaml config

# Local native development: load the private services/.env into the process
# environment, then start the Go API. Production should inject environment
# variables through its process manager or Compose instead.
dev-go:
	@test -f .env || (echo "missing services/.env; copy .env.example first" >&2; exit 1)
	@set -a; . ./.env; set +a; cd go-api && go run ./cmd/api

test-go:
	@cd go-api && go test ./...
