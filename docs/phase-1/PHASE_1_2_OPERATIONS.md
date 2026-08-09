# 阶段 1.2：Mac 本地 MySQL/Redis 开发环境

阶段 1.2 的开发环境调整为：MySQL 和 Redis 直接安装、运行在开发者的 Mac 上。Docker Compose、云服务器、SSH 隧道和云端数据卷推迟到后续部署阶段，不属于当前阶段的启动步骤。

本阶段不创建聊天业务表；业务 migration 放在阶段 2。

## 1. 本地拓扑

```text
本机 Go API ──▶ 127.0.0.1:3306 MySQL
        └─────▶ 127.0.0.1:6379 Redis
```

开发配置使用 `services/.env.example`，复制为 `services/.env` 后按本机密码修改。Go 使用独立的 `flowmind_dev` 数据库和 `flowmind_app` 应用账号，不使用 MySQL root 连接应用。

## 2. 使用 Homebrew 安装

当前机器已经安装并运行以下版本：

| 组件 | 实际版本/状态 |
|---|---|
| Homebrew | 6.0.15 |
| MySQL | Homebrew `mysql@8.0`，客户端 8.0.46，服务已监听 `127.0.0.1:3306` |
| Redis | 8.10.0，Homebrew 服务已启动，监听 `127.0.0.1:6379` |

如果其他 Mac 尚未安装 Homebrew，请先按官方文档安装。当前开发机不需要重复安装；安装命令如下：

```sh
brew update
brew install mysql@8.0 redis
```

确认版本：

```sh
mysql --version
redis-server --version
```

阶段 1.0 原先记录的 MySQL 8.4 LTS 和 Redis 7.4.x 是技术选型目标，不是当前开发机实际版本。当前开发机使用 MySQL 8.0.46 和 Redis 8.10.0；后续部署镜像版本必须在完成兼容性验证后再固定，不能直接假设与本机版本完全一致。

## 3. 启动本地服务

```sh
brew services start mysql@8.0
brew services start redis
brew services list
```

本地服务只监听回环地址时，不会对局域网或公网开放。检查监听地址：

```sh
lsof -nP -iTCP:3306 -sTCP:LISTEN
lsof -nP -iTCP:6379 -sTCP:LISTEN
```

期望看到 `127.0.0.1:3306` 和 `127.0.0.1:6379`，不要把 MySQL 或 Redis 配置成 `0.0.0.0` 监听。

## 4. 创建开发数据库和应用账号

第一次初始化时，用本机管理员账号进入 MySQL：

```sh
mysql -u root
```

在 MySQL shell 中执行以下 SQL。密码是示例值，必须替换成只保存在本机 `.env` 中的随机密码：

```sql
CREATE DATABASE IF NOT EXISTS flowmind_dev
  CHARACTER SET utf8mb4
  COLLATE utf8mb4_0900_ai_ci;

CREATE USER IF NOT EXISTS 'flowmind_app'@'localhost'
  IDENTIFIED BY 'replace-with-a-local-password';

ALTER USER 'flowmind_app'@'localhost'
  IDENTIFIED BY 'replace-with-a-local-password';

GRANT ALL PRIVILEGES ON flowmind_dev.* TO 'flowmind_app'@'localhost';
FLUSH PRIVILEGES;
```

退出后，将同一个本地密码写入 `services/.env` 的 `MYSQL_PASSWORD`。真实密码不要写入仓库、脚本或聊天记录。

验证数据库和字符集：

```sh
mysql --host=127.0.0.1 --port=3306 \\
  --user=flowmind_app --password flowmind_dev \\
  -e 'SELECT DATABASE(), @@character_set_database, @@collation_database;'
```

## 5. 配置 Redis 本地认证和持久化

当前 Redis 实例已按本节配置为：`protected-mode=yes`、绑定 `127.0.0.1`/`::1`、启用密码认证和 AOF。配置文件位于开发机用户目录，不进入仓库。

为避免开发环境无密码运行 Redis，创建本机用户配置目录和配置文件：

```sh
mkdir -p "$HOME/.config/flowmind/redis" "$HOME/.local/share/flowmind/redis"
chmod 700 "$HOME/.config/flowmind/redis" "$HOME/.local/share/flowmind/redis"
```

创建 `$HOME/.config/flowmind/redis/redis.conf`，内容如下，并将密码替换为本机 `.env` 中的 `REDIS_PASSWORD`：

```conf
bind 127.0.0.1 ::1
protected-mode yes
port 6379
requirepass replace-with-a-local-password
dir /Users/YOUR_MAC_USER/.local/share/flowmind/redis
appendonly yes
appendfsync everysec
save 900 1
save 300 10
```

将 `YOUR_MAC_USER` 替换为实际 macOS 用户名。启动这个配置实例：

```sh
redis-server "$HOME/.config/flowmind/redis/redis.conf" --daemonize yes
```

验证认证和持久化目录：

```sh
redis-cli -h 127.0.0.1 -p 6379 -a 'your-local-redis-password' --no-auth-warning ping
redis-cli -h 127.0.0.1 -p 6379 -a 'your-local-redis-password' --no-auth-warning CONFIG GET dir appendonly
```

停止该实例：

```sh
redis-cli -h 127.0.0.1 -p 6379 -a 'your-local-redis-password' --no-auth-warning shutdown save
```

如果使用 Homebrew 的 `brew services start redis`，应确保它读取了等价的回环监听、密码和持久化配置；不要同时启动两个 Redis 实例占用 6379。

## 6. 本地环境变量

```sh
cp services/.env.example services/.env
chmod 600 services/.env
```

本阶段本地关键变量应为：

```dotenv
MYSQL_HOST=127.0.0.1
MYSQL_PORT=3306
MYSQL_DATABASE=flowmind_dev
MYSQL_USER=flowmind_app
MYSQL_PASSWORD=本机MySQL应用账号密码
REDIS_ADDR=127.0.0.1:6379
REDIS_PASSWORD=本机Redis密码
```

Go 服务正式实现后读取这些变量。当前阶段不要求 Go 已经能够连接，因为 Go 客户端属于阶段 1.6/1.7。

## 7. 本地备份和恢复

创建备份目录时不要放在仓库中：

```sh
mkdir -p "$HOME/.local/share/flowmind/backups/mysql"
chmod 700 "$HOME/.local/share/flowmind/backups/mysql"
```

MySQL 逻辑备份（MySQL 8 使用非 root 应用账号时增加 `--no-tablespaces`，避免要求 `PROCESS` 权限）：

```sh
mysqldump --single-transaction --routines --triggers \\
  --no-tablespaces \\
  --host=127.0.0.1 --port=3306 \\
  --user=flowmind_app --password flowmind_dev \\
  > "$HOME/.local/share/flowmind/backups/mysql/flowmind_dev_$(date +%Y%m%d_%H%M%S).sql"
chmod 600 "$HOME/.local/share/flowmind/backups/mysql"/*.sql
```

恢复到开发数据库前，先确认目标数据库和备份文件，再执行：

```sh
mysql --host=127.0.0.1 --port=3306 \\
  --user=flowmind_app --password flowmind_dev \\
  < "$HOME/.local/share/flowmind/backups/mysql/backup.sql"
```

Redis 开启 AOF/RDB 后，数据位于配置的 `dir`；这里的数据只是临时状态，不是聊天历史的事实来源。恢复前不要删除 Redis 数据目录，因为删除数据是不可逆的本地破坏操作。

## 8. 后续迁移到 Docker

当进入容器化/部署阶段时：

1. 使用 `services/.env.docker.example` 作为云端配置模板。
2. Go 在 Compose 网络内使用 `MYSQL_HOST=mysql`、`REDIS_ADDR=redis:6379`。
3. MySQL/Redis 使用命名卷，不发布 3306/6379 公网端口。
4. 先完成备份，再将需要保留的开发数据导入云端新数据库。
5. 在云端完成重启持久化和恢复演练后，再切换 Go 的部署配置。

当前保留的 `services/docker-compose.yaml` 是后续云端部署基线，不是 Mac 本地开发方案。

## 9. 消息队列说明

MVP 不涉及消息队列是可行的，且建议如此。普通聊天采用同步链路：

```text
移动端 → Go HTTP → 写入用户消息 MySQL → Go gRPC 调 Python → 写入 AI 回复 MySQL → 返回
```

RabbitMQ、Kafka、Redis Streams 都不加入当前开发环境。未来如果需要通知、统计、异步标题生成或其他非关键任务，再引入 Outbox + MQ；MQ 故障不应让普通聊天主链路不可用。

## 10. 阶段 1.2 验收清单

- [x] 开发环境改为 Mac 本机 MySQL/Redis，不依赖 Docker。
- [x] 本地 MySQL 使用独立数据库和非 root 应用账号。
- [x] MySQL 实际服务端版本、utf8mb4 和 Asia/Shanghai 时区已通过应用账号验证。
- [x] Redis 使用回环监听、密码认证和 AOF/RDB 持久化。
- [x] 本地备份和恢复方式已明确。
- [x] 后续云端 Docker 迁移边界已明确。
- [x] MVP 不引入消息队列的同步方案已明确。
- [x] 本机 MySQL/Redis 已安装并启动，版本和监听端口已记录。
- [x] 在本机完成账号、认证、持久化、连接和恢复演练。
