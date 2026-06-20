package ossx

import (
	"fmt"
	"time"
)

// Config is the module-owned configuration. Composition roots populate
// it from external sources (e.g., configx via env / files) and pass it to
// NewBlobStore. ossx itself does NOT import any configuration loader (BR-002).
type Config struct {
	Endpoint  string
	Region    string
	Bucket    string
	PathStyle bool
	UseSSL    bool // secure (HTTPS) transport — Aliyun OSS default true
	CNAME     string

	// Credentials are populated by the composition root from an operator-owned
	// secret store (for example, sre/secrets/env/dev.md). Sanitize masks them
	// for logs.
	AccessKey string
	SecretKey string

	Timeouts  Timeouts
	Checksum  ChecksumPolicy
	Multipart MultipartPolicy
	Presign   PresignPolicy
	Policy    PolicyConfig // FR-007: lifecycle / retention / permission
	Retry     RetryConfig
}

// Timeouts captures connect and operation deadlines.
type Timeouts struct {
	Connect   time.Duration
	Operation time.Duration
}

// ChecksumPolicy controls integrity validation.
type ChecksumPolicy struct {
	Required   bool
	Algorithms []ChecksumAlgorithm
}

// MultipartPolicy bounds multipart upload behavior.
type MultipartPolicy struct {
	MinPartSize    int64
	MaxParts       int
	StaleTTL       time.Duration // abandoned upload cleanup threshold (FR-005)
	MaxConcurrency int           // concurrent part uploads
}

// PresignPolicy bounds presigned URL issuance (BR-008).
type PresignPolicy struct {
	MaxTTL            time.Duration
	AllowedOperations []PresignOperation
}

// PolicyConfig captures lifecycle / retention / permission policy (FR-007).
type PolicyConfig struct {
	Lifecycle  LifecyclePolicy
	Retention  RetentionPolicy
	Permission PermissionPolicy
}

// LifecyclePolicy bounds object lifecycle transitions (FR-007).
type LifecyclePolicy struct {
	Enabled      bool
	MinDays      int // days before transition; negative invalid
	StorageClass string
}

// RetentionPolicy bounds object retention (FR-007). Negative or contradictory
// values are rejected at Validate().
type RetentionPolicy struct {
	Mode    RetentionMode
	MaxDays int
}

// RetentionMode enumerates retention governance.
type RetentionMode string

const (
	RetentionModeNone       RetentionMode = "none"
	RetentionModeGovernance RetentionMode = "governance"
	RetentionModeCompliance RetentionMode = "compliance"
)

// PermissionPolicy bounds who may perform operations (FR-007). Validated before
// presign and write operations.
type PermissionPolicy struct {
	AllowedPrefixes []string // object key prefixes that may be written/presigned
	DeniedPrefixes  []string
}

// RetryConfig bounds resiliencx retry/circuit behavior (FR-003/005).
type RetryConfig struct {
	MaxAttempts      int
	InitialWait      time.Duration
	MaxWait          time.Duration
	Multiplier       float64
	CircuitThreshold int           // consecutive failures before breaker opens
	CircuitCooldown  time.Duration // breaker open duration
}

// ChecksumAlgorithm enumerates supported integrity algorithms.
type ChecksumAlgorithm string

const (
	ChecksumSHA256 ChecksumAlgorithm = "sha256"
	ChecksumMD5    ChecksumAlgorithm = "md5"
	ChecksumCRC32  ChecksumAlgorithm = "crc32"
)

// PresignOperation enumerates allowed presigned operations.
type PresignOperation string

const (
	PresignGet PresignOperation = "GET"
	PresignPut PresignOperation = "PUT"
)

// MaxAllowedPresignTTL is the absolute upper bound per BR-008.
const MaxAllowedPresignTTL = 15 * time.Minute

// DefaultRetry returns the default retry policy used when RetryConfig is zero.
func DefaultRetry() RetryConfig {
	return RetryConfig{
		MaxAttempts:      3,
		InitialWait:      100 * time.Millisecond,
		MaxWait:          5 * time.Second,
		Multiplier:       2,
		CircuitThreshold: 5,
		CircuitCooldown:  30 * time.Second,
	}
}

// Validate checks Config invariants. Returns an *Error of kind config for any
// violation per FR-001 / AC-OSS-001 / AC-OSS-007.
func (c Config) Validate() error {
	if c.Endpoint == "" {
		return newError(ErrorKindConfig, "config", "endpoint empty")
	}
	if c.Bucket == "" {
		return newError(ErrorKindConfig, "config", "bucket empty")
	}
	if c.Region == "" {
		return newError(ErrorKindConfig, "config", "region empty")
	}
	if c.Timeouts.Operation < 0 {
		return newError(ErrorKindConfig, "config", "negative operation timeout")
	}
	if c.Timeouts.Connect < 0 {
		return newError(ErrorKindConfig, "config", "negative connect timeout")
	}
	if c.Multipart.MinPartSize < 0 {
		return newError(ErrorKindConfig, "config", "negative multipart min part size")
	}
	if c.Multipart.MaxParts < 0 {
		return newError(ErrorKindConfig, "config", "negative multipart max parts")
	}
	if c.Multipart.StaleTTL < 0 {
		return newError(ErrorKindConfig, "config", "negative multipart stale TTL")
	}
	if c.Presign.MaxTTL > MaxAllowedPresignTTL {
		return newError(ErrorKindConfig, "config", fmt.Sprintf("presign max TTL %s exceeds %s", c.Presign.MaxTTL, MaxAllowedPresignTTL))
	}
	if c.Presign.MaxTTL < 0 {
		return newError(ErrorKindConfig, "config", "negative presign TTL")
	}
	for _, alg := range c.Checksum.Algorithms {
		switch alg {
		case ChecksumSHA256, ChecksumMD5, ChecksumCRC32:
		default:
			return newError(ErrorKindConfig, "config", fmt.Sprintf("unsupported checksum algorithm %q", alg))
		}
	}
	// FR-007: lifecycle policy validation (AC-OSS-007).
	if c.Policy.Lifecycle.Enabled {
		if c.Policy.Lifecycle.MinDays < 0 {
			return newError(ErrorKindConfig, "config", "lifecycle MinDays negative")
		}
		if c.Policy.Lifecycle.StorageClass == "" {
			return newError(ErrorKindConfig, "config", "lifecycle enabled without StorageClass")
		}
	}
	// FR-007: retention policy validation — reject negative/contradictory values.
	switch c.Policy.Retention.Mode {
	case RetentionModeNone, RetentionModeGovernance, RetentionModeCompliance, "":
	default:
		return newError(ErrorKindConfig, "config", fmt.Sprintf("unsupported retention mode %q", c.Policy.Retention.Mode))
	}
	if c.Policy.Retention.Mode != RetentionModeNone && c.Policy.Retention.Mode != "" {
		if c.Policy.Retention.MaxDays < 0 {
			return newError(ErrorKindConfig, "config", "retention MaxDays negative")
		}
	}
	// FR-007: permission policy — denied must not overlap allowed.
	for _, a := range c.Policy.Permission.AllowedPrefixes {
		for _, d := range c.Policy.Permission.DeniedPrefixes {
			if a == d {
				return newError(ErrorKindConfig, "config", fmt.Sprintf("prefix %q both allowed and denied", a))
			}
		}
	}
	// Retry config sanity (zero values are filled by DefaultRetry at construct).
	if c.Retry.MaxAttempts < 0 {
		return newError(ErrorKindConfig, "config", "retry MaxAttempts negative")
	}
	if c.Retry.Multiplier < 0 {
		return newError(ErrorKindConfig, "config", "retry Multiplier negative")
	}
	return nil
}

// Sanitize returns a copy of Config with credential fields masked so it is safe
// to log or serialize (FR-009 / BR-009 / AC-OSS-009). Mirrors redisx Config.Sanitize.
func (c Config) Sanitize() Config {
	out := c
	if out.AccessKey != "" {
		out.AccessKey = maskSecret(out.AccessKey)
	}
	if out.SecretKey != "" {
		out.SecretKey = maskSecret(out.SecretKey)
	}
	return out
}

// maskSecret returns a masked form: first 2 + last 2 chars with middle elided.
func maskSecret(s string) string {
	if len(s) <= 4 {
		return "***"
	}
	return s[:2] + "***" + s[len(s)-2:]
}

// withDefaults fills zero-valued policy/retry fields with safe defaults.
func (c Config) withDefaults() Config {
	out := c
	if out.Retry.MaxAttempts == 0 && out.Retry.InitialWait == 0 {
		out.Retry = DefaultRetry()
	}
	if out.Multipart.MaxParts == 0 {
		out.Multipart.MaxParts = 10000
	}
	if out.Multipart.MaxConcurrency == 0 {
		out.Multipart.MaxConcurrency = 4
	}
	if out.Presign.MaxTTL == 0 {
		out.Presign.MaxTTL = MaxAllowedPresignTTL
	}
	return out
}
