# 服务端工程边界

`services/` 包含 FlowMind 的 Go API、Python AI 服务和内部 protobuf 契约。移动端只依赖 Go API；Python、MySQL、Redis 不对移动端直接暴露。

## 目录职责

```text
services/
├── go-api/
│   ├── cmd/api/              # 进程入口；只负责组装依赖和生命周期
│   └── internal/             # Go 私有实现
│       ├── config/           # 环境变量解析与校验
│       ├── httpserver/       # HTTP Server、路由和生命周期
│       ├── health/            # healthz/readyz 与依赖探针
│       ├── middleware/        # request_id、日志、错误响应等横切能力
│       ├── mysql/             # MySQL 连接池与探针
│       ├── redis/             # Redis 客户端与探针
│       ├── model/             # 领域模型、枚举常量与 owner_key 派生
│       ├── repository/        # Session/Message/Outbox 数据访问接口与 GORM 实现
│       ├── migrate/           # 内嵌 SQL 迁移 + 启动时按版本执行
│       └── grpcclient/        # Python AI gRPC 客户端与探针
├── ai-service/
│   ├── app/                  # gRPC 进程入口和配置
│   └── providers/            # Provider 抽象及 Fake/真实实现
├── proto/                    # Go/Python 共用的 protobuf 源文件
├── docker-compose.yaml       # 完整本地服务端：Go API、Python AI、MySQL、Redis
├── .env.example              # 非敏感配置契约
└── Makefile                  # 生成代码、启动、检查和测试入口
```

## 边界规则

- Go API 拥有 REST API、匿名设备归属、会话/消息业务和数据访问协调权。
- Python AI 只拥有模型 Provider 适配和 AI gRPC；不访问 MySQL，不拥有会话业务。
- MySQL 是聊天事实来源；Redis 已用于幂等、会话锁和限流，但不保存聊天正文。
- `services/proto` 下的 proto 是内部通信契约源文件，生成物不手写。
- 模型密钥、Provider URL 和 Provider 选择只进入 Python AI 服务配置。
- Go API 已包含聊天 migration、Repository、同步聊天业务 API 与 Redis 可靠性控制。

开发阶段复制 `.env.example` 为 `.env`，连接 Mac 本机安装的 MySQL/Redis；`.env`、密钥和数据库持久化目录不得提交。

本地启动 Go API 时推荐使用 `make dev-go`，该命令会将私有的 `services/.env` 注入当前进程。Go 程序本身只读取进程环境变量，不负责解析或搜索 `.env`。使用 IDE 时，应在 Run/Debug Configuration 中显式配置环境变量；生产环境由进程管理器或容器注入环境变量。运行测试可使用 `make test-go`。

Python AI 服务只提供 gRPC，不引入 FastAPI/Uvicorn/HTTP 健康检查入口。`ChatService.Complete` 可通过配置切换 `FakeProvider` 与 OpenAI-compatible HY3 Provider，`providers/` 是 Provider 抽象与实现边界。本地启动使用 `make dev-ai`；首次运行前先 `make proto-generate-python` 生成 Python stub（输出到 `ai-service/gen/python`，已被 gitignore）。静态检查与测试分别使用 `make lint-ai` 和 `make test-ai`。

## 服务端就绪验证

`make test-server-readiness` 是移动端接入前的严格验证入口。它通过真实的 MySQL、受密码保护的 Redis、Go REST API，以及 Python gRPC Fake Provider 验证以下链路：

- 创建会话、发送消息、顺序历史与 `request_id` 跨 Go/Python 日志关联；
- 同一 `client_message_id` 的幂等回放、匿名 owner 越权隐藏，以及仅重启 Go 后历史仍可恢复；
- Python 服务不可用时的 `502 AI_UNAVAILABLE`、Fake Provider 延迟导致的 `504 AI_TIMEOUT`；
- Redis 不可用时新发送返回 `503 REDIS_UNAVAILABLE`，同时已有 MySQL 历史仍可读。

该目标不会把“跳过的集成测试”计作成功，并且会在失败时保留 Go/Python 进程日志。它会启动本机 Go 与 Python 子进程，**不会**启动 Docker、真实 HY3 或其他外部模型 Provider。

运行前必须在私有 `services/.env` 中显式配置（可参考 [.env.readiness.example](.env.readiness.example)）：

- `FLOWMIND_READINESS=1`；
- `MYSQL_DATABASE` 必须以 `_test` 结尾；脚本只删除本次创建 session 的记录；
- `REDIS_DB` 必须为非零专用 DB；脚本只删除本次衍生的 Redis key；
- `FLOWMIND_READINESS_REDIS_STOP_COMMAND` 与 `FLOWMIND_READINESS_REDIS_START_COMMAND` 必须分别停止、恢复该专用 Redis 实例。不要将它们指向共享或日常开发 Redis。

脚本拒绝不满足这些安全条件的配置。建议先启动专用 MySQL/Redis，再运行：

```bash
cd services
make test-server-readiness
```

## 指标与日志关联

Go API 提供未认证、只读的 `GET /metrics` Prometheus 文本端点。它独立于 `/readyz`：即使 MySQL、Redis 或 Python gRPC 未就绪，指标仍可被抓取用于诊断。

| 指标 | 语义 |
|---|---|
| `chat_request_total` | 已结束的同步发送请求，按 outcome 和 HTTP status 计数 |
| `chat_request_failed_total` | 对外返回失败的发送请求，按 error code 和 HTTP status 计数 |
| `chat_request_latency` | 从 HTTP handler 观察的完整发送耗时（秒） |
| `ai_grpc_latency` | Go 到 Python gRPC 调用耗时（秒） |
| `ai_timeout_total` | 触发 Go AI RPC deadline 的次数 |
| `message_persist_failed_total` | user/assistant 消息持久化失败次数 |
| `redis_lock_failed_total` | Redis 会话锁 acquire/release 失败次数 |

标签仅限 `outcome`、`status_code`、`error_code` 和 `stage` 等有限枚举。严禁将 `request_id`、匿名 client/owner、session/message ID、消息内容、模型输出、Provider URL 或带参数的原始路径放入指标标签。定位单个请求时，使用 Go/Python JSON 结构化日志中透传的 `request_id`。

阶段 1.10 已完成 Go–Python gRPC 联通：Go 通过 `internal/grpcclient` 调用 Python `ChatService`（Fake Provider），并把 `python_grpc` 接入 `/readyz`。Go stub 由 `make proto-generate` 生成到 `go-api/internal/grpcclient/pb`（需 `$HOME/go/bin` 上的 protoc-gen-go / protoc-gen-go-grpc，通过 `go install` 安装）。proto 的 `go_package` 已统一为 `ai-flowmind/services/go-api/internal/grpcclient/pb`。

阶段 4/5 已将 Go 聊天 REST API 的生产补全链路切换为 Python `ChatService` gRPC：`go-api/internal/chat.GRPCCompleter` 透传 request_id、应用 RPC deadline，并将模型名称和 token usage 写回助手消息。Python 可通过 `AI_PROVIDER=hy3` 使用 OpenAI-compatible HY3 服务；Provider URL、API Key 和 `AI_MODEL` 只由 Python 读取。`AI_PROVIDER_TIMEOUT` 使用秒并必须小于 Go 的 `AI_GRPC_TIMEOUT` 与 HTTP 写超时。生产 `/readyz` 将 Python gRPC 作为必要依赖；Python 不可用、Provider 失败和超时分别映射为 502 `AI_UNAVAILABLE`/`AI_PROVIDER_ERROR` 与 504 `AI_TIMEOUT`。Fake Provider 仍保留给离线和确定性测试。

阶段 6 已启用 Redis 发送可靠性：`POST /api/v1/sessions/{session_id}/messages` 使用短期 Redis 状态实现 48 小时幂等回放、30 秒会话生成锁与每匿名设备每 UTC 分钟 20 次的固定窗口限流。相同请求成功后返回原消息对；同会话另一条正在生成的请求返回 `409 SESSION_BUSY`；超限返回 `429 RATE_LIMITED`。Redis 无法执行必要协调时发送请求返回 `503 REDIS_UNAVAILABLE`，但 MySQL 驱动的历史读取不受影响。Redis 只存 token、指纹和消息 ID，不存聊天正文，也不替代 MySQL；其记录失效后数据库唯一键仍阻止重复写入并返回 `409 DUPLICATE_REQUEST`。

## Docker Compose 完整本地服务端

`docker-compose.yaml` 会在内部 `backend` 网络启动 MySQL、Redis、Python AI gRPC 服务和 Go API；只有 Go API 的 HTTP 端口会映射到宿主机，Python gRPC、MySQL 与 Redis 不会暴露。Go API 在 MySQL/Redis 健康后启动，并自动执行嵌入式数据库迁移。

首次使用请将 `.env.docker.example` 复制为私有的 `.env.docker`，替换其中的示例密码及（如使用真实 Provider）密钥，然后执行：

```bash
cd services
make compose-up
curl http://localhost:8080/readyz
make compose-down
```

只想校验 Compose 配置而不启动容器时，使用 `make compose-config`。`make compose-up` 不读取原生开发用的 `.env`，以避免把本机 MySQL/Redis 地址和测试配置带入容器。镜像构建会在 Python 镜像内生成 protobuf stub，因此本机不需要预先提交或生成 `ai-service/gen/`。

MVP 不引入消息队列。聊天同步主链路由 Go 直接调用 Python gRPC；未来如果需要通知、统计或其他异步副作用，再单独评估 RabbitMQ、Redis Streams 或其他 MQ，并确保它不成为普通聊天请求的必要依赖。

移动端接入与开发地址见 [`mobile/README.md`](../mobile/README.md)，MVP 阶段记录与最终验收见 [`docs/roadmap/MVP_EXECUTION_PLAN.md`](../docs/roadmap/MVP_EXECUTION_PLAN.md)，完整文档导航见 [`docs/README.md`](../docs/README.md)。
