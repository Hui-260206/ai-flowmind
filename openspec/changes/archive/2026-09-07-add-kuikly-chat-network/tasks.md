## 1. 契约和共享模型

- [x] 1.1 使用与 Go 公开契约匹配的会话、消息、发送结果、API 错误及结构化 `ChatException` 模型，替换废弃的基于历史记录的 `ChatRepository` API 和 FastAPI DTO。
- [x] 1.2 将 `MockChatRepository` 适配为面向会话的接口，为 UI 开发和测试提供确定性的创建/列表/历史/发送/删除行为。
- [x] 1.3 更新仓储提供方和最小化调用方构造，使项目可使用新契约编译，同时不新增第 10 阶段的会话状态或 UI 行为。

## 2. Kuikly 传输层和远程仓储

- [x] 2.1 创建窄化的、支持 suspend 的聊天 HTTP 传输接口，以及从当前活跃 Pager/ComposeContainer 获取的 `NetworkModule` 适配器；覆盖 GET、JSON POST、通过显式 `"DELETE"` 方法执行的 `DELETE`、自定义请求头、配置的超时和回调失败处理。
- [x] 2.2 针对 `/api/v1/sessions` 实现 `RemoteChatRepository`，支持会话创建/列表、消息历史和同步消息发送；以防御式方式解析 Go API DTO 的必填字段。
- [x] 2.3 实现集中式错误信封解析，并将 HTTP、NetworkModule、格式错误的响应体和缺失字段失败转换为带有请求关联的稳定 `ChatException` 值。
- [x] 2.4 使用 NetworkModule 的显式 `"DELETE"` 方法、标准身份/关联请求头和 `204 No Content` 成功处理，实现 `deleteSession`。
- [x] 2.5 使用伪传输层新增仓储/传输层测试，覆盖成功、Go API 错误、状态缺失、格式错误 JSON、必填字段验证以及 DELETE/204 行为。

## 3. 身份和端点配置

- [x] 3.1 定义 `ClientIdentity`、请求 ID 生成和 `ChatApiConfig` 边界；验证非空白客户端身份、新生成的请求 ID、有限超时、规范化的基础 URL 和 HTTPS 生产配置。
- [x] 3.2 通过共享抽象为 Android、iOS 和 OpenHarmony 实现安装实例作用域的客户端 ID 持久化，并测试重复获取时身份不会重置。
- [x] 3.3 为 Android 模拟器、iOS 模拟器、OpenHarmony、物理设备开发和生产环境新增显式的平台/环境 API 基础 URL 装配；移除硬编码回环默认值。
- [x] 3.4 配置并记录范围严格限定的本地开发网络要求，包括 iOS ATS 行为和生产 HTTPS 预期，同时不扩大生产环境的明文流量访问。

## 4. 集成验证

- [x] 4.1 运行 Go Fake-Provider 就绪环境，验证远程仓储路径可创建会话、携带两个身份请求头发送消息、获取有序历史记录，并在错误中保留服务端请求 ID。（2026-09-07：`/readyz` 返回 MySQL、Redis、Python gRPC 全部 ready；以 UUID `X-Client-ID` 和独立 `X-Request-ID` 实测 `201` 创建、`200` 发送、按 `user,assistant` 顺序读取历史；无效客户端 ID 返回 `400 INVALID_ARGUMENT`，JSON 信封和响应头均保留请求 ID。）
- [x] 4.2 使用选定的开发端点构建并冒烟测试 Android 和 iOS HTTP 请求；在 OpenHarmony 工具链可用时执行等效验证。（2026-09-07：Android 无线真机和 iOS 真机均已完成会话创建、消息发送及模型回复 smoke；OpenHarmony 工具链未在当前环境提供，未执行。）
- [x] 4.3 更新 `docs/roadmap/MVP_EXECUTION_PLAN.md` 的第 9 阶段清单和移动端网络文档，记录已完成能力、端点配置、DELETE 支持和剩余的第 10 阶段工作。
