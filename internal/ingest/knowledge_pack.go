package ingest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"CleanCaregent/internal/service"

	"go.yaml.in/yaml/v3"
)

var ErrInvalidKnowledgePack = errors.New("invalid knowledge pack")

type KnowledgePackDocument struct {
	DocID         string         `json:"doc_id" yaml:"doc_id"`
	Title         string         `json:"title" yaml:"title"`
	Content       string         `json:"content" yaml:"content"`
	ContentFile   string         `json:"content_file" yaml:"content_file"`
	ContentFormat string         `json:"content_format" yaml:"content_format"`
	Category      string         `json:"category" yaml:"category"`
	Brand         string         `json:"brand" yaml:"brand"`
	DocType       string         `json:"doc_type" yaml:"doc_type"`
	Version       string         `json:"version" yaml:"version"`
	EffectiveTime *time.Time     `json:"effective_time" yaml:"effective_time"`
	ExpireTime    *time.Time     `json:"expire_time" yaml:"expire_time"`
	Source        string         `json:"source" yaml:"source"`
	IntentTags    []string       `json:"intent_tags" yaml:"intent_tags"`
	Model         string         `json:"model" yaml:"model"`
	Models        []string       `json:"models" yaml:"models"`
	Metadata      map[string]any `json:"metadata" yaml:"metadata"`
}

type loadedKnowledgeDocument struct {
	spec        KnowledgePackDocument
	contentPath string
	sourcePath  string
}

// LoadKnowledgeDocuments reads JSON knowledge packs and Markdown documents
// from files or directories and converts them into normal ingest requests.
func LoadKnowledgeDocuments(paths ...string) ([]service.IngestDocumentRequest, error) {
	var documents []service.IngestDocumentRequest
	for _, rawPath := range compactPathList(paths) {
		loaded, err := loadKnowledgePath(rawPath)
		if err != nil {
			return nil, err
		}
		documents = append(documents, loaded...)
	}
	return documents, nil
}

func loadKnowledgePath(path string) ([]service.IngestDocumentRequest, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("%w: stat %s: %w", ErrInvalidKnowledgePack, path, err)
	}
	if !info.IsDir() {
		return loadKnowledgeFile(path)
	}

	var documents []service.IngestDocumentRequest
	if err := filepath.WalkDir(path, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if !isKnowledgePackFile(current) {
			return nil
		}
		loaded, err := loadKnowledgeFile(current)
		if err != nil {
			return err
		}
		documents = append(documents, loaded...)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("%w: walk %s: %w", ErrInvalidKnowledgePack, path, err)
	}
	return documents, nil
}

func loadKnowledgeFile(path string) ([]service.IngestDocumentRequest, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return loadKnowledgeJSON(path)
	case ".md", ".markdown":
		return loadKnowledgeMarkdown(path)
	default:
		return nil, fmt.Errorf("%w: unsupported knowledge pack file %s", ErrInvalidKnowledgePack, path)
	}
}

func loadKnowledgeJSON(path string) ([]service.IngestDocumentRequest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: read %s: %w", ErrInvalidKnowledgePack, path, err)
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, fmt.Errorf("%w: empty JSON file %s", ErrInvalidKnowledgePack, path)
	}

	var specs []KnowledgePackDocument
	if raw[0] == '[' {
		if err := json.Unmarshal(raw, &specs); err != nil {
			return nil, fmt.Errorf("%w: decode %s: %w", ErrInvalidKnowledgePack, path, err)
		}
	} else {
		var spec KnowledgePackDocument
		if err := json.Unmarshal(raw, &spec); err != nil {
			return nil, fmt.Errorf("%w: decode %s: %w", ErrInvalidKnowledgePack, path, err)
		}
		specs = append(specs, spec)
	}

	requests := make([]service.IngestDocumentRequest, 0, len(specs))
	for _, spec := range specs {
		request, err := buildIngestRequest(loadedKnowledgeDocument{spec: spec, sourcePath: path})
		if err != nil {
			return nil, err
		}
		requests = append(requests, request)
	}
	return requests, nil
}

func loadKnowledgeMarkdown(path string) ([]service.IngestDocumentRequest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: read %s: %w", ErrInvalidKnowledgePack, path, err)
	}
	frontMatter, body := splitFrontMatter(string(raw))
	var spec KnowledgePackDocument
	if strings.TrimSpace(frontMatter) != "" {
		if err := yaml.Unmarshal([]byte(frontMatter), &spec); err != nil {
			return nil, fmt.Errorf("%w: decode front matter %s: %w", ErrInvalidKnowledgePack, path, err)
		}
	}
	spec.Content = strings.TrimSpace(body)
	if spec.Title == "" {
		spec.Title = firstMarkdownHeading(spec.Content)
	}
	request, err := buildIngestRequest(loadedKnowledgeDocument{spec: spec, sourcePath: path})
	if err != nil {
		return nil, err
	}
	return []service.IngestDocumentRequest{request}, nil
}

func BuildKnowledgeDocument(
	spec KnowledgePackDocument,
	sourcePath string,
) (service.IngestDocumentRequest, error) {
	return buildIngestRequest(loadedKnowledgeDocument{spec: spec, sourcePath: sourcePath})
}

func buildIngestRequest(loaded loadedKnowledgeDocument) (service.IngestDocumentRequest, error) {
	spec := loaded.spec
	sourcePath := loaded.sourcePath
	if spec.DocID == "" {
		spec.DocID = "kb_" + slugFilename(strings.TrimSuffix(filepath.Base(sourcePath), filepath.Ext(sourcePath)))
	}
	if spec.Title == "" {
		spec.Title = strings.TrimSuffix(filepath.Base(sourcePath), filepath.Ext(sourcePath))
	}
	if spec.Category == "" {
		spec.Category = "cleaning_tool"
	}
	if spec.DocType == "" {
		spec.DocType = "product_detail"
	}
	if spec.Version == "" {
		spec.Version = "kb-v1"
	}
	if spec.ContentFormat == "" && spec.ContentFile == "" {
		spec.ContentFormat = contentFormatForPath(sourcePath)
	}
	content, err := resolveKnowledgeContent(spec, sourcePath)
	if err != nil {
		return service.IngestDocumentRequest{}, err
	}
	spec.Content = content
	if spec.ContentFormat == "" {
		spec.ContentFormat = contentFormatForPath(sourcePath)
		if spec.ContentFile != "" {
			spec.ContentFormat = FormatFromFilename(spec.ContentFile)
		}
	}
	if spec.Source == "" {
		spec.Source = fileSource(sourcePath)
	}
	if len(spec.IntentTags) == 0 {
		spec.IntentTags = defaultIntentTagsForDocType(spec.DocType)
	}
	metadata := cloneKnowledgeMetadata(spec.Metadata)
	metadata["content_format"] = spec.ContentFormat
	metadata["source_path"] = filepath.ToSlash(sourcePath)
	if spec.Model != "" {
		metadata["model"] = strings.TrimSpace(spec.Model)
	}
	if models := compactPathList(spec.Models); len(models) > 0 {
		metadata["models"] = models
	}

	return service.IngestDocumentRequest{
		DocID:         spec.DocID,
		Title:         spec.Title,
		Content:       spec.Content,
		Category:      spec.Category,
		Brand:         spec.Brand,
		DocType:       spec.DocType,
		Version:       spec.Version,
		EffectiveTime: spec.EffectiveTime,
		ExpireTime:    spec.ExpireTime,
		Source:        spec.Source,
		IntentTags:    spec.IntentTags,
		Metadata:      metadata,
	}, nil
}

func resolveKnowledgeContent(spec KnowledgePackDocument, sourcePath string) (string, error) {
	if spec.ContentFile != "" {
		contentPath := spec.ContentFile
		if !filepath.IsAbs(contentPath) {
			contentPath = filepath.Join(filepath.Dir(sourcePath), contentPath)
		}
		file, err := os.Open(contentPath)
		if err != nil {
			return "", fmt.Errorf("%w: open content_file %s: %w", ErrInvalidKnowledgePack, contentPath, err)
		}
		defer file.Close()
		format := strings.TrimSpace(spec.ContentFormat)
		if format == "" {
			format = FormatFromFilename(contentPath)
		}
		content, err := ParseDocument(file, format)
		if err != nil {
			return "", fmt.Errorf("%w: parse content_file %s: %w", ErrInvalidKnowledgePack, contentPath, err)
		}
		return content, nil
	}
	format := spec.ContentFormat
	if format == "" {
		format = "markdown"
	}
	content, err := NormalizeContent(format, spec.Content)
	if err != nil {
		return "", fmt.Errorf("%w: normalize content for %s: %w", ErrInvalidKnowledgePack, sourcePath, err)
	}
	return content, nil
}

func isKnowledgePackFile(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	if strings.HasPrefix(base, "readme.") || strings.HasPrefix(base, "_") {
		return false
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json", ".md", ".markdown":
		return true
	default:
		return false
	}
}

func contentFormatForPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return "markdown"
	case ".md", ".markdown":
		return "markdown"
	default:
		return FormatFromFilename(path)
	}
}

func defaultIntentTagsForDocType(docType string) []string {
	switch strings.TrimSpace(docType) {
	case "product_detail":
		return []string{"product_parameter", "purchase_recommendation"}
	case "product_parameter":
		return []string{"product_parameter"}
	case "product_comparison":
		return []string{"product_comparison"}
	case "purchase_guide":
		return []string{"purchase_recommendation"}
	case "accessory_compatibility":
		return []string{"accessory_compatibility"}
	case "user_manual":
		return []string{"usage_instruction"}
	case "troubleshooting":
		return []string{"troubleshooting"}
	case "after_sales_policy":
		return []string{"return_eligibility", "warranty_query"}
	case "faq":
		return []string{"usage_instruction", "product_parameter"}
	default:
		return nil
	}
}

func splitFrontMatter(content string) (string, string) {
	lines := strings.SplitAfter(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", strings.TrimSpace(content)
	}
	for index := 1; index < len(lines); index++ {
		if strings.TrimSpace(lines[index]) == "---" {
			return strings.TrimSpace(strings.Join(lines[1:index], "")),
				strings.TrimSpace(strings.Join(lines[index+1:], ""))
		}
	}
	return "", strings.TrimSpace(content)
}

func firstMarkdownHeading(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			return strings.TrimSpace(strings.TrimLeft(line, "#"))
		}
	}
	return ""
}

func cloneKnowledgeMetadata(source map[string]any) map[string]any {
	result := make(map[string]any, len(source)+4)
	for key, value := range source {
		result[key] = value
	}
	return result
}

func fileSource(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		absolute = path
	}
	return "file://" + filepath.ToSlash(absolute)
}

var slugUnsafePattern = regexp.MustCompile(`[^a-z0-9]+`)

func slugFilename(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.Map(func(current rune) rune {
		if current <= unicode.MaxASCII {
			return current
		}
		return '-'
	}, value)
	value = slugUnsafePattern.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if value == "" {
		return "document"
	}
	return value
}

func compactPathList(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
