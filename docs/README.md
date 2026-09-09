# FlowMind 文档中心

FlowMind MVP 已于 2026-09-09 完成。当前文档以“如何理解、运行、验证和继续演进项目”为主，不再保留已经过时的逐阶段临时说明。

## 文档导航

- [项目首页](../README.md)：能力概览、技术栈、快速开始和 API 入口。
- [MVP 执行计划](./roadmap/MVP_EXECUTION_PLAN.md)：阶段 0–12 的实施记录、验收结论、明确延期项和后续版本顺序。
- [服务端说明](../services/README.md)：Go/Python 边界、本机与 Docker Compose 启动、严格就绪验证、指标与可靠性策略。
- [移动端说明](../mobile/README.md)：会话体验、Repository 边界、Android/iOS 开发地址及测试命令。
- [OpenSpec 规格](../openspec/specs/)：当前能力的规范化行为定义。
- [OpenSpec 归档](../openspec/changes/archive/)：MVP 各项变更的提案、设计、任务与验证记录。

## 推荐阅读顺序

```text
README.md
    ↓
docs/roadmap/MVP_EXECUTION_PLAN.md
    ↓
mobile/README.md 或 services/README.md
    ↓
openspec/specs/ 与对应实现代码
```

## 当前架构决策

- 移动端只调用 Go REST API，不感知 Python、gRPC、MySQL 或 Redis。
- Python AI 服务只提供内部 gRPC 和 Provider 适配，不拥有会话业务与数据库。
- MySQL 是会话和消息的事实来源；Redis 只保存短期幂等、锁和限流状态。
- 普通聊天采用同步 HTTP + gRPC；消息队列不属于 MVP 主链路。
- 匿名安装实例通过 `X-Client-ID` 隔离数据，所有请求可通过 `X-Request-ID` 关联日志。
- 本机开发和 Docker Compose 均有配置入口；真实密钥只放私有环境文件。

## 文档维护规则

- 代码与 `openspec/specs/` 是行为事实来源；路线图记录范围、进度和验收结论。
- 新能力优先通过 OpenSpec 变更描述，完成后同步主规格并归档。
- 不再引用已删除的 `docs/phase-*` 文档；运行说明分别维护在 `mobile/README.md` 和 `services/README.md`。
- 流式输出、登录、图片/视觉、Agent、Tool Executor、RAG、消息队列等内容统一视为后续版本，不计入 MVP 完成度。
