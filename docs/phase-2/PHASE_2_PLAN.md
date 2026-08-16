# 阶段 2 计划：数据库和领域模型

> 本文把 `roadmap/MVP_EXECUTION_PLAN.md` 第 6 节「阶段 2：数据库和领域模型」拆成可执行的实现计划。
> 数据契约来源：`docs/phase-0/MVP_REQUIREMENTS.md` 第 9 节、`docs/phase-0/PHASE_0_CONTRACTS.md`。

## 1. 目标与范围

### 目标

建立聊天数据的持久化模型，MySQL 作为事实来源。具体产出：

1. 3 张表的可重复执行 migration（`chat_sessions`、`chat_messages`、`outbox_events`）；
2. Go 领域模型（`model` 包）与 Repository（`repository` 包）；
3. 会话归属校验与稳定的消息顺序（`seq`）。

### 本阶段不做什么（边界）

- 不写任何 HTTP 业务接口（创建会话、发送消息等属于**阶段 3**）；
- 不碰 Redis 幂等/锁/限流（属于**阶段 6**）；
- 不改 Python AI 服务；
- 不接 RabbitMQ / Outbox 消费者（表先建好，逻辑属于**阶段 7**）；
- 不改移动端。

## 2. 已确认决策

| # | 决策 | 选择 | 一句话理由 |
|---|---|---|---|
| 1 | Repository 实现方式 | **GORM** | 已定；`mysql.Client` 已返回 `*gorm.DB` |
| 2 | 迁移工具 | **golang-migrate**（`migrate/v4`） | 版本化、可 up/down、可回滚、可重复执行 |
| 3 | `seq` 生成 | **B（锁行再算）+ C（唯一索引兜底）** | 锁保证正常流程串行；唯一索引是最后防线 |
| 4 | `owner_key` 派生 | **收口成一个函数** | 未来 `client:` → `user:` 迁移只改一处 |
| 5 | 幂等粒度 | **DB 管会话内 / Redis 管设备级，各管各的** | 两层范围不同是有意的，不是 bug |
| 6 | `ai_runs` | **先不建表** | 字段未定义，避免建空表再迁移 |

## 3. 目录结构（新增 / 修改）

```text
services/
├── go-api/
│   ├── cmd/api/main.go                # 改：启动早期执行迁移
│   ├── go.mod / go.sum                # 改：新增 golang-migrate 依赖
│   └── internal/
│       ├── model/                     # 新：领域模型 + 枚举常量
│       │   ├── owner.go
│       │   ├── session.go
│       │   ├── message.go
│       │   └── outbox.go
│       ├── repository/                # 新：Repository 接口 + GORM 实现
│       │   ├── errors.go
│       │   ├── session.go
│       │   ├── message.go
│       │   └── outbox.go
│       ├── migrate/                   # 新：embed 迁移文件 + 执行入口
│       │   ├── migrate.go
│       │   └── migrations/            # 新：SQL 迁移文件（embed 进 Go 二进制）
│       │       ├── 000001_create_chat_sessions.up.sql
│       │       ├── 000001_create_chat_sessions.down.sql
│       │       ├── 000002_create_chat_messages.up.sql
│       │       ├── 000002_create_chat_messages.down.sql
│       │       ├── 000003_create_outbox_events.up.sql
│       │       └── 000003_create_outbox_events.down.sql
│       └── mysql/client.go            # 改：暴露 `*sql.DB` 给 migrate 用
├── Makefile                           # 改：新增 migrate 相关目标
└── README.md                          # 改：补 migrations/model/repository 职责
```

> **关于 `migrations/` 位置**：roadmap 第 2 节「建议目录」画的是顶层 `services/migrations/`，但 Go 的 `//go:embed` 只能嵌入「包目录及其子目录」的文件，不能引用 `..`。为了让 `make dev-go` 启动时自动迁移（自包含二进制），本计划把迁移文件放在 `services/go-api/internal/migrate/migrations/`，与 runner 同包。后续需要在 roadmap 第 2 节的目录草图上同步这一点。

## 4. 数据库设计（DDL 草稿）

所有表统一：`ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`。主键 `id` 用 `VARCHAR(36)`（UUID 字符串，生成策略见第 9 节开放问题）。

### 4.1 chat_sessions

```sql
-- 000001_create_chat_sessions.up.sql
CREATE TABLE chat_sessions (
    id              VARCHAR(36)  NOT NULL,                     -- UUID
    owner_key       VARCHAR(128) NOT NULL,                     -- 归属键，如 client:<uuid>
    title           VARCHAR(255) NOT NULL DEFAULT '',          -- 会话标题，阶段 3 才更新
    model_profile   VARCHAR(64)  NOT NULL DEFAULT 'default',   -- 服务端模型 profile
    status          VARCHAR(16)  NOT NULL DEFAULT 'active',    -- 预留，枚举未定义（见开放问题）
    created_at      DATETIME(3)  NOT NULL,
    updated_at      DATETIME(3)  NOT NULL,
    last_message_at DATETIME(3)  NULL,                         -- 会话列表排序用
    deleted_at      DATETIME(3)  NULL,                         -- 软删除标记
    PRIMARY KEY (id),
    KEY idx_sessions_owner_updated (owner_key, updated_at)     -- 会话列表按最近更新排序
);
```

```sql
-- 000001_create_chat_sessions.down.sql
DROP TABLE IF EXISTS chat_sessions;
```

**要点：**
- `deleted_at` 非 NULL 即视为已删除（软删除），阶段 3 的 `DELETE` 走软删。
- `last_message_at` 与 `updated_at` 分开：前者表示「最后一次有消息」，后者表示「记录被改过」。阶段 3 发消息时两者通常一起更新。

### 4.2 chat_messages

```sql
-- 000002_create_chat_messages.up.sql
CREATE TABLE chat_messages (
    id                VARCHAR(36)   NOT NULL,                  -- UUID
    session_id        VARCHAR(36)   NOT NULL,
    seq               BIGINT        NOT NULL,                  -- 会话内序号，从 1 递增
    role              VARCHAR(16)   NOT NULL,                  -- system/user/assistant/tool
    content           MEDIUMTEXT    NOT NULL,                  -- 文本，最大 32,000 字符
    status            VARCHAR(16)   NOT NULL,                  -- pending/completed/failed
    client_message_id VARCHAR(128)  NULL,                      -- 幂等键，仅用户消息有
    model_name        VARCHAR(128)  NULL,                      -- 实际调用的模型名（阶段 4 填充）
    prompt_tokens     INT           NULL,
    completion_tokens INT           NULL,
    created_at        DATETIME(3)   NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uq_messages_session_seq (session_id, seq),                 -- C：兜底唯一
    UNIQUE KEY uq_messages_session_client (session_id, client_message_id),-- 幂等兜底
    KEY idx_messages_session_created (session_id, created_at)             -- 按时间取历史
);
```

```sql
-- 000002_create_chat_messages.down.sql
DROP TABLE IF EXISTS chat_messages;
```

**要点：**
- `content` 用 `MEDIUMTEXT`（16MB）而非 `TEXT`（64KB）：32,000 个 Unicode 字符最坏情况按 4 字节/字 = 128KB，`TEXT` 装不下。
- 三个索引正好对应 roadmap 任务 143–148 行：
  - `(session_id, seq)` 唯一 → 会话内稳定排序 + 兜底防 seq 重复；
  - `(session_id, client_message_id)` 唯一 → 会话内幂等兜底；
  - `(session_id, created_at)` → 按时间拉取消息历史。
- `role` / `status` 用 `VARCHAR` + Go 层枚举常量校验，不用 MySQL 的 `ENUM` 类型（ENUM 加值要改表，不灵活）。`is_user` 这类布尔字段明确不用。

### 4.3 outbox_events

```sql
-- 000003_create_outbox_events.up.sql
CREATE TABLE outbox_events (
    id             BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    event_type     VARCHAR(64)  NOT NULL,                     -- chat.completed / chat.failed ...
    aggregate_type VARCHAR(64)  NOT NULL,                     -- 聚合根类型，如 conversation
    aggregate_id   VARCHAR(36)  NOT NULL,                     -- 聚合根 id，如 session_id
    payload        JSON         NOT NULL,                     -- 事件体
    status         VARCHAR(16)  NOT NULL DEFAULT 'pending',   -- pending/published/failed
    retry_count    INT          NOT NULL DEFAULT 0,
    next_retry_at  DATETIME(3)  NULL,
    created_at     DATETIME(3)  NOT NULL,
    published_at   DATETIME(3)  NULL,
    KEY idx_outbox_status_next (status, next_retry_at)        -- 发布器扫 pending 用
);
```

```sql
-- 000003_create_outbox_events.down.sql
DROP TABLE IF EXISTS outbox_events;
```

**要点：**
- 表结构先建好，但**本阶段不写任何 Outbox 读写逻辑**（阶段 7 才做）。
- 阶段 7 要求「在保存 AI 消息的同一事务中写 Outbox」，所以这张表的建表语句现在就要和 `chat_messages` 一起定下来，避免到时候返工。

## 5. Go 领域模型（`internal/model`）

### 5.1 session.go

```go
package model

import "time"

type Session struct {
    ID            string     `gorm:"column:id;primaryKey"`
    OwnerKey      string     `gorm:"column:owner_key"`
    Title         string     `gorm:"column:title"`
    ModelProfile  string     `gorm:"column:model_profile"`
    Status        string     `gorm:"column:status"`
    CreatedAt     time.Time  `gorm:"column:created_at"`
    UpdatedAt     time.Time  `gorm:"column:updated_at"`
    LastMessageAt *time.Time `gorm:"column:last_message_at"`
    DeletedAt     *time.Time `gorm:"column:deleted_at"`
}
```

### 5.2 message.go（含枚举常量）

```go
package model

import "time"

// MessageRole 是可扩展枚举，禁止用 is_user 布尔表达角色。
type MessageRole string

const (
    RoleSystem    MessageRole = "system"
    RoleUser      MessageRole = "user"
    RoleAssistant MessageRole = "assistant"
    RoleTool      MessageRole = "tool"
)

type MessageStatus string

const (
    StatusPending   MessageStatus = "pending"
    StatusCompleted MessageStatus = "completed"
    StatusFailed    MessageStatus = "failed"
)

type Message struct {
    ID               string        `gorm:"column:id;primaryKey"`
    SessionID        string        `gorm:"column:session_id"`
    Seq              int64         `gorm:"column:seq"`
    Role             MessageRole   `gorm:"column:role"`
    Content          string        `gorm:"column:content"`
    Status           MessageStatus `gorm:"column:status"`
    ClientMessageID  *string       `gorm:"column:client_message_id"`
    ModelName        *string       `gorm:"column:model_name"`
    PromptTokens     *int          `gorm:"column:prompt_tokens"`
    CompletionTokens *int          `gorm:"column:completion_tokens"`
    CreatedAt        time.Time     `gorm:"column:created_at"`
}
```

### 5.3 outbox.go

```go
package model

import "time"

type OutboxStatus string

const (
    OutboxPending   OutboxStatus = "pending"
    OutboxPublished OutboxStatus = "published"
    OutboxFailed    OutboxStatus = "failed"
)

type OutboxEvent struct {
    ID           uint64      `gorm:"column:id;primaryKey;autoIncrement"`
    EventType    string      `gorm:"column:event_type"`
    AggregateType string     `gorm:"column:aggregate_type"`
    AggregateID  string      `gorm:"column:aggregate_id"`
    Payload      string      `gorm:"column:payload"` // JSON 字符串
    Status       OutboxStatus `gorm:"column:status"`
    RetryCount   int         `gorm:"column:retry_count"`
    NextRetryAt  *time.Time  `gorm:"column:next_retry_at"`
    CreatedAt    time.Time   `gorm:"column:created_at"`
    PublishedAt  *time.Time  `gorm:"column:published_at"`
}
```

> `owner_key` 派生收口（决策 4）单独放一个包，例如 `internal/model/owner.go`：
>
> ```go
> // OwnerKey 是「如何从 client_id 得到 owner_key」的唯一实现点。
> // 未来接入登录时只需在此改成 user:{user_id}。
> func OwnerKey(clientID string) string { return "client:" + clientID }
> ```
>
> 全项目只允许通过 `model.OwnerKey(...)` 构造 owner_key，禁止各处手拼字符串。

## 6. Repository 接口（`internal/repository`）

Repository 用 GORM 实现，接口与实现分离，便于阶段 8 写单测（可换内存 fake）。

### 6.1 session.go

```go
type SessionRepository interface {
    // Create 新建会话；调用方已填好 OwnerKey、Title、ModelProfile、时间戳。
    Create(ctx context.Context, s *model.Session) error

    // GetByID 按 id + owner_key 取会话；找不到或不属于该 owner 都返回 ErrNotFound。
    // 这是「会话归属校验」的唯一入口，阶段 3 的删除/发消息都必须先走这里。
    GetByID(ctx context.Context, ownerKey, id string) (*model.Session, error)

    // ListByOwner 返回该 owner 的会话，按 updated_at 倒序（列表页）。
    ListByOwner(ctx context.Context, ownerKey string) ([]model.Session, error)

    // UpdateTitleAndTime 发消息后更新标题与时间；同样带 owner_key 条件。
    UpdateTitleAndTime(ctx context.Context, ownerKey, id, title string, lastMessageAt time.Time) error

    // SoftDelete 软删除：置 deleted_at，不物理删除。
    SoftDelete(ctx context.Context, ownerKey, id string) error
}
```

**关键设计**：所有查询都带 `owner_key` 条件。`GetByID` 把「不存在」和「不属于该 owner」合并成同一个 `ErrNotFound`，避免向客户端泄露「会话存在但不归你」的信息（阶段 3 直接复用这个语义）。

### 6.2 message.go

```go
type MessageRepository interface {
    // AppendMessage 在【单个事务】内完成：锁定会话行 → 计算 seq → 写入消息。
    // 这就是决策 3 的 B 方案：悲观锁保证同会话并发写入时 seq 单调且不重复。
    // 传入的 m.Seq 会被忽略，返回实际分配的 seq（同时写回 m.Seq）。
    AppendMessage(ctx context.Context, m *model.Message) (int64, error)

    // ListBySession 按 seq 升序返回消息（阶段 3 上下文读取用）。
    ListBySession(ctx context.Context, sessionID string, limit int) ([]model.Message, error)

    // GetByClientMessageID 幂等查询：同会话内按 client_message_id 找用户消息。
    // 阶段 3 发送消息时先用它判断是否重复请求。
    GetByClientMessageID(ctx context.Context, sessionID, clientMessageID string) (*model.Message, error)
}
```

**`AppendMessage` 事务伪代码（决策 3 的 B 方案）：**

```go
func (r *messageRepo) AppendMessage(ctx context.Context, m *model.Message) (int64, error) {
    var seq int64
    err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        // 1. 锁住会话行（同会话串行化）
        var session model.Session
        if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
            Where("id = ?", m.SessionID).First(&session).Error; err != nil {
            return err
        }
        // 2. 锁内计算下一个 seq
        var max *int64
        tx.Model(&model.Message{}).
            Where("session_id = ?", m.SessionID).
            Select("MAX(seq)").Scan(&max)
        seq = 1
        if max != nil { seq = *max + 1 }
        // 3. 写入
        m.Seq = seq
        return tx.Create(m).Error
    })
    return seq, err
}
```

**两层保障对应决策 3：**
- B（锁行再算）= 上面的 `FOR UPDATE` 事务，保证正常流程串行；
- C（唯一索引兜底）= `uq_messages_session_seq`，万一将来有人绕过了锁，数据库也会拒绝重复 seq。

### 6.3 outbox.go

```go
type OutboxRepository interface {
    Create(ctx context.Context, e *model.OutboxEvent) error
    ListPending(ctx context.Context, limit int) ([]model.OutboxEvent, error)
    MarkPublished(ctx context.Context, id uint64, publishedAt time.Time) error
}
```

> 阶段 2 只定义接口并给出 GORM 实现（用于编译通过和最小单测），**业务上阶段 7 才会真正调用**。

## 7. 迁移执行（`internal/migrate`）

用 golang-migrate 的 `iofs` source + `mysql` database 驱动，把 `migrations/*.sql` embed 进二进制，启动时自动执行。

```go
// internal/migrate/migrate.go
package migrate

import "embed"

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Up 用 golang-migrate 把 migrationsFS 里未执行的版本全部应用到 db。
func Up(db *sql.DB) error { /* migrate.NewWithSourceInstance("iofs", ...) */ }
```

**依赖（`go.mod` 新增）：**

```text
github.com/golang-migrate/migrate/v4
github.com/golang-migrate/migrate/v4/source/iofs
github.com/golang-migrate/migrate/v4/database/mysql
```

**需要改 `internal/mysql/client.go`：** 当前 `Client` 只暴露 `DB() *gorm.DB`，`sqlDB` 是私有字段。给 `Client` 增加一个访问器：

```go
func (c *Client) SQLDB() *sql.DB { return c.sqlDB }
```

**`cmd/api/main.go` 启动顺序调整：** 在 `mysql.Open` 之后、`httpserver.New` 之前插入：

```go
if err := migrate.Up(db.SQLDB()); err != nil {
    logger.Error("run migrations failed", "error", err)
    os.Exit(1)
}
```

这样 `make dev-go` 一启动就自动建表，无需手动跑迁移。手动 up/down 仍可走 `make migrate-up` / `make migrate-down`（用 golang-migrate CLI 或提供 `-migrate` 子命令，二选一，见第 9 节）。

**`Makefile` 新增目标：**

```makefile
# 用 golang-migrate CLI 手动管理（需本机安装 migrate 工具）
migrate-create:
	@read -p "migration name: " name; \
	migrate create -ext sql -dir go-api/internal/migrate/migrations -seq $$name

migrate-up:
	@set -a; . ./.env; set +a; \
	migrate -path go-api/internal/migrate/migrations -database "mysql://$${MYSQL_USER}:$${MYSQL_PASSWORD}@tcp($${MYSQL_HOST}:$${MYSQL_PORT})/$${MYSQL_DATABASE}" up

migrate-down:
	@set -a; . ./.env; set +a; \
	migrate -path go-api/internal/migrate/migrations -database "mysql://$${MYSQL_USER}:$${MYSQL_PASSWORD}@tcp($${MYSQL_HOST}:$${MYSQL_PORT})/$${MYSQL_DATABASE}" down 1
```

## 8. 要修改的现有文件

| 文件 | 改动 |
|---|---|
| `go-api/go.mod` / `go.sum` | 新增 golang-migrate 三个依赖 |
| `go-api/internal/mysql/client.go` | 加 `SQLDB() *sql.DB` 访问器 |
| `go-api/cmd/api/main.go` | 启动早期执行 `migrate.Up(...)`；Repository 构造与注入留给阶段 3 |
| `go-api/internal/README.md` | 补 `model`/`repository`/`migrate` 包职责说明 |
| `services/README.md` | 目录职责图补 `model/`、`repository/`、`migrate/`（含 `migrate/migrations/`） |
| `services/Makefile` | 加 `migrate-create` / `migrate-up` / `migrate-down` |
| `docs/roadmap/MVP_EXECUTION_PLAN.md` | 勾选阶段 2 完成的 checkbox；目录草图上 `migrations/` 位置修正 |
| `docs/phase-1/PHASE_1_0_DECISIONS.md` | 修正「MySQL 用 database/sql」与实际的矛盾——补充说明 Repository 层用 GORM |

## 9. 开放问题（实现前确认）

1. **`id` 生成策略**：`VARCHAR(36)` 定死，但具体用 UUID v4、UUID v7 还是 ULID 未定。推荐 UUID v4（与 `client_id` 一致、最简单）；若想要「按时间可排序」可换 ULID。**默认 UUID v4。**
2. **`chat_sessions.status` 枚举**：阶段 0 契约只列了字段没定义枚举，MVP 也用不到。已保留为 `DEFAULT 'active'`。可选择「先删掉、需要时再加」或「保留为 active」。**默认保留。**
3. **手动迁移入口**：`make migrate-up` 依赖本机安装 `migrate` CLI。若不想装 CLI，可在 `cmd/api` 加一个 `-migrate` 子命令复用 embed。**默认用 CLI + 启动自动迁移双轨，不强依赖 CLI。**

## 10. 落地步骤（checklist）

- [x] 1. 确认第 9 节 3 个开放问题的默认选项。
- [x] 2. 建 `internal/migrate/migrations/`，写 3 组 up/down SQL。
- [x] 3. `go-api/internal/model/`：`session.go`、`message.go`、`outbox.go`、`owner.go`。
- [x] 4. `go-api/internal/migrate/migrate.go`：embed + `Up()`。
- [x] 5. `go-api/internal/mysql/client.go` 加 `SQLDB()`；`main.go` 加迁移调用。
- [x] 6. `go-api/internal/repository/`：三个接口 + GORM 实现（`AppendMessage` 按 6.2 事务写）。
- [x] 7. `go.mod` 加 golang-migrate 依赖，`go mod tidy`。
- [x] 8. 写单测：会话可建/查/删；消息 `AppendMessage` seq 递增；跨 owner 隔离（A 查不到 B 的会话）。
- [x] 9. `make test-go` + `make test-go-integration` 通过；重复执行迁移不报错（验证「可重复执行」）。
- [ ] 10. 更新 `services/README.md`、`internal/README.md`、`Makefile`、roadmap checkbox。

> 说明：验收通过集成测试（`make test-go-integration`，内部调用 `migrate.Up` 两次验证可重复）完成，不依赖本机 `migrate` CLI；`make migrate-up/down/create` 目标需要额外安装 CLI，属可选。

## 11. 验收标准（对照 roadmap 第 6 节）

- [x] migration 可重复执行（同版本不重复跑）；
- [x] 会话和消息可创建、查询、删除；
- [x] 服务重启后数据仍然存在（存 MySQL，不靠内存）；
- [x] 不同匿名设备（不同 `owner_key`）不能访问彼此的会话。
