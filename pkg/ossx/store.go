package ossx

import (
	"context"
	"io"
)

// StoreAdapter is the Storage Provider Interface that concrete adapters
// (adapters/aliyun, InMemoryAdapter, fake adapters) implement.
//
// Design (SPEC §13, FR-008): the SPI is exported so external adapter packages
// can explicitly implement it (`var _ ossx.StoreAdapter = (*aliyun.Adapter)(nil)`),
// but the public BlobStore API never exposes it — callers interact only via
// BlobStore. It is streaming-first (io.Reader / io.ReadCloser) per FR-004 / §16
// — adapters MUST NOT buffer whole objects. Provider SDK types never cross
// this boundary; adapters translate provider errors to ossx typed *Error at
// every method exit (SPEC §11, BR-011).
//
// The public BlobStore (blobstore.go) wraps a StoreAdapter and adds: context
// cancellation, policy validation, retry/circuit (retry.go), observability
// hooks (observability.go), and typed-error guarantees.
type StoreAdapter interface {
	// Name returns the adapter identifier (e.g., "aliyun-oss", "in-memory").
	Name() string

	// PutObject streams body to key. size hints total bytes (-1 = unknown).
	// Adapters MUST stream without whole-object buffering (FR-004).
	PutObject(ctx context.Context, key string, body io.Reader, size int64, opts PutAdapterOptions) (ObjectInfo, error)

	// GetObject returns a streaming reader the caller MUST Close (FR-004).
	// Adapters MUST NOT read the whole object into memory.
	GetObject(ctx context.Context, key string) (io.ReadCloser, ObjectInfo, error)

	// HeadObject returns metadata without downloading the body.
	HeadObject(ctx context.Context, key string) (ObjectInfo, error)

	// DeleteObject removes key. Idempotent for missing objects unless strict.
	DeleteObject(ctx context.Context, key string, strict bool) error

	// CopyObject duplicates source to target (server-side when supported).
	CopyObject(ctx context.Context, source, target string, opts CopyAdapterOptions) (ObjectInfo, error)

	// ListObjects returns a bounded page matching prefix (BR-006).
	ListObjects(ctx context.Context, prefix string, max int, continuation string) (ListPage, error)

	// --- Multipart (FR-005) ---

	// InitiateMultipart starts a multipart upload, returns upload id.
	InitiateMultipart(ctx context.Context, key string, opts PutAdapterOptions) (UploadID, error)

	// UploadPart appends one part; partNumber is 1-based.
	UploadPart(ctx context.Context, id UploadID, partNumber int, body io.Reader, size int64) (PartETag, error)

	// ListParts returns currently uploaded parts for an upload.
	ListParts(ctx context.Context, id UploadID) ([]PartETag, error)

	// CompleteMultipart finalizes the upload; parts must be the full ordered set.
	CompleteMultipart(ctx context.Context, id UploadID, parts []PartETag) (ObjectInfo, error)

	// AbortMultipart cancels an upload and frees staged parts. Idempotent.
	AbortMultipart(ctx context.Context, id UploadID) error

	// --- Presign (FR-006) ---

	// PresignURL generates a signed URL for op with ttl-second expiry.
	PresignURL(ctx context.Context, key string, op PresignOperation, ttlSeconds int64, opts PresignAdapterOptions) (PresignedURL, error)

	// --- Lifecycle ---

	// Health probes the provider backend (lightweight, no writes).
	Health(ctx context.Context) error

	// Close releases adapter resources. Idempotent.
	Close(ctx context.Context) error
}

// PutAdapterOptions carries adapter-level put metadata (not policy). Exported
// so external adapters (adapters/aliyun) can read its fields.
type PutAdapterOptions struct {
	ContentType  string
	Metadata     map[string]string
	Tags         map[string]string
	ChecksumAlgo ChecksumAlgorithm
	ChecksumHex  string
}

// CopyAdapterOptions carries adapter-level copy metadata. Exported for adapters.
type CopyAdapterOptions struct {
	Metadata    map[string]string
	ContentType string
}

// PresignAdapterOptions carries adapter-level presign parameters. Exported for adapters.
type PresignAdapterOptions struct {
	ContentType string
	ContentMD5  string
}

// compile-time guards: every concrete adapter must satisfy StoreAdapter.
var (
	_ StoreAdapter = (*InMemoryAdapter)(nil)
)
