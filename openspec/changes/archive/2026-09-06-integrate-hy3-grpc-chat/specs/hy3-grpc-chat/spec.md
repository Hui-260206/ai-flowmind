## ADDED Requirements

### Requirement: HY3 真实模型补全
Python AI 服务 SHALL 在 `AI_PROVIDER=hy3` 时，使用仅由 Python 环境读取的 OpenAI-compatible 服务地址、API Key 和模型名调用 HY3 Chat Completions 服务。服务 MUST 不在日志、gRPC 响应或 Go 配置中暴露 API Key，且 MUST 返回助手文本、实际模型名称与可用的 token usage。

#### Scenario: 通过 HY3 获得补全结果
- **WHEN** Python `ChatService.Complete` 收到包含有效文本上下文和受支持 profile 的请求，且 HY3 配置完整并可访问
- **THEN** 服务调用 HY3 并返回一条 `completed` assistant 文本消息、模型名称和 token usage

#### Scenario: 配置缺失时拒绝启动或调用
- **WHEN** `AI_PROVIDER=hy3` 但服务地址、API Key 或模型名为空
- **THEN** Python 服务 MUST 返回明确的非敏感配置错误，且不得尝试向 Provider 发出请求

### Requirement: Provider 超时与 gRPC 故障分类
Python AI 服务 SHALL 对每次真实 Provider 调用施加配置的正数超时，并将配置错误、Provider 不可用、Provider 超时和其他 Provider 响应失败映射为稳定且可识别的 gRPC status。服务 MUST 保持调用的 `request_id` 在请求日志中可关联。

#### Scenario: Provider 调用超时
- **WHEN** HY3 在 Provider timeout 内未返回响应
- **THEN** `ChatService.Complete` 返回 `DEADLINE_EXCEEDED`，且日志保留该请求的 request_id

#### Scenario: Provider 服务不可用
- **WHEN** HY3 连接失败或返回可识别的临时不可用响应
- **THEN** `ChatService.Complete` 返回 `UNAVAILABLE`，且不暴露上游响应正文或密钥

### Requirement: Go 通过 gRPC 使用真实补全
Go 聊天应用的生产组合根 SHALL 使用 gRPC 完成器调用 Python `ChatService.Complete`，而不是进程内 Fake 完成器。该完成器 MUST 透传 HTTP 请求的 request_id、为 RPC 应用超时、支持调用方取消，并将 Python 的模型名称和 token usage 保存至助手消息。

#### Scenario: REST 请求得到真实模型回复
- **WHEN** 调用方对有效会话发送新消息，Python AI 服务和 HY3 均可用
- **THEN** Go 保存用户消息和来自 gRPC 的 completed assistant 消息，返回 `200`，并使 Python 日志与 HTTP 响应使用同一个 request_id

#### Scenario: 调用方取消请求
- **WHEN** HTTP 调用方在 Python 返回前取消请求
- **THEN** Go MUST 取消对应 gRPC 请求，且不得在取消后继续等待模型响应

### Requirement: 对外 AI 故障响应
Go REST API SHALL 将 Python gRPC 的 AI 不可用或 Provider 错误映射为 HTTP 502，将超时映射为 HTTP 504，并继续使用标准 `{request_id, error}` 错误包络。生产 `/readyz` MUST 将 Python gRPC 作为必要依赖。

#### Scenario: Python 服务不可用
- **WHEN** Python AI 服务停止或 gRPC 连接不可用
- **THEN** 发送消息接口返回 `502 AI_UNAVAILABLE`，且 `/readyz` 返回非就绪状态并报告 `python_grpc` 失败

#### Scenario: Python 调用超时
- **WHEN** Python `ChatService.Complete` 返回 `DEADLINE_EXCEEDED`
- **THEN** 发送消息接口返回 `504 AI_TIMEOUT`，并带有当前请求的 request_id
