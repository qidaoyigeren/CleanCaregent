package skill

import (
	"strings"
	"testing"

	"CleanCaregent/internal/intent"
)

func TestLoadAndBuildConfiguredSkills(t *testing.T) {
	definitions, err := LoadDefinitions(strings.NewReader(`
skills:
  - name: product_comparison
    enabled: true
    intents: [product_comparison]
    doc_types: [product_detail, product_parameter]
    retrieval:
      dense_top_k: 9
      keyword_top_k: 7
      rerank_top_k: 4
      min_dense_score: 0.25
  - name: fault_diagnosis
    enabled: false
`))
	if err != nil {
		t.Fatal(err)
	}
	values, err := BuildConfigured(definitions, Dependencies{})
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 || values[0].Name() != ProductComparisonSkill {
		t.Fatalf("skills = %#v", values)
	}
	if !values[0].CanHandle(intent.ProductComparison) {
		t.Fatal("configured intent was not applied")
	}
	workflow := values[0].(*Workflow)
	if workflow.config.DenseTopK != 9 || workflow.config.MinDenseScore != 0.25 {
		t.Fatalf("workflow config = %#v", workflow.config)
	}
}
