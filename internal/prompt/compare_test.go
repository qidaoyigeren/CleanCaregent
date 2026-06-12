package prompt

import (
	"context"
	"testing"
)

type fixedVersionEvaluator struct{}

func (fixedVersionEvaluator) Evaluate(
	_ context.Context,
	template *Template,
	_ []EvaluationCase,
) (VersionScore, error) {
	value := 0.6
	if template.Version == "v-b" {
		value = 0.9
	}
	return VersionScore{
		Version: template.Version, CaseCount: 1,
		Faithfulness: value, Correctness: value,
	}, nil
}

func TestRegistryCompareVersions(t *testing.T) {
	registry := NewRegistry()
	registry.Set(&Template{Scenario: ScenarioGenerateGeneric, Version: "v-a", System: "A"})
	registry.Set(&Template{Scenario: ScenarioGenerateGeneric, Version: "v-b", System: "B"})
	result, err := registry.CompareVersions(
		context.Background(),
		ScenarioGenerateGeneric,
		"v-a",
		"v-b",
		[]EvaluationCase{{ID: "1", Query: "Q", Expected: "E"}},
		fixedVersionEvaluator{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Winner != "v-b" {
		t.Fatalf("result = %#v", result)
	}
}
