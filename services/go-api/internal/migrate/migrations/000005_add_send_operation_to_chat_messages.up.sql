ALTER TABLE chat_messages
    ADD COLUMN send_operation_id VARCHAR(36) NULL,
    ADD KEY idx_messages_send_operation (session_id, send_operation_id);
