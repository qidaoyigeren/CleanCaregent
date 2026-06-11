CREATE TABLE IF NOT EXISTS users (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    user_no VARCHAR(64) NOT NULL,
    nickname VARCHAR(100) NULL,
    phone_hash CHAR(64) NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uk_users_user_no (user_no),
    KEY idx_users_phone_hash (phone_hash)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS products (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    product_code VARCHAR(64) NOT NULL,
    name VARCHAR(200) NOT NULL,
    category VARCHAR(64) NOT NULL,
    brand VARCHAR(100) NOT NULL,
    model VARCHAR(100) NOT NULL,
    attributes_json JSON NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uk_products_product_code (product_code),
    KEY idx_products_category_brand (category, brand),
    KEY idx_products_model (model)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS product_skus (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    sku_code VARCHAR(64) NOT NULL,
    product_id BIGINT UNSIGNED NOT NULL,
    sku_name VARCHAR(200) NOT NULL,
    specs_json JSON NULL,
    list_price DECIMAL(10,2) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uk_product_skus_sku_code (sku_code),
    KEY idx_product_skus_product_id (product_id),
    CONSTRAINT fk_product_skus_product FOREIGN KEY (product_id) REFERENCES products (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS orders (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    order_no VARCHAR(64) NOT NULL,
    user_id BIGINT UNSIGNED NOT NULL,
    status VARCHAR(32) NOT NULL,
    total_amount DECIMAL(10,2) NOT NULL,
    paid_at DATETIME(6) NULL,
    delivered_at DATETIME(6) NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uk_orders_order_no (order_no),
    KEY idx_orders_user_created (user_id, created_at),
    KEY idx_orders_user_status (user_id, status),
    CONSTRAINT fk_orders_user FOREIGN KEY (user_id) REFERENCES users (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS order_items (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    order_id BIGINT UNSIGNED NOT NULL,
    product_id BIGINT UNSIGNED NOT NULL,
    sku_id BIGINT UNSIGNED NOT NULL,
    quantity INT UNSIGNED NOT NULL,
    unit_price DECIMAL(10,2) NOT NULL,
    warranty_months INT UNSIGNED NOT NULL DEFAULT 12,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    KEY idx_order_items_order_id (order_id),
    KEY idx_order_items_product_id (product_id),
    CONSTRAINT fk_order_items_order FOREIGN KEY (order_id) REFERENCES orders (id),
    CONSTRAINT fk_order_items_product FOREIGN KEY (product_id) REFERENCES products (id),
    CONSTRAINT fk_order_items_sku FOREIGN KEY (sku_id) REFERENCES product_skus (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS coupons (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    coupon_code VARCHAR(64) NOT NULL,
    name VARCHAR(200) NOT NULL,
    discount_type VARCHAR(32) NOT NULL,
    discount_value DECIMAL(10,2) NOT NULL,
    start_at DATETIME(6) NOT NULL,
    end_at DATETIME(6) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uk_coupons_coupon_code (coupon_code),
    KEY idx_coupons_status_time (status, start_at, end_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS user_coupons (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    user_id BIGINT UNSIGNED NOT NULL,
    coupon_id BIGINT UNSIGNED NOT NULL,
    status VARCHAR(32) NOT NULL,
    claimed_at DATETIME(6) NOT NULL,
    used_at DATETIME(6) NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uk_user_coupons_user_coupon (user_id, coupon_id),
    KEY idx_user_coupons_user_status (user_id, status),
    CONSTRAINT fk_user_coupons_user FOREIGN KEY (user_id) REFERENCES users (id),
    CONSTRAINT fk_user_coupons_coupon FOREIGN KEY (coupon_id) REFERENCES coupons (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS kb_documents (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    doc_id VARCHAR(128) NOT NULL,
    title VARCHAR(255) NOT NULL,
    product_id BIGINT UNSIGNED NULL,
    category VARCHAR(64) NOT NULL,
    brand VARCHAR(100) NULL,
    doc_type VARCHAR(64) NOT NULL,
    version VARCHAR(64) NOT NULL,
    effective_time DATETIME(6) NULL,
    expire_time DATETIME(6) NULL,
    source VARCHAR(500) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    content_hash CHAR(64) NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uk_kb_documents_doc_version (doc_id, version),
    KEY idx_kb_documents_type_status (doc_type, status),
    KEY idx_kb_documents_product_type (product_id, doc_type),
    KEY idx_kb_documents_effective_time (effective_time, expire_time),
    CONSTRAINT fk_kb_documents_product FOREIGN KEY (product_id) REFERENCES products (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS kb_chunks (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    chunk_id VARCHAR(128) NOT NULL,
    document_id BIGINT UNSIGNED NOT NULL,
    section_path VARCHAR(500) NULL,
    content MEDIUMTEXT NOT NULL,
    token_count INT UNSIGNED NOT NULL,
    intent_tags_json JSON NULL,
    metadata_json JSON NULL,
    vector_point_id VARCHAR(64) NULL,
    content_hash CHAR(64) NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uk_kb_chunks_chunk_id (chunk_id),
    KEY idx_kb_chunks_document_id (document_id),
    KEY idx_kb_chunks_vector_point (vector_point_id),
    FULLTEXT KEY ftx_kb_chunks_content (content),
    CONSTRAINT fk_kb_chunks_document FOREIGN KEY (document_id) REFERENCES kb_documents (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS conversations (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    conversation_no VARCHAR(64) NOT NULL,
    user_id BIGINT UNSIGNED NOT NULL,
    title VARCHAR(100) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    last_message_at DATETIME(6) NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uk_conversations_conversation_no (conversation_no),
    KEY idx_conversations_user_last_message (user_id, last_message_at),
    CONSTRAINT fk_conversations_user FOREIGN KEY (user_id) REFERENCES users (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS messages (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    message_no VARCHAR(64) NOT NULL,
    conversation_id BIGINT UNSIGNED NOT NULL,
    role VARCHAR(32) NOT NULL,
    content MEDIUMTEXT NOT NULL,
    intent VARCHAR(64) NULL,
    trace_id VARCHAR(64) NULL,
    token_count INT UNSIGNED NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uk_messages_message_no (message_no),
    KEY idx_messages_conversation_id_id (conversation_id, id),
    KEY idx_messages_trace_id (trace_id),
    CONSTRAINT fk_messages_conversation FOREIGN KEY (conversation_id) REFERENCES conversations (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS tool_call_logs (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    trace_id VARCHAR(64) NOT NULL,
    call_id VARCHAR(64) NOT NULL,
    tool_name VARCHAR(100) NOT NULL,
    args_masked_json JSON NULL,
    result_summary_json JSON NULL,
    status VARCHAR(32) NOT NULL,
    error_code VARCHAR(100) NULL,
    latency_ms BIGINT UNSIGNED NOT NULL,
    idempotency_key VARCHAR(128) NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uk_tool_call_logs_call_id (call_id),
    KEY idx_tool_call_logs_trace_tool (trace_id, tool_name),
    KEY idx_tool_call_logs_status_created (status, created_at),
    KEY idx_tool_call_logs_idempotency (idempotency_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS agent_traces (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    trace_id VARCHAR(64) NOT NULL,
    conversation_id BIGINT UNSIGNED NULL,
    message_id BIGINT UNSIGNED NULL,
    intent VARCHAR(64) NULL,
    route_mode VARCHAR(32) NULL,
    plan_json JSON NULL,
    step_summary_json JSON NULL,
    evidence_ids_json JSON NULL,
    model_name VARCHAR(100) NULL,
    prompt_version VARCHAR(64) NULL,
    input_tokens INT UNSIGNED NOT NULL DEFAULT 0,
    output_tokens INT UNSIGNED NOT NULL DEFAULT 0,
    latency_ms BIGINT UNSIGNED NOT NULL DEFAULT 0,
    status VARCHAR(32) NOT NULL,
    error_code VARCHAR(100) NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uk_agent_traces_trace_id (trace_id),
    KEY idx_agent_traces_conversation_created (conversation_id, created_at),
    KEY idx_agent_traces_status_intent (status, intent),
    KEY idx_agent_traces_prompt_version (prompt_version),
    CONSTRAINT fk_agent_traces_conversation FOREIGN KEY (conversation_id) REFERENCES conversations (id),
    CONSTRAINT fk_agent_traces_message FOREIGN KEY (message_id) REFERENCES messages (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS eval_cases (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    case_id VARCHAR(100) NOT NULL,
    query TEXT NOT NULL,
    intent VARCHAR(64) NOT NULL,
    difficulty VARCHAR(32) NOT NULL,
    expected_docs_json JSON NULL,
    expected_tools_json JSON NULL,
    expected_tool_params_json JSON NULL,
    standard_answer MEDIUMTEXT NOT NULL,
    should_clarify BOOLEAN NOT NULL DEFAULT FALSE,
    should_reject BOOLEAN NOT NULL DEFAULT FALSE,
    expected_evidence_ids_json JSON NULL,
    tags_json JSON NULL,
    version VARCHAR(64) NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uk_eval_cases_case_version (case_id, version),
    KEY idx_eval_cases_intent_difficulty (intent, difficulty)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS eval_runs (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    run_no VARCHAR(100) NOT NULL,
    dataset_version VARCHAR(64) NOT NULL,
    system_version VARCHAR(64) NOT NULL,
    model_config_json JSON NULL,
    status VARCHAR(32) NOT NULL,
    started_at DATETIME(6) NULL,
    finished_at DATETIME(6) NULL,
    summary_json JSON NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uk_eval_runs_run_no (run_no),
    KEY idx_eval_runs_status_started (status, started_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS eval_results (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    run_id BIGINT UNSIGNED NOT NULL,
    case_id BIGINT UNSIGNED NOT NULL,
    trace_id VARCHAR(64) NULL,
    actual_intent VARCHAR(64) NULL,
    actual_tools_json JSON NULL,
    answer MEDIUMTEXT NULL,
    metrics_json JSON NULL,
    passed BOOLEAN NOT NULL DEFAULT FALSE,
    error_type VARCHAR(100) NULL,
    latency_ms BIGINT UNSIGNED NOT NULL DEFAULT 0,
    token_count INT UNSIGNED NOT NULL DEFAULT 0,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uk_eval_results_run_case (run_id, case_id),
    KEY idx_eval_results_run_passed (run_id, passed),
    KEY idx_eval_results_error_type (error_type),
    CONSTRAINT fk_eval_results_run FOREIGN KEY (run_id) REFERENCES eval_runs (id),
    CONSTRAINT fk_eval_results_case FOREIGN KEY (case_id) REFERENCES eval_cases (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS after_sales_tickets (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    ticket_no VARCHAR(64) NOT NULL,
    user_id BIGINT UNSIGNED NOT NULL,
    order_id BIGINT UNSIGNED NOT NULL,
    order_item_id BIGINT UNSIGNED NOT NULL,
    issue_type VARCHAR(64) NOT NULL,
    description TEXT NOT NULL,
    diagnosis_summary TEXT NULL,
    evidence_ids_json JSON NULL,
    status VARCHAR(32) NOT NULL,
    idempotency_key VARCHAR(128) NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uk_after_sales_tickets_ticket_no (ticket_no),
    UNIQUE KEY uk_after_sales_tickets_idempotency (idempotency_key),
    KEY idx_after_sales_tickets_user_created (user_id, created_at),
    KEY idx_after_sales_tickets_order_item (order_item_id),
    CONSTRAINT fk_after_sales_tickets_user FOREIGN KEY (user_id) REFERENCES users (id),
    CONSTRAINT fk_after_sales_tickets_order FOREIGN KEY (order_id) REFERENCES orders (id),
    CONSTRAINT fk_after_sales_tickets_order_item FOREIGN KEY (order_item_id) REFERENCES order_items (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

