# Go API 内部包

各子目录按 `services/README.md` 的职责划分创建。Go API 使用 GORM 连接 MySQL；`mysql` 包只负责连接池、Ping、关闭和暴露 `*sql.DB`（供迁移使用）。

- `model` 包是领域模型与枚举常量的边界，也是 `owner_key` 派生的唯一实现点（`model.OwnerKey`）。
- `repository` 包定义 Session/Message/Outbox 的数据访问接口并用 GORM 实现；所有会话查询都必须带 `owner_key` 条件，`AppendMessage` 在单个事务内「锁会话行 → 算 seq → 写入」。
- `chat` 包是聊天应用层：协调会话/消息 Repository、上下文裁剪和可替换的 `Completer`。阶段 3 使用进程内 Fake 完成器，阶段 5 将提供 gRPC Adapter；HTTP handler 不直接编排数据库或模型调用。
- `migrate` 包用 `//go:embed` 内嵌 `migrate/migrations/*.sql`，在启动时用 golang-migrate 按版本执行；`schema_migrations` 表保证每个版本只跑一次。

`health` 包负责存活与就绪探针，HTTP server 只负责路由和服务生命周期。

`redis` 包使用 `go-redis/v9` 管理 Redis 客户端，负责配置、启动 Ping、就绪检查和关闭连接池；幂等、会话锁和限流属于后续阶段（阶段 6）。
