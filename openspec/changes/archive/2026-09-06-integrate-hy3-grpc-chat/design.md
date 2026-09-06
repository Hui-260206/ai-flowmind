## Context

阶段 3 的 Go 聊天应用服务已经抽象出 `Completer` 端口，但生产组合根注入的是进程内 `FakeCompleter`。Python 服务已有 `ChatService`、Fake Provider 和 gRPC Server 骨架；Go 已有共享 proto 的长连接 Client 与就绪探针。当前 Python 会把所有 Provider 错误压成 `INTERNAL`，而 Go 会将所有补全错误作为 HTTP 500，尚不符合阶段 0 规定的 502/504 语义。

HY3 的 URL 和 API Key 已写入未提交的 `services/.env`，但当前 `AI_PROVIDER=fake`。密钥、真实 URL 和实际模型名必须只由 Python 服务读取；Go、移动端、日志和 OpenSpec 文档均不得包含这些值。

## Goals / Non-Goals

**Goals:**

- 通过 OpenAI Chat Completions 兼容协议调用配置的 HY3 模型。
- 让 Go 聊天主链路复用现有 `Completer` 抽象改走 Python gRPC，不改 REST 请求和响应成功形状。
- 透传 HTTP 的 `request_id` 至 Python 日志，带超时、取消、消息大小限制和可识别的错误分类。
- 保留 Fake Provider，令 Python 与 Go 的单元测试无需真实密钥和网络。

**Non-Goals:**

- 不实现移动端、流式 token、WebSocket、Ollama 专属协议或模型路由策略。
- 不改变数据库 schema；AI 失败时沿用阶段 3 的语义：已保存用户消息、不会写入助手消息。
- 不在阶段 4/5 实现 Redis 幂等重放、会话锁、限流或 gRPC 自动重试。

## Decisions

### 1. HY3 采用通用 OpenAI-compatible Provider

Python 使用标准库 `urllib` 在线程中执行阻塞的 HTTP 请求，避免为一次 API 适配引入 OpenAI SDK 或 HTTP 客户端依赖。Provider 向 `{base_url}/chat/completions` 发送 Bearer token、模型名和文本 messages，并解析 `choices[0].message.content` 与 `usage`。

选择通用实现而非 HY3 专有 SDK，因为现有资料表明 Hunyuan 提供 OpenAI-compatible 接口，且它便于以同一安全边界支持兼容服务。URL 是否已包含 `/v1` 会被规范化处理，但密钥不会被记录。若未来 HY3 需要兼容协议以外的功能，再单独增加专属 Provider。

### 2. Provider 返回显式结果，错误按可恢复性分类

`ChatProvider.complete` 返回包含助手消息、实际模型名称和 token usage 的 `ProviderResult`。`ProviderError` 持有分类：配置错误、不可用、超时或 Provider 响应错误。`ChatService` 映射为 `FAILED_PRECONDITION`、`UNAVAILABLE`、`DEADLINE_EXCEEDED` 和 `INTERNAL`，并只给调用方稳定、非敏感的错误详情。

选择异常分类而非让 Go 解析错误文本，避免 HTTP Provider 的细节穿透服务边界；也不返回上游响应正文，防止泄露服务信息或密钥。

### 3. Profile 仅验证服务端允许值，模型选择仍在 Python

`MODEL_PROFILE` 是唯一允许的 profile 名称，收到其他 profile 的 Python 服务返回 `INVALID_ARGUMENT`。`AI_MODEL` 指定实际 HY3 模型；`AI_PROVIDER_TIMEOUT` 指定 Python 到 Provider 的单次调用上限。Go 只传 `model_profile`，不读取任何 Provider 或模型配置。

单 profile 配置与当前数据库和 Go 校验一致；多 profile 路由推迟到后续模型路由阶段。

### 4. Go 使用 gRPC Completer 适配现有聊天端口

新增 `chat.GRPCCompleter`，将 `CompletionRequest` 的上下文消息映射为 proto `ChatMessage/TextPart`，并调用共享的 `grpcclient.Client`。HTTP middleware 将 `request_id` 放入标准 `context.Context`，适配器将其填入 `RequestContext`；客户端为每个 RPC 创建 timeout context。适配器校验返回内容为 completed assistant 文本，并将 usage 映射回领域结果。

选择适配器而非让 `chat.Service` 直接依赖 protobuf，可保持 HTTP 和领域用例与传输实现解耦，也使单元测试继续使用内存替身。

### 5. 错误映射和就绪性随生产切换同步更新

Go 适配器将 `Unavailable`、`FailedPrecondition`、`Internal` 映射为 AI 不可用或 Provider 错误，将 `DeadlineExceeded` 映射为超时；HTTP Handler 分别返回 502 `AI_UNAVAILABLE` / `AI_PROVIDER_ERROR` 与 504 `AI_TIMEOUT`。生产组合根改为注入 gRPC Completer，并设置 `RequirePythonGRPC=true`。

不进行无条件重试。此时 MySQL 已可能持久化用户消息，阶段 6 的幂等与会话锁才负责重试与并发治理。

## Risks / Trade-offs

- [HY3 的 endpoint 路径或请求字段与通用兼容协议不一致] → 使用真实 smoke 请求验证；仅将路径规范化逻辑集中在 Provider，便于修正。
- [网络抖动导致调用结果未知] → 不自动重试；保留用户消息，后续由阶段 6 的可重放幂等语义解决。
- [Provider 返回非文本或缺少 usage] → 返回稳定 Provider 错误；usage 缺失时按零值返回，不阻断有效文本回复。
- [HTTP 服务写超时早于 gRPC] → 默认 Provider timeout 小于 Go HTTP 写超时，并在部署配置中保持该关系。
- [真实配置误提交或被记录] → `.env` 保持忽略；测试仅断言配置是否存在且不输出值；日志不输出 URL、Authorization 或上游 body。

## Migration Plan

1. 补齐 Python Provider、配置、错误映射和测试；用 Fake 配置完成离线回归。
2. 实现 Go gRPC Completer、request_id 透传、错误映射和测试。
3. 将本地私有 `.env` 的 `AI_PROVIDER` 设为 `hy3`，启动 Python 与 Go。
4. 使用 REST 创建会话并发送一条无敏感内容的真实请求，确认模型名称、消息持久化和双端 request_id 日志。
5. 回滚时将 `AI_PROVIDER` 改回 `fake`；若需要完全解除 Python 依赖，恢复组合根的 `FakeCompleter` 和 `RequirePythonGRPC=false`。

## Open Questions

- 无。HY3 以现有已配置的 OpenAI-compatible URL、Key 和模型名为准；若真实 smoke 返回兼容性错误，再以该错误的非敏感字段调整 Provider 请求形状。
