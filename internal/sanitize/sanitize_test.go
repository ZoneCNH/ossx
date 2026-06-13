package sanitize

import "testing"

func TestSecret(t *testing.T) {
	if got := Secret(""); got != "" {
		t.Fatalf("empty secret should stay empty, got %q", got)
	}
	if got := Secret("secret-value"); got != "***" {
		t.Fatalf("secret should be redacted, got %q", got)
	}
}
