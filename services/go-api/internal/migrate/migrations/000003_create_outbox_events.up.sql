CREATE TABLE outbox_events (
    id             BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    event_type     VARCHAR(64)  NOT NULL,
    aggregate_type VARCHAR(64)  NOT NULL,
    aggregate_id   VARCHAR(36)  NOT NULL,
    payload        JSON         NOT NULL,
    status         VARCHAR(16)  NOT NULL DEFAULT 'pending',
    retry_count    INT          NOT NULL DEFAULT 0,
    next_retry_at  DATETIME(3)  NULL,
    created_at     DATETIME(3)  NOT NULL,
    published_at   DATETIME(3)  NULL,
    KEY idx_outbox_status_next (status, next_retry_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
