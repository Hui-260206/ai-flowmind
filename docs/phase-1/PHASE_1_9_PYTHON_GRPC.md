# 阶段 1.9：创建 Python gRPC 服务和 Fake Provider

阶段 1.9 建立内部 AI 服务边界：`ChatProvider` 抽象 + `FakeProvider` 实现 +
`ChatService` gRPC 服务。不依赖真实模型，用固定回复完成最小联通。

## 1. 目标与边界

- 使用阶段 0 的 proto（`services/proto/ai/v1/*.proto`）生成 Python stub。
- 只实现最小 `Chat` 方法 `Complete`；`CompleteStream` 返回 `UNIMPLEMENTED`，留待后续。
- Provider 只负责「输入消息 → 输出助手消息」，不感知 gRPC / 会话 / 数据库。
- 错误以异常向上抛出，由 `ChatService` 统一记录并转换为 gRPC 状态码。

## 2. 代码生成

Python stub 用 `grpcio-tools` 生成（`python -m grpc_tools.protoc`，它内置
`protoc` 与 `grpc_python_plugin`）。buf 无法复用该插件二进制，因此 Python 侧
不走 `buf generate`，见 `services/buf.gen.yaml` 注释与 `make proto-generate-python`。

```sh
make proto-generate-python   # 生成到 services/ai-service/gen/python
```

生成包名为 `ai.v1`（源路径相对）。这与 proto 包名 `flowmind.ai.v1` 不一致，
源于 `ai/v1` 目录与包名的历史错位（buf.yaml 的 `PACKAGE_DIRECTORY_MATCH` 例外），
留待专用 protobuf 维护变更统一。`gen/` 已加入 `.gitignore`，首次运行前必须先生成。

## 3. 目录结构

```text
services/ai-service/
├── app/
│   ├── main.py              # 入口：bootstrap gen 路径 → 配置 → 日志 → serve
│   ├── chat_service.py      # ChatService gRPC servicer
│   └── grpc_server.py       # 注册 ChatService + 生命周期
├── providers/
│   ├── base.py              # ChatProvider 抽象 + ProviderError
│   ├── fake.py              # FakeProvider（固定回复）
│   └── __init__.py          # build_provider(name) 工厂
├── gen/python/              # 生成代码（不入库）
└── tests/                   # test_provider.py / test_chat_service.py
```

## 4. Provider 抽象

```python
class ChatProvider(ABC):
    name: str = ""

    @abstractmethod
    async def complete(self, context, messages, max_output_tokens, temperature) -> ChatMessage:
        ...
```

`FakeProvider` 返回固定文本回复；`build_provider("fake")` 构造实例，未知名称抛
`ProviderError`。真实 Provider 接入时只需新增实现并扩展工厂。

## 5. ChatService 与错误边界

`Complete` 流程：提取 `request_id` → 写入 contextvar → 调用 Provider → 记录耗时 →
组装 `CompleteResponse`。Provider 抛异常时 `logger.exception` 记录（含 request_id）
并 `context.abort(INTERNAL, "AI provider error")` 转换，不向调用方泄露内部细节。

## 6. 运行与验证

```sh
cd services
make proto-generate-python   # 首次
make lint-ai                 # ruff
make test-ai                 # pytest（12 例）
make dev-ai                  # 启动，Ctrl+C 优雅退出
```

联通验证：客户端 `ChatServiceStub.Complete` 返回 `model_name=fake` 与固定回复，
日志中 request_id 正确透传。

## 7. 阶段 1.9 验收清单

- [x] Python gRPC 服务可以启动（`127.0.0.1:50051`）。
- [x] Fake Provider 返回固定回复。
- [x] gRPC 异常被记录并转换为 `INTERNAL`。
- [x] Provider 抽象与错误边界清晰。
