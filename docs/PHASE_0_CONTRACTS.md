# 阶段 0 契约约定

## 身份与边界

- 移动端生成并持久化 UUID `client_id`，通过 `X-Client-ID` 发送。
- Go 将 `owner_key` 定义为 `client:{client_id}`；未来登录后可改为 `user:{user_id}`，数据库字段和所有权查询保持不变。
- `X-Request-ID` 可由客户端提供；缺失时由 Go 生成并在 REST、gRPC 日志中透传。
- 移动端只能传服务端配置的 `model_profile`（默认 `default`），不能传 Provider、模型密钥或模型 URL。

## 消息与限制

- 角色固定为 `system`、`user`、`assistant`、`tool`；禁止用 `is_user` 表达角色。
- 状态固定为 `pending`、`completed`、`failed`。
- 单条文本最大 32,000 Unicode 字符；`client_message_id` 最大 128 字符。
- Go 读取最近消息构造上下文，按会话内 `seq` 升序，从最旧消息开始丢弃，直到上下文不超过 32,000 tokens；当前用户消息必须保留。未来可按 profile 配置不同上限。
- Proto 使用 `ContentPart`，当前支持文本，图片仅传对象存储 `object_key`，不在 gRPC 中传大段 Base64。

## 错误码

| HTTP | code | 说明 |
|---|---|---|
| 400 | `INVALID_ARGUMENT` | 参数、空消息或超长 |
| 404 | `SESSION_NOT_FOUND` | 会话不存在或不属于当前 owner |
| 409 | `DUPLICATE_REQUEST` / `SESSION_BUSY` | 幂等请求或会话锁冲突 |
| 429 | `RATE_LIMITED` | 超出匿名客户端/会话限流 |
| 500 | `INTERNAL_ERROR` | Go 内部错误 |
| 502 | `AI_UNAVAILABLE` / `AI_PROVIDER_ERROR` | Python 或 Provider 异常 |
| 504 | `AI_TIMEOUT` | AI 调用超时 |

错误响应统一为 `{request_id, error: {code, message}}`。

## Redis 与事件边界

```text
idempotency:{owner_key}:{client_message_id}
lock:conversation:{session_id}
rate_limit:owner:{owner_key}:minute
```

幂等 TTL 24 小时，会话锁必须有 TTL；Redis 只保存临时状态，MySQL 才是会话和消息事实来源。预留事件类型：`chat.completed`、`chat.failed`、`ai.usage.recorded`。
