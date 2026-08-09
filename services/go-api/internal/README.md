# Go API 内部包

各子目录按 `services/README.md` 的职责划分创建。Go API 使用 GORM 连接 MySQL；`mysql` 包只负责连接池、Ping 和关闭，不在阶段 1 创建业务表或 Repository。`health` 包负责存活与就绪探针，HTTP server 只负责路由和服务生命周期。

`redis` 包使用 `go-redis/v9` 管理 Redis 客户端，负责配置、启动 Ping、就绪检查和关闭连接池；幂等、会话锁和限流不属于阶段 1.7。
