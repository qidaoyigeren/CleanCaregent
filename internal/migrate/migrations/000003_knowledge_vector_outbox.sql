CREATE TABLE IF NOT EXISTS knowledge_vector_outbox (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    doc_id VARCHAR(128) NOT NULL,
    version VARCHAR(64) NOT NULL,
    action VARCHAR(64) NOT NULL,
    point_ids_json JSON NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    attempts INT UNSIGNED NOT NULL DEFAULT 0,
    last_error TEXT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uk_knowledge_vector_outbox_doc_action (doc_id, version, action),
    KEY idx_knowledge_vector_outbox_status (status, attempts, updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
