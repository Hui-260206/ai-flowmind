CREATE TABLE chat_sessions (
    id              VARCHAR(36)  NOT NULL,
    owner_key       VARCHAR(128) NOT NULL,
    title           VARCHAR(255) NOT NULL DEFAULT '',
    model_profile   VARCHAR(64)  NOT NULL DEFAULT 'default',
    status          VARCHAR(16)  NOT NULL DEFAULT 'active',
    created_at      DATETIME(3)  NOT NULL,
    updated_at      DATETIME(3)  NOT NULL,
    last_message_at DATETIME(3)  NULL,
    deleted_at      DATETIME(3)  NULL,
    PRIMARY KEY (id),
    KEY idx_sessions_owner_updated (owner_key, updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
