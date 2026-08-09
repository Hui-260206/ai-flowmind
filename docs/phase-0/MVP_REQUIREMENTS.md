# FlowMind MVP 需求说明

## 1. 文档目的

本文档定义 FlowMind 第一阶段 MVP 的产品范围、系统边界、核心用例、接口约束和验收标准，作为服务端和 Kuikly 移动端开发的共同依据。

## 2. MVP 目标

FlowMind MVP 面向移动端提供基础 AI 对话能力，完成以下闭环：

```text
Kuikly 移动端
    ↓ HTTPS JSON
Go API 服务
    ├── MySQL：会话和聊天消息持久化
    ├── Redis：幂等、会话锁、限流
    └── Outbox/MQ：MVP 不启用，未来处理非关键异步事件
    └── gRPC
          ↓
Python AI 服务
    └── 大模型 Provider
```

MVP 的首要目标不是实现完整 AI 平台，而是建立可扩展、可测试、可演进的对话基础能力。

## 3. MVP 范围

### 3.1 必须实现

- 匿名设备身份标识，不实现用户注册、登录和认证页面；
- 创建会话；
- 查询当前匿名设备的会话列表；
- 查询会话聊天记录；
- 发送文本消息；
- Go 服务通过 gRPC 调用 Python AI 服务；
- 保存用户消息和 AI 回复；
- 重新进入页面后恢复聊天记录；
- Redis 幂等控制；
- Redis 会话级并发锁；
- 基础限流；
- 服务端统一错误响应；
- 基础日志、request_id 和健康检查；
- Android 与 iOS Kuikly 移动端联调。

### 3.2 预留但暂不实现

- Agent 实际运行逻辑；
- 图片上传和图片识别；
- 多模态消息实际处理；
- 流式对话；
- 工具调用；
- 用户登录认证；
- RAG 和向量检索；
- 多租户和组织权限；
- 复杂后台管理能力。

### 3.3 可选的 MVP Engineering 能力

以下能力不阻塞 MVP Core，但建议在服务端核心闭环稳定后补齐：

- Outbox 事件表；
- RabbitMQ 异步事件发布；
- `chat.completed`、`chat.failed` 事件；
- AI 调用用量统计；
- 异步生成会话标题；
- 基础指标和链路追踪。

## 4. 系统边界

### 4.1 Kuikly 移动端

负责：

- 聊天页面和会话列表；
- 文本输入与发送；
- 消息展示；
- 加载、失败、重试状态；
- 保存匿名 `client_id`；
- 调用 Go API。

不负责：

- 维护完整模型上下文；
- 直接调用 Python 服务；
- 保存聊天记录的唯一副本；
- 保存模型密钥。

### 4.2 Go API 服务

负责：

- 对移动端提供唯一业务 API；
- 会话和消息业务流程；
- 匿名设备数据隔离；
- MySQL 持久化；
- 上下文读取和长度控制；
- 调用 Python gRPC 服务；
- Redis 幂等、锁和限流；
- 统一错误处理；
- 后续的工具权限和业务编排边界。

### 4.3 Python AI 服务

负责：

- 模型调用；
- Provider 适配；
- 模型参数校验；
- AI 错误转换；
- 后续 Agent、Vision 和工具编排能力。

不负责：

- 移动端 API；
- 用户身份和会话所有权；
- 业务数据库直接读写；
- 业务权限判断。

### 4.4 MySQL

MySQL 是会话和消息的事实来源。Redis、移动端本地状态和进程内存都不能替代 MySQL。

### 4.5 Redis

MVP 中只承担临时和高频状态：

- 请求幂等；
- 会话级锁；
- 限流计数；
- 可选的短期缓存。

Redis 不承担永久聊天记录和大段上下文的唯一存储职责。

### 4.6 消息队列（MVP 不启用）

MVP 不引入 RabbitMQ、Kafka、Redis Streams 或其他消息队列。普通聊天使用同步 HTTP + gRPC 主链路。未来若启用 MQ，只处理非关键异步任务，例如：

- 聊天完成事件；
- AI 用量统计；
- 会话标题生成；
- 审计和异步清理。

## 5. 匿名身份

由于 MVP 不实现用户登录，移动端首次启动时生成 UUID 形式的 `client_id`，并持久化到本地。

请求头：

```http
X-Client-ID: <client_id>
X-Request-ID: <request_id>
```

匿名身份的限制：

- 清除应用数据后可能无法恢复历史；
- 更换设备不能自动同步；
- `client_id` 不是安全认证凭证；
- 后续登录上线时需要支持匿名会话迁移到正式账户。

## 6. 核心用户用例

### 6.1 首次进入

1. 移动端读取或生成 `client_id`；
2. 查询会话列表；
3. 如果没有会话，创建新会话；
4. 展示空状态和输入栏。

### 6.2 发送消息

1. 用户输入文本并点击发送；
2. 移动端生成 `client_message_id`；
3. 移动端立即展示用户消息和 AI loading 占位；
4. Go 校验会话归属、消息长度和幂等键；
5. Go 保存用户消息；
6. Go 读取最近上下文；
7. Go 通过 gRPC 调用 Python；
8. Go 保存 AI 回复；
9. Go 返回用户消息和 AI 消息；
10. 移动端替换 loading 占位。

### 6.3 重新进入会话

1. 移动端获取会话 ID；
2. 请求会话消息；
3. 按服务端返回顺序展示历史。

### 6.4 重复发送

同一个 `client_message_id` 重复到达时，服务端不得重复创建用户消息或重复调用模型，应返回已有处理结果或明确的处理中状态。

### 6.5 AI 服务异常

用户消息可以保留，AI 回复记录为失败或由业务状态标记失败，移动端展示可重试错误。Python 服务异常不能导致用户消息丢失。

## 7. 对外 API 需求

### 7.1 健康检查

```http
GET /healthz
GET /readyz
```

`healthz` 表示进程存活；`readyz` 检查 MySQL、Redis 和 Python gRPC 依赖是否满足服务要求。

### 7.2 创建会话

```http
POST /api/v1/sessions
X-Client-ID: <client_id>
```

响应至少包含：

```json
{
  "id": "session_01J...",
  "title": "新会话",
  "created_at": "2026-08-08T12:00:00Z",
  "updated_at": "2026-08-08T12:00:00Z"
}
```

### 7.3 查询会话列表

```http
GET /api/v1/sessions
X-Client-ID: <client_id>
```

只返回当前 `client_id` 所属的会话。

### 7.4 查询消息

```http
GET /api/v1/sessions/{session_id}/messages
X-Client-ID: <client_id>
```

消息至少包含：

```json
{
  "id": "msg_01J...",
  "role": "user",
  "content": "你好",
  "status": "completed",
  "created_at": "2026-08-08T12:00:00Z"
}
```

角色预留：`system`、`user`、`assistant`、`tool`。

### 7.5 发送消息

```http
POST /api/v1/sessions/{session_id}/messages
X-Client-ID: <client_id>
Content-Type: application/json
```

请求：

```json
{
  "content": "介绍一下 Go",
  "client_message_id": "client-msg-001"
}
```

响应：

```json
{
  "request_id": "req_001",
  "user_message": {
    "id": "msg_001",
    "role": "user",
    "content": "介绍一下 Go",
    "status": "completed"
  },
  "assistant_message": {
    "id": "msg_002",
    "role": "assistant",
    "content": "Go 是一种静态类型编程语言。",
    "status": "completed"
  }
}
```

### 7.6 删除会话

```http
DELETE /api/v1/sessions/{session_id}
X-Client-ID: <client_id>
```

删除只能作用于当前 `client_id` 所属的会话。推荐使用软删除或可恢复删除策略。

## 8. 错误响应

统一格式：

```json
{
  "request_id": "req_001",
  "error": {
    "code": "AI_TIMEOUT",
    "message": "AI 服务响应超时"
  }
}
```

建议 HTTP 状态码：

| 状态码 | 场景 |
|---|---|
| 400 | 参数错误、消息为空或超长 |
| 404 | 会话不存在 |
| 409 | 重复请求或会话正在处理 |
| 429 | 超出限流 |
| 500 | Go 服务内部错误 |
| 502 | Python 或模型服务异常 |
| 504 | AI 服务超时 |

## 9. 数据模型需求

### 9.1 chat_sessions

```text
id
owner_key
title
model_profile
status
created_at
updated_at
last_message_at
deleted_at
```

### 9.2 chat_messages

```text
id
session_id
seq
role
content
status
client_message_id
model_name
prompt_tokens
completion_tokens
created_at
```

要求：

- `session_id + seq` 保证会话内稳定排序；
- `session_id + client_message_id` 支持幂等；
- `owner_key` 用于匿名设备或未来用户归属；
- 不使用 `is_user` 这种只能表达两种角色的字段；
- `role` 为字符串或可扩展枚举。

### 9.3 outbox_events（可选）

```text
id
event_type
aggregate_type
aggregate_id
payload
status
retry_count
next_retry_at
created_at
published_at
```

如果启用 RabbitMQ，消息数据和 Outbox 事件应在同一 MySQL 事务中提交。

## 10. Redis Key 规范

```text
idempotency:{owner_key}:{client_message_id}
lock:conversation:{session_id}
rate_limit:owner:{owner_key}:minute
```

建议：

- 幂等记录 TTL 为 24～48 小时；
- 会话锁必须有 TTL，避免异常后永久占锁；
- 限流按匿名客户端和会话分别控制；
- Redis 数据丢失后，MySQL 数据仍然完整。

## 11. gRPC 扩展要求

内部 RPC 使用 protobuf，建议预留：

```text
ChatService
AgentService
VisionService
ToolService
```

公共上下文至少包含：

```text
request_id
trace_id
conversation_id
user_id 或 owner_key
model_profile
```

消息内容建议使用可扩展的 `ContentPart`，当前实现文本，未来支持图片：

```text
TextPart
ImagePart
```

图片不建议在 gRPC 中传大量 Base64。未来推荐：

```text
移动端上传图片 → Go 保存对象存储 → 获得 object_key → Go 调用 VisionService
```

## 12. 安全和可靠性约束

- Python AI 服务不对公网暴露；
- 模型 API Key 只保存在 Python 服务或安全配置中心；
- 移动端不能传入任意 Provider 的密钥；
- Go 必须校验会话归属；
- Go 必须限制消息长度和上下文长度；
- AI 请求必须有超时；
- 同一会话默认只允许一个生成请求；
- 所有请求都应带 `request_id`；
- 不在日志中记录 API Key、密码或未脱敏敏感内容。

## 13. MVP 验收标准

### 功能验收

- 首次启动可以创建会话；
- 可以发送文本并收到 AI 回复；
- 用户消息和 AI 消息都写入 MySQL；
- 重新进入应用可以恢复历史；
- 可以查看多个会话；
- 可以删除自己的会话；
- 服务重启后聊天记录不丢失。

### 可靠性验收

- 重复 `client_message_id` 不生成重复消息；
- 同一会话并发发送不会造成上下文交错；
- AI 超时返回明确错误；
- Python 服务不可用时 Go 不崩溃；
- Redis 限流生效；
- 其他 `client_id` 无法访问当前会话。

### 移动端验收

- Android 可用；
- iOS 可用；
- 输入框和键盘行为正常；
- 消息列表能够滚动到底部；
- AI 回复期间有 loading 状态；
- 失败后可以重试；
- 重新打开会话后消息顺序正确。
