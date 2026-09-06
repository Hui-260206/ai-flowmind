ALTER TABLE chat_messages
    DROP INDEX idx_messages_send_operation,
    DROP COLUMN send_operation_id;
