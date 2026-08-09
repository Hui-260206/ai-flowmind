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
│       └── grpcclient/        # Python AI gRPC 客户端与探针
├── ai-service/
│   ├── app/                  # gRPC 进程入口和配置
│   └── providers/            # Provider 抽象及 Fake/真实实现
├── proto/                    # Go/Python 共用的 protobuf 源文件
├── docker-compose.yaml       # 后续云端部署的 MySQL/Redis 基础设施编排
├── .env.example              # 非敏感配置契约
└── Makefile                  # 生成代码、启动、检查和测试入口
```

## 边界规则

- Go API 拥有 REST API、匿名设备归属、会话/消息业务和数据访问协调权。
- Python AI 只拥有模型 Provider 适配和 AI gRPC；不访问 MySQL，不拥有会话业务。
- MySQL 是聊天事实来源；Redis 只用于后续阶段的幂等、锁和限流。
- `services/proto` 下的 proto 是内部通信契约源文件，生成物不手写。
- 模型密钥、Provider URL 和 Provider 选择只进入 Python AI 服务配置。
- 阶段 1.1 不创建聊天表、migration、Repository 或业务 API。

开发阶段复制 `.env.example` 为 `.env`，连接 Mac 本机安装的 MySQL/Redis；`.env`、密钥和数据库持久化目录不得提交。

本地启动 Go API 时推荐使用 `make dev-go`，该命令会将私有的 `services/.env` 注入当前进程。Go 程序本身只读取进程环境变量，不负责解析或搜索 `.env`。使用 IDE 时，应在 Run/Debug Configuration 中显式配置环境变量；生产环境由进程管理器或容器注入环境变量。运行测试可使用 `make test-go`。

后续部署阶段使用 `.env.docker.example` 作为模板，并在云服务器上注入真实配置。`docker-compose.yaml` 不属于当前 Mac 本地开发启动路径。

MVP 不引入消息队列。聊天同步主链路由 Go 直接调用 Python gRPC；未来如果需要通知、统计或其他异步副作用，再单独评估 RabbitMQ、Redis Streams 或其他 MQ，并确保它不成为普通聊天请求的必要依赖。

阶段 1.2 的本地启动、备份和恢复流程见 [`docs/phase-1/PHASE_1_2_OPERATIONS.md`](../docs/phase-1/PHASE_1_2_OPERATIONS.md)。完整文档分类见 [`docs/README.md`](../docs/README.md)。
