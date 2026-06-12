package mysql

import (
	"strings"
	"testing"
)

func TestBuildBooleanQueryEscapesOperators(t *testing.T) {
	query := buildBooleanQuery(`T20 +(吸力) "参数"`, []string{"6000Pa", "T20"})
	for _, want := range []string{"+T20*", "+吸力*", "+参数*", "+6000Pa*"} {
		if !strings.Contains(query, want) {
			t.Fatalf("query = %q, missing %q", query, want)
		}
	}
	if strings.Contains(query, "\"") || strings.Contains(query, "(") {
		t.Fatalf("query contains unsafe boolean operators: %q", query)
	}
}
