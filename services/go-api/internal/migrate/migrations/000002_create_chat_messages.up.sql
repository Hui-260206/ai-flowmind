CREATE TABLE chat_messages (
    id                VARCHAR(36)   NOT NULL,
    session_id        VARCHAR(36)   NOT NULL,
    seq               BIGINT        NOT NULL,
    role              VARCHAR(16)   NOT NULL,
    content           MEDIUMTEXT    NOT NULL,
    status            VARCHAR(16)   NOT NULL,
    client_message_id VARCHAR(128)  NULL,
    model_name        VARCHAR(128)  NULL,
    prompt_tokens     INT           NULL,
    completion_tokens INT           NULL,
    created_at        DATETIME(3)   NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uq_messages_session_seq (session_id, seq),
    UNIQUE KEY uq_messages_session_client (session_id, client_message_id),
    KEY idx_messages_session_created (session_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
