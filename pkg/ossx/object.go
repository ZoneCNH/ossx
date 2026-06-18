package ossx

import (
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"
)

// Key is a normalized object path. Use NewKey to construct.
type Key string

// Prefix is a normalized listing prefix.
type Prefix string

// MaxMetadataKeys bounds user-supplied metadata size per FR-002.
const MaxMetadataKeys = 64

// MaxMetadataValueLen bounds individual metadata value length.
const MaxMetadataValueLen = 2048

// NewKey validates and normalizes an object key per FR-002 / BR-002.
//
// Rejects:
//   - empty
//   - leading "/"
//   - traversal segments ".." or "."
//   - non-UTF8
//   - keys longer than 1024 chars
func NewKey(raw string) (Key, error) {
	if raw == "" {
		return "", fmt.Errorf("%w: empty", ErrInvalidKey)
	}
	if !utf8.ValidString(raw) {
		return "", fmt.Errorf("%w: not utf-8", ErrInvalidKey)
	}
	if len(raw) > 1024 {
		return "", fmt.Errorf("%w: too long (%d chars)", ErrInvalidKey, len(raw))
	}
	if strings.HasPrefix(raw, "/") {
		return "", fmt.Errorf("%w: absolute path", ErrInvalidKey)
	}
	for _, seg := range strings.Split(raw, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return "", fmt.Errorf("%w: ambiguous segment %q", ErrInvalidKey, seg)
		}
	}
	return Key(raw), nil
}

// String implements fmt.Stringer; identity for Key.
func (k Key) String() string { return string(k) }

// SanitizedScope returns a key prefix safe for logging (first 32 chars +
// ellipsis if longer). Per FR-009 / BR-009 raw keys may contain PII.
func (k Key) SanitizedScope() string {
	s := string(k)
	if len(s) <= 32 {
		return s
	}
	return s[:32] + "…"
}

// ObjectInfo is the metadata snapshot per SPEC §9.
type ObjectInfo struct {
	Key          Key
	Size         int64
	ContentType  string
	Metadata     map[string]string
	Tags         map[string]string
	ChecksumAlgo ChecksumAlgorithm
	ChecksumHex  string
	ETag         string
	StorageClass string
	Version      string
	Location     string // object URL (multipart complete result)
	CreatedAt    time.Time
	ModifiedAt   time.Time
}

// validateMetadata enforces the FR-002 metadata bounds.
func validateMetadata(md map[string]string) error {
	if len(md) > MaxMetadataKeys {
		return fmt.Errorf("%w: too many keys (%d > %d)", ErrInvalidMetadata, len(md), MaxMetadataKeys)
	}
	for k, v := range md {
		if k == "" {
			return fmt.Errorf("%w: empty metadata key", ErrInvalidMetadata)
		}
		if len(v) > MaxMetadataValueLen {
			return fmt.Errorf("%w: metadata value for %q exceeds %d bytes", ErrInvalidMetadata, k, MaxMetadataValueLen)
		}
	}
	return nil
}

// PutOptions captures Put-time policy.
type PutOptions struct {
	ContentType  string
	Metadata     map[string]string
	Tags         map[string]string
	ChecksumAlgo ChecksumAlgorithm
}

// GetOptions captures Get-time policy.
type GetOptions struct {
	VerifyChecksum bool
}

// DeleteOptions captures Delete-time policy.
type DeleteOptions struct {
	StrictNotFound bool // when true, missing objects produce ErrNotFound; otherwise idempotent.
}

// CopyOptions captures Copy-time policy.
type CopyOptions struct {
	Metadata    map[string]string
	ContentType string
}

// ListOptions captures List-time policy.
type ListOptions struct {
	MaxKeys           int
	ContinuationToken string
}

// ListPage is a bounded list slice (FR-003 / BR-006).
type ListPage struct {
	Items            []ObjectInfo
	NextContinuation string
	IsTruncated      bool
}

// ObjectReader is an io.ReadCloser plus metadata and checksum result.
type ObjectReader struct {
	io.ReadCloser
	Info             ObjectInfo
	ChecksumVerified bool
}
