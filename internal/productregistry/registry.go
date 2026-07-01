package productregistry

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

type Product struct {
	ProductCode string
	Model       string
	Category    string
	Brand       string
	EntityType  string
	Aliases     []string
}

type Document struct {
	DocID    string
	Title    string
	Content  string
	Category string
	Brand    string
	DocType  string
	Metadata map[string]any
}

type Registry struct {
	productsByModel map[string]Product
	aliases         []aliasEntry
}

type aliasEntry struct {
	key        string
	display    string
	product    Product
	entityRank int
}

const (
	EntityProductModel   = "product_model"
	EntityHostModel      = "host_model"
	EntityAccessoryModel = "accessory_model"
	EntitySKU            = "sku"
	EntityAlias          = "alias"
)

func New(products []Product) *Registry {
	registry := &Registry{
		productsByModel: make(map[string]Product, len(products)),
	}
	aliasIndexes := map[string]int{}
	for _, product := range products {
		product = normalizeProduct(product)
		if product.Model == "" {
			continue
		}
		modelKey := modelKey(product.Model)
		registry.productsByModel[modelKey] = product
		for _, alias := range append([]string{product.Model, product.ProductCode}, product.Aliases...) {
			alias = strings.TrimSpace(alias)
			if alias == "" {
				continue
			}
			key := aliasKey(alias)
			if key == "" {
				continue
			}
			entry := aliasEntry{
				key:        key,
				display:    alias,
				product:    product,
				entityRank: entityRank(product.EntityType),
			}
			if existingIndex, exists := aliasIndexes[key]; exists {
				if betterAliasEntry(entry, registry.aliases[existingIndex]) {
					registry.aliases[existingIndex] = entry
				}
				continue
			}
			aliasIndexes[key] = len(registry.aliases)
			registry.aliases = append(registry.aliases, entry)
		}
	}
	sort.SliceStable(registry.aliases, func(i, j int) bool {
		left := registry.aliases[i]
		right := registry.aliases[j]
		if len([]rune(left.key)) != len([]rune(right.key)) {
			return len([]rune(left.key)) > len([]rune(right.key))
		}
		if left.entityRank != right.entityRank {
			return left.entityRank < right.entityRank
		}
		return modelKey(left.product.Model) < modelKey(right.product.Model)
	})
	return registry
}

func (r *Registry) Empty() bool {
	return r == nil || (len(r.productsByModel) == 0 && len(r.aliases) == 0)
}

func (r *Registry) MatchModels(text string) []string {
	if r.Empty() || strings.TrimSpace(text) == "" {
		return nil
	}
	normalized := aliasKey(text)
	seen := map[string]struct{}{}
	var matches []modelMatch
	for _, alias := range r.aliases {
		index := findAlias(normalized, alias.key)
		if index < 0 {
			continue
		}
		model := alias.product.Model
		key := modelKey(model)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		matches = append(matches, modelMatch{
			model:      model,
			index:      index,
			entityRank: entityRank(alias.product.EntityType),
		})
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].entityRank != matches[j].entityRank {
			return matches[i].entityRank < matches[j].entityRank
		}
		if matches[i].index != matches[j].index {
			return matches[i].index < matches[j].index
		}
		return modelKey(matches[i].model) < modelKey(matches[j].model)
	})
	result := make([]string, 0, len(matches))
	for _, match := range matches {
		result = append(result, match.model)
	}
	return result
}

func (r *Registry) CategoryForModel(model string) string {
	if r.Empty() {
		return ""
	}
	product := r.productsByModel[modelKey(model)]
	return product.Category
}

func (r *Registry) CategoriesForModels(models string) []string {
	if r.Empty() {
		return nil
	}
	seen := map[string]struct{}{}
	var categories []string
	for _, model := range strings.Split(models, ",") {
		category := strings.TrimSpace(r.CategoryForModel(model))
		if category == "" {
			continue
		}
		if _, exists := seen[category]; exists {
			continue
		}
		seen[category] = struct{}{}
		categories = append(categories, category)
	}
	return categories
}

func (r *Registry) Products() []Product {
	if r.Empty() {
		return nil
	}
	products := make([]Product, 0, len(r.productsByModel))
	for _, product := range r.productsByModel {
		products = append(products, product)
	}
	sort.SliceStable(products, func(i, j int) bool {
		return products[i].Model < products[j].Model
	})
	return products
}

func ProductsFromDocuments(documents []Document) []Product {
	var products []Product
	for _, document := range documents {
		candidates := documentModelCandidates(document)
		if len(candidates) == 0 {
			continue
		}
		category := firstMetadataString(document.Metadata, "category", "product_category")
		if category == "" {
			category = document.Category
		}
		brand := firstMetadataString(document.Metadata, "brand", "product_brand")
		if brand == "" {
			brand = document.Brand
		}
		productCode := firstMetadataString(document.Metadata, "product_code", "product_id")
		productAliases := documentAliases(document)
		accessoryAliases := metadataStrings(document.Metadata, "accessory_alias", "accessory_aliases")
		for _, candidate := range candidates {
			code := productCode
			if code == "" {
				code = "KB-" + strings.ToUpper(strings.ReplaceAll(candidate.model, " ", "-"))
			}
			candidateCategory := category
			if candidate.entityType == EntityHostModel && strings.EqualFold(candidateCategory, "accessory") {
				candidateCategory = ""
			}
			var aliases []string
			switch candidate.entityType {
			case EntityProductModel:
				aliases = productAliases
			case EntityAccessoryModel:
				aliases = accessoryAliases
			}
			products = append(products, Product{
				ProductCode: code,
				Model:       candidate.model,
				Category:    candidateCategory,
				Brand:       brand,
				EntityType:  candidate.entityType,
				Aliases:     aliases,
			})
		}
	}
	return products
}

type modelMatch struct {
	model      string
	index      int
	entityRank int
}

type modelCandidate struct {
	model      string
	entityType string
	explicit   bool
}

func normalizeProduct(product Product) Product {
	product.ProductCode = strings.TrimSpace(product.ProductCode)
	product.Model = strings.TrimSpace(product.Model)
	product.Category = strings.TrimSpace(product.Category)
	product.Brand = strings.TrimSpace(product.Brand)
	product.EntityType = normalizeEntityType(product.EntityType)
	product.Aliases = compactStrings(product.Aliases)
	return product
}

func normalizeEntityType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case EntityProductModel, "product", "model":
		return EntityProductModel
	case EntityHostModel, "host":
		return EntityHostModel
	case EntityAccessoryModel, "accessory":
		return EntityAccessoryModel
	case EntitySKU:
		return EntitySKU
	case EntityAlias:
		return EntityAlias
	default:
		return EntityProductModel
	}
}

func entityRank(entityType string) int {
	switch normalizeEntityType(entityType) {
	case EntityProductModel:
		return 0
	case EntityHostModel:
		return 1
	case EntityAccessoryModel:
		return 2
	case EntitySKU:
		return 3
	case EntityAlias:
		return 4
	default:
		return 5
	}
}

func betterAliasEntry(candidate, current aliasEntry) bool {
	if candidate.entityRank != current.entityRank {
		return candidate.entityRank < current.entityRank
	}
	return modelKey(candidate.product.Model) < modelKey(current.product.Model)
}

func modelKey(value string) string {
	return strings.ToUpper(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

func aliasKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.Join(strings.Fields(value), " ")
	return value
}

func findAlias(text, alias string) int {
	if text == "" || alias == "" {
		return -1
	}
	index := strings.Index(text, alias)
	for index >= 0 {
		if aliasNeedsBoundary(alias) && !hasAliasBoundary(text, index, index+len(alias)) {
			next := strings.Index(text[index+1:], alias)
			if next < 0 {
				return -1
			}
			index += next + 1
			continue
		}
		return index
	}
	return -1
}

var asciiModelPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9 _.-]*$`)

var documentModelPattern = regexp.MustCompile(`(?i)\b[A-Z][A-Z0-9]{0,8}(?:[ -]?[A-Z0-9]{1,8})?(?:-[A-Z0-9]{1,8})?\b`)

func aliasNeedsBoundary(alias string) bool {
	return asciiModelPattern.MatchString(alias)
}

func hasAliasBoundary(text string, start, end int) bool {
	beforeOK := start == 0 || !isModelRune(rune(text[start-1]))
	afterOK := end >= len(text) || !isModelRune(rune(text[end]))
	return beforeOK && afterOK
}

func isModelRune(current rune) bool {
	return unicode.IsLetter(current) || unicode.IsDigit(current) || current == '-' || current == '_'
}

func compactStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

func documentModelCandidates(document Document) []modelCandidate {
	var candidates []modelCandidate
	for _, model := range metadataStrings(document.Metadata, "model", "models", "product_model", "product_models") {
		candidates = append(candidates, modelCandidate{model: model, entityType: EntityProductModel, explicit: true})
	}
	for _, model := range metadataStrings(document.Metadata, "accessory_model", "accessory_models") {
		candidates = append(candidates, modelCandidate{model: model, entityType: EntityAccessoryModel, explicit: true})
	}
	candidates = append(candidates, labeledModelCandidates(document.Title, document)...)
	candidates = append(candidates, labeledModelCandidates(document.Content, document)...)
	for _, model := range modelTokens(document.Title) {
		candidates = append(candidates, modelCandidate{
			model:      model,
			entityType: defaultDocumentEntityType(document),
		})
	}
	return compactCandidates(candidates)
}

func documentAliases(document Document) []string {
	values := []string{document.Title, document.DocID}
	values = append(values, metadataStrings(document.Metadata, "alias", "aliases", "product_name", "name", "sku_code", "sku_name")...)
	return compactStrings(values)
}

func labeledModelCandidates(value string, document Document) []modelCandidate {
	var candidates []modelCandidate
	for _, line := range strings.Split(value, "\n") {
		lower := strings.ToLower(line)
		if !strings.Contains(line, "型号") &&
			!strings.Contains(line, "适配主机") &&
			!strings.Contains(line, "蝙句捷") &&
			!strings.Contains(lower, "model") {
			continue
		}
		for _, model := range modelTokens(line) {
			candidates = append(candidates, modelCandidate{
				model:      model,
				entityType: entityTypeForLabeledModel(line, model, document),
			})
		}
	}
	return candidates
}

func modelTokens(value string) []string {
	var result []string
	for _, token := range documentModelPattern.FindAllString(value, -1) {
		token = strings.TrimSpace(strings.Join(strings.Fields(token), " "))
		if token == "" || !plausibleDocumentModel(token) {
			continue
		}
		result = append(result, token)
	}
	return result
}

func plausibleDocumentModel(token string) bool {
	upper := strings.ToUpper(strings.TrimSpace(token))
	if len([]rune(upper)) < 2 || len([]rune(upper)) > 18 {
		return false
	}
	if strings.HasPrefix(upper, "HTTP") || strings.HasPrefix(upper, "SKU") || strings.HasPrefix(upper, "DOC") || strings.HasPrefix(upper, "CC") {
		return false
	}
	digits := 0
	letters := 0
	for _, char := range upper {
		switch {
		case unicode.IsDigit(char):
			digits++
		case unicode.IsLetter(char):
			letters++
		case char == '-' || char == ' ' || char == '_' || char == '.':
		default:
			return false
		}
	}
	return digits > 0 && letters > 0 && digits <= 8
}

func firstMetadataString(metadata map[string]any, keys ...string) string {
	for _, key := range keys {
		for _, value := range metadataStrings(metadata, key) {
			return value
		}
	}
	return ""
}

func metadataStrings(metadata map[string]any, keys ...string) []string {
	if len(metadata) == 0 {
		return nil
	}
	var values []string
	for _, key := range keys {
		value, ok := metadataLookup(metadata, key)
		if !ok {
			continue
		}
		values = append(values, anyStrings(value)...)
	}
	return compactStrings(values)
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

func anyStrings(value any) []string {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		return splitAliasList(typed)
	case []string:
		return typed
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			values = append(values, anyStrings(item)...)
		}
		return values
	default:
		text := strings.TrimSpace(fmt.Sprint(typed))
		if text != "" {
			return []string{text}
		}
		return nil
	}
}

func splitAliasList(value string) []string {
	parts := strings.FieldsFunc(value, func(current rune) bool {
		return current == ',' || current == ';' || current == '，' || current == '、' || current == '/'
	})
	if len(parts) == 0 {
		return []string{value}
	}
	return parts
}

func defaultDocumentEntityType(document Document) string {
	category := strings.ToLower(strings.TrimSpace(firstNonEmpty(
		firstMetadataString(document.Metadata, "category", "product_category"),
		document.Category,
	)))
	docType := strings.ToLower(strings.TrimSpace(document.DocType))
	if category == "accessory" || strings.Contains(category, "配件") || strings.Contains(docType, "accessory") {
		return EntityAccessoryModel
	}
	return EntityProductModel
}

func entityTypeForLabeledModel(line, model string, document Document) string {
	lower := strings.ToLower(line)
	for _, accessoryModel := range metadataStrings(document.Metadata, "accessory_model", "accessory_models") {
		if strings.EqualFold(strings.TrimSpace(accessoryModel), strings.TrimSpace(model)) {
			return EntityAccessoryModel
		}
	}
	hasHostCue := strings.Contains(line, "适配主机") ||
		strings.Contains(line, "主机") ||
		strings.Contains(lower, "host") ||
		strings.Contains(line, "主机") ||
		strings.Contains(line, "适配主机") ||
		strings.Contains(line, "适用主机")
	hasAccessoryCue := strings.Contains(line, "配件") ||
		strings.Contains(line, "替换") ||
		strings.Contains(line, "耗材") ||
		strings.Contains(line, "掸套") ||
		strings.Contains(lower, "accessory") ||
		strings.Contains(line, "配件") ||
		strings.Contains(line, "替换") ||
		strings.Contains(line, "刷头") ||
		strings.Contains(line, "喷头") ||
		strings.Contains(line, "掸套") ||
		strings.Contains(line, "刮条")
	switch {
	case hasHostCue && !hasAccessoryCue:
		return EntityHostModel
	case hasAccessoryCue && strings.Contains(model, "-"):
		return EntityAccessoryModel
	case hasAccessoryCue && !hasHostCue:
		return EntityAccessoryModel
	default:
		return defaultDocumentEntityType(document)
	}
}

func compactCandidates(values []modelCandidate) []modelCandidate {
	seen := map[string]int{}
	result := make([]modelCandidate, 0, len(values))
	for _, value := range values {
		value.model = strings.TrimSpace(value.model)
		value.entityType = normalizeEntityType(value.entityType)
		if value.model == "" {
			continue
		}
		key := modelKey(value.model)
		if existingIndex, exists := seen[key]; exists {
			current := result[existingIndex]
			if entityRank(value.entityType) < entityRank(current.entityType) ||
				(entityRank(value.entityType) == entityRank(current.entityType) && value.explicit && !current.explicit) {
				result[existingIndex] = value
			}
			continue
		}
		seen[key] = len(result)
		result = append(result, value)
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
