## ADDED Requirements

### Requirement: Session-oriented mobile chat repository
移动端共享层 SHALL 暴露 `ChatRepository` 抽象，提供创建会话、列出当前客户端的会话、获取自有会话的消息、使用调用方提供的 `client_message_id` 同步发送一条消息，以及删除自有会话的操作。该抽象 MUST 将公开的会话和消息数据与仅供 UI 使用的待处理/错误展示分开建模；其远程实现 MUST 仅调用 Go 的 `/api/v1/sessions` REST API。

#### Scenario: Read server-owned session data through the repository
- **WHEN** 移动端代码通过远程仓储列出会话，或获取一个自有会话的消息
- **THEN** 仓储按 API 原有顺序返回相应 Go API 的 `items`，且不直接调用 Python、gRPC、Redis 或 MySQL

#### Scenario: Send a message with server idempotency input
- **WHEN** 移动端代码以非空白的会话 ID、内容和客户端消息 ID 调用 `sendMessage`
- **THEN** 远程仓储向会话消息端点提交 `content` 和 `client_message_id`，并从成功响应中返回已持久化的用户消息和助手消息

#### Scenario: Substitute the mock repository
- **WHEN** 本地 UI 开发或确定性测试选择模拟仓储
- **THEN** 调用方无需真实 HTTP 端点即可使用相同的面向会话仓储接口

### Requirement: Kuikly network transport integration
远程仓储 SHALL 对该运行时支持的所有 HTTP 方法，使用从当前活跃 Kuikly Pager/ComposeContainer 的 `NetworkModule` 获取的传输层。该传输层 MUST 使用 `Content-Type: application/json` 发送 JSON POST 请求体，应用已配置的有限超时，并在解析响应体前检查 NetworkModule 的成功标志。仓储 MUST 不依赖 Ktor 或直接面向特定后端的移动端客户端。

#### Scenario: Complete a successful JSON POST
- **WHEN** NetworkModule 报告创建会话或发送消息的请求成功，且响应包含有效 JSON
- **THEN** 仓储解析所需的响应字段并返回领域 DTO

#### Scenario: NetworkModule reports a failed request
- **WHEN** NetworkModule 报告 `success = false`，无论 HTTP 状态是否可用
- **THEN** 仓储返回结构化的 `ChatException`，而不是将载荷当作成功响应解析

### Requirement: Stable anonymous identity and request correlation
移动端网络组合 SHALL 向远程仓储提供稳定、非空白且作用域为安装实例的匿名客户端 ID，并为每个 API 请求提供新生成的有效请求 ID。远程仓储 MUST 分别通过 `X-Client-ID` 和 `X-Request-ID` 发送它们；只要服务端通过 JSON 信封或响应头提供请求 ID，MUST 在错误中保留该 ID。

#### Scenario: Reopen the application with the same anonymous owner
- **WHEN** 同一安装实例在应用重启前后获取客户端 ID
- **THEN** 身份提供方返回相同的非空白值，且应用可使用 `X-Client-ID` 访问相同的服务端会话

#### Scenario: Correlate a server validation failure
- **WHEN** Go API 针对移动端请求返回 JSON 错误信封
- **THEN** 得到的 `ChatException` 包含 HTTP 状态、API 错误码、安全的错误消息以及返回的请求 ID

### Requirement: Portable endpoint configuration
移动应用 SHALL 从目标/环境配置中获取 API 基础 URL 和超时，而非使用硬编码的回环端点。开发配置 MUST 选择可从 Android 模拟器、iOS 模拟器、OpenHarmony 目标设备或物理设备访问的端点；生产配置 MUST 使用 HTTPS。

#### Scenario: Use an Android emulator development endpoint
- **WHEN** Android 模拟器构建选择其开发配置
- **THEN** 远程仓储使用已配置、宿主机可达的端点，而不是假定设备本地的 `127.0.0.1`

#### Scenario: Use a production endpoint
- **WHEN** 生产配置构造远程仓储
- **THEN** 其配置的基础 URL 使用 `https` 协议

### Requirement: Delete session through Kuikly NetworkModule
移动端仓储 SHALL 保持 Go API 的 `DELETE /api/v1/sessions/{session_id}` 语义。其 Kuikly 传输适配器 MUST 将底层 NetworkModule 的 HTTP 方法设为 `"DELETE"`，发送标准的 `X-Client-ID` 和 `X-Request-ID` 请求头，并接受 `204 No Content`，且不得尝试解析 JSON 响应体。

#### Scenario: Delete an owned session
- **WHEN** 调用方通过远程仓储删除一个自有会话
- **THEN** 仓储发出 `DELETE /api/v1/sessions/{session_id}`，将 `204 No Content` 视为成功，并正常返回

#### Scenario: Delete response is an API error
- **WHEN** Go API 针对删除请求返回非 2xx 错误信封
- **THEN** 仓储使用 API 错误码和请求 ID 将其映射为 `ChatException`，且不发出替代的 POST 请求

### Requirement: Stable mobile error translation
远程仓储 SHALL 将 HTTP 错误信封、传输失败、格式错误的载荷和缺失必填成功字段转换为带有稳定移动端错误码的 `ChatException`。它 MUST 保留可识别的 Go API 错误码，且 MUST NOT 将原始提供方、后端或序列化异常详情作为面向用户的消息暴露。

#### Scenario: Preserve a recognized API error code
- **WHEN** Go 返回错误码为 `AI_TIMEOUT`、`AI_UNAVAILABLE`、`SESSION_BUSY`、`RATE_LIMITED`、`REDIS_UNAVAILABLE`、`SESSION_NOT_FOUND` 或 `INVALID_ARGUMENT` 的非 2xx JSON 错误
- **THEN** 仓储返回包含相同错误码和服务端关联 ID 的 `ChatException`

#### Scenario: Reject a malformed success response
- **WHEN** 请求报告成功，但其 JSON 缺失所请求会话或消息结果的必填字段
- **THEN** 仓储返回稳定的“响应格式错误” `ChatException`，且不构造不完整的领域对象
