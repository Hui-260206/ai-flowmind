.PHONY: proto-check proto-generate proto-generate-python compose-config dev-go test-go dev-ai lock-ai lint-ai test-ai

# 阶段 1.1 记录工具链入口；具体实现由后续相应阶段在 Go/Python 服务初始化后补充。
proto-check:
	buf lint proto

# Go stub 需要 PATH 上有 protoc-gen-go 和 protoc-gen-go-grpc（通过 `go install`
# 安装到 $HOME/go/bin）。
proto-generate:
	@PATH="$$HOME/go/bin:$$PATH" buf generate proto

# Python stub 用 grpcio-tools（grpc_tools.protoc）生成，它内置 protoc 和
# grpc_python_plugin；buf 无法复用该插件二进制。
proto-generate-python:
	@mkdir -p ai-service/gen/python
	@cd ai-service && uv run python -m grpc_tools.protoc \
		-I ../proto \
		--python_out=gen/python \
		--grpc_python_out=gen/python \
		ai/v1/chat.proto ai/v1/common.proto ai/v1/agent.proto ai/v1/tool.proto ai/v1/vision.proto

compose-config:
	docker compose --env-file .env.docker.example -f docker-compose.yaml config

# 本地原生开发：把私有的 services/.env 加载进进程环境后启动 Go API；生产环境
# 应由进程管理器或 Compose 注入环境变量。
dev-go:
	@test -f .env || (echo "missing services/.env; copy .env.example first" >&2; exit 1)
	@set -a; . ./.env; set +a; cd go-api && go run ./cmd/api

test-go:
	@cd go-api && go test ./...

# 本地原生开发启动 Python AI 服务，与 dev-go 采用相同的 .env 注入契约；
# 服务本身只读取进程环境变量。
dev-ai:
	@test -f .env || (echo "missing services/.env; copy .env.example first" >&2; exit 1)
	@set -a; . ./.env; set +a; cd ai-service && uv run python -m app.main

lock-ai:
	@cd ai-service && uv lock

lint-ai:
	@cd ai-service && uv run ruff check .

test-ai:
	@cd ai-service && uv run pytest
