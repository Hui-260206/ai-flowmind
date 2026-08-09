# FlowMind

> 一个面向移动端的 AI 对话应用：Kuikly 跨平台客户端 + Go/Python 微服务后端。当前处于 MVP 开发阶段。

## 项目简介

FlowMind 的首个目标是建立一套 **可扩展、可测试、可演进** 的对话基础能力，而非一次性堆砌完整 AI 平台。MVP 闭环如下：

```text
Kuikly 移动端 (Android / iOS)
        │  HTTPS JSON
        ▼
Go API 服务  ── gRPC ──▶  Python AI 服务 ──▶ 大模型 Provider
   ├─ MySQL   (会话 / 消息，事实来源)
   ├─ Redis   (幂等 / 会话锁 / 限流)
   └─ RabbitMQ (可选：Outbox 异步事件)
```

核心特性（MVP）：
- 匿名设备身份（`client_id`），不实现登录注册；
- 创建 / 列出 / 删除会话，发送文本消息并接收 AI 回复；
- 退出重进可恢复历史；
- 服务端幂等、会话级并发锁、基础限流；
- 统一错误响应与 `request_id` 链路。

## 仓库结构

```text
ai-flowmind/
├── mobile/                 # 客户端（Kuikly KMP）
│   ├── shared/             # 唯一 KMP 模块，UI/逻辑都在 src/commonMain
│   ├── androidApp/         # Android 壳工程
│   ├── iosApp/             # iOS 壳工程（CocoaPods）
│   ├── ohosApp/            # 鸿蒙脚手架（保留但暂不维护）
│   ├── static_server/      # 本地调试静态服务 + whistle
│   └── buildSrc/           # 版本与构建变量（KotlinBuildVar.kt）
├── services/               # 服务端（待搭建）
│   ├── go-api/             # Go REST/gRPC 服务，唯一对外 API
│   └── ai-service/         # Python AI 服务（Provider 适配 + gRPC）
├── docs/
│   ├── MVP_REQUIREMENTS.md # MVP 需求与系统边界
│   └── MVP_EXECUTION_PLAN.md # 12 阶段执行计划
├── CLAUDE.md               # 详细架构与构建说明（AI 助手向）
└── AGENT.md                # AI Agent 工作流与约束
```

> 说明：早期设想后端为单一 Python/FastAPI，当前 MVP 已演进为 **Go API + Python AI 服务** 的双服务架构，详情见 `docs/`。

## 技术栈

| 层 | 技术 |
|----|------|
| 客户端 | Kuikly 2.7.0-2.1.21（Kotlin Multiplatform），Kuikly Compose |
| 客户端壳 | Android（Kotlin/AGP 7.4.2）、iOS（部署目标 14.1）、鸿蒙（搁置） |
| 后端 API | Go（计划） |
| AI 服务 | Python + FastAPI（计划） |
| 通信 | 对外 REST/JSON，内部 gRPC（protobuf） |
| 存储 | MySQL（主）、Redis（临时状态）、RabbitMQ（异步，可选） |

## 当前状态

- ✅ **客户端**：`mobile/shared` 已有可运行的单聊天页（`SessionPage` + `SessionViewModel` + `ChatRepository`），含消息气泡、输入栏、消息列表等组件，Android/iOS 可构建。
- ⬜ **服务端**：`services/go-api`、`services/ai-service` 为空，按计划从接口/边界设计开始搭建。
- ⬜ **移动端联网**：将 Repository 从 Mock 切换为真实 Go API（见 `MVP_EXECUTION_PLAN.md` 阶段 9–11）。

详细路线图见 [`docs/MVP_EXECUTION_PLAN.md`](./docs/MVP_EXECUTION_PLAN.md)（12 阶段、4 个里程碑）。

## 快速开始

### 客户端（移动端）

前置：JDK 17+、Android SDK、Xcode（iOS）、Node（调试服务器）、Gradle 8.5（已含 wrapper）。

```sh
cd mobile

# Android Debug APK
./gradlew :androidApp:assembleDebug

# iOS 框架（CocoaPods）
./gradlew :shared:generateDummyFramework
cd iosApp && pod install && cd ..   # Xcode 打开 iosApp/iosApp.xcworkspace

# 单页 JS 产物（分包构建加 -PpageName=flowmind_session_page）
./gradlew :shared:jsBrowserProductionWebpack

# 本地热更新（静态服务 + whistle 代理）
npm run serve        # :8017 静态，:8083 whistle
```

测试：

```sh
./gradlew :shared:allTests   # 所有 KMP 目标
./gradlew :shared:test        # 仅 JVM
```

### 服务端（规划中）

按 `docs/MVP_EXECUTION_PLAN.md` 阶段 0–8 搭建：

- `go-api`：Go module + `cmd/api/main.go` + MySQL/Redis/gRPC 客户端 + REST API；
- `ai-service`：Python `pyproject.toml` + FastAPI 健康检查 + `grpc.aio` ChatService + Provider 抽象；
- 通过 `docker-compose.yml` 拉起 MySQL / Redis（及可选 RabbitMQ）；
- 接口契约见 `docs/MVP_REQUIREMENTS.md` 第 7 节（REST）与第 11 节（gRPC）。

## 对外 API 概览（MVP）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/healthz`、`/readyz` | 健康检查 / 依赖就绪 |
| POST | `/api/v1/sessions` | 创建会话 |
| GET | `/api/v1/sessions` | 会话列表（当前 `client_id`） |
| GET | `/api/v1/sessions/{id}/messages` | 消息历史 |
| POST | `/api/v1/sessions/{id}/messages` | 发送消息 |
| DELETE | `/api/v1/sessions/{id}` | 删除会话 |

所有业务请求携带 `X-Client-ID` 与 `X-Request-ID`。统一错误格式与状态码见需求文档第 8 节。

## 贡献与约定

- 客户端代码集中在 `mobile/shared/src/commonMain`，避免平台特定 source set。
- 改 Kuikly 版本只改 `mobile/buildSrc/.../KotlinBuildVar.kt`。
- 继承 `BasePage`（而非 `ComposeContainer`）以保留深色模式等默认行为。
- 复用 `base/BasicWidget.kt` 的封装，不要重造 Compose 辅助。
- 严禁提交密钥；`.env`/配置文件中的真实密码与 API Key 不入库。
- HarmonyOS 脚手架暂不维护，改动客户端时无需顾及。

详见 [`CLAUDE.md`](./CLAUDE.md)（架构与构建）与 [`AGENT.md`](./AGENT.md)（AI 助手工作流）。
