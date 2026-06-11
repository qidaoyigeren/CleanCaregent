package prompt

import "testing"

func TestRegistryKeepsHistoryAndCanRollback(t *testing.T) {
	registry := NewRegistry()
	original := registry.Version(ScenarioIntent)

	registry.Set(&Template{
		Scenario: ScenarioIntent,
		Version:  "v2-test",
		System:   "system-v2",
		User:     "{query}",
	})
	if got := registry.Version(ScenarioIntent); got != "v2-test" {
		t.Fatalf("active version = %q, want v2-test", got)
	}

	if err := registry.Activate(ScenarioIntent, original); err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
	template, err := registry.Get(ScenarioIntent)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if template.Version != original {
		t.Fatalf("rolled back version = %q, want %q", template.Version, original)
	}
	if len(registry.Versions(ScenarioIntent)) < 2 {
		t.Fatalf("Versions() = %#v, want at least two versions", registry.Versions(ScenarioIntent))
	}
}

func TestRegistryGetReturnsCopy(t *testing.T) {
	registry := NewRegistry()
	first, err := registry.Get(ScenarioSystem)
	if err != nil {
		t.Fatal(err)
	}
	first.System = "mutated"
	second, err := registry.Get(ScenarioSystem)
	if err != nil {
		t.Fatal(err)
	}
	if second.System == "mutated" {
		t.Fatal("Get() exposed mutable registry state")
	}
}
