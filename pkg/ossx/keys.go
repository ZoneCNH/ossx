package ossx

import (
	"errors"
	"strings"
	"unicode"
)

const maxObjectKeyBytes = 1024

func validateObjectKey(op string, key string) error {
	if err := validateObjectPath("key", key, false); err != nil {
		return validationError(op, err.Error(), err)
	}
	return nil
}

func validateObjectPrefix(op string, prefix string) error {
	if err := validateObjectPath("prefix", prefix, true); err != nil {
		return validationError(op, err.Error(), err)
	}
	return nil
}

func validateObjectPath(label string, value string, allowEmpty bool) error {
	if value == "" {
		if allowEmpty {
			return nil
		}
		return errors.New(label + " is required")
	}
	if len(value) > maxObjectKeyBytes {
		return errors.New(label + " exceeds 1024 bytes")
	}
	if strings.HasPrefix(value, "/") {
		return errors.New(label + " must be relative")
	}
	if strings.Contains(value, "\\") {
		return errors.New(label + " must use forward slashes")
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return errors.New(label + " contains a control character")
		}
	}
	for _, part := range strings.Split(value, "/") {
		if part == "." || part == ".." {
			return errors.New(label + " must not contain path traversal segments")
		}
	}
	return nil
}
