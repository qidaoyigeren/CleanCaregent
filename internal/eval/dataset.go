package eval

import (
	"encoding/json"
	"fmt"

	evaldata "CleanCaregent/docs/eval"
)

// DefaultCases loads the canonical, hand-authored v2 evaluation dataset.
// Invalid embedded data is a build artifact error and therefore fails fast.
func DefaultCases() []Case {
	var cases []Case
	if err := json.Unmarshal(evaldata.CasesV2(), &cases); err != nil {
		panic(fmt.Sprintf("decode embedded evaluation dataset: %v", err))
	}
	if err := validateCases(cases); err != nil {
		panic(err)
	}
	return cases
}

func validateCases(cases []Case) error {
	if len(cases) == 0 {
		return fmt.Errorf("evaluation dataset is empty")
	}
	seen := make(map[string]struct{}, len(cases))
	for index, item := range cases {
		if item.CaseID == "" || item.Query == "" || item.Intent == "" ||
			item.Difficulty == "" || item.StandardAnswer == "" {
			return fmt.Errorf("evaluation case at index %d is missing required fields", index)
		}
		if _, exists := seen[item.CaseID]; exists {
			return fmt.Errorf("evaluation dataset contains duplicate case_id %q", item.CaseID)
		}
		seen[item.CaseID] = struct{}{}
	}
	return nil
}
