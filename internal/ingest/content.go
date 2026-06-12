package ingest

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
)

var ErrUnsupportedContentFormat = errors.New("unsupported knowledge content format")

func NormalizeContent(format, content string) (string, error) {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = "markdown"
	}
	content = strings.TrimSpace(content)
	switch format {
	case "markdown", "text":
		return content, nil
	case "json":
		var value any
		if err := json.Unmarshal([]byte(content), &value); err != nil {
			return "", err
		}
		var output bytes.Buffer
		encoder := json.NewEncoder(&output)
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(value); err != nil {
			return "", err
		}
		return strings.TrimSpace(output.String()), nil
	default:
		return "", ErrUnsupportedContentFormat
	}
}
