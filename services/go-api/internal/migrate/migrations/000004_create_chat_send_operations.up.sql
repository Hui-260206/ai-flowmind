CREATE TABLE chat_send_operations (
    id                   VARCHAR(36)  NOT NULL,
    owner_key            VARCHAR(128) NOT NULL,
    session_id           VARCHAR(36)  NOT NULL,
    client_message_id    VARCHAR(128) NOT NULL,
    fingerprint          CHAR(64)     NOT NULL,
    status               VARCHAR(16)  NOT NULL,
    user_message_id      VARCHAR(36)  NOT NULL DEFAULT '',
    assistant_message_id VARCHAR(36)  NOT NULL DEFAULT '',
    created_at           DATETIME(3)  NOT NULL,
    updated_at           DATETIME(3)  NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uq_send_operations_owner_session_client (owner_key, session_id, client_message_id),
    KEY idx_send_operations_session_status (session_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
