## 背景

移动应用目前只有单页、基于内存的模拟聊天流程，其仓储面向已经废弃的 FastAPI 风格契约。Go 聊天 API 现已可供移动端使用，因此应用需要稳定的 Kuikly 仓储边界来访问真实的会话和消息端点，同时不向 UI 代码暴露 Python、gRPC、Redis 或 MySQL。

## 变更内容

- 将基于历史记录的 `sendChat(history)` 移动端仓储契约替换为与 Go REST API 一致的面向会话的创建、列表、历史记录、发送和删除操作。
- 新增由 Kuikly `NetworkModule` 驱动的远程仓储，支持 JSON DTO 映射、`X-Client-ID` 和 `X-Request-ID` 请求头、请求超时处理及结构化的移动端领域错误。
- 新增持久化的匿名客户端身份边界，使同一安装实例在应用重启后仍保有其服务端会话的所有权。
- 为 Android、iOS 和 OpenHarmony 的开发及生产环境定义按平台区分、可配置的 API 基础 URL。
- 通过 Kuikly `NetworkModule` 保持并实现现有服务端 `DELETE` 契约：将底层 HTTP 方法设置为 `"DELETE"`。
- 保持模拟仓储可切换，以支持 UI 开发和确定性测试。

## 能力

### 新增能力

- `kuikly-chat-network`：面向会话的移动端 Go 聊天 API 仓储访问，包括身份标识、请求关联、错误转换和平台端点配置。

### 修改的能力

- 无。

## 影响

- 影响 `mobile/shared` 的数据模型、仓储实现和提供方装配；除将调用方适配到新仓储接口外，第 10 阶段的状态/UI 工作仍不在本次范围内。
- 使用 Kuikly `NetworkModule` 及其 JSON 类型；不引入 Ktor，也不实现移动端直连 Python 的集成。
- 需要为持久化客户端 ID 和基础 URL 完成平台存储及应用配置装配，并验证 Android/iOS/OpenHarmony 开发网络。
- 不改变现有 Go REST API，也不改变其所有权、幂等性和错误信封保障。
