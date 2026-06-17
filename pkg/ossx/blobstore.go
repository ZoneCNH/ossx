package ossx

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
)

// BlobStore is the public storage API per SPEC §8.
// All operations accept context.Context per BR-001.
type BlobStore interface {
	Put(ctx context.Context, key Key, body io.Reader, opts PutOptions) (ObjectInfo, error)
	Get(ctx context.Context, key Key, opts GetOptions) (*ObjectReader, error)
	Delete(ctx context.Context, key Key, opts DeleteOptions) error
	Copy(ctx context.Context, source Key, target Key, opts CopyOptions) (ObjectInfo, error)
	Head(ctx context.Context, key Key) (ObjectInfo, error)
	Exists(ctx context.Context, key Key) (bool, error)
	List(ctx context.Context, prefix Prefix, opts ListOptions) (ListPage, error)
	Multipart(ctx context.Context) MultipartSession
	Presign(ctx context.Context, key Key, op PresignOperation, opts PresignOptions) (PresignedURL, error)
	Health(ctx context.Context) HealthReport
	Close(ctx context.Context) error
}

// ObjectStorageAdapter is the SPI per FR-008. Concrete adapters (e.g.,
// adapters/s3) implement this without leaking SDK types into the public API.
type ObjectStorageAdapter interface {
	PutObject(ctx context.Context, key string, body []byte, contentType string, metadata map[string]string) (string /* etag */, error)
	GetObject(ctx context.Context, key string) ([]byte, ObjectInfo, error)
	DeleteObject(ctx context.Context, key string) error
	HeadObject(ctx context.Context, key string) (ObjectInfo, error)
	ListObjects(ctx context.Context, prefix string, max int, token string) ([]ObjectInfo, string, error)
	CloseAdapter(ctx context.Context) error
	Name() string
}

// Hooks captures observability hooks per FR-009; nil-safe via no-op default.
type Hooks struct {
	OnOperation func(name string, key Key, latencyNs int64, sizeBytes int64, errorClass string)
}

// blobStore is the concrete implementation backed by an adapter.
type blobStore struct {
	cfg     Config
	adapter ObjectStorageAdapter
	hooks   Hooks

	mu     sync.RWMutex
	closed bool
}

// NewBlobStore constructs a BlobStore per FR-001.
//
// adapter MUST NOT be nil; for tests use NewInMemoryAdapter.
// hooks may be zero-value (no-op).
func NewBlobStore(cfg Config, adapter ObjectStorageAdapter, hooks Hooks) (BlobStore, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if adapter == nil {
		return nil, fmt.Errorf("%w: adapter is nil", ErrInvalidConfig)
	}
	return &blobStore{cfg: cfg, adapter: adapter, hooks: hooks}, nil
}

func (b *blobStore) checkClosed() error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.closed {
		return ErrClosed
	}
	return nil
}

func (b *blobStore) emit(op string, key Key, start time.Time, size int64, err error) {
	if b.hooks.OnOperation == nil {
		return
	}
	class := "ok"
	if err != nil {
		class = errorClass(err)
	}
	b.hooks.OnOperation(op, key, time.Since(start).Nanoseconds(), size, class)
}

// errorClass returns a stable category string for hook payloads.
func errorClass(err error) string {
	switch {
	case errors.Is(err, ErrNotFound):
		return "not_found"
	case errors.Is(err, ErrConflict):
		return "conflict"
	case errors.Is(err, ErrPermission):
		return "permission"
	case errors.Is(err, ErrChecksumMismatch):
		return "checksum_mismatch"
	case errors.Is(err, ErrTimeout):
		return "timeout"
	case errors.Is(err, ErrCancelled):
		return "cancelled"
	case errors.Is(err, ErrInvalidKey), errors.Is(err, ErrInvalidConfig), errors.Is(err, ErrInvalidMetadata):
		return "validation"
	case errors.Is(err, ErrClosed):
		return "closed"
	case errors.Is(err, ErrNotImplemented):
		return "not_implemented"
	case errors.Is(err, ErrProviderFailure):
		return "provider"
	default:
		return "other"
	}
}

func (b *blobStore) Put(ctx context.Context, key Key, body io.Reader, opts PutOptions) (info ObjectInfo, err error) {
	start := time.Now()
	defer func() { b.emit("put", key, start, info.Size, err) }()

	if err := b.checkClosed(); err != nil {
		return ObjectInfo{}, err
	}
	if err := ctx.Err(); err != nil {
		return ObjectInfo{}, ctxErr(err)
	}
	if _, kerr := NewKey(string(key)); kerr != nil {
		return ObjectInfo{}, kerr
	}
	if err := validateMetadata(opts.Metadata); err != nil {
		return ObjectInfo{}, err
	}
	if opts.ChecksumAlgo != "" {
		if err := validateChecksumAlgo(opts.ChecksumAlgo, b.cfg.Checksum.Algorithms); err != nil {
			return ObjectInfo{}, err
		}
	}

	data, err := io.ReadAll(body)
	if err != nil {
		return ObjectInfo{}, fmt.Errorf("%w: read body: %v", ErrProviderFailure, err)
	}

	etag, err := b.adapter.PutObject(ctx, string(key), data, opts.ContentType, opts.Metadata)
	if err != nil {
		return ObjectInfo{}, err
	}

	info = ObjectInfo{
		Key:          key,
		Size:         int64(len(data)),
		ContentType:  opts.ContentType,
		Metadata:     copyStringMap(opts.Metadata),
		Tags:         copyStringMap(opts.Tags),
		ChecksumAlgo: opts.ChecksumAlgo,
		ChecksumHex:  computeChecksum(opts.ChecksumAlgo, data),
		ETag:         etag,
		ModifiedAt:   time.Now().UTC(),
		CreatedAt:    time.Now().UTC(),
	}
	return info, nil
}

func (b *blobStore) Get(ctx context.Context, key Key, opts GetOptions) (reader *ObjectReader, err error) {
	start := time.Now()
	var size int64
	defer func() {
		if reader != nil {
			size = reader.Info.Size
		}
		b.emit("get", key, start, size, err)
	}()

	if err := b.checkClosed(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, ctxErr(err)
	}
	if _, kerr := NewKey(string(key)); kerr != nil {
		return nil, kerr
	}

	data, info, err := b.adapter.GetObject(ctx, string(key))
	if err != nil {
		return nil, err
	}
	verified := false
	if opts.VerifyChecksum && info.ChecksumAlgo != "" && info.ChecksumHex != "" {
		got := computeChecksum(info.ChecksumAlgo, data)
		if got != info.ChecksumHex {
			return nil, fmt.Errorf("%w: have %s want %s", ErrChecksumMismatch, got, info.ChecksumHex)
		}
		verified = true
	}
	return &ObjectReader{
		ReadCloser:       io.NopCloser(bytes.NewReader(data)),
		Info:             info,
		ChecksumVerified: verified,
	}, nil
}

func (b *blobStore) Delete(ctx context.Context, key Key, opts DeleteOptions) (err error) {
	start := time.Now()
	defer func() { b.emit("delete", key, start, 0, err) }()

	if err := b.checkClosed(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return ctxErr(err)
	}
	if _, kerr := NewKey(string(key)); kerr != nil {
		return kerr
	}

	err = b.adapter.DeleteObject(ctx, string(key))
	if err != nil && errors.Is(err, ErrNotFound) && !opts.StrictNotFound {
		return nil // idempotent
	}
	return err
}

func (b *blobStore) Copy(ctx context.Context, source Key, target Key, opts CopyOptions) (info ObjectInfo, err error) {
	start := time.Now()
	defer func() { b.emit("copy", target, start, info.Size, err) }()

	if err := b.checkClosed(); err != nil {
		return ObjectInfo{}, err
	}
	if err := ctx.Err(); err != nil {
		return ObjectInfo{}, ctxErr(err)
	}
	if _, kerr := NewKey(string(source)); kerr != nil {
		return ObjectInfo{}, kerr
	}
	if _, kerr := NewKey(string(target)); kerr != nil {
		return ObjectInfo{}, kerr
	}
	data, srcInfo, err := b.adapter.GetObject(ctx, string(source))
	if err != nil {
		return ObjectInfo{}, err
	}
	ct := opts.ContentType
	if ct == "" {
		ct = srcInfo.ContentType
	}
	md := opts.Metadata
	if md == nil {
		md = srcInfo.Metadata
	}
	etag, err := b.adapter.PutObject(ctx, string(target), data, ct, md)
	if err != nil {
		return ObjectInfo{}, err
	}
	info = srcInfo
	info.Key = target
	info.ContentType = ct
	info.Metadata = copyStringMap(md)
	info.ETag = etag
	info.ModifiedAt = time.Now().UTC()
	return info, nil
}

func (b *blobStore) Head(ctx context.Context, key Key) (info ObjectInfo, err error) {
	start := time.Now()
	defer func() { b.emit("head", key, start, info.Size, err) }()

	if err := b.checkClosed(); err != nil {
		return ObjectInfo{}, err
	}
	if err := ctx.Err(); err != nil {
		return ObjectInfo{}, ctxErr(err)
	}
	if _, kerr := NewKey(string(key)); kerr != nil {
		return ObjectInfo{}, kerr
	}
	return b.adapter.HeadObject(ctx, string(key))
}

func (b *blobStore) Exists(ctx context.Context, key Key) (bool, error) {
	_, err := b.Head(ctx, key)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return false, err
}

func (b *blobStore) List(ctx context.Context, prefix Prefix, opts ListOptions) (page ListPage, err error) {
	start := time.Now()
	defer func() { b.emit("list", Key(prefix), start, int64(len(page.Items)), err) }()

	if err := b.checkClosed(); err != nil {
		return ListPage{}, err
	}
	if err := ctx.Err(); err != nil {
		return ListPage{}, ctxErr(err)
	}
	max := opts.MaxKeys
	if max <= 0 || max > 1000 {
		max = 1000 // BR-006 bounded page size default cap.
	}
	items, next, err := b.adapter.ListObjects(ctx, string(prefix), max, opts.ContinuationToken)
	if err != nil {
		return ListPage{}, err
	}
	return ListPage{Items: items, NextContinuation: next, IsTruncated: next != ""}, nil
}

func (b *blobStore) Multipart(ctx context.Context) MultipartSession {
	return &notImplementedSession{}
}

func (b *blobStore) Presign(ctx context.Context, key Key, op PresignOperation, opts PresignOptions) (PresignedURL, error) {
	if err := b.checkClosed(); err != nil {
		return PresignedURL{}, err
	}
	if _, kerr := NewKey(string(key)); kerr != nil {
		return PresignedURL{}, kerr
	}
	allowed := false
	for _, allowedOp := range b.cfg.Presign.AllowedOperations {
		if allowedOp == op {
			allowed = true
			break
		}
	}
	if !allowed {
		return PresignedURL{}, fmt.Errorf("%w: presign op %q not allowed", ErrPermission, op)
	}
	if opts.TTL <= 0 {
		return PresignedURL{}, fmt.Errorf("%w: presign TTL must be positive", ErrInvalidConfig)
	}
	maxTTL := int64(b.cfg.Presign.MaxTTL.Seconds())
	if maxTTL == 0 {
		maxTTL = int64(MaxAllowedPresignTTL.Seconds())
	}
	if opts.TTL > maxTTL {
		return PresignedURL{}, fmt.Errorf("%w: presign TTL %ds exceeds max %ds", ErrInvalidConfig, opts.TTL, maxTTL)
	}
	// v1.0.2-alpha: signature implementation deferred to v1.1.0 with adapter SDK.
	return PresignedURL{}, fmt.Errorf("%w: presign signing requires SDK adapter (planned v1.1.0)", ErrNotImplemented)
}

func (b *blobStore) Health(ctx context.Context) HealthReport {
	if err := b.checkClosed(); err != nil {
		return HealthReport{Ready: false, Error: err.Error(), LastCheckedAt: time.Now().Unix()}
	}
	return HealthReport{
		Ready:          true,
		ProviderStatus: b.adapter.Name(),
		LastCheckedAt:  time.Now().Unix(),
	}
}

func (b *blobStore) Close(ctx context.Context) error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil // idempotent (BR / FR-010)
	}
	b.closed = true
	b.mu.Unlock()
	return b.adapter.CloseAdapter(ctx)
}

// notImplementedSession is the v1.0.2-alpha multipart placeholder.
type notImplementedSession struct{}

func (notImplementedSession) Initiate(_ context.Context, _ Key, _ PutOptions) (UploadID, error) {
	return "", ErrNotImplemented
}
func (notImplementedSession) UploadPart(_ context.Context, _ UploadID, _ int, _ io.Reader) (PartETag, error) {
	return PartETag{}, ErrNotImplemented
}
func (notImplementedSession) ListParts(_ context.Context, _ UploadID) ([]PartETag, error) {
	return nil, ErrNotImplemented
}
func (notImplementedSession) Complete(_ context.Context, _ UploadID, _ []PartETag) (ObjectInfo, error) {
	return ObjectInfo{}, ErrNotImplemented
}
func (notImplementedSession) Abort(_ context.Context, _ UploadID) error {
	return ErrNotImplemented
}

func validateChecksumAlgo(alg ChecksumAlgorithm, allowed []ChecksumAlgorithm) error {
	if len(allowed) == 0 {
		switch alg {
		case ChecksumSHA256, ChecksumMD5, ChecksumCRC32:
			return nil
		default:
			return fmt.Errorf("%w: unsupported checksum %q", ErrInvalidConfig, alg)
		}
	}
	for _, a := range allowed {
		if a == alg {
			return nil
		}
	}
	return fmt.Errorf("%w: checksum %q not in policy", ErrInvalidConfig, alg)
}

func computeChecksum(alg ChecksumAlgorithm, data []byte) string {
	switch alg {
	case ChecksumSHA256:
		sum := sha256.Sum256(data)
		return hex.EncodeToString(sum[:])
	case ChecksumMD5:
		sum := md5.Sum(data)
		return hex.EncodeToString(sum[:])
	case ChecksumCRC32:
		sum := crc32.ChecksumIEEE(data)
		return fmt.Sprintf("%08x", sum)
	default:
		return ""
	}
}

func ctxErr(err error) error {
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("%w: %v", ErrCancelled, err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: %v", ErrTimeout, err)
	}
	return err
}

func copyStringMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// SortedKeys returns ObjectInfo items sorted by Key — used by the in-memory
// adapter to produce stable List output (BR-006).
func SortedKeys(items []ObjectInfo) []ObjectInfo {
	out := make([]ObjectInfo, len(items))
	copy(out, items)
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// HasPrefix is a small helper exported for adapter implementations.
func HasPrefix(key, prefix string) bool { return strings.HasPrefix(key, prefix) }
