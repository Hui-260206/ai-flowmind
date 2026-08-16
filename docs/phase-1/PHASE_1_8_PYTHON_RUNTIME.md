# 阶段 1.8：初始化 Python AI 服务运行时

阶段 1.8 建立 Python AI 服务的项目结构、配置和进程基础，为阶段 1.9 的
gRPC Server 与 Fake Provider 提供运行环境。本阶段不实现 gRPC 业务方法、
不生成 protobuf 代码、不接入真实模型，也不引入 HTTP。

## 1. 目标与边界

- 只提供 gRPC 入口，**不提供 FastAPI / Uvicorn / HTTP 健康检查入口**。
- gRPC 只监听内部网络地址，不对公网开放。
- 模型 Provider 的 URL 与密钥只进入本服务，不进入 Go API。
- 配置、日志、request_id 透传等横切能力与 Go API 保持一致的约定。

## 2. 目录结构

```text
services/ai-service/
├── .python-version          # 固定 3.12（uv 使用）
├── pyproject.toml           # 依赖、ruff 与 pytest 配置
├── uv.lock                  # uv lock 生成，不手写
├── app/
│   ├── main.py              # 统一启动入口：配置 → 日志 → gRPC 生命周期
│   ├── config.py            # pydantic-settings，只读进程环境变量
│   ├── context.py           # request_id / trace_id 的 contextvars 约定
│   ├── logging_config.py    # 单行 JSON 结构化日志 + request_id 注入
│   └── grpc_server.py       # grpc.aio server 生命周期脚手架
├── providers/               # 预留：1.9 放 ChatProvider / FakeProvider
└── tests/
    └── test_config.py       # 配置加载与校验的最小测试
```

## 3. 配置约定

配置优先级与 Go 一致：代码安全默认值 < 环境变量 < 部署环境注入。Python
只读取进程环境变量，**不自动搜索或解析 `.env`**；本地开发由 `make dev-ai`
显式加载 `services/.env` 后注入，生产环境由进程管理器或容器注入。

| 环境变量 | 默认值 | 说明 |
|---|---|---|
| `AI_ENV` | `local` | 运行环境标识 |
| `AI_LOG_LEVEL` | `info` | `debug`/`info`/`warning`/`error` |
| `AI_GRPC_LISTEN_ADDR` | `127.0.0.1:50051` | gRPC 监听地址（本机开发） |
| `AI_PROVIDER` | `fake` | Provider 标识，1.8 不接真实模型 |
| `AI_PROVIDER_BASE_URL` | 空 | Provider 地址，仅本服务使用 |
| `AI_PROVIDER_API_KEY` | 空 | Provider 密钥，仅本服务使用 |
| `MODEL_PROFILE` | `default` | 服务端允许的模型 profile |

正式 Compose 中 `AI_GRPC_LISTEN_ADDR=0.0.0.0:50051`（容器网络内监听），Go
通过 `AI_GRPC_ADDR=ai-service:50051` 连接；`AI_GRPC_ADDR` 是 Go 客户端配置，
属阶段 1.10。

## 4. 结构化日志

日志使用标准库 `logging` + 自定义 JSON Formatter 输出单行 JSON，字段命名
对齐 Go 的 `log/slog`：`time`、`level`、`logger`、`msg`，并在 request_id /
trace_id 存在时附带 `request_id`、`trace_id`。不引入 structlog 等额外依赖。

## 5. request_id 约定

- `request_id` 来源：gRPC 请求 `RequestContext.request_id`（见
  `services/proto/ai/v1/common.proto`）。
- 调用方未提供时，服务端生成 `uuid4().hex`。
- 通过 `contextvars.ContextVar` 在异步链中透传；阶段 1.9 的 gRPC
  interceptor 在每次 RPC 前设置、结束后清理。

## 6. 运行与验证

```sh
# 首次：生成锁文件并安装依赖（uv 会自动下载 Python 3.12）
cd services && make lock-ai

# 静态检查
make lint-ai

# 测试
make test-ai

# 启动（Ctrl+C 优雅退出）
make dev-ai
```

## 7. 阶段 1.8 验收清单

- [x] Python 服务可以独立启动（`python -m app.main`）。
- [x] Python 项目可以通过 `uv` 安装依赖并通过 `ruff` 静态检查。
- [x] 配置可以通过环境变量传入，非法配置清晰报错。
- [x] 服务启动与退出日志语义明确（starting / started / signal / stopped）。
- [x] 不引入 FastAPI、Uvicorn 或 HTTP 健康检查入口。
- [x] gRPC server 生命周期脚手架就位，1.9 只需注册 ChatService。
