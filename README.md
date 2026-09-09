# FlowMind

> 面向移动端的 AI 多轮对话应用：Kuikly 跨平台客户端 + Go/Python 双服务后端。MVP 已于 2026-09-09 完成。

## 项目简介

FlowMind 已完成从移动端交互、REST API、持久化、可靠性控制到真实模型调用的 MVP 闭环：

```text
Kuikly 移动端（Android / iOS）
        │  HTTP(S) JSON
        ▼
Go API 服务  ── gRPC ──▶  Python AI 服务 ──▶ 大模型 Provider
   ├─ MySQL   （会话与消息的事实来源）
   └─ Redis   （幂等、会话锁与限流）
```

MVP 已提供：

- 匿名安装实例身份（`X-Client-ID`），无需登录即可隔离数据；
- 会话创建、列表、切换、删除与上次选择恢复；
- 同一会话连续多轮聊天、历史恢复、动态标题和服务端排序；
- 乐观发送、稳定 `client_message_id`、重复点击保护、失败对账与幂等重试；
- Go REST API、Python gRPC `ChatService`、Fake/HY3 Provider 切换；
- MySQL 持久化，以及 Redis 幂等、会话锁和匿名设备限流；
- 统一错误包络、跨服务 `request_id`、就绪探针和 Prometheus 指标；
- Android/iOS Debug 本地服务地址注入及真实 HTTP 联调。

流式输出、登录、图片/视觉、Agent、Tool Executor、RAG 与消息队列不属于本次 MVP，见[后续路线](./docs/roadmap/MVP_EXECUTION_PLAN.md#18-后续版本顺序)。

## 仓库结构

```text
ai-flowmind/
├── mobile/                 # Kuikly KMP 客户端
│   ├── shared/             # 公共 UI、状态、领域模型和数据层
│   ├── androidApp/         # Android 壳工程
│   ├── iosApp/             # iOS 壳工程
│   └── ohosApp/            # OpenHarmony 脚手架（MVP 不维护）
├── services/
│   ├── go-api/             # 唯一对外 REST API
│   ├── ai-service/         # Python gRPC AI/Provider 服务
│   ├── proto/              # Go/Python 共用 protobuf 契约
│   ├── docker-compose.yaml # Go、Python、MySQL、Redis 编排
│   └── Makefile            # 生成、启动、测试与验收入口
├── docs/
│   ├── README.md           # 文档中心
│   └── roadmap/            # MVP 执行与验收记录
├── openspec/               # 已归档变更与当前能力规格
├── CLAUDE.md               # 架构、构建和开发说明
└── AGENT.md                # AI Agent 工作流约束
```

## 技术栈

| 层 | 技术 |
|---|---|
| 客户端 | Kotlin Multiplatform、Kuikly 2.7.0-2.1.21、Kuikly Compose |
| 客户端壳 | Android、iOS 14.1+ |
| API 服务 | Go、Gin、GORM |
| AI 服务 | Python 3.12、`grpc.aio`、OpenAI-compatible Provider |
| 通信 | 移动端 REST/JSON；Go 与 Python 间 gRPC/protobuf |
| 存储 | MySQL 8、Redis 7 |
| 可观测性 | JSON 结构化日志、`request_id`、Prometheus 指标 |

## 快速开始

### 1. 启动服务端

本机开发需要 Go、Python/uv、MySQL 和 Redis：

```sh
cd services
cp .env.example .env
# 编辑私有 .env，配置本机 MySQL、Redis 和 Provider
make proto-generate-python
make dev-ai
```

另开终端启动 Go API：

```sh
cd services
make dev-go
curl http://localhost:8080/readyz
```

也可以使用 Docker Compose 启动完整拓扑：

```sh
cd services
cp .env.docker.example .env.docker
# 替换示例密码；需要真实模型时再填写 Provider 配置
make compose-up
curl http://localhost:8080/readyz
```

详细配置、安全限制和严格验收命令见 [`services/README.md`](./services/README.md)。

### 2. 构建移动端

```sh
cd mobile

# Android Debug；USB 真机连接本机 API 前先执行 adb reverse tcp:8080 tcp:8080
./gradlew :androidApp:assembleDebug

# iOS 首次准备
./gradlew :shared:generateDummyFramework
cd iosApp && pod install && cd ..
# 使用 Xcode 打开 iosApp/iosApp.xcworkspace
```

Android/iOS 的 Debug 默认地址、无线真机地址和 Release HTTPS 要求见 [`mobile/README.md`](./mobile/README.md)。

### 3. 运行测试

```sh
# 服务端
cd services
make test-go
make lint-ai
make test-ai
# 配置专用测试 MySQL/Redis 后：make test-server-readiness

# 移动端共享层
cd ../mobile
./gradlew :shared:test
```

## 对外 API

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/healthz` | 进程健康检查 |
| GET | `/readyz` | MySQL、Redis、Python gRPC 就绪检查 |
| GET | `/metrics` | Prometheus 指标 |
| POST | `/api/v1/sessions` | 创建会话 |
| GET | `/api/v1/sessions` | 列出当前匿名客户端的会话 |
| DELETE | `/api/v1/sessions/{id}` | 删除会话 |
| GET | `/api/v1/sessions/{id}/messages` | 查询有序消息历史 |
| POST | `/api/v1/sessions/{id}/messages` | 同步发送消息并返回用户/助手消息对 |

业务请求使用 UUID 格式的 `X-Client-ID`，并可携带 `X-Request-ID` 关联 Go/Python 日志。完整设计、阶段记录和验收场景见 [`docs/roadmap/MVP_EXECUTION_PLAN.md`](./docs/roadmap/MVP_EXECUTION_PLAN.md)。

## 开发约定

- 客户端公共实现集中在 `mobile/shared/src/commonMain`，Compose API 来自 `com.tencent.kuikly.compose.*`。
- Go API 是移动端唯一后端入口；Python AI 服务不直接暴露给移动端，也不拥有聊天数据。
- MySQL 是聊天记录事实来源；Redis 不保存聊天正文，也不能替代 MySQL。
- MVP 的聊天主链路保持同步 HTTP + gRPC，不依赖 RabbitMQ。
- 禁止提交 `.env`、真实密码、Provider API Key 或其他密钥。
- OpenHarmony 脚手架保留，但不属于当前 MVP 构建和验收范围。

文档入口见 [`docs/README.md`](./docs/README.md)，Agent 开发约束见 [`AGENT.md`](./AGENT.md)。
