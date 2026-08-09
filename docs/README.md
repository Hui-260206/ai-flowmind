# FlowMind 文档中心

文档以开发阶段作为第一层分类。跨阶段的总体路线图单独放在 `roadmap/`；阶段内的需求、契约、API、实现计划和运维说明放在对应 `phase-*` 目录中。

## 阶段 0：接口与边界设计

- [MVP 需求说明](./phase-0/MVP_REQUIREMENTS.md)：产品范围、系统边界、核心用例和验收标准。
- [阶段 0 契约约定](./phase-0/PHASE_0_CONTRACTS.md)：匿名身份、消息、错误码、Redis Key 和事件边界。
- [OpenAPI](./phase-0/api/openapi.yaml)：Go API 对外 REST/JSON 契约。
- `services/proto/`：Go API 与 Python AI 之间的 gRPC protobuf 契约源文件。

## 阶段 1：服务端基础工程

- [阶段 1 总计划](./phase-1/PHASE_1_PLAN.md)：阶段 1.0–1.13 的拆分、任务和验收标准。
- [阶段 1.0 技术边界](./phase-1/PHASE_1_0_DECISIONS.md)：技术版本、拓扑、端口、配置和健康检查语义。
- [阶段 1.2 Mac 本地数据库环境](./phase-1/PHASE_1_2_OPERATIONS.md)：本机 MySQL/Redis 安装、配置、备份、恢复以及后续 Docker 迁移。

## 跨阶段路线图

- [MVP 执行计划](./roadmap/MVP_EXECUTION_PLAN.md)：从阶段 0 到阶段 12 的总体路线图。

## 推荐阅读顺序

```text
roadmap/MVP_EXECUTION_PLAN.md
        ↓
phase-0/MVP_REQUIREMENTS.md
        ↓
phase-0/PHASE_0_CONTRACTS.md + phase-0/api/openapi.yaml
        ↓
phase-1/PHASE_1_PLAN.md
        ↓
phase-1/PHASE_1_0_DECISIONS.md
        ↓
phase-1/PHASE_1_2_OPERATIONS.md
```

## 当前重要决策

- 开发阶段 MySQL 和 Redis 直接运行在 Mac 本机，不依赖 Docker。
- 后续部署阶段再迁移到云服务器 Docker Compose。
- MVP 普通聊天使用 Go HTTP + Python gRPC 同步链路，不引入消息队列。
- MySQL 是聊天记录事实来源，Redis 只保存后续幂等、锁和限流等临时状态。
