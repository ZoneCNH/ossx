package ossx

import (
	"errors"
	"time"

	"github.com/ZoneCNH/ossx/internal/sanitize"
	"github.com/ZoneCNH/ossx/internal/validation"
)

// Provider represents the object storage provider type.
type Provider string

const (
	ProviderS3    Provider = "s3"
	ProviderOSS   Provider = "oss"
	ProviderMinIO Provider = "minio"
	ProviderAzure Provider = "azure"
	ProviderGCS   Provider = "gcs"
)

// Config holds the configuration for connecting to an object storage service.
type Config struct {
	Name            string
	Provider        Provider
	Endpoint        string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	UseSSL          bool
	Timeout         time.Duration
}

// SanitizedConfig is the safe-to-log version of Config with secrets redacted.
type SanitizedConfig struct {
	Name            string
	Provider        Provider
	Endpoint        string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	UseSSL          bool
	Timeout         time.Duration
}

// Validate checks that the configuration is complete and consistent.
func (c Config) Validate() error {
	if err := validation.RequireNonEmpty("name", c.Name); err != nil {
		return validationError("Config.Validate", err.Error(), err)
	}
	if err := validation.RequireNonEmpty("provider", string(c.Provider)); err != nil {
		return validationError("Config.Validate", err.Error(), err)
	}
	if err := validation.RequireNonEmpty("bucket", c.Bucket); err != nil {
		return validationError("Config.Validate", err.Error(), err)
	}
	if err := validation.RequireNonEmpty("endpoint", c.Endpoint); err != nil {
		return validationError("Config.Validate", err.Error(), err)
	}
	if err := validation.RequireNonEmpty("access_key_id", c.AccessKeyID); err != nil {
		return validationError("Config.Validate", err.Error(), err)
	}
	if err := validation.RequireNonEmpty("secret_access_key", c.SecretAccessKey); err != nil {
		return validationError("Config.Validate", err.Error(), err)
	}
	if c.Timeout < 0 {
		err := errors.New("timeout must not be negative")
		return validationError("Config.Validate", err.Error(), err)
	}
	switch c.Provider {
	case ProviderS3, ProviderOSS, ProviderMinIO, ProviderAzure, ProviderGCS:
	default:
		err := errors.New("unsupported provider: " + string(c.Provider))
		return validationError("Config.Validate", err.Error(), err)
	}
	return nil
}

// Sanitize returns a copy of the config with secrets redacted for safe logging.
func (c Config) Sanitize() SanitizedConfig {
	return SanitizedConfig{
		Name:            c.Name,
		Provider:        c.Provider,
		Endpoint:        c.Endpoint,
		Region:          c.Region,
		Bucket:          c.Bucket,
		AccessKeyID:     sanitize.Secret(c.AccessKeyID),
		SecretAccessKey: sanitize.Secret(c.SecretAccessKey),
		UseSSL:          c.UseSSL,
		Timeout:         c.Timeout,
	}
}
