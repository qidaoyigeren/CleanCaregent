package agent

import (
	"context"
	"strings"
	"testing"

	"CleanCaregent/internal/intent"
)

func TestClarifierUsesMissingInformationStrategy(t *testing.T) {
	clarifier := NewClarifier(nil, nil)
	tests := []struct {
		name    string
		query   string
		missing []string
		want    string
	}{
		{name: "model ambiguity", query: "那款多少钱", missing: []string{"产品型号"}, want: "T20 还是 X20 Pro"},
		{name: "comparison models", query: "哪个好", missing: []string{"比较型号"}, want: "哪两款产品"},
		{name: "vague parameter", query: "够不够用", missing: []string{"参数含义"}, want: "清洁面积"},
		{name: "vague intent", query: "帮我看看", missing: []string{"意图"}, want: "查产品参数"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			answer := clarifier.Clarify(
				context.Background(),
				test.query,
				intent.Clarification,
				map[string]string{},
				test.missing,
			)
			if !strings.Contains(answer, test.want) {
				t.Fatalf("answer = %q, want substring %q", answer, test.want)
			}
		})
	}
}
