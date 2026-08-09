# 阶段 1.0 技术边界与验收契约

本文固定阶段 1 服务端基础工程的技术选择。它只定义边界，不提前实现 HTTP、数据库、Redis 或 gRPC 业务代码。

## 1. 技术基线

| 领域 | 决策 | 约束 |
|---|---|---|
| Go API | Go 1.24.x；标准库 `net/http` | Go API 是唯一对移动端开放的业务 HTTP 服务 |
| Go HTTP | `net/http.Server` | 后续阶段统一注册 `/healthz`、`/readyz` 和 REST 路由 |
| MySQL | 开发机当前为 Homebrew `mysql@8.0`，客户端 8.0.46 | `utf8mb4`；Go 使用 `database/sql` + `github.com/go-sql-driver/mysql`；部署基线仍需另行决定是否升级 8.4 LTS |
| Redis | 开发机当前为 Redis 8.10.0 | Go 使用 `github.com/redis/go-redis/v9`；只保存临时状态；部署镜像版本需与本地兼容性验证后固定 |
| Python AI | Python 3.12.x | 使用 `uv` 管理 `pyproject.toml` 与锁文件 |
| Python HTTP | FastAPI + Uvicorn | 仅提供 AI 服务自身健康检查/运维入口，不承载移动端业务 API |
| Python RPC | `grpcio` / `grpcio-tools` | 使用 `grpc.aio`；服务监听 Compose 内部的 50051 |
| protobuf | `protoc` 27.x + Buf 1.x | `services/proto` 是唯一源文件；生成代码不手写 |
| 容器 | Docker Compose v2 | 仅用于后续云端部署；Mac 本地开发阶段不依赖 Docker |

版本使用小版本范围而非浮动 `latest`。真正创建镜像和锁定补丁版本时，再由依赖更新流程提交具体 patch 版本。

### 当前 Mac 环境快照（2026-08-09）

- Homebrew：6.0.15
- MySQL：Homebrew `mysql@8.0`，客户端 `8.0.46`，服务进程监听 `127.0.0.1:3306`
- Redis：`8.10.0`，服务状态为 started，监听 `127.0.0.1:6379`
- Redis 当前实例已启用 `protected-mode` 和回环监听；AOF 与密码认证尚未按阶段 1.2 目标完成配置。
- MySQL 服务端 SQL 元数据尚未读取成功，因为当前 root 连接需要密码；后续使用开发应用账号验证服务端版本、字符集和时区。

## 2. 拓扑和通信边界

### Mac 本地开发

```text
本机 go-api ── HTTP ──▶ 本机 ai-service
     ├── MySQL：Mac 本机安装并运行
     └── Redis：Mac 本机安装并运行
```

本地配置使用 `services/.env`（不入 Git），示例使用 `services/.env.example`。本机数据库使用独立开发数据库名和账号；Go 不使用 MySQL root。

### 正式环境

```text
公网 HTTPS ──▶ go-api:8080
                 ├── mysql:3306（Compose 内部）
                 ├── redis:6379（Compose 内部）
                 └── ai-service:50051（Compose 内部）
```

正式环境只发布 Go API 的 HTTPS 入口。MySQL、Redis 和 AI gRPC 不发布宿主机公网端口；真实 `.env` 或密钥系统只存在于服务器。云端 Compose 配置见 `services/docker-compose.yaml`。

## 3. 端口规划

| 服务 | 容器端口 | 本地默认映射 | 是否公网暴露 | 说明 |
|---|---:|---:|---|---|
| Go API HTTP | 8080 | 8080 | 仅通过 HTTPS 入口 | `/healthz` 表示进程存活 |
| AI HTTP | 8000 | 不映射 | 否 | 后续用于 AI 服务运维健康检查 |
| AI gRPC | 50051 | 50051（仅开发可选） | 否 | Go 使用 `ai-service:50051` |
| MySQL | 3306 | Mac 本机 `127.0.0.1:3306`；云端不映射 | 否 | 本地 Go 使用 `127.0.0.1`，云端 Go 使用 `mysql` |
| Redis | 6379 | Mac 本机 `127.0.0.1:6379`；云端不映射 | 否 | 本地 Go 使用 `127.0.0.1:6379`，云端 Go 使用 `redis:6379` |

## 4. 配置分层

配置优先级固定为：代码安全默认值 < 环境变量 < 部署环境 `.env`/密钥系统。

- 代码默认值：仅用于非敏感、可安全回退的开发参数，例如监听端口。
- 环境变量：服务运行时唯一读取入口，变量名见 `services/.env.example`。
- `.env.example`：只记录变量名、示例格式和说明，不能放真实密码、Token 或模型 Key。
- 本地 `.env`：开发者私有文件，加入 Git 忽略规则。
- 正式环境：由服务器安全配置系统注入；模型密钥只进入 Python AI 服务。

Go 不读取任何模型 Provider、模型 URL 或模型 API Key。Go 只传递服务端允许的 `MODEL_PROFILE`；Python 负责将 profile 映射到 Provider。

## 5. 消息队列边界

MVP 不引入 RabbitMQ、Kafka、Redis Streams 或其他消息队列。普通聊天请求采用同步 HTTP + gRPC，消息先写入 MySQL，再调用 Python AI，AI 回复也写入 MySQL。未来的通知、统计、标题生成等非关键异步副作用可以通过 Outbox + MQ 扩展，但 MQ 不应成为普通聊天请求成功的前置依赖。

## 6. 健康检查语义

- `GET /healthz`：只证明进程能接受 HTTP 请求，成功返回 200；不检查 MySQL、Redis 或 AI。
- `GET /readyz`：检查 Go 当前需要的 MySQL、Redis、AI gRPC 依赖；全部通过返回 200，否则返回非 2xx，并返回各依赖状态。
- AI 服务自己的健康检查只证明 AI 进程可用；Go 的 `/readyz` 才是对外业务入口的依赖就绪判断。

## 7. 1.0 验收清单

- [x] Go、Python、MySQL、Redis、protobuf 工具链版本方案已固定。
- [x] Go 使用标准库 `net/http` 的决策已固定。
- [x] Go/MySQL、Go/Redis 客户端库已固定。
- [x] 本地、云端开发、正式环境的连接方式和配置边界已固定。
- [x] 端口、网络暴露原则和健康检查语义已固定。
- [x] 真实凭据不进入仓库的规则已固定。
