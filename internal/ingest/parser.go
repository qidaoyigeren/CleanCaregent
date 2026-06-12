package ingest

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	stdhtml "html"
	"io"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
)

var ErrUnsupportedDocumentFormat = errors.New("不支持的文档格式")

// ParseDocument extracts normalized text from JSON, CSV, XLSX, PDF, DOCX,
// HTML, Markdown, or text input.
func ParseDocument(reader io.Reader, format string) (string, error) {
	if reader == nil {
		return "", errors.New("文档读取器不能为空")
	}
	raw, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("读取文档失败: %w", err)
	}
	format = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(format)), ".")
	switch format {
	case "txt", "text", "md", "markdown":
		return strings.TrimSpace(string(raw)), nil
	case "json":
		return parseJSON(raw)
	case "csv":
		return parseCSV(bytes.NewReader(raw))
	case "xlsx":
		return parseXLSX(raw)
	case "html", "htm":
		return parseHTML(bytes.NewReader(raw))
	case "docx":
		return parseDOCX(raw)
	case "pdf":
		return parsePDF(raw)
	default:
		return "", fmt.Errorf("%w: %s", ErrUnsupportedDocumentFormat, format)
	}
}

func parseJSON(raw []byte) (string, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("解析 JSON 失败: %w", err)
	}
	normalized, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", fmt.Errorf("规范化 JSON 失败: %w", err)
	}
	return string(normalized), nil
}

func parseCSV(reader io.Reader) (string, error) {
	csvReader := csv.NewReader(reader)
	csvReader.FieldsPerRecord = -1
	records, err := csvReader.ReadAll()
	if err != nil {
		return "", fmt.Errorf("解析 CSV 失败: %w", err)
	}
	var lines []string
	for _, record := range records {
		for index := range record {
			record[index] = strings.TrimSpace(record[index])
		}
		lines = append(lines, strings.Join(record, "\t"))
	}
	return strings.TrimSpace(strings.Join(lines, "\n")), nil
}

func parseXLSX(raw []byte) (string, error) {
	archive, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return "", fmt.Errorf("打开 XLSX 失败: %w", err)
	}
	sharedStrings := []string{}
	worksheets := make(map[string]*zip.File)
	for _, file := range archive.File {
		switch {
		case file.Name == "xl/sharedStrings.xml":
			sharedStrings, err = readSharedStrings(file)
			if err != nil {
				return "", err
			}
		case strings.HasPrefix(file.Name, "xl/worksheets/") &&
			strings.HasSuffix(file.Name, ".xml"):
			worksheets[file.Name] = file
		}
	}
	if len(worksheets) == 0 {
		return "", errors.New("XLSX 缺少工作表")
	}
	names := make([]string, 0, len(worksheets))
	for name := range worksheets {
		names = append(names, name)
	}
	sort.Strings(names)
	var sections []string
	for _, name := range names {
		content, parseErr := readWorksheet(worksheets[name], sharedStrings)
		if parseErr != nil {
			return "", parseErr
		}
		if strings.TrimSpace(content) != "" {
			sections = append(sections, "## "+filepath.Base(name)+"\n"+content)
		}
	}
	if len(sections) == 0 {
		return "", errors.New("XLSX 未提取到可用单元格")
	}
	return strings.Join(sections, "\n\n"), nil
}

func readSharedStrings(file *zip.File) ([]string, error) {
	reader, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("读取 XLSX 共享字符串失败: %w", err)
	}
	defer reader.Close()
	decoder := xml.NewDecoder(reader)
	var (
		values  []string
		current strings.Builder
		inItem  bool
		inText  bool
	)
	for {
		token, decodeErr := decoder.Token()
		if errors.Is(decodeErr, io.EOF) {
			break
		}
		if decodeErr != nil {
			return nil, fmt.Errorf("解析 XLSX 共享字符串失败: %w", decodeErr)
		}
		switch value := token.(type) {
		case xml.StartElement:
			if value.Name.Local == "si" {
				inItem = true
				current.Reset()
			}
			inText = inItem && value.Name.Local == "t"
		case xml.CharData:
			if inText {
				current.Write(value)
			}
		case xml.EndElement:
			if value.Name.Local == "t" {
				inText = false
			}
			if value.Name.Local == "si" {
				values = append(values, current.String())
				inItem = false
			}
		}
	}
	return values, nil
}

func readWorksheet(file *zip.File, sharedStrings []string) (string, error) {
	reader, err := file.Open()
	if err != nil {
		return "", fmt.Errorf("读取 XLSX 工作表失败: %w", err)
	}
	defer reader.Close()
	var sheet struct {
		Rows []struct {
			Cells []struct {
				Reference string `xml:"r,attr"`
				Type      string `xml:"t,attr"`
				Value     string `xml:"v"`
				Inline    string `xml:"is>t"`
			} `xml:"c"`
		} `xml:"sheetData>row"`
	}
	if err := xml.NewDecoder(reader).Decode(&sheet); err != nil {
		return "", fmt.Errorf("解析 XLSX 工作表失败: %w", err)
	}
	var lines []string
	for _, row := range sheet.Rows {
		values := make([]string, 0, len(row.Cells))
		currentColumn := 0
		for _, cell := range row.Cells {
			column := xlsxColumnIndex(cell.Reference)
			for currentColumn < column {
				values = append(values, "")
				currentColumn++
			}
			value := strings.TrimSpace(cell.Value)
			switch cell.Type {
			case "s":
				index, parseErr := strconv.Atoi(value)
				if parseErr == nil && index >= 0 && index < len(sharedStrings) {
					value = sharedStrings[index]
				}
			case "inlineStr":
				value = strings.TrimSpace(cell.Inline)
			}
			values = append(values, value)
			currentColumn++
		}
		lines = append(lines, strings.TrimRight(strings.Join(values, "\t"), "\t"))
	}
	return strings.TrimSpace(strings.Join(lines, "\n")), nil
}

func xlsxColumnIndex(reference string) int {
	index := 0
	for _, value := range reference {
		if value < 'A' || value > 'Z' {
			break
		}
		index = index*26 + int(value-'A'+1)
	}
	if index == 0 {
		return 0
	}
	return index - 1
}

// FormatFromFilename returns the lower-case file extension without a dot.
func FormatFromFilename(name string) string {
	return strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
}

func parseHTML(reader io.Reader) (string, error) {
	raw, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("读取 HTML 失败: %w", err)
	}
	value := string(raw)
	for _, pattern := range []*regexp.Regexp{
		regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script>`),
		regexp.MustCompile(`(?is)<style\b[^>]*>.*?</style>`),
		regexp.MustCompile(`(?is)<noscript\b[^>]*>.*?</noscript>`),
	} {
		value = pattern.ReplaceAllString(value, " ")
	}
	value = regexp.MustCompile(`(?i)</?(?:p|div|h[1-6]|li|tr|br|section|article)\b[^>]*>`).
		ReplaceAllString(value, "\n")
	value = regexp.MustCompile(`(?s)<[^>]+>`).ReplaceAllString(value, " ")
	value = stdhtml.UnescapeString(value)
	var lines []string
	for _, line := range strings.Split(value, "\n") {
		if line = strings.Join(strings.Fields(line), " "); line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n"), nil
}

func parseDOCX(raw []byte) (string, error) {
	archive, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return "", fmt.Errorf("打开 DOCX 失败: %w", err)
	}
	for _, file := range archive.File {
		if file.Name != "word/document.xml" {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			return "", fmt.Errorf("读取 DOCX 正文失败: %w", err)
		}
		text, parseErr := parseWordXML(reader)
		closeErr := reader.Close()
		if parseErr != nil {
			return "", parseErr
		}
		if closeErr != nil {
			return "", fmt.Errorf("关闭 DOCX 正文失败: %w", closeErr)
		}
		return text, nil
	}
	return "", errors.New("DOCX 缺少 word/document.xml")
}

func parseWordXML(reader io.Reader) (string, error) {
	decoder := xml.NewDecoder(reader)
	var (
		builder strings.Builder
		inText  bool
	)
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("解析 DOCX XML 失败: %w", err)
		}
		switch value := token.(type) {
		case xml.StartElement:
			inText = value.Name.Local == "t"
		case xml.EndElement:
			if value.Name.Local == "p" {
				builder.WriteByte('\n')
			}
			if value.Name.Local == "t" {
				inText = false
			}
		case xml.CharData:
			if inText {
				builder.Write(value)
			}
		}
	}
	return strings.TrimSpace(builder.String()), nil
}

var (
	pdfStreamPattern = regexp.MustCompile(`(?s)stream\r?\n(.*?)\r?\nendstream`)
	pdfTextPattern   = regexp.MustCompile(`(?s)(\((?:\\.|[^\\)])*\)|<[0-9A-Fa-f]+>)\s*(?:Tj|'|")`)
	pdfArrayPattern  = regexp.MustCompile(`(?s)\[(.*?)\]\s*TJ`)
	pdfArrayToken    = regexp.MustCompile(`(\((?:\\.|[^\\)])*\)|<[0-9A-Fa-f]+>)`)
)

func parsePDF(raw []byte) (string, error) {
	streams := pdfStreamPattern.FindAllSubmatchIndex(raw, -1)
	var texts []string
	for _, match := range streams {
		if len(match) < 4 {
			continue
		}
		content := append([]byte(nil), raw[match[2]:match[3]]...)
		headerStart := maxInt(0, match[0]-256)
		if bytes.Contains(raw[headerStart:match[0]], []byte("/FlateDecode")) {
			reader := flate.NewReader(bytes.NewReader(content))
			decoded, err := io.ReadAll(reader)
			closeErr := reader.Close()
			if err != nil {
				continue
			}
			if closeErr != nil {
				return "", fmt.Errorf("关闭 PDF 解压流失败: %w", closeErr)
			}
			content = decoded
		}
		texts = append(texts, extractPDFText(content)...)
	}
	if len(texts) == 0 {
		texts = extractPDFText(raw)
	}
	result := strings.TrimSpace(strings.Join(texts, "\n"))
	if result == "" {
		return "", errors.New("PDF 未提取到可用文本，可能是扫描件或使用了不支持的字体编码")
	}
	return result, nil
}

func extractPDFText(raw []byte) []string {
	var result []string
	for _, match := range pdfTextPattern.FindAllSubmatch(raw, -1) {
		if text := decodePDFToken(string(match[1])); text != "" {
			result = append(result, text)
		}
	}
	for _, array := range pdfArrayPattern.FindAllSubmatch(raw, -1) {
		var builder strings.Builder
		for _, token := range pdfArrayToken.FindAllSubmatch(array[1], -1) {
			builder.WriteString(decodePDFToken(string(token[1])))
		}
		if text := strings.TrimSpace(builder.String()); text != "" {
			result = append(result, text)
		}
	}
	return result
}

func decodePDFToken(token string) string {
	if strings.HasPrefix(token, "<") {
		raw, err := hex.DecodeString(strings.Trim(token, "<>"))
		if err != nil {
			return ""
		}
		if len(raw) >= 2 && raw[0] == 0xFE && raw[1] == 0xFF {
			values := make([]uint16, 0, (len(raw)-2)/2)
			for index := 2; index+1 < len(raw); index += 2 {
				values = append(values, uint16(raw[index])<<8|uint16(raw[index+1]))
			}
			return string(utf16.Decode(values))
		}
		return string(raw)
	}
	value := strings.TrimSuffix(strings.TrimPrefix(token, "("), ")")
	var builder strings.Builder
	for index := 0; index < len(value); index++ {
		if value[index] != '\\' || index+1 >= len(value) {
			builder.WriteByte(value[index])
			continue
		}
		index++
		switch value[index] {
		case 'n':
			builder.WriteByte('\n')
		case 'r':
			builder.WriteByte('\r')
		case 't':
			builder.WriteByte('\t')
		case '(', ')', '\\':
			builder.WriteByte(value[index])
		default:
			if value[index] >= '0' && value[index] <= '7' {
				end := index + 1
				for end < len(value) && end < index+3 && value[end] >= '0' && value[end] <= '7' {
					end++
				}
				parsed, err := strconv.ParseUint(value[index:end], 8, 8)
				if err == nil {
					builder.WriteByte(byte(parsed))
				}
				index = end - 1
			} else {
				builder.WriteByte(value[index])
			}
		}
	}
	return strings.TrimSpace(builder.String())
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
