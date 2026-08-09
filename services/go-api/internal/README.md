# Go API 内部包

各子目录按 `services/README.md` 的职责划分创建。Go API 使用 GORM 连接 MySQL；`mysql` 包只负责连接池、Ping 和关闭，不在阶段 1 创建业务表或 Repository。`health` 包负责存活与就绪探针，HTTP server 只负责路由和服务生命周期。
