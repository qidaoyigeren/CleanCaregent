package productpack

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"CleanCaregent/internal/compatibility"
	"CleanCaregent/internal/diagnosis"
	"CleanCaregent/internal/ingest"
	"CleanCaregent/internal/productregistry"
	"CleanCaregent/internal/service"

	"go.yaml.in/yaml/v3"
)

var ErrInvalidProductPack = errors.New("invalid product pack")

type Pack struct {
	PackID         string                 `json:"pack_id" yaml:"pack_id"`
	Version        string                 `json:"version" yaml:"version"`
	Products       []ProductSpec          `json:"products" yaml:"products"`
	Compatibility  []CompatibilitySpec    `json:"compatibility" yaml:"compatibility"`
	Diagnosis      []DiagnosisNodeSpec    `json:"diagnosis" yaml:"diagnosis"`
	SafetyKeywords []string               `json:"safety_keywords" yaml:"safety_keywords"`
	Documents      []KnowledgeDocumentRef `json:"documents" yaml:"documents"`
	Metadata       map[string]any         `json:"metadata" yaml:"metadata"`
	SourcePath     string                 `json:"-" yaml:"-"`
}

type ProductSpec struct {
	ProductCode string         `json:"product_code" yaml:"product_code"`
	Name        string         `json:"name" yaml:"name"`
	Category    string         `json:"category" yaml:"category"`
	Brand       string         `json:"brand" yaml:"brand"`
	Model       string         `json:"model" yaml:"model"`
	Aliases     []string       `json:"aliases" yaml:"aliases"`
	Status      string         `json:"status" yaml:"status"`
	Attributes  map[string]any `json:"attributes" yaml:"attributes"`
	SKUs        []SKUSpec      `json:"skus" yaml:"skus"`
}

type SKUSpec struct {
	SKUCode           string         `json:"sku_code" yaml:"sku_code"`
	SKUName           string         `json:"sku_name" yaml:"sku_name"`
	Aliases           []string       `json:"aliases" yaml:"aliases"`
	Status            string         `json:"status" yaml:"status"`
	Specs             map[string]any `json:"specs" yaml:"specs"`
	ListPriceCents    int64          `json:"list_price_cents" yaml:"list_price_cents"`
	CurrentPriceCents int64          `json:"current_price_cents" yaml:"current_price_cents"`
	Currency          string         `json:"currency" yaml:"currency"`
	AvailableStock    int            `json:"available_stock" yaml:"available_stock"`
	ReservedStock     int            `json:"reserved_stock" yaml:"reserved_stock"`
	WarrantyMonths    int            `json:"warranty_months" yaml:"warranty_months"`
}

type CompatibilitySpec struct {
	HostModel      string               `json:"host_model" yaml:"host_model"`
	AccessoryModel string               `json:"accessory_model" yaml:"accessory_model"`
	AccessoryType  string               `json:"accessory_type" yaml:"accessory_type"`
	Status         compatibility.Status `json:"status" yaml:"status"`
	Reason         string               `json:"reason" yaml:"reason"`
	EvidenceDocID  string               `json:"evidence_doc_id" yaml:"evidence_doc_id"`
}

type DiagnosisNodeSpec struct {
	ID            string                `json:"id" yaml:"id"`
	ProductModel  string                `json:"product_model" yaml:"product_model"`
	Symptom       string                `json:"symptom" yaml:"symptom"`
	Question      string                `json:"question" yaml:"question"`
	Guidance      string                `json:"guidance" yaml:"guidance"`
	YesNext       string                `json:"yes_next" yaml:"yes_next"`
	NoNext        string                `json:"no_next" yaml:"no_next"`
	Resolution    string                `json:"resolution" yaml:"resolution"`
	Terminal      bool                  `json:"terminal" yaml:"terminal"`
	NeedHuman     bool                  `json:"need_human" yaml:"need_human"`
	SafetyLevel   diagnosis.SafetyLevel `json:"safety_level" yaml:"safety_level"`
	EvidenceDocID string                `json:"evidence_doc_id" yaml:"evidence_doc_id"`
	Root          bool                  `json:"root" yaml:"root"`
}

type KnowledgeDocumentRef struct {
	ingest.KnowledgePackDocument `json:",inline" yaml:",inline"`
}

type DiagnosisData struct {
	Nodes          []diagnosis.Node
	Roots          map[string]string
	SafetyKeywords []string
}

func Load(paths ...string) ([]Pack, error) {
	var packs []Pack
	for _, path := range compactStrings(paths) {
		loaded, err := loadPath(path)
		if err != nil {
			return nil, err
		}
		packs = append(packs, loaded...)
	}
	return normalizePacks(packs), nil
}

func Validate(packs []Pack) []error {
	var errs []error
	seenPack := map[string]string{}
	seenProduct := map[string]string{}
	seenSKU := map[string]string{}
	seenModel := map[string]string{}
	seenDocument := map[string]string{}

	for _, pack := range normalizePacks(packs) {
		source := pack.SourcePath
		if source == "" {
			source = pack.PackID
		}
		if pack.PackID == "" {
			errs = append(errs, fmt.Errorf("%w: %s pack_id is required", ErrInvalidProductPack, source))
		} else if previous := seenPack[pack.PackID]; previous != "" {
			errs = append(errs, fmt.Errorf("%w: duplicate pack_id %s in %s and %s", ErrInvalidProductPack, pack.PackID, previous, source))
		} else {
			seenPack[pack.PackID] = source
		}
		if pack.Version == "" {
			errs = append(errs, fmt.Errorf("%w: %s version is required", ErrInvalidProductPack, source))
		}
		for _, product := range pack.Products {
			errs = append(errs, validateProduct(pack, product, seenProduct, seenModel)...)
			for _, sku := range product.SKUs {
				errs = append(errs, validateSKU(pack, product, sku, seenSKU)...)
			}
		}
		for _, entry := range pack.Compatibility {
			errs = append(errs, validateCompatibility(pack, entry)...)
		}
		errs = append(errs, validateDiagnosis(pack)...)
		for _, document := range pack.Documents {
			doc := document.KnowledgePackDocument
			version := strings.TrimSpace(doc.Version)
			if version == "" {
				version = "kb-v1"
			}
			if strings.TrimSpace(doc.DocID) == "" {
				errs = append(errs, fmt.Errorf("%w: %s document doc_id is required", ErrInvalidProductPack, source))
				continue
			}
			key := strings.TrimSpace(doc.DocID) + "@" + version
			if previous := seenDocument[key]; previous != "" {
				errs = append(errs, fmt.Errorf("%w: duplicate document %s in %s and %s", ErrInvalidProductPack, key, previous, source))
			} else {
				seenDocument[key] = source
			}
		}
	}
	return errs
}

func Registry(packs []Pack) *productregistry.Registry {
	var products []productregistry.Product
	for _, pack := range normalizePacks(packs) {
		for _, product := range pack.Products {
			aliases := append([]string(nil), product.Aliases...)
			aliases = append(aliases, product.Name)
			for _, sku := range product.SKUs {
				aliases = append(aliases, sku.SKUCode, sku.SKUName)
				aliases = append(aliases, sku.Aliases...)
			}
			products = append(products, productregistry.Product{
				ProductCode: product.ProductCode,
				Model:       product.Model,
				Category:    product.Category,
				Brand:       product.Brand,
				EntityType:  productregistry.EntityProductModel,
				Aliases:     aliases,
			})
		}
	}
	return productregistry.New(products)
}

func CompatibilityEntries(packs []Pack) []compatibility.Entry {
	var entries []compatibility.Entry
	for _, pack := range normalizePacks(packs) {
		for _, item := range pack.Compatibility {
			entries = append(entries, compatibility.Entry{
				HostModel:      item.HostModel,
				AccessoryModel: item.AccessoryModel,
				AccessoryType:  item.AccessoryType,
				Status:         item.Status,
				Reason:         item.Reason,
				EvidenceDocID:  item.EvidenceDocID,
			})
		}
	}
	return entries
}

func DiagnosisEntries(packs []Pack) DiagnosisData {
	data := DiagnosisData{Roots: map[string]string{}}
	for _, pack := range normalizePacks(packs) {
		data.SafetyKeywords = append(data.SafetyKeywords, pack.SafetyKeywords...)
		parentIDs := map[string]struct{}{}
		for _, item := range pack.Diagnosis {
			if item.YesNext != "" {
				parentIDs[item.YesNext] = struct{}{}
			}
			if item.NoNext != "" {
				parentIDs[item.NoNext] = struct{}{}
			}
		}
		for _, item := range pack.Diagnosis {
			node := diagnosis.Node{
				ID:            item.ID,
				ProductModel:  item.ProductModel,
				Symptom:       item.Symptom,
				Question:      item.Question,
				Guidance:      item.Guidance,
				YesNext:       item.YesNext,
				NoNext:        item.NoNext,
				Resolution:    item.Resolution,
				Terminal:      item.Terminal,
				NeedHuman:     item.NeedHuman,
				SafetyLevel:   item.SafetyLevel,
				EvidenceDocID: item.EvidenceDocID,
			}
			data.Nodes = append(data.Nodes, node)
			rootKey := strings.ToLower(strings.TrimSpace(item.ProductModel)) + "|" + strings.TrimSpace(item.Symptom)
			if item.Root {
				data.Roots[rootKey] = item.ID
				continue
			}
			if _, isChild := parentIDs[item.ID]; !isChild {
				if _, exists := data.Roots[rootKey]; !exists {
					data.Roots[rootKey] = item.ID
				}
			}
		}
	}
	data.SafetyKeywords = compactStrings(data.SafetyKeywords)
	return data
}

func KnowledgeDocuments(packs []Pack) ([]service.IngestDocumentRequest, error) {
	var documents []service.IngestDocumentRequest
	for _, pack := range normalizePacks(packs) {
		for _, document := range pack.Documents {
			request, err := ingest.BuildKnowledgeDocument(document.KnowledgePackDocument, pack.SourcePath)
			if err != nil {
				return nil, err
			}
			documents = append(documents, request)
		}
	}
	return documents, nil
}

func loadPath(path string) ([]Pack, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("%w: stat %s: %w", ErrInvalidProductPack, path, err)
	}
	if !info.IsDir() {
		return loadFile(path)
	}
	var packs []Pack
	if err := filepath.WalkDir(path, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if !isPackFile(current) {
			return nil
		}
		loaded, err := loadFile(current)
		if err != nil {
			return err
		}
		packs = append(packs, loaded...)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("%w: walk %s: %w", ErrInvalidProductPack, path, err)
	}
	return packs, nil
}

func loadFile(path string) ([]Pack, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: read %s: %w", ErrInvalidProductPack, path, err)
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, fmt.Errorf("%w: empty file %s", ErrInvalidProductPack, path)
	}
	var packs []Pack
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		if raw[0] == '[' {
			if err := json.Unmarshal(raw, &packs); err != nil {
				return nil, fmt.Errorf("%w: decode %s: %w", ErrInvalidProductPack, path, err)
			}
		} else {
			var pack Pack
			if err := json.Unmarshal(raw, &pack); err != nil {
				return nil, fmt.Errorf("%w: decode %s: %w", ErrInvalidProductPack, path, err)
			}
			packs = append(packs, pack)
		}
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(raw, &packs); err != nil {
			var pack Pack
			if err := yaml.Unmarshal(raw, &pack); err != nil {
				return nil, fmt.Errorf("%w: decode %s: %w", ErrInvalidProductPack, path, err)
			}
			packs = append(packs, pack)
		}
	default:
		return nil, fmt.Errorf("%w: unsupported product pack file %s", ErrInvalidProductPack, path)
	}
	for index := range packs {
		packs[index].SourcePath = path
	}
	return packs, nil
}

func isPackFile(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	if strings.HasPrefix(base, "readme.") || strings.HasPrefix(base, "_") {
		return false
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json", ".yaml", ".yml":
		return true
	default:
		return false
	}
}

func normalizePacks(packs []Pack) []Pack {
	result := make([]Pack, len(packs))
	for index, pack := range packs {
		pack.PackID = strings.TrimSpace(pack.PackID)
		pack.Version = strings.TrimSpace(pack.Version)
		pack.SourcePath = strings.TrimSpace(pack.SourcePath)
		pack.SafetyKeywords = compactStrings(pack.SafetyKeywords)
		for productIndex := range pack.Products {
			product := &pack.Products[productIndex]
			product.ProductCode = strings.TrimSpace(product.ProductCode)
			product.Name = strings.TrimSpace(product.Name)
			product.Category = strings.TrimSpace(product.Category)
			product.Brand = strings.TrimSpace(product.Brand)
			product.Model = strings.TrimSpace(product.Model)
			product.Aliases = compactStrings(product.Aliases)
			product.Status = defaultString(product.Status, "active")
			if product.Attributes == nil {
				product.Attributes = map[string]any{}
			}
			for skuIndex := range product.SKUs {
				sku := &product.SKUs[skuIndex]
				sku.SKUCode = strings.TrimSpace(sku.SKUCode)
				sku.SKUName = strings.TrimSpace(sku.SKUName)
				sku.Aliases = compactStrings(sku.Aliases)
				sku.Status = defaultString(sku.Status, "active")
				sku.Currency = strings.ToUpper(defaultString(sku.Currency, "CNY"))
				if sku.SKUName == "" {
					sku.SKUName = product.Name
				}
				if sku.CurrentPriceCents == 0 {
					sku.CurrentPriceCents = sku.ListPriceCents
				}
				if sku.Specs == nil {
					sku.Specs = map[string]any{}
				}
				if sku.WarrantyMonths > 0 {
					sku.Specs["warranty_months"] = sku.WarrantyMonths
				}
			}
		}
		for compatibilityIndex := range pack.Compatibility {
			item := &pack.Compatibility[compatibilityIndex]
			item.HostModel = strings.TrimSpace(item.HostModel)
			item.AccessoryModel = strings.TrimSpace(item.AccessoryModel)
			item.AccessoryType = strings.TrimSpace(item.AccessoryType)
			item.Reason = strings.TrimSpace(item.Reason)
			item.EvidenceDocID = strings.TrimSpace(item.EvidenceDocID)
		}
		for diagnosisIndex := range pack.Diagnosis {
			item := &pack.Diagnosis[diagnosisIndex]
			item.ID = strings.TrimSpace(item.ID)
			item.ProductModel = strings.TrimSpace(item.ProductModel)
			item.Symptom = strings.TrimSpace(item.Symptom)
			item.YesNext = strings.TrimSpace(item.YesNext)
			item.NoNext = strings.TrimSpace(item.NoNext)
			item.EvidenceDocID = strings.TrimSpace(item.EvidenceDocID)
		}
		result[index] = pack
	}
	return result
}

func validateProduct(pack Pack, product ProductSpec, seenProduct, seenModel map[string]string) []error {
	var errs []error
	source := sourceLabel(pack)
	if product.ProductCode == "" {
		errs = append(errs, fmt.Errorf("%w: %s product_code is required", ErrInvalidProductPack, source))
	} else if previous := seenProduct[product.ProductCode]; previous != "" {
		errs = append(errs, fmt.Errorf("%w: duplicate product_code %s in %s and %s", ErrInvalidProductPack, product.ProductCode, previous, source))
	} else {
		seenProduct[product.ProductCode] = source
	}
	if product.Name == "" {
		errs = append(errs, fmt.Errorf("%w: product %s name is required", ErrInvalidProductPack, product.ProductCode))
	}
	if product.Category == "" {
		errs = append(errs, fmt.Errorf("%w: product %s category is required", ErrInvalidProductPack, product.ProductCode))
	}
	if product.Brand == "" {
		errs = append(errs, fmt.Errorf("%w: product %s brand is required", ErrInvalidProductPack, product.ProductCode))
	}
	if product.Model == "" {
		errs = append(errs, fmt.Errorf("%w: product %s model is required", ErrInvalidProductPack, product.ProductCode))
	} else {
		modelKey := strings.ToUpper(strings.Join(strings.Fields(product.Model), " "))
		if previous := seenModel[modelKey]; previous != "" {
			errs = append(errs, fmt.Errorf("%w: duplicate model %s in %s and %s", ErrInvalidProductPack, product.Model, previous, source))
		} else {
			seenModel[modelKey] = source
		}
	}
	if len(product.SKUs) == 0 {
		errs = append(errs, fmt.Errorf("%w: product %s must define at least one sku", ErrInvalidProductPack, product.ProductCode))
	}
	return errs
}

func validateSKU(pack Pack, product ProductSpec, sku SKUSpec, seenSKU map[string]string) []error {
	var errs []error
	source := sourceLabel(pack)
	if sku.SKUCode == "" {
		errs = append(errs, fmt.Errorf("%w: product %s sku_code is required", ErrInvalidProductPack, product.ProductCode))
	} else if previous := seenSKU[sku.SKUCode]; previous != "" {
		errs = append(errs, fmt.Errorf("%w: duplicate sku_code %s in %s and %s", ErrInvalidProductPack, sku.SKUCode, previous, source))
	} else {
		seenSKU[sku.SKUCode] = source
	}
	if sku.SKUName == "" {
		errs = append(errs, fmt.Errorf("%w: sku %s sku_name is required", ErrInvalidProductPack, sku.SKUCode))
	}
	if sku.ListPriceCents < 0 || sku.CurrentPriceCents < 0 {
		errs = append(errs, fmt.Errorf("%w: sku %s price cannot be negative", ErrInvalidProductPack, sku.SKUCode))
	}
	if sku.AvailableStock < 0 || sku.ReservedStock < 0 {
		errs = append(errs, fmt.Errorf("%w: sku %s stock cannot be negative", ErrInvalidProductPack, sku.SKUCode))
	}
	if len(sku.Currency) != 3 {
		errs = append(errs, fmt.Errorf("%w: sku %s currency must be ISO-4217 code", ErrInvalidProductPack, sku.SKUCode))
	}
	return errs
}

func validateCompatibility(pack Pack, item CompatibilitySpec) []error {
	var errs []error
	if item.HostModel == "" || item.AccessoryModel == "" {
		errs = append(errs, fmt.Errorf("%w: %s compatibility host_model and accessory_model are required", ErrInvalidProductPack, sourceLabel(pack)))
	}
	switch item.Status {
	case compatibility.Compatible, compatibility.Incompatible, compatibility.Unknown:
	default:
		errs = append(errs, fmt.Errorf("%w: compatibility %s/%s has invalid status %q", ErrInvalidProductPack, item.HostModel, item.AccessoryModel, item.Status))
	}
	return errs
}

func validateDiagnosis(pack Pack) []error {
	var errs []error
	ids := map[string]DiagnosisNodeSpec{}
	for _, node := range pack.Diagnosis {
		if node.ID == "" {
			errs = append(errs, fmt.Errorf("%w: %s diagnosis node id is required", ErrInvalidProductPack, sourceLabel(pack)))
			continue
		}
		if _, exists := ids[node.ID]; exists {
			errs = append(errs, fmt.Errorf("%w: duplicate diagnosis node id %s", ErrInvalidProductPack, node.ID))
		}
		ids[node.ID] = node
		if node.ProductModel == "" || node.Symptom == "" {
			errs = append(errs, fmt.Errorf("%w: diagnosis node %s product_model and symptom are required", ErrInvalidProductPack, node.ID))
		}
		if node.Question == "" && node.Resolution == "" {
			errs = append(errs, fmt.Errorf("%w: diagnosis node %s needs question or resolution", ErrInvalidProductPack, node.ID))
		}
		if !node.Terminal && (node.YesNext == "" || node.NoNext == "") {
			errs = append(errs, fmt.Errorf("%w: non-terminal diagnosis node %s needs yes_next and no_next", ErrInvalidProductPack, node.ID))
		}
	}
	for _, node := range ids {
		for _, next := range []string{node.YesNext, node.NoNext} {
			if next == "" {
				continue
			}
			if _, exists := ids[next]; !exists {
				errs = append(errs, fmt.Errorf("%w: diagnosis node %s references missing next node %s", ErrInvalidProductPack, node.ID, next))
			}
		}
	}
	return errs
}

func sourceLabel(pack Pack) string {
	if pack.SourcePath != "" {
		return pack.SourcePath
	}
	if pack.PackID != "" {
		return pack.PackID
	}
	return "<memory>"
}

func defaultString(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
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
	sort.Strings(result)
	return result
}
