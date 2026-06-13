package validation

import "fmt"

// RequireNonEmpty checks that a string field is not empty.
func RequireNonEmpty(field string, value string) error {
	if value == "" {
		return fmt.Errorf("%s is required", field)
	}
	return nil
}
