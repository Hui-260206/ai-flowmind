.PHONY: proto-check proto-generate proto-generate-python compose-config dev-go test-go dev-ai lock-ai lint-ai test-ai

# Stage 1.1 records the toolchain entry points. Implementations are added in
# the corresponding later stages after the Go/Python services are initialized.
proto-check:
	buf lint proto

proto-generate:
	buf generate proto

# Python stubs are generated with grpcio-tools (grpc_tools.protoc), which
# bundles protoc + grpc_python_plugin; buf cannot reuse that plugin binary.
proto-generate-python:
	@mkdir -p ai-service/gen/python
	@cd ai-service && uv run python -m grpc_tools.protoc \
		-I ../proto \
		--python_out=gen/python \
		--grpc_python_out=gen/python \
		ai/v1/chat.proto ai/v1/common.proto ai/v1/agent.proto ai/v1/tool.proto ai/v1/vision.proto

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

# Local native development for the Python AI service. Same .env injection
# contract as dev-go; the service itself reads only process environment vars.
dev-ai:
	@test -f .env || (echo "missing services/.env; copy .env.example first" >&2; exit 1)
	@set -a; . ./.env; set +a; cd ai-service && uv run python -m app.main

lock-ai:
	@cd ai-service && uv lock

lint-ai:
	@cd ai-service && uv run ruff check .

test-ai:
	@cd ai-service && uv run pytest
