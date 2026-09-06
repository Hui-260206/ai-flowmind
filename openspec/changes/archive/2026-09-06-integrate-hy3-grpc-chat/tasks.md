## 1. Python HY3 Provider

- [x] 1.1 扩展 Python 配置，校验 HY3 Provider、模型名和调用超时，并保持敏感值不进入日志。
- [x] 1.2 定义 Provider 结果与分类错误，实现 OpenAI-compatible HY3 Chat Completions Provider。
- [x] 1.3 更新 `ChatService`，校验请求、映射 Provider 结果与错误为 gRPC 状态，并补齐单元测试。

## 2. Go gRPC 聊天适配

- [x] 2.1 扩展 Go gRPC 客户端配置，增加 RPC timeout、keepalive 和消息大小保护。
- [x] 2.2 实现 gRPC `Completer`，完成消息/usage 映射、request_id 透传、取消和 gRPC 错误分类。
- [x] 2.3 更新 HTTP 错误映射与生产组合根，启用 Python gRPC 就绪依赖。

## 3. 验证与文档

- [x] 3.1 补齐 Python 与 Go 单元测试，运行静态检查和测试套件。
- [x] 3.2 更新环境变量示例、服务文档和 MVP 阶段四/五进度。
- [x] 3.3 使用私有 `.env` 启动双服务，通过 REST 完成一次真实 HY3 模型请求并验证持久化与 request_id 链路。
