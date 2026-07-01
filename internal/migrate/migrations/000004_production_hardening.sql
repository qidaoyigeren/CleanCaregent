SET @column_exists := (
    SELECT COUNT(*)
    FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'messages'
      AND COLUMN_NAME = 'client_message_id'
);
SET @ddl := IF(
    @column_exists = 0,
    'ALTER TABLE messages ADD COLUMN client_message_id VARCHAR(128) NULL AFTER trace_id',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @index_exists := (
    SELECT COUNT(*)
    FROM information_schema.STATISTICS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'messages'
      AND INDEX_NAME = 'uk_messages_client_message'
);
SET @ddl := IF(
    @index_exists = 0,
    'CREATE UNIQUE INDEX uk_messages_client_message ON messages (conversation_id, role, client_message_id)',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

CREATE TABLE IF NOT EXISTS conversation_message_requests (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    conversation_id BIGINT UNSIGNED NOT NULL,
    client_message_id VARCHAR(128) NOT NULL,
    status VARCHAR(32) NOT NULL,
    assistant_message_no VARCHAR(64) NULL,
    trace_id VARCHAR(64) NULL,
    error_message VARCHAR(1000) NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uk_conversation_message_requests_client (conversation_id, client_message_id),
    KEY idx_conversation_message_requests_status (status, updated_at),
    CONSTRAINT fk_conversation_message_requests_conversation FOREIGN KEY (conversation_id) REFERENCES conversations (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

DELETE duplicate_ticket
FROM after_sales_tickets duplicate_ticket
JOIN after_sales_tickets kept_ticket
  ON kept_ticket.user_id = duplicate_ticket.user_id
 AND kept_ticket.order_id = duplicate_ticket.order_id
 AND kept_ticket.order_item_id = duplicate_ticket.order_item_id
 AND kept_ticket.issue_type = duplicate_ticket.issue_type
 AND kept_ticket.id < duplicate_ticket.id;

SET @index_exists := (
    SELECT COUNT(*)
    FROM information_schema.STATISTICS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'after_sales_tickets'
      AND INDEX_NAME = 'uk_after_sales_business_action'
);
SET @ddl := IF(
    @index_exists = 0,
    'CREATE UNIQUE INDEX uk_after_sales_business_action ON after_sales_tickets (user_id, order_id, order_item_id, issue_type)',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
