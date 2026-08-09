# 阶段 1：服务端基础工程计划

## 1. 文档目的

本文档将 [`docs/roadmap/MVP_EXECUTION_PLAN.md`](../roadmap/MVP_EXECUTION_PLAN.md) 中的阶段 1 拆分为多个足够小、可以独立 review 和验收的开发阶段。

本阶段只建设服务端基础工程和运行环境，不实现聊天业务、数据库业务表、真实模型调用、Redis 幂等/锁/限流，也不进入阶段 2 的领域模型开发。

每个小阶段都遵循以下流程：

1. 先说明设计目的和取舍；
2. 明确本阶段修改的文件和代码范围；
3. 完成实现；
4. 进行最小必要验证；
5. 停下来进行人工 review；
6. review 通过后再进入下一个小阶段。

## 2. 已确定的部署前提

### 2.1 开发阶段

开发阶段的应用代码、MySQL 和 Redis 都先运行在开发者的 Mac 本机：

```text
本机 Go API / Python AI 服务
          ├── 127.0.0.1:3306 MySQL
          └── 127.0.0.1:6379 Redis
```

开发阶段的安全原则：

- 不默认将 MySQL `3306` 和 Redis `6379` 暴露到公网；
- 本地数据库只监听回环地址，不对局域网或公网开放；
- 本地开发数据库连接配置与正式环境配置分开；
- 本机真实密码只保存在本地 `.env`，不提交 Git；
- 开发阶段仍然使用独立的开发数据库和账号，不能使用正式环境数据库。

后续部署阶段再将 MySQL 和 Redis 迁移到云服务器 Docker Compose；届时使用 SSH 访问服务器，不直接暴露数据库公网端口。

### 2.2 正式环境

正式环境中，Go API、Python AI、MySQL、Redis 全部运行在同一台云服务器的 Docker Compose 网络中：

```text
同一台云服务器
└── Docker Compose 网络
    ├── go-api
    ├── ai-service
    ├── mysql
    └── redis
```

服务之间使用 Compose 服务名通信：

```text
MYSQL_HOST=mysql
REDIS_ADDR=redis:6379
AI_GRPC_ADDR=ai-service:50051
```

正式环境的网络原则：

- Go API 是唯一对移动端提供业务 HTTP API 的服务；
- MySQL、Redis 和 Python gRPC 只加入内部 Compose 网络；
- MySQL、Redis、Python gRPC 不直接对公网开放；
- 对公网只开放必要的 HTTPS 入口；
- SSH 端口限制来源 IP，使用密钥登录；
- 不使用 MySQL root 账号连接 Go API；
- 不将真实密码、API Key 或服务器 `.env` 提交到仓库。

## 3. 阶段 1 的完成标准

阶段 1 完成后，应达到以下结果：

- Go API 可以独立启动；
- Python AI 服务可以独立启动；
- MySQL 和 Redis 可以在开发 Mac 本机启动；
- Go `/healthz` 可以报告进程存活；
- Go `/readyz` 可以检查 MySQL、Redis 和 Python gRPC；
- Go 可以连接 MySQL；
- Go 可以连接 Redis；
- Go 可以通过 gRPC 调用 Python Fake Provider；
- 后续正式环境 Compose 的网络、端口和数据卷方案已经明确；
- 云服务器 Docker MySQL/Redis 的配置、备份和恢复方案已经完成教学和验证；
- 所有真实凭据都没有进入 Git。

阶段 1 不包含：

- `chat_sessions`、`chat_messages` 和 `outbox_events` 表；
- Session、Message、Outbox Repository；
- 创建会话、发送消息等业务 API；
- Redis 幂等、会话锁和限流；
- RabbitMQ；
- OpenAI 或其他真实模型 Provider；
- 移动端联网。

## 4. 阶段拆分

### 阶段 1.0：确认技术边界、版本和验收契约

目标：在创建服务端代码前固定基础技术选择。

任务：

- 确认 Go、Python、MySQL、Redis 版本；
- 确认 Go 使用 Gin 构建 HTTP 路由和中间件，底层使用标准库 `net/http.Server` 启动服务；
- 确认 MySQL 和 Redis 客户端库；
- 确认 Python 依赖管理方式；
- 确认 protobuf 代码生成工具链；
- 确认开发阶段 Mac 本机数据库连接方式；
- 确认正式环境 Compose 内部连接方式；
- 确认本地、开发服务器、正式环境的配置边界；
- 确认端口规划和健康检查语义。

验收：

- 技术版本和依赖方案明确；
- 开发环境与正式环境拓扑明确；
- 端口和网络暴露原则明确；
- 你 review 并确认基础设计。

### 阶段 1.1：设计目录、配置和环境变量契约

目标：先建立清晰的服务端工程边界。

计划目录：

```text
services/
├── go-api/
│   ├── cmd/api/main.go
│   ├── internal/config/
│   ├── internal/httpserver/
│   ├── internal/health/
│   ├── internal/middleware/
│   ├── internal/mysql/
│   ├── internal/redis/
│   └── internal/grpcclient/
├── ai-service/
│   ├── app/main.py
│   ├── app/config.py
│   ├── app/grpc_server.py
│   └── app/providers/
├── proto/
├── docker-compose.yaml
├── .env.example
└── Makefile
```

任务：

- 定义 Go 和 Python 服务职责；
- 定义配置变量名称；
- 定义 `.env.example`；
- 区分代码默认值、环境变量和部署环境 `.env`；
- 确认服务之间使用服务名还是本机地址；
- 确认密钥只能进入 Python AI 服务的配置。

验收：

- 目录职责没有明显重叠；
- `.env.example` 不包含真实凭据；
- 开发和正式环境的配置方式分别说明；
- 你 review 目录和配置契约。

### 阶段 1.2：Mac 本地 MySQL/Redis 基础设施基线

目标：在开发阶段先准备 Mac 本机的开发 MySQL 和 Redis，不修改业务代码；云服务器 Docker 迁移放到后续部署阶段。

任务：

- 确认 Mac、Homebrew、MySQL 和 Redis 版本；
- 创建本地开发专用 `.env`；
- 配置本地 MySQL 数据目录；
- 配置本地 Redis 数据目录；
- 配置 MySQL `utf8mb4` 和时区；
- 配置 Redis 认证和持久化策略；
- 配置 MySQL/Redis healthcheck；
- 确保开发数据库使用独立数据库名和账号；
- 确保 MySQL/Redis 只监听 `127.0.0.1`；
- 编写后续迁移到云服务器 Docker 的说明。

验收：

- MySQL 和 Redis 本机服务可以启动；
- 服务重启后数据仍然存在；
- Go 后续可以通过 `127.0.0.1` 连接数据库；
- 局域网和外部网络无法直接访问数据库端口；
- 你 review 本地账号、监听地址和后续云端迁移边界。

说明：本小阶段不创建聊天业务表，业务 migration 放在阶段 2。

### 阶段 1.3：初始化 Go Module 和最小 HTTP 服务

目标：让 Go API 先作为一个最小进程运行。

任务：

- 初始化 Go Module；
- 创建 `cmd/api/main.go`；
- 加载最小配置；
- 创建 HTTP Server；
- 注册 `/healthz`；
- 监听可配置地址和端口；
- 处理启动失败。

验收：

- Go 服务可以独立启动；
- `/healthz` 可以访问；
- 端口可以通过配置修改；
- 配置错误能够清晰报错；
- 你 review `main.go` 的启动结构。

### 阶段 1.4：加入 Go 配置、日志、request_id 和优雅退出

目标：先解决所有后续业务都会依赖的横切能力。

任务：

- 实现配置模块；
- 实现结构化日志；
- 实现 `X-Request-ID` 读取；
- 缺失 request ID 时由服务端生成；
- 将 request ID 写入响应头和日志；
- 实现 HTTP 请求日志；
- 实现 graceful shutdown；
- 设置启动和关闭超时；
- 统一基础错误响应结构。

验收：

- 每个 HTTP 请求都有 request ID；
- Ctrl+C 后服务可以正常退出；
- 统一错误响应包含 request ID；
- 日志能够按 request ID 定位请求；
- 你 review 中间件和生命周期管理。

### 阶段 1.5：实现 `/healthz` 和 `/readyz`

目标：区分进程存活和服务可用。

规则：

- `/healthz` 只检查 Go 进程，不检查外部依赖；
- `/readyz` 检查 MySQL、Redis 和 Python gRPC；
- 依赖不可用时 `/readyz` 返回失败状态；
- 返回结果要能指出具体失败依赖。

验收：

- Go 进程正常但依赖未启动时，`/healthz` 成功；
- 依赖未启动时，`/readyz` 失败；
- 所有依赖正常时，`/readyz` 成功；
- 你 review 健康检查的语义和状态码。

### 阶段 1.6：接入 Go MySQL 客户端

目标：建立 MySQL 连接池和生命周期管理，不实现业务表。

任务：

- 增加 MySQL 驱动；
- 实现 DSN 构造；
- 从环境变量加载连接参数；
- 设置连接池参数；
- 启动时执行 `Ping`；
- 将 MySQL 检查接入 `/readyz`；
- 服务退出时关闭连接池。

连接地址规则：

- 开发阶段本地进程连接 Mac 本机 `127.0.0.1:3306`；
- 正式 Compose 中使用 `MYSQL_HOST=mysql`；
- Go 不使用 root 账号连接业务数据库。

验收：

- 正确配置可以连接 MySQL；
- 错误密码或数据库不可用时有清晰错误；
- MySQL 不可用时 `/readyz` 失败；
- 你 review DSN、连接池和账号权限。

### 阶段 1.7：接入 Go Redis 客户端

目标：建立 Redis 客户端和生命周期管理，不实现 Redis 业务能力。

任务：

- 增加 Redis 客户端；
- 加载地址、密码和数据库编号；
- 启动时执行 `Ping`；
- 将 Redis 检查接入 `/readyz`；
- 服务退出时关闭 Redis 客户端。

验收：

- 正确配置可以连接 Redis；
- Redis 不可用时 `/readyz` 失败；
- Redis 认证错误能够被识别；
- 你 review Redis 配置和生命周期管理。

说明：幂等、会话锁和限流属于阶段 6。

### 阶段 1.8：初始化 Python AI 服务运行时

目标：建立 Python AI 服务的项目结构、配置和进程基础，为后续
gRPC Server 和 Fake Provider 实现提供运行环境。

任务：

- 创建 `pyproject.toml`；
- 固定 Python、grpcio、protobuf 和测试工具版本；
- 创建配置模块；
- 定义 gRPC 监听地址和端口等配置；
- 定义模型 Provider 配置，但暂不接入真实模型；
- 配置结构化日志；
- 配置 request ID 的记录和透传约定；
- 创建统一的 Python 服务启动入口；
- 预留 gRPC Server 生命周期管理；
- 明确 Python 服务不提供 FastAPI/HTTP 业务接口；
- 明确 Python gRPC 只加入内部网络，不对公网开放。

验收：

- Python 服务可以独立启动；
- Python 项目可以安装依赖并通过静态检查；
- 配置可以通过环境变量传入；
- 服务启动和退出日志语义明确；
- 不引入 FastAPI、Uvicorn 或 HTTP 健康检查入口；
- 你 review Python 项目结构和进程边界。

### 阶段 1.9：创建 Python gRPC 服务和 Fake Provider

目标：建立内部 AI 服务边界，但不依赖真实模型。

任务：

- 使用阶段 0 的 proto；
- 创建 `grpc.aio` server；
- 创建 `ChatService`；
- 定义 `ChatProvider` 接口；
- 实现 `FakeProvider`；
- 实现最小 Chat 方法；
- 记录调用耗时；
- 配置 gRPC 监听地址。

验收：

- Python gRPC 服务可以启动；
- Fake Provider 可以返回固定回复；
- gRPC 异常能够记录并转换；
- 你 review Provider 抽象和错误边界。

### 阶段 1.10：生成 protobuf 并完成 Go–Python gRPC 联通

目标：让 Go 使用同一份 proto 调用 Python。

任务：

- 固定 protobuf 编译工具版本；
- 生成 Python protobuf 代码；
- 按需要生成 Go protobuf/gRPC 代码；
- 创建 Go gRPC Client；
- 配置 gRPC 地址；
- 实现连接关闭；
- 将 gRPC 可用性接入 `/readyz`；
- 用 Fake Provider 完成一次最小调用。

连接地址规则：

- 开发阶段本地 Go 连接本机 AI gRPC 服务；如果未来 AI 服务先部署到云端，再使用受控端口或 SSH 隧道；
- 正式 Compose 中使用 `AI_GRPC_ADDR=ai-service:50051`；
- Python gRPC 不对公网开放。

验收：

- Go 和 Python 使用同一份 proto；
- Go 可以调用 Fake Provider；
- Python 停止时 Go 能识别依赖不可用；
- 你 review 生成代码和 gRPC Client 生命周期。

### 阶段 1.11：服务容器化和 Compose 编排（后续部署阶段）

目标：使用 Compose 统一启动正式环境的全部服务。

Compose 服务：

```text
mysql
redis
ai-service
go-api
```

任务：

- 为 Go API 创建多阶段 Dockerfile；
- 为 Python AI 创建 Dockerfile；
- 使用固定基础镜像版本；
- 不将 `.env` 或密钥复制进镜像；
- 配置 Compose 内部网络；
- 配置服务 healthcheck；
- 配置 MySQL 和 Redis 数据卷；
- 配置服务间使用 Compose 服务名；
- 明确对外端口和内部端口；
- 增加启动、停止和日志查看命令。

正式环境拓扑：

```text
公网 HTTPS
    ↓
Go API
    ├── mysql:3306
    ├── redis:6379
    └── ai-service:50051
```

只有 Go API 的必要 HTTP/HTTPS 端口对外提供服务，MySQL、Redis 和 Python gRPC 保持内部可达。

验收：

- `docker compose up --build` 可以启动全部服务；
- Go `/healthz` 成功；
- Go `/readyz` 成功；
- Go 可以连接 MySQL、Redis 和 Python gRPC；
- 你 review Dockerfile、网络、端口和数据卷。

### 阶段 1.12：云服务器 Docker 数据库配置与运维教学（后续部署阶段）

目标：掌握从 Mac 本地数据库迁移到云服务器 Docker 数据库、以及正式环境内部数据库的配置差异。

#### 1.12.1 服务器安全基线

- 使用 SSH 密钥登录；
- 限制 SSH 来源 IP；
- 只开放必要的 HTTP/HTTPS 端口；
- 不开放 3306、6379、50051；
- 配置云厂商安全组和系统防火墙；
- 使用非 root 用户管理项目；
- 确认 Docker 和 Compose 版本。

#### 1.12.2 云端部署数据库

- 创建开发环境专用 Compose 项目；
- 创建开发数据库和应用账号；
- 创建 MySQL/Redis 数据卷；
- 配置 healthcheck；
- 迁移前完成本地数据库备份；
- 使用 Docker Compose 创建云端开发数据库；
- 如需本地调试，再用 SSH 隧道提供受控访问；
- 验证部署配置和 Go 的 `/readyz`。

#### 1.12.3 正式环境数据库

- 创建正式环境专用 Compose 项目或正式配置文件；
- 使用独立的正式数据库名、账号和密码；
- MySQL 和 Redis 只加入内部 Compose 网络；
- Go 使用 `mysql` 和 `redis` 作为服务名；
- 配置固定数据卷；
- 不使用 root 连接 Go API；
- 不直接暴露数据库端口。

#### 1.12.4 MySQL 初始化注意事项

MySQL 官方镜像的初始化环境变量通常只在空数据目录首次初始化时生效。修改 `.env` 并不会自动修改已经存在的数据库用户密码。

因此后续教学必须区分：

- 第一次初始化空数据卷；
- 修改已有用户密码；
- 创建新数据库用户；
- 删除并重建环境；
- 删除数据卷前的备份确认。

#### 1.12.5 备份和恢复

- 设计备份频率和保留周期；
- 将备份放到与数据库不同的存储位置；
- 限制备份文件权限；
- 记录恢复步骤；
- 在非正式环境完成恢复演练；
- 数据卷删除前确认备份可恢复。

验收：

- 可以说明开发和正式环境的连接方式；
- 可以解释为什么数据库端口不应直接暴露公网；
- 可以验证 MySQL/Redis 容器持久化；
- 完成至少一次备份恢复演练；
- 你 review 服务器配置和安全边界。

### 阶段 1.13：联合验收和阶段 2 入口

目标：确认阶段 1 的基础工程稳定，再进入数据库模型开发。

验收场景：

1. 全部 Compose 服务启动；
2. Go `/healthz` 成功；
3. Go `/readyz` 成功；
4. 停止 MySQL，确认 `/readyz` 失败；
5. 停止 Redis，确认 `/readyz` 失败；
6. 停止 Python，确认 `/readyz` 失败；
7. 重启依赖，确认服务恢复；
8. Go 调用 Python Fake Provider；
9. 重启 MySQL 容器，确认数据卷仍然存在；
10. 检查 Git，确认没有真实密码和 API Key；
11. review 阶段 1 的目录、配置、Dockerfile、Compose 和部署文档。

阶段 1 通过后，进入阶段 2：

- 创建 `chat_sessions` migration；
- 创建 `chat_messages` migration；
- 创建 `outbox_events` migration；
- 实现 Session、Message 和 Outbox Repository。

## 5. 推荐的阶段推进顺序

```text
1.0 技术边界
  → 1.1 目录与配置
  → 1.2 云服务器开发数据库
  → 1.3 Go 最小服务
  → 1.4 Go 工程能力
  → 1.5 健康检查
  → 1.6 MySQL 客户端
  → 1.7 Redis 客户端
  → 1.8 Python 最小服务
  → 1.9 Python gRPC
  → 1.10 Go–Python 联通
  → 1.11 Compose 容器化
  → 1.12 云服务器配置与运维教学
  → 1.13 联合验收
```

每个节点完成后暂停，等待 review，不连续跨越多个小阶段实现。
