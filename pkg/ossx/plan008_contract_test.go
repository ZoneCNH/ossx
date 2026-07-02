package ossx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlan008READMEContractDocumentsObjectVersioning(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	readme := string(data)

	for _, token := range []string{
		"ObjectInfo.Version",
		"version_id",
		"delete marker",
		"ETag",
		"request_id",
		"destruction proof",
	} {
		if !strings.Contains(readme, token) {
			t.Fatalf("README.md must document %q", token)
		}
	}
}
