CREATE TABLE IF NOT EXISTS product_inventory (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    sku_id BIGINT UNSIGNED NOT NULL,
    current_price DECIMAL(10,2) NOT NULL,
    available_stock INT UNSIGNED NOT NULL DEFAULT 0,
    reserved_stock INT UNSIGNED NOT NULL DEFAULT 0,
    currency CHAR(3) NOT NULL DEFAULT 'CNY',
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uk_product_inventory_sku (sku_id),
    KEY idx_product_inventory_updated (updated_at),
    CONSTRAINT fk_product_inventory_sku FOREIGN KEY (sku_id) REFERENCES product_skus (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
