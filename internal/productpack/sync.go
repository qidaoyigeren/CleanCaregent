package productpack

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

type SyncResult struct {
	Products  int
	SKUs      int
	Inventory int
}

func SyncBusinessData(ctx context.Context, db *sql.DB, packs []Pack) (SyncResult, error) {
	if db == nil {
		return SyncResult{}, fmt.Errorf("%w: mysql db is nil", ErrInvalidProductPack)
	}
	packs = normalizePacks(packs)
	if errs := Validate(packs); len(errs) > 0 {
		return SyncResult{}, errs[0]
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return SyncResult{}, fmt.Errorf("begin product pack sync: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var result SyncResult
	for _, pack := range packs {
		for _, product := range pack.Products {
			if err := upsertProduct(ctx, tx, product); err != nil {
				return SyncResult{}, err
			}
			result.Products++
			for _, sku := range product.SKUs {
				if err := upsertSKU(ctx, tx, product, sku); err != nil {
					return SyncResult{}, err
				}
				result.SKUs++
				if err := upsertInventory(ctx, tx, sku); err != nil {
					return SyncResult{}, err
				}
				result.Inventory++
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return SyncResult{}, fmt.Errorf("commit product pack sync: %w", err)
	}
	return result, nil
}

func upsertProduct(ctx context.Context, tx *sql.Tx, product ProductSpec) error {
	attributes, err := json.Marshal(product.Attributes)
	if err != nil {
		return fmt.Errorf("encode product %s attributes: %w", product.ProductCode, err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO products (
			product_code, name, category, brand, model, attributes_json, status
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			name = VALUES(name),
			category = VALUES(category),
			brand = VALUES(brand),
			model = VALUES(model),
			attributes_json = VALUES(attributes_json),
			status = VALUES(status),
			updated_at = UTC_TIMESTAMP(6)
	`, product.ProductCode, product.Name, product.Category, product.Brand, product.Model, attributes, product.Status)
	if err != nil {
		return fmt.Errorf("upsert product %s: %w", product.ProductCode, err)
	}
	return nil
}

func upsertSKU(ctx context.Context, tx *sql.Tx, product ProductSpec, sku SKUSpec) error {
	specs, err := json.Marshal(sku.Specs)
	if err != nil {
		return fmt.Errorf("encode sku %s specs: %w", sku.SKUCode, err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO product_skus (
			sku_code, product_id, sku_name, specs_json, list_price, status
		)
		SELECT ?, p.id, ?, ?, ?, ?
		FROM products p
		WHERE p.product_code = ?
		ON DUPLICATE KEY UPDATE
			product_id = VALUES(product_id),
			sku_name = VALUES(sku_name),
			specs_json = VALUES(specs_json),
			list_price = VALUES(list_price),
			status = VALUES(status),
			updated_at = UTC_TIMESTAMP(6)
	`, sku.SKUCode, sku.SKUName, specs, centsDecimal(sku.ListPriceCents), sku.Status, product.ProductCode)
	if err != nil {
		return fmt.Errorf("upsert sku %s: %w", sku.SKUCode, err)
	}
	return nil
}

func upsertInventory(ctx context.Context, tx *sql.Tx, sku SKUSpec) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO product_inventory (
			sku_id, current_price, available_stock, reserved_stock, currency
		)
		SELECT s.id, ?, ?, ?, ?
		FROM product_skus s
		WHERE s.sku_code = ?
		ON DUPLICATE KEY UPDATE
			current_price = VALUES(current_price),
			available_stock = VALUES(available_stock),
			reserved_stock = VALUES(reserved_stock),
			currency = VALUES(currency),
			updated_at = UTC_TIMESTAMP(6)
	`, centsDecimal(sku.CurrentPriceCents), sku.AvailableStock, sku.ReservedStock, sku.Currency, sku.SKUCode)
	if err != nil {
		return fmt.Errorf("upsert inventory %s: %w", sku.SKUCode, err)
	}
	return nil
}

func centsDecimal(cents int64) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	return fmt.Sprintf("%s%d.%02d", sign, cents/100, cents%100)
}
