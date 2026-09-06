## Why

Go 聊天 API 当前仍使用进程内固定回复完成请求，因此 MVP 尚不能调用已配置的真实模型。需要以 Python 服务拥有 Provider 配置和密钥、Go 经现有内部 gRPC 边界调用的方式，安全接入 HY3。

## What Changes

- 为 Python AI 服务增加兼容 OpenAI Chat Completions 协议的 HY3 Provider，由服务端配置选择，并保留 Fake Provider 供确定性测试使用。
- 校验模型配置和补全参数，应用 Provider 超时，并返回实际模型名称和 token 用量。
- 将 Python Provider 故障分类为可识别的 gRPC 状态。
- 增加 Go gRPC 完成器，透传请求标识，并将 gRPC 故障映射为对外的 AI 不可用、Provider 错误和超时响应。
- 将 Go 生产组合根从进程内 Fake 切换为 gRPC 完成器，并使 Python gRPC 成为就绪检查的必要依赖。

## Capabilities

### New Capabilities

- `hy3-grpc-chat`：通过内部 Python gRPC 服务完成真实模型调用，覆盖 Provider 选择、故障语义和 Go 服务接入。

### Modified Capabilities

- `go-chat-api`：生产消息补全从进程内 Fake 实现切换为 Python gRPC 支撑的真实模型实现，并定义 AI 调用失败响应。

## Impact

- Python AI 的配置、Provider、gRPC 服务、依赖和测试。
- Go gRPC 客户端配置、聊天补全适配器、HTTP 错误映射、组合根和测试。
- 服务环境变量示例与 MVP 执行计划。
