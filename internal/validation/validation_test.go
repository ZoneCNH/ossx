package validation

import "testing"

func TestRequireNonEmpty(t *testing.T) {
	if err := RequireNonEmpty("field", "value"); err != nil {
		t.Fatalf("expected non-empty value to pass: %v", err)
	}
	if err := RequireNonEmpty("field", ""); err == nil {
		t.Fatal("expected empty value to fail")
	}
}
