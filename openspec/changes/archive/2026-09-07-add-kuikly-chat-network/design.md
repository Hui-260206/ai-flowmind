## 上下文

Go API 已在 `/api/v1/sessions` 下暴露匿名客户端会话生命周期和同步消息端点。其契约要求 `X-Client-ID`，接受或生成 `X-Request-ID`，并返回标准 JSON 错误信封。移动应用仍使用基于历史记录的 `ChatRepository.sendChat(history)` 契约和模拟实现；其远程骨架面向已移除的 FastAPI 端点及 Ktor。

Kuikly 的跨平台 `NetworkModule` 从 `Pager`/`ComposeContainer` 获取，而非从进程级全局 HTTP 客户端获取。尽管其高级示例主要聚焦于 GET 和 POST，底层请求 API 接受诸如 `"DELETE"` 的显式 HTTP 方法；Go API 特意使用 DELETE 来移除会话。Android 已具备网络权限和临时明文流量许可；iOS 与 OpenHarmony 的网络配置需要根据环境审慎处理。

## 目标 / 非目标

**目标：**

- 定义与 Go 会话/消息 REST 契约对齐的移动端仓储契约。
- 使 UI 和视图模型代码独立于 HTTP 客户端、后端拓扑和 JSON 信封。
- 对受支持的请求使用 `NetworkModule`，保留可切换的模拟仓储，并将传输/API 失败映射为结构化 `ChatException`。
- 提供稳定、作用域为安装实例的匿名客户端 ID，以及每个请求的关联 ID。
- 按平台和环境显式配置端点。

**非目标：**

- 实现会话列表 UI、初始会话选择、重试 UX 或完整历史状态管理；这些属于第 10 阶段的工作。
- 新增身份验证、流式响应、WebSocket、离线同步或直接调用 Python AI 服务。
- 修改 Go REST 端点、其幂等性保证或 DELETE 语义。
- 为支持本地 HTTP 开发而全局降低生产移动端传输安全性。

## 决策

### 1. 使用会话操作替换基于历史记录的仓储

`ChatRepository` 将暴露 `createSession`、`listSessions`、`getMessages`、`sendMessage(sessionId, content, clientMessageId)` 和 `deleteSession`。领域 DTO 将映射公开的 Go 会话/消息字段；仅供 UI 使用的待处理/错误展示仍为本地状态，不与持久化 API 消息状态混淆。

这使 Go 服务端成为历史记录的唯一事实来源，并让第 10 阶段无需重新设计 API 即可恢复、切换和删除会话。继续发送完整的客户端历史记录会重复持久化权威，且无法正确表达会话所有权或幂等性。

### 2. 从页面组合根注入由 NetworkModule 驱动的传输层

页面/Pager 将获取 `NetworkModule`，并构造或向视图模型提供远程仓储。仓储接收一个窄化的传输层抽象，而不是自行定位全局 Pager。其 `suspend` API 将桥接 NetworkModule 回调，并始终在解析响应数据前检查 `success`。

这保留了可测试性，并支持模拟传输层/仓储。拒绝在独立提供方中获取 NetworkModule 或使用 Ktor：前者缺少有效的 Kuikly 生命周期所有者，后者会引入平台 HTTP 引擎，尽管路线图已选定 NetworkModule。

### 3. 将客户端身份、请求身份和端点配置视为注入依赖

`ClientIdentity` 返回一个非空白、作用域为安装实例的 ID，并使用适合平台的存储方式持久化。`RequestIDGenerator` 为每次 API 调用提供新生成的有效 ID。`ChatApiConfig` 提供基础 URL 和请求超时；它由平台/环境装配选择，而非嵌入仓储源代码。

远程仓储为每个请求设置 `X-Client-ID` 和 `X-Request-ID`，为 JSON POST 设置 JSON 内容类型，并从 JSON 错误信封或 `X-Request-ID` 响应头确定最终请求 ID。每个请求生成客户端 ID，或将 `127.0.0.1` 保留为源码默认值，都会破坏所有权和物理设备连接能力。

### 4. 使用 NetworkModule 的显式 DELETE 方法

仓储契约包含 `deleteSession`，因为 Go API 要求 HTTP DELETE。Kuikly 传输适配器将底层 NetworkModule HTTP 方法设置为 `"DELETE"`，从而发出 `DELETE /api/v1/sessions/{session_id}`；它会包含标准身份/关联请求头，并将 `204 No Content` 视为成功。

拒绝将服务端端点改为 POST，因为这会弱化既有 REST 契约并造成移动端特有的服务端分歧。该操作不需要原生 HTTP 桥接。

### 5. 将所有非成功响应转换为统一的移动端错误形态

`ChatException` 在已知时保留 HTTP 状态、稳定的 API 错误码、安全消息和请求 ID。对于 JSON API 信封，它读取 `{request_id, error: {code, message}}`；对于传输失败、格式错误的成功响应体、缺失必填字段或 HTTP 状态缺失，它返回稳定的本地错误码，而非泄漏原始异常。

这让第 10 阶段可以根据 `AI_TIMEOUT`、`AI_UNAVAILABLE`、`SESSION_BUSY` 和 `RATE_LIMITED` 等稳定类别决定重试行为。拒绝将原始 NetworkModule 失败字符串直接返回给 UI，因为它们不可移植，也无法指导用户操作。

## 风险 / 权衡

- [DELETE 请求使用空的 `204` 响应] → 让传输层和仓储将 204 视为成功而不尝试 JSON 响应体解析，并通过测试覆盖。
- [重新生成的客户端 ID 会隐藏之前的服务端历史] → 持久化并验证安装实例身份，测试覆盖重复读取。
- [Localhost 在模拟器、仿真器和设备之间不同] → 按目标配置端点；绝不依赖共享的 `127.0.0.1` 默认值。
- [本地 HTTP 在生产环境中被阻止或不安全] → 仅允许范围严格限定的开发例外；生产配置 MUST 使用 HTTPS。
- [NetworkModule 回调行为可能产生格式错误/无状态的响应] → 集中处理回调到 suspend 的转换及错误解析，并使用传输层伪实现覆盖这些路径。
- [新仓储契约迫使调用方更新] → 本次变更只适配编译所需的依赖装配；会话状态行为留待第 10 阶段。

## 迁移计划

1. 与废弃的历史 DTO 并行或替代性地新增共享 API DTO、错误和新仓储接口。
2. 在同一接口后实现模拟仓储和远程仓储，并通过提供方/组合装配按配置选择模拟或远程实现。
3. 新增身份/配置适配器，并在 Go Fake-Provider 就绪环境中验证远程路径。
4. 仅在明确选择的开发配置中启用远程仓储；保留回退到模拟仓储的开关。
5. 在生产构建前提供 HTTPS 基础 URL 和平台传输配置；移除或限制开发明文流量例外。

回退只需选择 `MockChatRepository`；不需要服务端迁移或移动端持久化数据迁移。安装实例客户端 ID 为不透明值，可在回退后保留。

## 待确认问题

- Android、iOS 和 OpenHarmony 的客户端 ID 持久化目前可用或推荐使用哪种共享存储抽象？此处已确定接口；具体跨平台机制需要在实施期间结合仓储/版本进行验证。
- 每个目标允许使用哪些开发和生产 API 主机，包括物理设备测试网络及 TLS 证书？
