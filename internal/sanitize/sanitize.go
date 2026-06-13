package sanitize

// Secret redacts a secret value for safe logging.
func Secret(value string) string {
	if value == "" {
		return ""
	}
	return "***"
}
