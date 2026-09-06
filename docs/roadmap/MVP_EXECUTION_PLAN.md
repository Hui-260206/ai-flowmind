# FlowMind MVP 执行计划

## 1. 执行原则

本项目采用“先服务端、后移动端”的开发顺序：

1. 先定义接口和数据契约；
2. 先完成 Go 服务端独立闭环；
3. 再接入 Python gRPC AI 服务；
4. 完成 Redis、Outbox 和基础测试；
5. 最后接入 Kuikly 移动端；
6. 移动端只依赖稳定的 Go API，不感知 Python、gRPC、Redis 和 RabbitMQ。

## 2. 建议目录

```text
ai-flowmind/
├── mobile/
├── services/
│   ├── go-api/
│   │   └── internal/migrate/migrations/   # SQL 迁移（阶段 2 起；embed 进 Go 二进制）
│   ├── ai-service/
│   ├── proto/
│   ├── docker-compose.yml
│   └── Makefile
├── docs/
│   ├── phase-0/              # 阶段 0：需求、契约、API
│   ├── phase-1/              # 阶段 1：服务端基础工程
│   └── roadmap/              # 本文件
└── CLAUDE.md
```

## 3. 阶段总览

| 阶段 | 任务 | 结果 |
|---|---|---|
| 0 | 接口和边界设计 | OpenAPI、proto、数据模型完成 |
| 1 | 服务端基础工程 | Go、Python、MySQL、Redis 可启动 |
| 2 | Go 数据层和领域模型 | 会话、消息、Outbox migration 完成 |
| 3 | Go 聊天 API | 使用 Go 进程内 Fake AI 完成服务端闭环 |
| 4 | Python AI 服务 | gRPC ChatService 和真实 Provider 完成 |
| 5 | Go–Python gRPC 联调 | Go 通过 gRPC 获取 AI 回复 |
| 6 | Redis 能力 | 幂等、会话锁、限流完成 |
| 7 | Outbox/RabbitMQ | MVP 不实施，未来按需评估 |
| 8 | 服务端测试和可观测性 | 服务端达到移动端接入条件 |
| 9 | Kuikly 网络层接入 | Remote Repository 连接真实 API |
| 10 | 移动端会话和历史 | 创建、切换、恢复会话 |
| 11 | Android/iOS 联调 | 两端完成体验验证 |
| 12 | MVP 验收 | 核心场景和异常场景通过 |

## 4. 阶段 0：接口和边界设计

### 目标

在写业务代码前确定外部 REST API、内部 gRPC API、数据库字段、Redis Key 和事件类型。

### 任务

- [x] 创建 `services/` 基础目录；
- [x] 创建 `services/proto/ai/v1/common.proto`；
- [x] 创建 `chat.proto`；
- [x] 创建 `agent.proto`；
- [x] 创建 `vision.proto`；
- [x] 创建 `tool.proto`；
- [x] 创建 `docs/phase-0/api/openapi.yaml`；
- [x] 确定 `owner_key`、`client_id` 和未来 `user_id` 的关系；
- [x] 确定消息角色：`system`、`user`、`assistant`、`tool`；
- [x] 确定消息状态：`pending`、`completed`、`failed`；
- [x] 确定请求错误码；
- [x] 确定消息最大长度和上下文截断策略；
- [x] 确定模型 profile，而不是让移动端直接传 Provider 名称。

### 验收标准

- Go 和 Python 使用同一份 proto；
- 对外 API 有请求和响应示例；
- Agent、Vision、Tool 接口已经预留；
- 不需要登录也能明确识别匿名设备数据归属。

## 5. 阶段 1：服务端基础工程

> ✅ 已完成（2026-08-16）。Go API 与 Python AI 服务可独立启动；Go 具备 `/healthz`、`/readyz`（mysql/redis/python_grpc 探针）、结构化日志、request_id、优雅退出与统一错误包络；Python 采用纯 gRPC 设计（不引入 FastAPI/HTTP，健康检查由 Go `/readyz` 的 gRPC 探针承担）；MySQL/Redis 已具备 Docker Compose 编排。Go/Python 容器镜像（Dockerfile）按计划推迟到后续部署阶段。

### 目标

让 Go API 和 Python AI 服务可以独立启动，并具备本地基础设施连接能力。

### Go 任务

- [x] 初始化 Go Module；
- [x] 创建 `cmd/api/main.go`；
- [x] 创建配置加载模块；
- [x] 创建 HTTP Server（Gin）；
- [x] 添加 `/healthz`；
- [x] 添加 `/readyz`（mysql/redis/python_grpc 依赖探针）；
- [x] 添加 graceful shutdown；
- [x] 添加结构化日志（slog JSON）；
- [x] 添加 request_id 中间件；
- [x] 初始化 MySQL；
- [x] 初始化 Redis；
- [x] 初始化 gRPC Client；
- [x] 创建统一错误响应模型（request_id + error.code/message 包络）。

### Python 任务

- [x] 初始化 `pyproject.toml`（uv 管理）；
- [x] 创建服务启动入口（纯 gRPC 设计，不引入 FastAPI/HTTP；健康检查经 Go `/readyz` 的 python_grpc 探针承担，见 [services/README.md](../../services/README.md)）；
- [x] 创建 `grpc.aio` 服务启动入口（含优雅停止）；
- [x] 创建配置模块；
- [x] 创建日志和 request_id 处理（contextvars 透传 + 日志过滤器）；
- [x] 创建 `ChatProvider` 接口；
- [x] 创建 Fake Provider；
- [x] 生成 Python protobuf 代码（`make proto-generate-python`，产物不入库）。

### 基础设施任务

- [x] 创建 MySQL Docker Compose 服务；
- [x] 创建 Redis Docker Compose 服务；
- [ ] 后续部署阶段创建 Go API Dockerfile；
- [ ] 后续部署阶段创建 Python AI Dockerfile；
- [x] 提供本地环境变量示例（`.env.example` / `.env.docker.example`）；
- [x] 禁止提交真实密码和 API Key（`.env` 已被 gitignore，仅示例入库）。

### 验收标准

```text
docker compose up
Go /healthz 返回成功
Go /readyz 全部通过（mysql、redis、python_grpc 三个探针）
Python gRPC 服务可启动，且能被 Go 调用
Go 成功连接 MySQL、Redis 和 Python gRPC
```

## 6. 阶段 2：数据库和领域模型

### 目标

建立聊天数据的持久化模型，MySQL 作为事实来源。

### 任务

- [x] 创建 `chat_sessions` migration；
- [x] 创建 `chat_messages` migration；
- [x] 创建 `outbox_events` migration；
- [x] 预留 `ai_runs` 模型或 migration（决策：先不建表，见 `docs/phase-2/PHASE_2_PLAN.md`）；
- [x] 增加 `session_id + seq` 索引；
- [x] 增加 `owner_key + updated_at` 索引；
- [x] 增加 `session_id + created_at` 索引；
- [x] 增加幂等键唯一约束；
- [x] 实现 Session Repository；
- [x] 实现 Message Repository；
- [x] 实现 Outbox Repository；
- [x] 实现会话归属校验；
- [x] 实现稳定的消息顺序。

### 关键设计

- 不使用 `is_user` Boolean 表达消息角色；
- 使用 `role` 支持未来 `tool` 消息；
- 不使用进程内存作为聊天记录来源；
- 不使用 Redis 替代 MySQL；
- 所有会话查询必须带 `owner_key` 条件。

### 验收标准

- migration 可重复执行；
- 会话和消息可创建、查询、删除；
- 服务重启后数据仍然存在；
- 不同匿名设备不能访问彼此的会话。

## 7. 阶段 3：Go 聊天 API

> ✅ 已完成（2026-09-06）。Go API 已实现匿名会话创建、列表、软删除、消息历史和同步发送消息；主链路使用确定性的 Go 进程内 Fake AI，不依赖 Python gRPC。重复 `client_message_id` 当前返回 `409 DUPLICATE_REQUEST`；可重放幂等结果、会话锁与限流仍由阶段 6 实现。

### 目标

先不依赖真实模型，使用 Fake AI Client 完成 Go 服务端的业务闭环。

### 任务

- [x] 实现创建会话；
- [x] 实现会话列表；
- [x] 实现消息列表；
- [x] 实现发送消息；
- [x] 实现删除会话；
- [x] 实现消息长度校验；
- [x] 实现上下文读取；
- [x] 实现上下文截断；
- [x] 实现 Go 进程内 Fake AI Client；
- [x] 保存用户消息；
- [x] 保存 AI 消息；
- [x] 更新会话时间和标题；
- [x] 实现统一错误状态码；
- [x] 为所有接口增加 request_id。

### 发送消息流程

```text
校验 owner_key 和 session_id
    ↓
检查 client_message_id
    ↓
保存 user message
    ↓
读取最近上下文
    ↓
调用 Fake AI Client
    ↓
保存 assistant message
    ↓
返回两个消息
```

### 验收标准

使用 curl 可以完成：

```text
创建会话
查询会话列表
发送消息
查询消息历史
删除会话
```

## 8. 阶段 4：Python AI 服务

> ✅ 已完成（2026-09-06）。Python `ChatService.Complete` 已支持 Fake 与 HY3 OpenAI-compatible Provider 切换、模型/参数校验、Provider 超时、错误分类、模型名称与 token usage 返回；流式 RPC 继续作为 proto 预留，未纳入 MVP。

### 目标

实现 Python gRPC ChatService，并统一不同模型 Provider 的调用方式。

### 任务

- [x] 实现 `ChatService.Complete`；
- [ ] 为流式能力实现 proto 预留，不要求 MVP 移动端使用；
- [x] 实现 Provider 抽象；
- [x] 实现 Fake Provider；
- [x] 实现 OpenAI Compatible HY3 Provider；
- [ ] 可选实现 Ollama Provider；
- [x] 实现模型 profile 选择；
- [x] 实现模型参数校验；
- [x] 实现超时；
- [x] 实现 Provider 错误转换；
- [x] 返回模型名称和 token usage；
- [x] 记录 AI 调用耗时。

### 预留能力

- [ ] 创建 `AgentService` 空实现或 `UNIMPLEMENTED`；
- [ ] 创建 `VisionService` 空实现或 `UNIMPLEMENTED`；
- [ ] 创建 `ToolService` 空实现或 `UNIMPLEMENTED`；
- [ ] 将文本消息设计成可扩展的 ContentPart。

### 验收标准

- Fake Provider 和真实 Provider 可配置切换；
- Provider 错误可被 Go 识别；
- Python 服务不会将数据库业务逻辑混入模型调用；
- 没有 API Key 时返回明确配置错误。

## 9. 阶段 5：Go–Python gRPC 联调

> ✅ 已完成（2026-09-06）。Go 生产组合根已用 gRPC Completer 替换进程内 Fake，支持长连接 keepalive、消息大小上限、单次 RPC deadline、request_id 透传、取消传播和 gRPC 状态映射；Python gRPC 已恢复为 `/readyz` 必要依赖。

### 目标

用真实 gRPC Client 替换 Go 的 Fake AI Client。

### 任务

- [x] Go 建立可复用 gRPC 长连接；
- [x] 设置 keepalive；
- [x] 每次调用设置 context timeout；
- [x] 透传 request_id（trace_id 继续预留）；
- [x] 将 gRPC status 转换成业务错误；
- [x] 设置消息大小上限；
- [x] 支持客户端取消；
- [ ] 不对所有模型请求进行无脑重试；
- [ ] 使用幂等机制解决网络重试问题；
- [x] 验证 Python 服务不可用和超时的单元测试映射；真实 HY3 REST smoke 见本变更验收记录。

### 验收标准

- Go 可以通过 gRPC 收到 AI 回复；
- Python 停止后 Go 返回 502 或 504；
- 请求超时后不会长时间占用 HTTP 连接；
- Go 和 Python 日志可以用 request_id 关联。

## 10. 阶段 6：Redis 能力

### 目标

让 MVP 具备重复请求保护、会话串行化和基础限流。

### 任务

- [ ] 实现幂等 Key：`idempotency:{owner_key}:{client_message_id}`；
- [ ] 实现 `processing/completed/failed` 状态；
- [ ] 缓存幂等结果关联的消息 ID；
- [ ] 实现会话锁：`lock:conversation:{session_id}`；
- [ ] 为锁设置 TTL；
- [ ] 实现匿名设备限流；
- [ ] 实现会话级并发限制；
- [ ] 增加 Redis 不可用时的错误策略；
- [ ] 测试重复请求和并发请求。

### 建议初始配置

```text
单匿名设备每分钟最多发送 20 条消息
同一会话同时只允许 1 个生成请求
幂等记录 TTL：24～48 小时
会话锁 TTL：不超过单次 AI 请求最大超时时间
```

### 验收标准

- 同一个 client_message_id 不会重复调用模型；
- 同一会话同时发送两条消息时不会造成上下文交错；
- 超过限流返回 429；
- Redis 数据丢失不会导致 MySQL 聊天记录丢失。

## 11. 阶段 7：Outbox 和 RabbitMQ（MVP 不实施，未来按需评估）

### 目标

为非关键异步任务提供可靠事件能力，不影响普通聊天同步响应。

### 推荐事件

```text
chat.completed
chat.failed
ai.usage.recorded
conversation.title.generate.requested
```

### 任务

- [ ] 在保存 AI 消息的同一事务中写入 Outbox；
- [ ] 实现 Outbox Publisher；
- [ ] 实现 RabbitMQ 连接和重试；
- [ ] 实现 `chat.completed` 消费者；
- [ ] 实现消费者幂等；
- [ ] 记录发布失败和重试次数；
- [ ] RabbitMQ 不可用时不影响聊天主链路；
- [ ] 验证重复发布和重复消费。

### 不应做的事情

不要把普通聊天流程改成：

```text
移动端 → RabbitMQ → AI → 移动端轮询
```

聊天主链路仍然保持同步 HTTP + gRPC。RabbitMQ 只处理异步副作用。

## 12. 阶段 8：服务端测试和可观测性

### 单元测试

- [ ] 会话创建和归属校验；
- [ ] 消息顺序；
- [ ] 消息长度限制；
- [ ] 上下文截断；
- [ ] 幂等逻辑；
- [ ] 会话锁；
- [ ] 限流；
- [ ] gRPC 错误转换；
- [ ] Provider 选择和错误处理。

### 集成测试

- [ ] Go + MySQL；
- [ ] Go + Redis；
- [ ] Go + Python gRPC；
- [ ] Go + RabbitMQ（如果启用）；
- [ ] 端到端发送消息；
- [ ] 服务重启恢复历史。

### 异常测试

- [ ] Python 服务不可用；
- [ ] Python 服务超时；
- [ ] MySQL 暂时不可用；
- [ ] Redis 暂时不可用；
- [ ] 重复发送；
- [ ] 并发发送；
- [ ] 会话不存在；
- [ ] 越权访问；
- [ ] 空消息和超长消息；
- [ ] HTTP 客户端取消请求。

### 基础指标

```text
chat_request_total
chat_request_failed_total
chat_request_latency
ai_grpc_latency
ai_timeout_total
message_persist_failed_total
redis_lock_failed_total
```

### 移动端接入前门槛

- [ ] curl 可以完整完成对话；
- [ ] 聊天记录可查询；
- [ ] 服务重启后数据不丢失；
- [ ] 重复请求有幂等保护；
- [ ] Python 不可用时错误可控；
- [ ] 会话越权测试通过；
- [ ] Docker Compose 可启动本地服务端；
- [ ] Go 和 Python 日志可关联。

## 13. 阶段 9：Kuikly 网络层接入

### 目标

让当前移动端通过 Repository 访问 Go API，不直接感知 Python 和内部基础设施。

### 现有代码

- [SessionPage.kt](/Users/helix/study/ai-flowmind/mobile/shared/src/commonMain/kotlin/com/heli/flowmind/page/SessionPage.kt)
- [SessionViewModel.kt](/Users/helix/study/ai-flowmind/mobile/shared/src/commonMain/kotlin/com/heli/flowmind/state/SessionViewModel.kt)
- [ChatRepository.kt](/Users/helix/study/ai-flowmind/mobile/shared/src/commonMain/kotlin/com/heli/flowmind/data/ChatRepository.kt)
- [RemoteChatRepository.kt](/Users/helix/study/ai-flowmind/mobile/shared/src/commonMain/kotlin/com/heli/flowmind/data/RemoteChatRepository.kt)

### 任务

- [ ] 将 Repository 从 `sendChat(history)` 改为会话业务接口；
- [ ] 增加 `createSession()`；
- [ ] 增加 `listSessions()`；
- [ ] 增加 `getMessages(sessionId)`；
- [ ] 增加 `sendMessage(sessionId, content, clientMessageId)`；
- [ ] 增加 `deleteSession(sessionId)`；
- [ ] 使用 Kuikly `NetworkModule`；
- [ ] 设置 JSON Content-Type；
- [ ] 设置 `X-Client-ID`；
- [ ] 设置 `X-Request-ID`；
- [ ] 统一包装网络错误为 `ChatException` 或领域异常；
- [ ] 配置不同平台的 API Base URL。

Kuikly 网络 API 参考：

- `/Users/helix/.agents/skills/kuikly-ui-framework/references/KuiklyUI/docs/API/modules/network.md`
- `/Users/helix/.agents/skills/kuikly-network-and-json/references/NETWORK_API_REFERENCE.md`

### 验收标准

- Mock Repository 和 Remote Repository 可以切换；
- Android 和 iOS 都能发起真实 HTTP 请求；
- HTTP 错误能转换成移动端可识别的状态。

## 14. 阶段 10：移动端会话和历史

### 目标

将当前单一聊天页面扩展为可恢复的会话体验。

### ViewModel 任务

- [ ] 增加当前 `sessionId`；
- [ ] 增加会话列表状态；
- [ ] 增加历史加载状态；
- [ ] 增加发送状态；
- [ ] 增加统一错误状态；
- [ ] 首次进入时加载或创建会话；
- [ ] 加载消息历史；
- [ ] 切换会话；
- [ ] 删除会话；
- [ ] 生成 `client_message_id`；
- [ ] 发送失败后重试；
- [ ] 发送期间禁止重复请求。

### UI 任务

- [ ] 会话列表入口；
- [ ] 当前会话标题；
- [ ] 空会话状态；
- [ ] 历史加载状态；
- [ ] AI 回复 loading 状态；
- [ ] AI 回复失败状态；
- [ ] 重试按钮；
- [ ] 删除会话入口；
- [ ] 长文本展示；
- [ ] 消息列表自动滚动。

### 验收标准

- 首次打开可以进入空会话；
- 发送消息后显示用户消息和 AI 回复；
- 退出并重新进入后历史仍存在；
- 切换会话后内容正确；
- 重复点击发送不会产生重复消息；
- 网络错误可以重试。

## 15. 阶段 11：Android/iOS 联调

### 任务

- [ ] Android 真机验证；
- [ ] iOS 模拟器或真机验证；
- [ ] 验证 API Base URL；
- [ ] 验证本地开发网络权限；
- [ ] 验证键盘弹出和收起；
- [ ] 验证消息列表滚动；
- [ ] 验证网络超时；
- [ ] 验证页面销毁和重新进入；
- [ ] 验证深色模式；
- [ ] 验证空状态和错误状态；
- [ ] 验证超长消息。

### 暂不处理

- [ ] 暂不接入流式 token；
- [ ] 暂不接入 WebSocket；
- [ ] 暂不接入图片选择器；
- [ ] 暂不接入登录页面。

## 16. 阶段 12：MVP 最终验收

### 场景一：首次使用

```text
安装 App
→ 生成 client_id
→ 创建会话
→ 发送第一条消息
→ 收到 AI 回复
→ 消息写入 MySQL
```

### 场景二：重新进入

```text
退出页面
→ 再次进入
→ 获取会话列表
→ 打开会话
→ 加载历史消息
```

### 场景三：重复请求

```text
同一个 client_message_id 发送两次
→ 只产生一条用户消息
→ 只产生一条 AI 回复
```

### 场景四：AI 服务异常

```text
关闭 Python 服务
→ 移动端发送消息
→ Go 返回明确错误
→ 移动端提供重试
```

### 场景五：服务重启

```text
Go 服务重启
→ MySQL 历史仍然存在
→ 会话列表仍然正常
→ 不依赖进程内存恢复数据
```

### 场景六：并发请求

```text
同一会话同时发送两条消息
→ Redis 会话锁生效
→ 不产生上下文交错
```

## 17. 里程碑

### Milestone 1：服务端可用

- [ ] curl 创建会话；
- [ ] curl 发送消息；
- [ ] curl 查询历史；
- [ ] Go 调用 Python gRPC；
- [ ] MySQL 保存消息。

### Milestone 2：服务端可靠

- [ ] Redis 幂等；
- [ ] Redis 会话锁；
- [ ] 基础限流；
- [ ] 统一错误；
- [ ] request_id；
- [ ] 服务端测试；
- [ ] 服务重启数据不丢失。

### Milestone 3：移动端可用

- [ ] Kuikly 连接 Go API；
- [ ] 发送消息；
- [ ] 加载历史；
- [ ] 切换会话；
- [ ] 失败重试；
- [ ] Android 验证；
- [ ] iOS 验证。

### Milestone 4：MVP Engineering 完成

- [ ] Outbox；
- [ ] RabbitMQ；
- [ ] 异步事件；
- [ ] AI 使用量统计；
- [ ] 基础指标；
- [ ] Agent、Vision、Tool 接口预留；
- [ ] Docker Compose 完整联调。

## 18. 后续版本顺序

MVP 完成后建议依次推进：

1. 流式输出；
2. 用户登录和匿名会话迁移；
3. 图片上传和图片识别；
4. Agent 短任务；
5. Tool Executor 和业务工具；
6. Agent 长任务和 RabbitMQ；
7. RAG；
8. 多模型路由；
9. 多租户和成本治理。
