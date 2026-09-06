## Why

阶段 2 已建立会话与消息的持久化模型、归属隔离和顺序写入能力，但 Go API 仍只暴露健康检查，移动端或 curl 无法完成一次持久化对话。阶段 3 需要先在不依赖真实模型服务的条件下验证 REST 业务闭环，并为阶段 5 替换为 gRPC AI 调用保留清晰边界。

## What Changes

- 新增面向匿名设备的 Go REST 会话 API：创建、列表、删除会话，以及读取会话消息。
- 新增发送文本消息的同步业务闭环：校验输入和会话归属，保存用户消息，读取并裁剪上下文，调用确定性的进程内 Fake AI，保存助手消息，并更新会话元数据。
- 新增 HTTP 请求到领域服务的依赖注入、请求/响应 DTO 和统一的领域错误映射；全部接口延续既有 `request_id` 错误包络。
- 定义可替换的 Go AI 完成端口，并在本变更提供进程内 Fake 实现；不在本变更从 HTTP 请求调用 Python gRPC 服务。
- 明确阶段 3 的重复 `client_message_id` 只做数据库级重复检测并返回冲突；可重放结果、会话锁和限流留给阶段 6 的 Redis 能力。

## Capabilities

### New Capabilities

- `go-chat-api`: Go API 面向匿名客户端提供会话、消息历史和同步发送消息的 REST 业务生命周期。

### Modified Capabilities

- None.

## Impact

- Affected code: `services/go-api/cmd/api/main.go`、`internal/httpserver/`，以及新增的业务服务、HTTP handler/DTO、AI 完成端口与 Fake 实现包。
- Affected API: 实现并兑现 `docs/phase-0/api/openapi.yaml` 中的 `/api/v1/sessions`、`/api/v1/sessions/{session_id}` 和 `/api/v1/sessions/{session_id}/messages` 契约。
- Dependencies: 复用阶段 2 的 GORM Repository 和既有 Gin/request_id 中间件；不新增外部服务或 Go 依赖，不改变 Python AI 服务和 proto。
- Follow-on work: 阶段 5 将以 gRPC Adapter 替换进程内 Fake；阶段 6 将补齐 Redis 幂等、会话锁和限流。
