package ossx

import (
	"fmt"
	"time"
)

// Config is the module-owned configuration. Composition roots populate
// it from external sources (e.g., configx); ossx itself does not import
// any configuration loader (BR-002).
type Config struct {
	Endpoint  string
	Region    string
	Bucket    string
	PathStyle bool

	Timeouts  Timeouts
	Checksum  ChecksumPolicy
	Multipart MultipartPolicy
	Presign   PresignPolicy
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
	MinPartSize int64
	MaxParts    int
}

// PresignPolicy bounds presigned URL issuance (BR-008).
type PresignPolicy struct {
	MaxTTL            time.Duration
	AllowedOperations []PresignOperation
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

// Validate checks Config invariants. Returns ErrInvalidConfig (wrapped)
// for any violation per FR-001.
func (c Config) Validate() error {
	if c.Endpoint == "" {
		return fmt.Errorf("%w: endpoint empty", ErrInvalidConfig)
	}
	if c.Bucket == "" {
		return fmt.Errorf("%w: bucket empty", ErrInvalidConfig)
	}
	if c.Region == "" {
		return fmt.Errorf("%w: region empty", ErrInvalidConfig)
	}
	if c.Timeouts.Operation < 0 {
		return fmt.Errorf("%w: negative operation timeout", ErrInvalidConfig)
	}
	if c.Multipart.MinPartSize < 0 {
		return fmt.Errorf("%w: negative multipart min part size", ErrInvalidConfig)
	}
	if c.Multipart.MaxParts < 0 {
		return fmt.Errorf("%w: negative multipart max parts", ErrInvalidConfig)
	}
	if c.Presign.MaxTTL > MaxAllowedPresignTTL {
		return fmt.Errorf("%w: presign max TTL %s exceeds %s", ErrInvalidConfig, c.Presign.MaxTTL, MaxAllowedPresignTTL)
	}
	if c.Presign.MaxTTL < 0 {
		return fmt.Errorf("%w: negative presign TTL", ErrInvalidConfig)
	}
	for _, alg := range c.Checksum.Algorithms {
		switch alg {
		case ChecksumSHA256, ChecksumMD5, ChecksumCRC32:
		default:
			return fmt.Errorf("%w: unsupported checksum algorithm %q", ErrInvalidConfig, alg)
		}
	}
	return nil
}
