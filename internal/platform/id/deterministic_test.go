package id

import (
	"regexp"
	"testing"
)

func TestDeterministicUUID(t *testing.T) {
	first := DeterministicUUID("doc:v1:1")
	second := DeterministicUUID("doc:v1:1")
	if first != second {
		t.Fatalf("UUIDs differ: %q != %q", first, second)
	}
	pattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-5[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !pattern.MatchString(first) {
		t.Fatalf("invalid UUID v5 format: %q", first)
	}
}
