package ossx

import (
	"context"
	"io"
)

// MultipartSession represents an in-flight multipart upload context.
// v1.0.2-alpha: returns ErrNotImplemented; full implementation in v1.1.0.
type MultipartSession interface {
	Initiate(ctx context.Context, key Key, opts PutOptions) (UploadID, error)
	UploadPart(ctx context.Context, id UploadID, partNumber int, body io.Reader) (PartETag, error)
	ListParts(ctx context.Context, id UploadID) ([]PartETag, error)
	Complete(ctx context.Context, id UploadID, parts []PartETag) (ObjectInfo, error)
	Abort(ctx context.Context, id UploadID) error
}

// UploadID identifies a multipart session.
type UploadID string

// PartETag binds a part number to its server-acknowledged ETag.
type PartETag struct {
	PartNumber int
	ETag       string
	Size       int64
}

// PresignedURL is the Presign result (BR-008/BR-009: opaque, signed by adapter).
type PresignedURL struct {
	URL       string
	Method    string
	ExpiresAt int64 // Unix seconds
}

// PresignOptions captures presign-time policy.
type PresignOptions struct {
	TTL          int64 // seconds; capped to PresignPolicy.MaxTTL
	ContentType  string
	ContentMD5   string
	StorageClass string
}

// HealthReport summarizes BlobStore readiness (FR-010).
type HealthReport struct {
	Ready          bool
	ProviderStatus string
	LastCheckedAt  int64
	Error          string
}
