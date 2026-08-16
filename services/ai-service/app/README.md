# AI service application

此目录承载 AI 服务的 gRPC 进程入口、配置和横切能力。Provider 实现放在同级
`providers/`，不得把数据库、HTTP API 或 Go API 业务逻辑放入此处。

## 目录职责

```text
app/
├── main.py            # 统一启动入口：bootstrap gen 路径 → 配置 → 日志 → 启动 gRPC
├── config.py          # pydantic-settings 配置，只读进程环境变量
├── context.py         # request_id / trace_id 的 contextvars 约定
├── logging_config.py  # 单行 JSON 结构化日志 + request_id 注入
├── chat_service.py    # ChatService gRPC servicer（Complete + 耗时 + 错误转换）
└── grpc_server.py     # grpc.aio server 生命周期，注册 ChatService

providers/
├── base.py            # ChatProvider 抽象 + ProviderError
├── fake.py            # FakeProvider（固定回复）
└── __init__.py        # build_provider(name) 工厂
```

## 运行

```sh
# 首次：生成 protobuf stub 并安装依赖
cd services && make proto-generate-python && make lock-ai

# 启动（Ctrl+C 优雅退出）
make dev-ai
```

本服务只读取进程环境变量，不自动搜索 `.env`。本地开发由 `make dev-ai`
注入；生产环境由进程管理器或容器注入。

## 边界

- 只提供 gRPC（`AI_GRPC_LISTEN_ADDR`），**不提供 FastAPI/Uvicorn/HTTP 健康检查入口**。
- gRPC 只监听内部网络地址，不对公网开放。
- 模型 Provider 的 URL 与密钥只进入本服务，不进入 Go API。
