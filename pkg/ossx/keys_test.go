package ossx

import (
	"strings"
	"testing"
)

func TestValidateObjectKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
		ok   bool
	}{
		{name: "valid", key: "a/b/c.txt", ok: true},
		{name: "empty", key: ""},
		{name: "too long", key: strings.Repeat("a", maxObjectKeyBytes+1)},
		{name: "absolute", key: "/a"},
		{name: "backslash", key: "a\\b"},
		{name: "control", key: "a\nb"},
		{name: "dot segment", key: "a/./b"},
		{name: "dotdot segment", key: "a/../b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateObjectKey("op", tt.key)
			if tt.ok && err != nil {
				t.Fatalf("expected valid key, got %v", err)
			}
			if !tt.ok {
				assertErrorKind(t, err, ErrorKindValidation)
			}
		})
	}
}

func TestValidateObjectPrefix(t *testing.T) {
	if err := validateObjectPrefix("op", ""); err != nil {
		t.Fatalf("empty prefix should be accepted: %v", err)
	}
	if err := validateObjectPrefix("op", "prefix/"); err != nil {
		t.Fatalf("valid prefix rejected: %v", err)
	}
	assertErrorKind(t, validateObjectPrefix("op", "/prefix"), ErrorKindValidation)
}
