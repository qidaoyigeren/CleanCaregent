package productpack

import (
	"os"
	"path/filepath"
	"testing"

	"CleanCaregent/internal/compatibility"
	"CleanCaregent/internal/diagnosis"
)

func TestLoadValidateAndProjectProductPack(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pack.yaml")
	if err := os.WriteFile(path, []byte(`
pack_id: brightclean-test
version: 2026-06
products:
  - product_code: P-MOP-BM-M1
    name: BrightClean Breeze Mop M1
    category: floor_mop
    brand: BrightClean
    model: BM-M1
    aliases: [Breeze Mop M1]
    skus:
      - sku_code: SKU-BM-M1-STD
        sku_name: BM-M1 Standard Kit
        list_price_cents: 12900
        current_price_cents: 9900
        available_stock: 12
compatibility:
  - host_model: BM-M1
    accessory_model: BM-M1-PAD
    accessory_type: mop_pad
    status: compatible
    reason: Same 38cm head connector.
    evidence_doc_id: kb_breeze_mop_m1_detail
diagnosis:
  - id: bm_m1_pad_slip
    product_model: BM-M1
    symptom: pad slip
    question: Is the pad clipped into both side slots?
    guidance: Check both side clips before wet mopping.
    yes_next: bm_m1_pad_dirty
    no_next: bm_m1_reclip
    root: true
  - id: bm_m1_reclip
    product_model: BM-M1
    symptom: pad slip
    resolution: Reinstall the pad and press both clips until they click.
    terminal: true
  - id: bm_m1_pad_dirty
    product_model: BM-M1
    symptom: pad slip
    resolution: Replace or wash the pad if the hook surface is clogged.
    terminal: true
`), 0o600); err != nil {
		t.Fatal(err)
	}

	packs, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if errs := Validate(packs); len(errs) > 0 {
		t.Fatalf("Validate() errors = %v", errs)
	}
	registry := Registry(packs)
	if got := registry.MatchModels("Can Breeze Mop M1 use BM-M1-PAD?"); len(got) != 1 || got[0] != "BM-M1" {
		t.Fatalf("registry models = %#v", got)
	}
	if got := registry.CategoryForModel("BM-M1"); got != "floor_mop" {
		t.Fatalf("category = %q", got)
	}
	matrix := compatibility.NewMatrix(CompatibilityEntries(packs))
	if got := matrix.Check("BM-M1", "BM-M1-PAD"); got.Status != compatibility.Compatible {
		t.Fatalf("compatibility = %#v", got)
	}
	data := DiagnosisEntries(packs)
	engine := diagnosis.NewEngine(data.Nodes, data.Roots, data.SafetyKeywords)
	_, decision, err := engine.Start("BM-M1", "pad slip during cleaning")
	if err != nil {
		t.Fatal(err)
	}
	if decision.NodeID != "bm_m1_pad_slip" {
		t.Fatalf("decision = %#v", decision)
	}
}

func TestValidateRejectsDuplicateModels(t *testing.T) {
	packs := []Pack{{
		PackID:  "dup-test",
		Version: "2026-06",
		Products: []ProductSpec{
			{
				ProductCode: "P1", Name: "One", Category: "floor_mop", Brand: "B", Model: "BM-M1",
				SKUs: []SKUSpec{{SKUCode: "S1", SKUName: "S1", ListPriceCents: 100}},
			},
			{
				ProductCode: "P2", Name: "Two", Category: "floor_mop", Brand: "B", Model: "BM-M1",
				SKUs: []SKUSpec{{SKUCode: "S2", SKUName: "S2", ListPriceCents: 100}},
			},
		},
	}}
	if errs := Validate(packs); len(errs) == 0 {
		t.Fatal("Validate() returned no errors for duplicate model")
	}
}
