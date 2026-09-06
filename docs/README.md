# FlowMind 文档中心

服务端的开发进度、阶段设计、关键决策与验收结果统一记录在跨阶段路线图 `roadmap/` 中。曾按阶段拆分的 `phase-*` 设计文档已完成使命并被移除；当前实现的权威来源是代码（`services/`）、路线图与需求契约。

## 路线图

- [MVP 执行计划](./roadmap/MVP_EXECUTION_PLAN.md)：从阶段 0 到阶段 12 的总体路线图，含各阶段目标、任务、验收标准、关键设计决策与进度标注。

## 推荐阅读顺序

```text
roadmap/MVP_EXECUTION_PLAN.md
        ↓
services/（代码即真相：go-api、ai-service、proto）
```

## 当前重要决策

- 开发阶段 MySQL 和 Redis 直接运行在 Mac 本机，不依赖 Docker。
- 后续部署阶段再迁移到云服务器 Docker Compose。
- MVP 普通聊天使用 Go HTTP + Python gRPC 同步链路，不引入消息队列。
- MySQL 是聊天记录事实来源，Redis 只保存后续幂等、锁和限流等临时状态。
