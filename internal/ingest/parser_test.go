package ingest

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func TestParseDocumentHTML(t *testing.T) {
	text, err := ParseDocument(strings.NewReader(
		`<html><style>hidden</style><body><h1>T20 参数</h1><p>额定吸力 6000Pa</p></body></html>`,
	), "html")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "hidden") || !strings.Contains(text, "6000Pa") {
		t.Fatalf("text = %q", text)
	}
}

func TestParseDocumentDOCX(t *testing.T) {
	var raw bytes.Buffer
	archive := zip.NewWriter(&raw)
	file, err := archive.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte(
		`<w:document xmlns:w="x"><w:body><w:p><w:r><w:t>T20 参数</w:t></w:r></w:p><w:p><w:r><w:t>额定吸力 6000Pa</w:t></w:r></w:p></w:body></w:document>`,
	)); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	text, err := ParseDocument(bytes.NewReader(raw.Bytes()), "docx")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "T20 参数") || !strings.Contains(text, "6000Pa") {
		t.Fatalf("text = %q", text)
	}
}

func TestParseDocumentPDFTextStream(t *testing.T) {
	raw := []byte("%PDF-1.4\n1 0 obj\nstream\nBT (T20 suction 6000Pa) Tj ET\nendstream\nendobj")
	text, err := ParseDocument(bytes.NewReader(raw), "pdf")
	if err != nil {
		t.Fatal(err)
	}
	if text != "T20 suction 6000Pa" {
		t.Fatalf("text = %q", text)
	}
}

func TestParseDocumentJSONAndCSV(t *testing.T) {
	jsonText, err := ParseDocument(strings.NewReader(`{"model":"T20","suction":6000}`), "json")
	if err != nil || !strings.Contains(jsonText, `"model": "T20"`) {
		t.Fatalf("json text = %q, error = %v", jsonText, err)
	}
	csvText, err := ParseDocument(strings.NewReader("model,suction\nT20,6000Pa\n"), "csv")
	if err != nil || csvText != "model\tsuction\nT20\t6000Pa" {
		t.Fatalf("csv text = %q, error = %v", csvText, err)
	}
}

func TestParseDocumentXLSX(t *testing.T) {
	var raw bytes.Buffer
	archive := zip.NewWriter(&raw)
	shared, err := archive.Create("xl/sharedStrings.xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := shared.Write([]byte(
		`<sst><si><t>型号</t></si><si><t>T20</t></si><si><t>6000Pa</t></si></sst>`,
	)); err != nil {
		t.Fatal(err)
	}
	sheet, err := archive.Create("xl/worksheets/sheet1.xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sheet.Write([]byte(
		`<worksheet><sheetData>` +
			`<row><c r="A1" t="s"><v>0</v></c><c r="B1" t="inlineStr"><is><t>吸力</t></is></c></row>` +
			`<row><c r="A2" t="s"><v>1</v></c><c r="B2" t="s"><v>2</v></c></row>` +
			`</sheetData></worksheet>`,
	)); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	text, err := ParseDocument(bytes.NewReader(raw.Bytes()), "xlsx")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"型号\t吸力", "T20\t6000Pa"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("xlsx text = %q, missing %q", text, expected)
		}
	}
}
