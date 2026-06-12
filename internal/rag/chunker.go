package rag

import (
	"context"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"CleanCaregent/internal/embedding"
)

type Chunk struct {
	SectionPath string
	Content     string
	TokenCount  int
}

type Chunker interface {
	Split(docType, title, content string) []Chunk
}

// ContextChunker supports context-aware chunking for semantic embedding calls.
type ContextChunker interface {
	Chunker
	SplitContext(ctx context.Context, docType, title, content string) []Chunk
}

type StructureAwareChunker struct {
	maxRunes          int
	overlap           int
	semanticThreshold float64
	profiles          map[string]ChunkProfile
	embedder          embedding.Embedder
}

type ChunkProfile struct {
	MaxRunes          int
	Overlap           int
	SemanticThreshold float64
}

type section struct {
	path    string
	content string
}

var headingPattern = regexp.MustCompile(`^(#{1,6})\s+(.+?)\s*$`)
var (
	faqBoundaryPattern    = regexp.MustCompile(`(?i)^(?:Q\s*[:：]|问题\s*[:：]|问\s*[:：])`)
	faultBoundaryPattern  = regexp.MustCompile(`(?i)^(?:node_id\s*[:：]|节点\s*[:：])`)
	policyBoundaryPattern = regexp.MustCompile(`^(?:第[一二三四五六七八九十百0-9]+条|条款\s*[一二三四五六七八九十百0-9]*\s*[:：])`)
)

func NewStructureAwareChunker(maxRunes, overlap int) *StructureAwareChunker {
	return &StructureAwareChunker{maxRunes: maxRunes, overlap: overlap}
}

func NewProfiledStructureAwareChunker(
	maxRunes, overlap int,
	profiles map[string]ChunkProfile,
) *StructureAwareChunker {
	return &StructureAwareChunker{
		maxRunes: maxRunes,
		overlap:  overlap,
		profiles: cloneProfiles(profiles),
	}
}

// NewSemanticProfiledStructureAwareChunker creates a profiled chunker that
// detects semantic sentence boundaries for configured unstructured documents.
func NewSemanticProfiledStructureAwareChunker(
	maxRunes, overlap int,
	profiles map[string]ChunkProfile,
	embedder embedding.Embedder,
) *StructureAwareChunker {
	return &StructureAwareChunker{
		maxRunes: maxRunes,
		overlap:  overlap,
		profiles: cloneProfiles(profiles),
		embedder: embedder,
	}
}

func (c *StructureAwareChunker) Split(docType, title, content string) []Chunk {
	return c.split(context.Background(), docType, title, content, false)
}

// SplitContext splits a document and enables semantic boundaries when the
// active profile has a positive threshold and an embedder is configured.
func (c *StructureAwareChunker) SplitContext(
	ctx context.Context,
	docType, title, content string,
) []Chunk {
	return c.split(ctx, docType, title, content, true)
}

func (c *StructureAwareChunker) split(
	ctx context.Context,
	docType, title, content string,
	semantic bool,
) []Chunk {
	active := c.forDocumentType(docType)
	content = normalizeContent(content)
	if content == "" {
		return nil
	}

	sections := parseSections(title, content)
	chunks := make([]Chunk, 0, len(sections))
	for _, current := range sections {
		if structured := active.splitStructured(docType, current); len(structured) > 0 {
			chunks = append(chunks, structured...)
			continue
		}
		if isTableDocument(docType) && containsMarkdownTable(current.content) {
			chunks = append(chunks, active.splitTable(current)...)
			continue
		}
		if semantic && active.canSemanticSplit(docType, current.content) {
			if semanticChunks := active.splitSemantic(ctx, current); len(semanticChunks) > 0 {
				chunks = append(chunks, semanticChunks...)
				continue
			}
		}
		chunks = append(chunks, active.splitText(current)...)
	}
	return chunks
}

func (c *StructureAwareChunker) forDocumentType(docType string) *StructureAwareChunker {
	if c == nil {
		return &StructureAwareChunker{maxRunes: 1200, overlap: 120}
	}
	profile, ok := c.profiles[docType]
	if !ok || profile.MaxRunes <= 0 || profile.Overlap < 0 || profile.Overlap >= profile.MaxRunes {
		return c
	}
	active := *c
	active.maxRunes = profile.MaxRunes
	active.overlap = profile.Overlap
	active.semanticThreshold = profile.SemanticThreshold
	return &active
}

func cloneProfiles(source map[string]ChunkProfile) map[string]ChunkProfile {
	if len(source) == 0 {
		return nil
	}
	result := make(map[string]ChunkProfile, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func (c *StructureAwareChunker) canSemanticSplit(docType, content string) bool {
	if c.embedder == nil || c.semanticThreshold <= 0 || c.semanticThreshold >= 1 {
		return false
	}
	switch docType {
	case "purchase_guide", "product_detail":
		return !containsMarkdownTable(content)
	default:
		return false
	}
}

var semanticSentencePattern = regexp.MustCompile(`[^。！？!?；;\n]+[。！？!?；;]?`)

func (c *StructureAwareChunker) splitSemantic(ctx context.Context, value section) []Chunk {
	sentences := semanticSentencePattern.FindAllString(value.content, -1)
	filtered := make([]string, 0, len(sentences))
	for _, sentence := range sentences {
		if sentence = strings.TrimSpace(sentence); sentence != "" {
			filtered = append(filtered, sentence)
		}
	}
	if len(filtered) < 2 {
		return nil
	}
	vectors, err := c.embedder.Embed(ctx, filtered)
	if err != nil || len(vectors) != len(filtered) {
		return nil
	}
	for _, vector := range vectors {
		if len(vector) != c.embedder.Dimension() {
			return nil
		}
	}

	var (
		result  []Chunk
		current strings.Builder
	)
	flush := func() {
		content := strings.TrimSpace(current.String())
		if content == "" {
			return
		}
		path := value.path
		if path != "" {
			path += " > "
		}
		path += "语义块 " + strconv.Itoa(len(result)+1)
		result = append(result, newChunk(path, content))
		current.Reset()
	}
	for index, sentence := range filtered {
		if index > 0 {
			similarity := cosineSimilarity(vectors[index-1], vectors[index])
			candidateRunes := utf8.RuneCountInString(current.String()) + utf8.RuneCountInString(sentence)
			minSemanticRunes := max(24, min(80, c.maxRunes/12))
			enoughContent := utf8.RuneCountInString(current.String()) >= minSemanticRunes
			if candidateRunes > c.maxRunes ||
				(similarity < c.semanticThreshold && enoughContent) {
				flush()
			}
		}
		current.WriteString(sentence)
	}
	flush()
	if len(result) <= 1 && utf8.RuneCountInString(value.content) > c.maxRunes {
		return nil
	}
	return result
}

func cosineSimilarity(left, right []float32) float64 {
	if len(left) == 0 || len(left) != len(right) {
		return 0
	}
	var dot, leftNorm, rightNorm float64
	for index := range left {
		lValue := float64(left[index])
		rValue := float64(right[index])
		dot += lValue * rValue
		leftNorm += lValue * lValue
		rightNorm += rValue * rValue
	}
	if leftNorm == 0 || rightNorm == 0 {
		return 0
	}
	return dot / (math.Sqrt(leftNorm) * math.Sqrt(rightNorm))
}

func (c *StructureAwareChunker) splitStructured(docType string, value section) []Chunk {
	var boundary *regexp.Regexp
	switch docType {
	case "faq":
		boundary = faqBoundaryPattern
	case "troubleshooting":
		boundary = faultBoundaryPattern
	case "after_sales_policy":
		boundary = policyBoundaryPattern
	default:
		return nil
	}
	blocks := splitByBoundary(value.content, boundary)
	if len(blocks) <= 1 {
		return nil
	}
	result := make([]Chunk, 0, len(blocks))
	for index, block := range blocks {
		path := value.path
		if path != "" {
			path += " > "
		}
		path += structuredBlockName(docType, index+1)
		result = append(result, c.splitText(section{path: path, content: block})...)
	}
	return result
}

func splitByBoundary(content string, boundary *regexp.Regexp) []string {
	lines := strings.Split(content, "\n")
	var (
		blocks  []string
		current strings.Builder
	)
	flush := func() {
		value := strings.TrimSpace(current.String())
		if value != "" {
			blocks = append(blocks, value)
		}
		current.Reset()
	}
	for _, line := range lines {
		if boundary.MatchString(strings.TrimSpace(line)) && current.Len() > 0 {
			flush()
		}
		current.WriteString(line)
		current.WriteByte('\n')
	}
	flush()
	return blocks
}

func structuredBlockName(docType string, index int) string {
	switch docType {
	case "faq":
		return "FAQ " + strconv.Itoa(index)
	case "troubleshooting":
		return "故障节点 " + strconv.Itoa(index)
	case "after_sales_policy":
		return "政策条款 " + strconv.Itoa(index)
	default:
		return strconv.Itoa(index)
	}
}

func (c *StructureAwareChunker) splitText(value section) []Chunk {
	runes := []rune(strings.TrimSpace(value.content))
	if len(runes) <= c.maxRunes {
		return []Chunk{newChunk(value.path, string(runes))}
	}

	result := make([]Chunk, 0, len(runes)/c.maxRunes+1)
	start := 0
	for start < len(runes) {
		end := min(start+c.maxRunes, len(runes))
		if end < len(runes) {
			end = bestBoundary(runes, start, end)
		}
		if end <= start {
			end = min(start+c.maxRunes, len(runes))
		}
		result = append(result, newChunk(value.path, string(runes[start:end])))
		if end == len(runes) {
			break
		}
		start = max(end-c.overlap, start+1)
	}
	return result
}

func (c *StructureAwareChunker) splitTable(value section) []Chunk {
	lines := strings.Split(value.content, "\n")
	headerEnd := tableHeaderEnd(lines)
	if headerEnd == 0 {
		return c.splitText(value)
	}
	header := strings.Join(lines[:headerEnd], "\n")
	rows := lines[headerEnd:]
	result := make([]Chunk, 0, len(rows)/8+1)
	current := header
	for _, row := range rows {
		candidate := current + "\n" + row
		if utf8.RuneCountInString(candidate) > c.maxRunes && current != header {
			result = append(result, newChunk(value.path, current))
			current = header + "\n" + row
			continue
		}
		current = candidate
	}
	if strings.TrimSpace(current) != strings.TrimSpace(header) {
		result = append(result, newChunk(value.path, current))
	}
	if len(result) == 0 {
		return c.splitText(value)
	}
	return result
}

func parseSections(title, content string) []section {
	lines := strings.Split(content, "\n")
	paths := make([]string, 0, 6)
	currentPath := strings.TrimSpace(title)
	var current strings.Builder
	result := make([]section, 0, 8)

	flush := func() {
		text := strings.TrimSpace(current.String())
		if text != "" {
			result = append(result, section{path: currentPath, content: text})
		}
		current.Reset()
	}

	for _, line := range lines {
		match := headingPattern.FindStringSubmatch(line)
		if len(match) == 3 {
			flush()
			level := len(match[1])
			for len(paths) >= level {
				paths = paths[:len(paths)-1]
			}
			paths = append(paths, strings.TrimSpace(match[2]))
			currentPath = strings.Join(paths, " > ")
			if strings.TrimSpace(title) != "" {
				currentPath = strings.TrimSpace(title) + " > " + currentPath
			}
			continue
		}
		current.WriteString(line)
		current.WriteByte('\n')
	}
	flush()
	return result
}

func normalizeContent(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	lines := strings.Split(content, "\n")
	for index := range lines {
		lines[index] = strings.TrimRight(lines[index], " \t")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func newChunk(path, content string) Chunk {
	content = strings.TrimSpace(content)
	return Chunk{
		SectionPath: path,
		Content:     content,
		TokenCount:  estimateTokens(content),
	}
}

func estimateTokens(content string) int {
	var latinRunes, otherRunes int
	for _, current := range content {
		if current <= 127 {
			latinRunes++
		} else {
			otherRunes++
		}
	}
	return max(1, otherRunes+(latinRunes+3)/4)
}

func bestBoundary(runes []rune, start, end int) int {
	lowerBound := start + (end-start)*2/3
	for index := end - 1; index >= lowerBound; index-- {
		switch runes[index] {
		case '\n', '。', '！', '？', '.', '!', '?', ';', '；':
			return index + 1
		}
	}
	return end
}

func isTableDocument(docType string) bool {
	switch docType {
	case "product_parameter", "product_comparison", "accessory_compatibility":
		return true
	default:
		return false
	}
}

func containsMarkdownTable(content string) bool {
	lines := strings.Split(content, "\n")
	for index := 1; index < len(lines); index++ {
		if strings.Contains(lines[index-1], "|") && isTableSeparator(lines[index]) {
			return true
		}
	}
	return false
}

func tableHeaderEnd(lines []string) int {
	for index := 1; index < len(lines); index++ {
		if strings.Contains(lines[index-1], "|") && isTableSeparator(lines[index]) {
			return index + 1
		}
	}
	return 0
}

func isTableSeparator(line string) bool {
	trimmed := strings.TrimSpace(strings.Trim(line, "|"))
	if trimmed == "" {
		return false
	}
	parts := strings.Split(trimmed, "|")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		part = strings.Trim(part, ":")
		if len(part) < 3 || strings.Trim(part, "-") != "" {
			return false
		}
	}
	return true
}
