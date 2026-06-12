package mysql

import "testing"

func TestParseDecimalCentsIsExact(t *testing.T) {
	tests := map[string]int64{
		"0.01":    1,
		"100.00":  10000,
		"3999.90": 399990,
		"-1.25":   -125,
	}
	for input, expected := range tests {
		actual, err := parseDecimalCents(input)
		if err != nil {
			t.Fatalf("parseDecimalCents(%q) error = %v", input, err)
		}
		if actual != expected {
			t.Fatalf("parseDecimalCents(%q) = %d, want %d", input, actual, expected)
		}
	}
}

func TestParseDecimalBasisPoints(t *testing.T) {
	actual, err := parseDecimalBasisPoints("0.90")
	if err != nil {
		t.Fatal(err)
	}
	if actual != 9000 {
		t.Fatalf("basis points = %d, want 9000", actual)
	}
}
