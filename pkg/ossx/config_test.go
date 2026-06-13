package ossx

import (
	"strings"
	"testing"
	"time"
)

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Config)
		kind ErrorKind
	}{
		{name: "valid oss"},
		{name: "valid s3", edit: func(c *Config) { c.Provider = ProviderS3 }},
		{name: "valid minio", edit: func(c *Config) { c.Provider = ProviderMinIO }},
		{name: "valid azure", edit: func(c *Config) { c.Provider = ProviderAzure }},
		{name: "valid gcs", edit: func(c *Config) { c.Provider = ProviderGCS }},
		{name: "missing name", edit: func(c *Config) { c.Name = "" }, kind: ErrorKindValidation},
		{name: "missing provider", edit: func(c *Config) { c.Provider = "" }, kind: ErrorKindValidation},
		{name: "missing bucket", edit: func(c *Config) { c.Bucket = "" }, kind: ErrorKindValidation},
		{name: "missing endpoint", edit: func(c *Config) { c.Endpoint = "" }, kind: ErrorKindValidation},
		{name: "missing access key", edit: func(c *Config) { c.AccessKeyID = "" }, kind: ErrorKindValidation},
		{name: "missing secret", edit: func(c *Config) { c.SecretAccessKey = "" }, kind: ErrorKindValidation},
		{name: "negative timeout", edit: func(c *Config) { c.Timeout = -time.Second }, kind: ErrorKindValidation},
		{name: "unsupported provider", edit: func(c *Config) { c.Provider = "localfs" }, kind: ErrorKindValidation},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			if tt.edit != nil {
				tt.edit(&cfg)
			}
			err := cfg.Validate()
			if tt.kind == "" {
				if err != nil {
					t.Fatalf("Validate returned error: %v", err)
				}
				return
			}
			assertErrorKind(t, err, tt.kind)
		})
	}
}

func TestConfigSanitize(t *testing.T) {
	cfg := validConfig()
	cfg.AccessKeyID = "access-key-id"
	cfg.SecretAccessKey = "secret-access-key"

	safe := cfg.Sanitize()
	if safe.Name != cfg.Name || safe.Provider != cfg.Provider || safe.Endpoint != cfg.Endpoint || safe.Bucket != cfg.Bucket {
		t.Fatalf("non-secret fields not preserved: %#v", safe)
	}
	if strings.Contains(safe.AccessKeyID, "access-key-id") || strings.Contains(safe.SecretAccessKey, "secret-access-key") {
		t.Fatalf("secrets were not redacted: %#v", safe)
	}
	if safe.AccessKeyID == "" || safe.SecretAccessKey == "" {
		t.Fatalf("expected stable redaction markers, got %#v", safe)
	}
}
