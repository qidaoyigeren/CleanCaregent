package seed

import (
	"context"
	"database/sql"
	"fmt"
)

func MockBusinessData(ctx context.Context, db *sql.DB) error {
	statements := []string{
		`INSERT INTO users (user_no, nickname, status)
		 VALUES ('demo-user', 'Demo User', 'active')
		 ON DUPLICATE KEY UPDATE nickname = VALUES(nickname), status = 'active'`,
		`INSERT INTO products (product_code, name, category, brand, model, attributes_json, status)
		 VALUES
		   ('P-ROBOT-T20', 'CleanCare T20 扫地机器人', 'robot_vacuum', 'CleanCare', 'T20',
		    JSON_OBJECT('suction_pa', 6000, 'pet_friendly', true, 'carpet_boost', true), 'active'),
		   ('P-ROBOT-X20PRO', 'CleanCare X20 Pro 扫地机器人', 'robot_vacuum', 'CleanCare', 'X20 Pro',
		    JSON_OBJECT('suction_pa', 8000, 'pet_friendly', true, 'carpet_boost', true, 'self_cleaning', true), 'active'),
		   ('P-AIR-P400', 'CleanCare P400 空气净化器', 'air_purifier', 'CleanCare', 'P400',
		    JSON_OBJECT('cadr_m3h', 450, 'recommended_area_m2', 55, 'filter_model', 'F400'), 'active'),
		   ('P-AIR-F400', 'CleanCare F400 复合滤芯', 'air_purifier_accessory', 'CleanCare', 'F400',
		    JSON_OBJECT('compatible_models', JSON_ARRAY('P400'), 'replacement_months', 12), 'active')
		 ON DUPLICATE KEY UPDATE
		   name = VALUES(name), category = VALUES(category), brand = VALUES(brand),
		   model = VALUES(model), attributes_json = VALUES(attributes_json), status = 'active'`,
		`INSERT INTO product_skus (sku_code, product_id, sku_name, specs_json, list_price, status)
		 SELECT 'SKU-T20-WHITE', id, 'T20 云石白', JSON_OBJECT('color', 'white'), 3999.00, 'active'
		 FROM products WHERE product_code = 'P-ROBOT-T20'
		 ON DUPLICATE KEY UPDATE list_price = VALUES(list_price), specs_json = VALUES(specs_json), status = 'active'`,
		`INSERT INTO product_skus (sku_code, product_id, sku_name, specs_json, list_price, status)
		 SELECT 'SKU-X20PRO-WHITE', id, 'X20 Pro 云石白', JSON_OBJECT('color', 'white'), 4999.00, 'active'
		 FROM products WHERE product_code = 'P-ROBOT-X20PRO'
		 ON DUPLICATE KEY UPDATE list_price = VALUES(list_price), specs_json = VALUES(specs_json), status = 'active'`,
		`INSERT INTO product_skus (sku_code, product_id, sku_name, specs_json, list_price, status)
		 SELECT 'SKU-P400-WHITE', id, 'P400 标准版', JSON_OBJECT('color', 'white'), 2299.00, 'active'
		 FROM products WHERE product_code = 'P-AIR-P400'
		 ON DUPLICATE KEY UPDATE list_price = VALUES(list_price), specs_json = VALUES(specs_json), status = 'active'`,
		`INSERT INTO product_skus (sku_code, product_id, sku_name, specs_json, list_price, status)
		 SELECT 'SKU-F400', id, 'F400 复合滤芯', JSON_OBJECT('pack', 1), 399.00, 'active'
		 FROM products WHERE product_code = 'P-AIR-F400'
		 ON DUPLICATE KEY UPDATE list_price = VALUES(list_price), specs_json = VALUES(specs_json), status = 'active'`,
		`INSERT INTO product_inventory (sku_id, current_price, available_stock, reserved_stock, currency)
		 SELECT id,
		   CASE sku_code
		     WHEN 'SKU-T20-WHITE' THEN 3599.00
		     WHEN 'SKU-X20PRO-WHITE' THEN 4699.00
		     WHEN 'SKU-P400-WHITE' THEN 2099.00
		     ELSE 359.00
		   END,
		   CASE sku_code WHEN 'SKU-X20PRO-WHITE' THEN 8 WHEN 'SKU-F400' THEN 36 ELSE 18 END,
		   0, 'CNY'
		 FROM product_skus
		 WHERE sku_code IN ('SKU-T20-WHITE', 'SKU-X20PRO-WHITE', 'SKU-P400-WHITE', 'SKU-F400')
		 ON DUPLICATE KEY UPDATE
		   current_price = VALUES(current_price), available_stock = VALUES(available_stock),
		   reserved_stock = VALUES(reserved_stock), currency = VALUES(currency)`,
		`INSERT INTO coupons (coupon_code, name, discount_type, discount_value, start_at, end_at, status)
		 VALUES ('CLEAN100', '清洁电器满减券', 'amount', 100.00, '2026-01-01', '2027-01-01', 'active')
		 ON DUPLICATE KEY UPDATE name = VALUES(name), discount_value = VALUES(discount_value),
		   start_at = VALUES(start_at), end_at = VALUES(end_at), status = 'active'`,
		`INSERT INTO user_coupons (user_id, coupon_id, status, claimed_at)
		 SELECT u.id, c.id, 'available', '2026-06-01'
		 FROM users u JOIN coupons c ON c.coupon_code = 'CLEAN100'
		 WHERE u.user_no = 'demo-user'
		 ON DUPLICATE KEY UPDATE status = 'available'`,
		`INSERT INTO orders (order_no, user_id, status, total_amount, paid_at, delivered_at, created_at)
		 SELECT 'CC20260603001', id, 'delivered', 2099.00, '2026-06-03 10:00:00', '2026-06-05 15:00:00', '2026-06-03 09:55:00'
		 FROM users WHERE user_no = 'demo-user'
		 ON DUPLICATE KEY UPDATE status = VALUES(status), total_amount = VALUES(total_amount),
		   paid_at = VALUES(paid_at), delivered_at = VALUES(delivered_at)`,
		`INSERT INTO orders (order_no, user_id, status, total_amount, paid_at, delivered_at, created_at)
		 SELECT 'CC20250522008', id, 'delivered', 3699.00, '2025-05-22 11:00:00', '2025-05-25 16:00:00', '2025-05-22 10:50:00'
		 FROM users WHERE user_no = 'demo-user'
		 ON DUPLICATE KEY UPDATE status = VALUES(status), total_amount = VALUES(total_amount),
		   paid_at = VALUES(paid_at), delivered_at = VALUES(delivered_at)`,
		`INSERT INTO order_items (order_id, product_id, sku_id, quantity, unit_price, warranty_months)
		 SELECT o.id, p.id, s.id, 1, 2099.00, 12
		 FROM orders o
		 JOIN products p ON p.product_code = 'P-AIR-P400'
		 JOIN product_skus s ON s.sku_code = 'SKU-P400-WHITE'
		 WHERE o.order_no = 'CC20260603001'
		   AND NOT EXISTS (SELECT 1 FROM order_items oi WHERE oi.order_id = o.id AND oi.sku_id = s.id)`,
		`INSERT INTO order_items (order_id, product_id, sku_id, quantity, unit_price, warranty_months)
		 SELECT o.id, p.id, s.id, 1, 3699.00, 12
		 FROM orders o
		 JOIN products p ON p.product_code = 'P-ROBOT-T20'
		 JOIN product_skus s ON s.sku_code = 'SKU-T20-WHITE'
		 WHERE o.order_no = 'CC20250522008'
		   AND NOT EXISTS (SELECT 1 FROM order_items oi WHERE oi.order_id = o.id AND oi.sku_id = s.id)`,
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin mock seed: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for index, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("execute mock seed statement %d: %w", index+1, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit mock seed: %w", err)
	}
	return nil
}
