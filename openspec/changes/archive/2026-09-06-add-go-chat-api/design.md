## Context

阶段 2 已提供 MySQL migration、`model.Session` / `model.Message` 与 GORM Repository：所有会话读取都以 `owner_key` 隔离，写入消息时以会话行锁和 `seq` 唯一索引保证稳定顺序。当前 `cmd/api/main.go` 只把依赖探针传给 `httpserver.New`，HTTP Server 只注册健康检查；因此 REST 契约尚未实际提供业务行为。

阶段 0 已定义匿名身份、OpenAPI 请求/响应、32,000 Unicode 字符的单条消息上限、上下文从最旧消息开始裁剪，以及标准错误包络。Python gRPC 服务和 Go gRPC Client 已为后续阶段存在，但本变更的目的正是先让 Go 业务闭环独立可验证。

约束如下：

- MySQL 是会话和消息事实来源；阶段 3 不用 Redis 保存或协调业务状态。
- HTTP 层不得绕过 `model.OwnerKey` 手工构造归属键，也不得绕过 `SessionRepository.GetByID` 做归属判断。
- 不改变阶段 0 OpenAPI 对外路径或数据形状，不新增移动端改动。
- 本仓库以小阶段教学与逐项 review 为工作方式；任务应保持可独立实现、验证和检查。

## Goals / Non-Goals

**Goals:**

- 兑现会话创建、列表、删除、消息历史和同步发送消息的 REST 契约。
- 以应用服务隔离 Gin/JSON、持久化 Repository 和 AI 完成调用，令业务规则能够在不启动 HTTP 或真实 AI 的条件下测试。
- 发送成功后持久化 user 与 assistant 两条消息，按 `seq` 恢复历史，并刷新会话的时间与首次消息标题。
- 用确定性、进程内的 Fake AI 完成器让 curl 与自动化测试可重复地验证完整链路。
- 为阶段 5 定义可替换的 AI 完成端口，而不是令 HTTP handler 直接依赖 gRPC Stub。

**Non-Goals:**

- 不从请求链路调用 Python gRPC，不实现 Provider 选择、真实模型、流式输出、token 用量或 gRPC 错误映射。
- 不实现 Redis 幂等结果缓存、处理状态、会话锁、限流或并发生成治理。
- 不修改 migration、数据表结构、Outbox 或移动端 Repository/UI。
- 不把消息内容、会话标题或模型 profile 的生成交给 AI。

## Decisions

### 1. 在 Go 应用层引入 AI 完成端口，并在阶段 3 使用进程内 Fake

应用服务依赖一个只表达“给定会话上下文，返回助手完成结果”的 Go 接口。阶段 3 注入固定且确定性的 Fake 实现；阶段 5 新增 gRPC Adapter 实现同一接口，替换组合根中的实现而不改变 HTTP 或领域流程。

选择此方案而非直接调用已有 Go gRPC Client，是为了让阶段 3 独立验证 REST、数据持久化和业务顺序，并使阶段 5 的学习目标聚焦在超时、状态映射与链路透传。也不把 Fake 留在 Gin handler，因为那会把可替换基础设施决策扩散到传输层。

### 2. HTTP handler 只负责传输适配，Chat 应用服务负责用例编排

HTTP 层负责读取 `X-Client-ID`、请求 JSON、路径参数和 request ID，并将领域结果映射为 OpenAPI DTO 与统一错误包络。一个 Chat 应用服务负责以下操作：创建 / 列表 / 删除会话、读取消息、发送消息。

该服务接收 SessionRepository、MessageRepository、AI 完成端口、时钟和 ID 生成器等协作项，以便单元测试替换边界。`main.go` 是唯一生产组合根：打开数据库后构造 Repository、业务服务和 HTTP Server。替代方案是让 handler 直接调用 GORM Repository；它会让 HTTP 测试成为验证业务规则的唯一手段，且阶段 5 的 AI 替换会侵入多个 handler。

### 3. 归属校验与错误映射保持单一语义

所有以 session ID 为目标的操作先调用 `SessionRepository.GetByID(ctx, model.OwnerKey(clientID), sessionID)`。`repository.ErrNotFound` 无论是不存在、已删除或属于其他 owner，均映射为 `404 SESSION_NOT_FOUND`。`X-Client-ID` 缺失、空白或超过合理 header 限制，以及 JSON / 路径 / body 校验错误，均映射为 `400 INVALID_ARGUMENT`；未预期错误为 `500 INTERNAL_ERROR`。

这延续阶段 2 的不泄漏归属语义和既有 request ID middleware。替代方案是 handler 先按 ID 查询再判断 owner，但会泄露会话存在性并重复安全规则。

### 4. 发送消息按持久化顺序同步执行，并显式限定阶段 3 重复请求行为

服务先确认会话归属，并在写入前以 `(session_id, client_message_id)` 查询已有用户消息。若已存在，返回 `409 DUPLICATE_REQUEST`，不再调用 Fake AI。若不存在，则保存 completed user message，读取上下文，调用 Fake AI，保存 completed assistant message，最后更新会话的 `last_message_at`、`updated_at` 和标题。

这利用既有唯一约束防止双写，但不宣称支持重试结果重放：当前 schema 没有 user/assistant 配对关系，也没有 Redis processing/completed 记录。阶段 6 才负责可重放幂等、会话锁和限流。替代方案是从现有消息序列猜测助手回复；其在 AI 失败或并发时不可靠。

### 5. 上下文构造先使用可替换、确定性的保守预算器

服务读取会话消息并将文本消息映射为 AI 输入；非 completed 消息不送入完成器。预算器从最旧消息开始丢弃，直到总估算量不超过上下文预算，且本次 user message 必须保留。阶段 3 以明确的近似规则（Unicode rune 数换算为保守 token 估计）验证顺序与裁剪语义；真实 Provider 接入时可替换为模型匹配 tokenizer。

选择近似预算而非伪装为精确 tokenizer，是因为具体模型和 tokenizer 仍是阶段 4/5 决策。选择读取全量并从最旧开始裁剪而非 SQL `LIMIT` 最近 N 条，是因为后者无法保证当前用户消息与预算边界的正确性。

### 6. 标题只在首条用户消息成功发送后以确定性规则生成

新会话标题固定为“新会话”。如果发送前会话尚无 `LastMessageAt`，发送成功后使用当前用户内容的 trim 后前 32 个 Unicode 字符作为标题；若有截断则添加省略号。后续消息不改标题。

此方案无需 AI 依赖、可预测且适合 MVP；异步 AI 标题生成仍是未来 Outbox 事件的可选能力。

## Risks / Trade-offs

- [同步 Fake 调用与两次消息写入之间发生错误] → user message 可能已保存而请求返回失败；阶段 3 保留该用户消息，后续 Redis 幂等 / retry 设计必须定义如何续跑或恢复结果。
- [同一会话并发请求] → Repository 保证单条消息的 `seq`，但不能保证两次完整生成的上下文不交错；阶段 3 明确不解决，阶段 6 用会话锁处理。
- [近似 token 预算不等于真实模型 token 数] → 命中具体 Provider 后以可替换 tokenizer 校正；阶段 3 测试只断言裁剪顺序和保留当前消息。
- [Fake 与真实 AI 行为不同] → Fake 只作为业务链路的确定性替身；gRPC 的超时、不可用和错误映射由阶段 5 单独验收。
- [客户端清除数据或伪造 client ID] → 匿名 `client_id` 不是认证凭证；这是 MVP 产品边界，不在本变更改善。

## Migration Plan

1. 不需要数据库迁移：阶段 2 表与 Repository 是本变更的前置条件。
2. 部署时在 Go 组合根中注册新的业务依赖和 API 路由；仍保留现有 `/healthz` 与 `/readyz`。
3. 先运行单元 / HTTP 测试，再在本地 MySQL 环境用 curl 验证创建会话、发送、读历史和删除闭环。
4. 若需要回滚，移除新的路由与应用层依赖即可；已写入的 MySQL 会话和消息符合既有 schema，保留而不破坏阶段 2。

## Open Questions

- 阶段 3 的 Fake AI 故障注入是否需要作为公开配置，还是仅由单元测试中的 fake stub 覆盖？默认采用后者，避免为测试制造生产配置。
- 初始上下文预算、最大输出 token 和 temperature 的精确默认值尚未在阶段 0 固化；本变更将它们封装在应用服务配置中，具体数值在实施前的首个小任务确认。
