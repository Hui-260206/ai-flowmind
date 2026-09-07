FROM ghcr.io/astral-sh/uv:python3.12-bookworm-slim

WORKDIR /app

COPY ai-service/pyproject.toml ai-service/uv.lock ./
RUN uv sync --frozen

COPY proto/ /proto/
COPY ai-service/ /app/
RUN mkdir -p /app/gen/python && uv run python -m grpc_tools.protoc \
    -I /proto \
    --python_out=/app/gen/python \
    --grpc_python_out=/app/gen/python \
    ai/v1/chat.proto ai/v1/common.proto ai/v1/agent.proto ai/v1/tool.proto ai/v1/vision.proto

CMD ["uv", "run", "--no-sync", "python", "-m", "app.main"]
