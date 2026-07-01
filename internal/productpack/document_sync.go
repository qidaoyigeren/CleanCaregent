package productpack

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"CleanCaregent/internal/service"
)

func SyncKnowledgeDocumentBusinessData(
	ctx context.Context,
	db *sql.DB,
	documents []service.IngestDocumentRequest,
) (SyncResult, error) {
	products := BusinessProductsFromKnowledgeDocuments(documents)
	if len(products) == 0 {
		return SyncResult{}, nil
	}
	return SyncBusinessData(ctx, db, []Pack{{
		PackID:   "kb-document-business-sync",
		Version:  "1.0",
		Products: products,
	}})
}

func BusinessProductsFromKnowledgeDocuments(documents []service.IngestDocumentRequest) []ProductSpec {
	type productBucket struct {
		product ProductSpec
		skus    map[string]SKUSpec
	}
	buckets := map[string]*productBucket{}
	order := make([]string, 0, len(documents))
	for _, document := range documents {
		product, sku, ok := businessProductFromDocument(document)
		if !ok {
			continue
		}
		key := product.ProductCode
		bucket, exists := buckets[key]
		if !exists {
			bucket = &productBucket{product: product, skus: map[string]SKUSpec{}}
			buckets[key] = bucket
			order = append(order, key)
		}
		bucket.product = mergeDocumentProduct(bucket.product, product)
		bucket.skus[sku.SKUCode] = sku
	}
	products := make([]ProductSpec, 0, len(order))
	for _, key := range order {
		bucket := buckets[key]
		for _, sku := range bucket.skus {
			bucket.product.SKUs = append(bucket.product.SKUs, sku)
		}
		products = append(products, bucket.product)
	}
	return products
}

func businessProductFromDocument(document service.IngestDocumentRequest) (ProductSpec, SKUSpec, bool) {
	metadata := document.Metadata
	model := firstMetadataValue(metadata, "model", "product_model", "models", "product_models")
	if model == "" {
		return ProductSpec{}, SKUSpec{}, false
	}
	if !hasBusinessMetadata(metadata) {
		return ProductSpec{}, SKUSpec{}, false
	}

	productCode := firstMetadataValue(metadata, "product_code", "product_id")
	if productCode == "" {
		productCode = "KB-" + sanitizeBusinessCode(model)
	}
	name := firstMetadataValue(metadata, "product_name", "name")
	if name == "" {
		name = document.Title
	}
	if name == "" {
		name = model
	}
	category := firstMetadataValue(metadata, "category", "product_category")
	if category == "" {
		category = document.Category
	}
	if category == "" {
		category = "cleaning_tool"
	}
	brand := firstMetadataValue(metadata, "brand", "product_brand")
	if brand == "" {
		brand = document.Brand
	}
	if brand == "" {
		brand = "CleanCare"
	}

	skuCode := firstMetadataValue(metadata, "sku_code", "sku_id")
	if skuCode == "" {
		skuCode = "KB-" + sanitizeBusinessCode(model) + "-SKU"
	}
	listPrice := firstMetadataInt64(metadata, "list_price_cents", "price_cents", "price")
	currentPrice := firstMetadataInt64(metadata, "current_price_cents", "sale_price_cents", "price_cents", "price")
	if currentPrice == 0 {
		currentPrice = listPrice
	}
	if listPrice == 0 {
		listPrice = currentPrice
	}
	skuName := firstMetadataValue(metadata, "sku_name")
	if skuName == "" {
		skuName = name
	}

	attributes := cloneMetadataMap(firstMetadataMap(metadata, "attributes"))
	attributes["source_doc_id"] = document.DocID
	attributes["source_doc_type"] = document.DocType
	specs := cloneMetadataMap(firstMetadataMap(metadata, "specs"))
	specs["source_doc_id"] = document.DocID
	if warranty := firstMetadataInt(metadata, "warranty_months"); warranty > 0 {
		specs["warranty_months"] = warranty
	}

	product := ProductSpec{
		ProductCode: productCode,
		Name:        name,
		Category:    category,
		Brand:       brand,
		Model:       model,
		Aliases:     metadataValues(metadata, "aliases", "alias", "product_name", "sku_name"),
		Status:      defaultString(firstMetadataValue(metadata, "status"), "active"),
		Attributes:  attributes,
	}
	sku := SKUSpec{
		SKUCode:           skuCode,
		SKUName:           skuName,
		Aliases:           metadataValues(metadata, "sku_aliases", "aliases", "alias"),
		Status:            defaultString(firstMetadataValue(metadata, "sku_status", "status"), "active"),
		Specs:             specs,
		ListPriceCents:    listPrice,
		CurrentPriceCents: currentPrice,
		Currency:          defaultString(firstMetadataValue(metadata, "currency"), "CNY"),
		AvailableStock:    firstMetadataInt(metadata, "available_stock", "stock"),
		ReservedStock:     firstMetadataInt(metadata, "reserved_stock"),
		WarrantyMonths:    firstMetadataInt(metadata, "warranty_months"),
	}
	return product, sku, true
}

func hasBusinessMetadata(metadata map[string]any) bool {
	for _, key := range []string{
		"sku_code",
		"sku_id",
		"list_price_cents",
		"current_price_cents",
		"sale_price_cents",
		"price_cents",
		"price",
		"available_stock",
		"stock",
		"reserved_stock",
	} {
		if _, exists := metadataLookup(metadata, key); exists {
			return true
		}
	}
	return false
}

func mergeDocumentProduct(existing, incoming ProductSpec) ProductSpec {
	if incoming.Name != "" {
		existing.Name = incoming.Name
	}
	if incoming.Category != "" {
		existing.Category = incoming.Category
	}
	if incoming.Brand != "" {
		existing.Brand = incoming.Brand
	}
	if incoming.Model != "" {
		existing.Model = incoming.Model
	}
	existing.Aliases = compactStrings(append(existing.Aliases, incoming.Aliases...))
	if existing.Attributes == nil {
		existing.Attributes = map[string]any{}
	}
	for key, value := range incoming.Attributes {
		existing.Attributes[key] = value
	}
	if incoming.Status != "" {
		existing.Status = incoming.Status
	}
	return existing
}

func firstMetadataValue(metadata map[string]any, keys ...string) string {
	for _, value := range metadataValues(metadata, keys...) {
		return value
	}
	return ""
}

func metadataValues(metadata map[string]any, keys ...string) []string {
	if len(metadata) == 0 {
		return nil
	}
	var result []string
	for _, key := range keys {
		value, ok := metadataLookup(metadata, key)
		if !ok {
			continue
		}
		result = append(result, metadataValueStrings(value)...)
	}
	return compactStrings(result)
}

func metadataValueStrings(value any) []string {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		return splitMetadataList(typed)
	case []string:
		return typed
	case []any:
		var result []string
		for _, item := range typed {
			result = append(result, metadataValueStrings(item)...)
		}
		return result
	default:
		return []string{strings.TrimSpace(fmt.Sprint(typed))}
	}
}

func firstMetadataInt64(metadata map[string]any, keys ...string) int64 {
	for _, key := range keys {
		value, ok := metadataLookup(metadata, key)
		if !ok {
			continue
		}
		if converted, ok := metadataInt64(value); ok {
			return converted
		}
	}
	return 0
}

func firstMetadataInt(metadata map[string]any, keys ...string) int {
	value := firstMetadataInt64(metadata, keys...)
	maxInt := int64(^uint(0) >> 1)
	minInt := -maxInt - 1
	if value > maxInt {
		return int(maxInt)
	}
	if value < minInt {
		return int(minInt)
	}
	return int(value)
}

func metadataInt64(value any) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	case int32:
		return int64(typed), true
	case float64:
		return int64(typed), true
	case float32:
		return int64(typed), true
	case json.Number:
		parsed, err := strconv.ParseInt(typed.String(), 10, 64)
		return parsed, err == nil
	case string:
		normalized := strings.TrimSpace(strings.ReplaceAll(typed, ",", ""))
		if normalized == "" {
			return 0, false
		}
		if parsed, err := strconv.ParseInt(normalized, 10, 64); err == nil {
			return parsed, true
		}
		if parsed, err := strconv.ParseFloat(normalized, 64); err == nil {
			return int64(math.Round(parsed * 100)), true
		}
	}
	return 0, false
}

func firstMetadataMap(metadata map[string]any, keys ...string) map[string]any {
	for _, key := range keys {
		value, ok := metadataLookup(metadata, key)
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case map[string]any:
			return typed
		case map[string]string:
			result := make(map[string]any, len(typed))
			for key, value := range typed {
				result[key] = value
			}
			return result
		}
	}
	return nil
}

func metadataLookup(metadata map[string]any, key string) (any, bool) {
	if value, ok := metadata[key]; ok {
		return value, true
	}
	for current, value := range metadata {
		if strings.EqualFold(current, key) {
			return value, true
		}
	}
	return nil, false
}

func cloneMetadataMap(source map[string]any) map[string]any {
	result := make(map[string]any, len(source)+2)
	for key, value := range source {
		result[key] = value
	}
	return result
}

func splitMetadataList(value string) []string {
	parts := strings.FieldsFunc(value, func(current rune) bool {
		return current == ',' || current == ';' || current == '，' || current == '、'
	})
	if len(parts) == 0 {
		return []string{value}
	}
	return parts
}

func sanitizeBusinessCode(value string) string {
	value = strings.ToUpper(strings.Join(strings.Fields(strings.TrimSpace(value)), "-"))
	var builder strings.Builder
	for _, char := range value {
		switch {
		case char >= 'A' && char <= 'Z':
			builder.WriteRune(char)
		case char >= '0' && char <= '9':
			builder.WriteRune(char)
		case char == '-' || char == '_':
			builder.WriteRune('-')
		}
	}
	result := strings.Trim(builder.String(), "-")
	if result == "" {
		return "DOC"
	}
	return result
}
