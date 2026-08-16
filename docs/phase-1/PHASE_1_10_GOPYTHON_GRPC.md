# 阶段 1.10：生成 protobuf 并完成 Go–Python gRPC 联通

阶段 1.10 让 Go 使用与 Python 同一份 proto 调用 AI 服务，并把 gRPC 可用性
接入 `/readyz`。

## 1. 目标与边界

- Go 与 Python 使用同一份 proto 源文件（`services/proto/ai/v1/*.proto`）。
- Go 生成 `pb.go` / `grpc.pb.go`，创建 gRPC Client 并管理连接生命周期。
- gRPC 可用性接入 `/readyz`（`python_grpc` 检查项）。
- 用 Fake Provider 完成一次最小调用，验证跨服务链路。

## 2. 关键决策：go_package 与模块路径

原 proto `go_package` 为 `github.com/flowmind/ai-flowmind/services/proto/ai/v1`，
与 go-api 的 module `ai-flowmind/services/go-api` 不一致，Go 无法直接 import。

本阶段把 5 个 proto 的 `go_package` 统一改为
`ai-flowmind/services/go-api/internal/grpcclient/pb`（package `aiv1`），生成代码
收敛到 go-api 内部、只被 go-api 使用，避免 `replace` 指令。Python 侧不受影响
（Python 只用 proto `package flowmind.ai.v1`）。

## 3. 代码生成工具版本

| 工具 | 版本 | 用途 |
|---|---|---|
| buf | 1.72.0 | Go 侧生成入口（lint + generate） |
| protoc-gen-go | v1.36.9 | 生成 Go message 代码 |
| protoc-gen-go-grpc | v1.5.1 | 生成 Go gRPC 代码 |
| grpcio-tools（grpc_tools.protoc） | 1.71.2 | Python 侧生成（阶段 1.9） |

Go 运行时依赖：`google.golang.org/grpc v1.83.0`、`google.golang.org/protobuf v1.36.11`。

```sh
# Go（需 $HOME/go/bin 上的 protoc-gen-go / protoc-gen-go-grpc）
make proto-generate            # → go-api/internal/grpcclient/pb

# Python
make proto-generate-python     # → ai-service/gen/python
```

生成代码的入库策略：Python 生成物（`ai-service/gen/python`）不入库，由
`make proto-generate-python` 重新生成；Go 生成物（`go-api/internal/grpcclient/pb`）
随源码入库（Go 惯例，模块自洽、可直接 `go build`），`make proto-generate` 可重新生成。

## 4. gRPC Client 生命周期

`go-api/internal/grpcclient/client.go`：

- `Open(cfg)`：懒连接（`grpc.NewClient`），不主动拨号；首次 RPC/就绪检查时建立连接。
- `Check(ctx)`：连接就绪检查，用 `AI_GRPC_TIMEOUT` 限制等待时长，避免 `/readyz` 无限阻塞。
- `Complete(ctx, req)`：最小 Chat 调用。
- `Close()`：关闭连接。

`main.go` 在启动时 `Open`、退出时 `Close`，并把 `Check` 注入 `httpserver.Dependencies.PythonGRPC`。

## 5. /readyz 集成

`/readyz` 的 `python_grpc` 检查项：

- Python 启动：`python_grpc: ok`，`status: ready`。
- Python 停止：`python_grpc: failed`，`status: not_ready`（`TRANSIENT_FAILURE`）。

## 6. 运行与验证

```sh
cd services
make proto-generate            # 首次生成 Go stub
make test-go                   # go test ./...（含 grpcclient 单测）
make dev-ai                    # 终端 A：启动 Python AI
make dev-go                    # 终端 B：启动 Go API
curl http://127.0.0.1:8080/readyz
```

端到端验证结果：

- `grpcdial`（临时 smoke 程序）对 Python Fake Provider 调用 `Complete`，返回
  `model_name=fake` 与固定回复。
- Python 停止后 `Check` 返回 `ai grpc connection is TRANSIENT_FAILURE`，`/readyz`
  的 `python_grpc` 转为 `failed`。

## 7. 阶段 1.10 验收清单

- [x] Go 和 Python 使用同一份 proto。
- [x] Go 可以调用 Fake Provider。
- [x] Python 停止时 Go 能识别依赖不可用。
- [x] 生成代码与 gRPC Client 生命周期清晰。
