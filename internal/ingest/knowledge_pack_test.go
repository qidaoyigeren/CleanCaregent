package ingest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadKnowledgeDocumentsMarkdownFrontMatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "breeze-mop-m1.md")
	if err := os.WriteFile(path, []byte(`---
doc_id: kb_breeze_mop_m1
title: Breeze Mop M1 平板拖把详情
category: floor_mop
brand: BrightClean
doc_type: product_detail
version: 2026-06
model: BM-M1
---
# Breeze Mop M1 平板拖把详情

Breeze Mop M1 是面向家庭地面清洁的平板拖把，适合木地板、瓷砖和日常湿拖。
它使用 38cm 超细纤维拖布，拖布可机洗，替换装型号为 BM-M1-PAD。
`), 0o600); err != nil {
		t.Fatal(err)
	}

	documents, err := LoadKnowledgeDocuments(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(documents) != 1 {
		t.Fatalf("documents = %d, want 1", len(documents))
	}
	document := documents[0]
	if document.DocID != "kb_breeze_mop_m1" || document.DocType != "product_detail" {
		t.Fatalf("document = %#v", document)
	}
	if document.Metadata["model"] != "BM-M1" {
		t.Fatalf("metadata = %#v", document.Metadata)
	}
	if !containsString(document.IntentTags, "purchase_recommendation") {
		t.Fatalf("intent tags = %#v", document.IntentTags)
	}
}

func TestLoadKnowledgeDocumentsJSONWithContentFile(t *testing.T) {
	dir := t.TempDir()
	contentPath := filepath.Join(dir, "window-tool.csv")
	if err := os.WriteFile(contentPath, []byte("model,width\nWS2,25cm\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	packPath := filepath.Join(dir, "window-tool.json")
	if err := os.WriteFile(packPath, []byte(`{
  "doc_id": "kb_window_squeegee_ws2_params",
  "title": "WS2 玻璃刮参数表",
  "content_file": "window-tool.csv",
  "content_format": "csv",
  "category": "window_cleaning",
  "brand": "BrightClean",
  "doc_type": "product_parameter",
  "version": "2026-06",
  "models": ["WS2"],
  "metadata": {"surface": "glass"}
}`), 0o600); err != nil {
		t.Fatal(err)
	}

	documents, err := LoadKnowledgeDocuments(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(documents) != 1 {
		t.Fatalf("documents = %d, want 1", len(documents))
	}
	document := documents[0]
	if !strings.Contains(document.Content, "model\twidth") || !strings.Contains(document.Content, "WS2\t25cm") {
		t.Fatalf("content = %q", document.Content)
	}
	if got := document.Metadata["content_format"]; got != "csv" {
		t.Fatalf("content_format = %#v", got)
	}
	if models, ok := document.Metadata["models"].([]string); !ok || len(models) != 1 || models[0] != "WS2" {
		t.Fatalf("models metadata = %#v", document.Metadata["models"])
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
