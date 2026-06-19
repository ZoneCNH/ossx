package ossx

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"
)

// BlobStore is the public storage API per SPEC §8.
// All operations accept context.Context per BR-001.
type BlobStore interface {
	// Put streams body to key without whole-object buffering (FR-003 / FR-004).
	Put(ctx context.Context, key Key, body io.Reader, opts PutOptions) (ObjectInfo, error)
	// Get returns a streaming reader the caller MUST Close (FR-004).
	Get(ctx context.Context, key Key, opts GetOptions) (*ObjectReader, error)
	// Delete removes key; idempotent for missing objects unless StrictNotFound (FR-003).
	Delete(ctx context.Context, key Key, opts DeleteOptions) error
	// Copy duplicates source to target server-side when supported (FR-003).
	Copy(ctx context.Context, source Key, target Key, opts CopyOptions) (ObjectInfo, error)
	// Head returns metadata without downloading the body (FR-003).
	Head(ctx context.Context, key Key) (ObjectInfo, error)
	// Exists reports whether key is present (FR-003).
	Exists(ctx context.Context, key Key) (bool, error)
	// List returns a bounded page matching prefix (FR-003 / BR-006).
	List(ctx context.Context, prefix Prefix, opts ListOptions) (ListPage, error)
}

// MultipartStarter exposes multipart upload sessions for callers that need
// FR-005 without widening the core BlobStore surface.
type MultipartStarter interface {
	// Multipart starts a multipart upload session (FR-005).
	Multipart(ctx context.Context) (MultipartSession, error)
}

// Presigner exposes presigned URL creation for callers that need FR-006.
type Presigner interface {
	// Presign generates a signed URL for op with ttl-second expiry (FR-006).
	Presign(ctx context.Context, key Key, op PresignOperation, opts PresignOptions) (PresignedURL, error)
}

// HealthChecker exposes readiness checks for callers that need FR-010.
type HealthChecker interface {
	// Health reports readiness, distinguishing config / unreachable / degraded (FR-010).
	Health(ctx context.Context) HealthReport
}

// StoreCloser exposes resource shutdown for callers that own a store instance.
type StoreCloser interface {
	// Close releases resources; idempotent (FR-010).
	Close(ctx context.Context) error
}

// Store is the concrete implementation backed by split adapter capabilities.
type Store struct {
	cfg       Config
	adapter   StoreAdapter
	multipart MultipartAdapter
	presigner PresignAdapter
	lifecycle AdapterLifecycle
	hooks     Hooks
	retry     retryPolicy
	breakers  map[string]*circuitBreaker // per-operation breakers
	bMu       sync.Mutex
	mu        sync.RWMutex
	closed    bool
}

// NewBlobStore constructs a Store per FR-001.
//
// adapter MUST NOT be nil (for tests use NewInMemoryAdapter). hooks may be
// zero-value (no-op). Config is validated and filled with defaults.
func NewBlobStore(cfg Config, adapter StoreAdapter, hooks Hooks) (*Store, error) {
	cfg = cfg.withDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if adapter == nil {
		return nil, newError(ErrorKindConfig, "NewBlobStore", "adapter is nil")
	}
	multipart, ok := adapter.(MultipartAdapter)
	if !ok {
		return nil, newError(ErrorKindConfig, "NewBlobStore", "adapter missing multipart capability")
	}
	presigner, ok := adapter.(PresignAdapter)
	if !ok {
		return nil, newError(ErrorKindConfig, "NewBlobStore", "adapter missing presign capability")
	}
	lifecycle, ok := adapter.(AdapterLifecycle)
	if !ok {
		return nil, newError(ErrorKindConfig, "NewBlobStore", "adapter missing lifecycle capability")
	}
	return &Store{
		cfg:       cfg,
		adapter:   adapter,
		multipart: multipart,
		presigner: presigner,
		lifecycle: lifecycle,
		hooks:     hooks.withDefaults(),
		retry:     retryPolicyFromConfig(cfg.Retry),
		breakers:  map[string]*circuitBreaker{},
	}, nil
}

// breakerFor returns (creating if needed) the breaker for an operation.
func (b *Store) breakerFor(op string) *circuitBreaker {
	b.bMu.Lock()
	defer b.bMu.Unlock()
	cb, ok := b.breakers[op]
	if !ok {
		cb = newCircuitBreaker(b.cfg.Retry.CircuitThreshold, b.cfg.Retry.CircuitCooldown)
		b.breakers[op] = cb
	}
	return cb
}

// run executes op through retry + circuit breaker, then emits metrics.
func (b *Store) run(ctx context.Context, op string, key Key, fn func(context.Context) error) (time.Duration, error) {
	start := time.Now()
	cb := b.breakerFor(op)
	err := cb.do(ctx, op, b.retry, fn)
	latency := time.Since(start)
	result := "ok"
	if err != nil {
		result = string(errorKind(err))
	}
	b.hooks.emit(op, result, key.SanitizedScope(), 0, latency)
	return latency, err
}

func (b *Store) checkClosed() error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.closed {
		return ErrClosed
	}
	return nil
}

// Put streams body to key (FR-003 / FR-004). The adapter receives the raw
// io.Reader; ossx does NOT buffer the whole object.
func (b *Store) Put(ctx context.Context, key Key, body io.Reader, opts PutOptions) (ObjectInfo, error) {
	if err := b.checkClosed(); err != nil {
		return ObjectInfo{}, err
	}
	if err := ctxErrCheck(ctx); err != nil {
		return ObjectInfo{}, err
	}
	if _, err := NewKey(string(key)); err != nil {
		return ObjectInfo{}, err
	}
	if err := validateMetadata(opts.Metadata); err != nil {
		return ObjectInfo{}, err
	}
	if opts.ChecksumAlgo != "" {
		if err := validateChecksumAlgo(opts.ChecksumAlgo, b.cfg.Checksum.Algorithms); err != nil {
			return ObjectInfo{}, err
		}
	}
	// FR-007: permission policy gate before write.
	if err := validateKeyPolicy(b.cfg.Policy.Permission, string(key)); err != nil {
		return ObjectInfo{}, err
	}

	adapterOpts := PutAdapterOptions{
		ContentType:  opts.ContentType,
		Metadata:     opts.Metadata,
		Tags:         opts.Tags,
		ChecksumAlgo: opts.ChecksumAlgo,
	}
	var info ObjectInfo
	_, err := b.run(ctx, "put", key, func(ctx context.Context) error {
		var perr error
		info, perr = b.adapter.PutObject(ctx, string(key), body, -1, adapterOpts)
		return perr
	})
	if err != nil {
		return ObjectInfo{}, err
	}
	return info, nil
}

// Get returns a streaming reader (FR-004). The caller MUST Close it.
func (b *Store) Get(ctx context.Context, key Key, opts GetOptions) (*ObjectReader, error) {
	if err := b.checkClosed(); err != nil {
		return nil, err
	}
	if err := ctxErrCheck(ctx); err != nil {
		return nil, err
	}
	if _, err := NewKey(string(key)); err != nil {
		return nil, err
	}

	var rc io.ReadCloser
	var info ObjectInfo
	_, err := b.run(ctx, "get", key, func(ctx context.Context) error {
		var gerr error
		rc, info, gerr = b.adapter.GetObject(ctx, string(key))
		return gerr
	})
	if err != nil {
		return nil, err
	}

	reader := &ObjectReader{ReadCloser: rc, Info: info}
	// FR-002 / BR-010: optional checksum verification streams through a tee.
	if opts.VerifyChecksum && info.ChecksumAlgo != "" && info.ChecksumHex != "" {
		reader = wrapChecksumVerifier(reader, info)
	}
	return reader, nil
}

// Delete removes key (FR-003). Idempotent for missing objects unless strict.
func (b *Store) Delete(ctx context.Context, key Key, opts DeleteOptions) error {
	if err := b.checkClosed(); err != nil {
		return err
	}
	if err := ctxErrCheck(ctx); err != nil {
		return err
	}
	if _, err := NewKey(string(key)); err != nil {
		return err
	}
	retentionEnabled := b.cfg.Policy.Retention.Mode != RetentionModeNone && b.cfg.Policy.Retention.Mode != ""
	if opts.StrictNotFound || retentionEnabled {
		var info ObjectInfo
		_, herr := b.run(ctx, "head", key, func(ctx context.Context) error {
			var err error
			info, err = b.adapter.HeadObject(ctx, string(key))
			return err
		})
		if herr != nil {
			if opts.StrictNotFound {
				return herr
			}
		} else if retentionEnabled {
			// FR-007: retention policy gate (need object metadata for age check).
			if err := validateRetentionDelete(b.cfg.Policy.Retention, info, time.Now()); err != nil {
				return err
			}
		}
	}
	_, err := b.run(ctx, "delete", key, func(ctx context.Context) error {
		return b.adapter.DeleteObject(ctx, string(key), opts.StrictNotFound)
	})
	if err != nil {
		// Idempotent: a missing object on strict=false is not an error.
		var se *Error
		if errors.As(err, &se) && se.Kind == ErrorKindNotFound && !opts.StrictNotFound {
			return nil
		}
	}
	return err
}

// Copy duplicates source to target (FR-003). Server-side when the adapter supports it.
func (b *Store) Copy(ctx context.Context, source Key, target Key, opts CopyOptions) (ObjectInfo, error) {
	if err := b.checkClosed(); err != nil {
		return ObjectInfo{}, err
	}
	if err := ctxErrCheck(ctx); err != nil {
		return ObjectInfo{}, err
	}
	if _, err := NewKey(string(source)); err != nil {
		return ObjectInfo{}, err
	}
	if _, err := NewKey(string(target)); err != nil {
		return ObjectInfo{}, err
	}
	if err := validateKeyPolicy(b.cfg.Policy.Permission, string(target)); err != nil {
		return ObjectInfo{}, err
	}
	adapterOpts := CopyAdapterOptions(opts)
	var info ObjectInfo
	_, err := b.run(ctx, "copy", target, func(ctx context.Context) error {
		var perr error
		info, perr = b.adapter.CopyObject(ctx, string(source), string(target), adapterOpts)
		return perr
	})
	if err != nil {
		return ObjectInfo{}, err
	}
	return info, nil
}

// Head returns metadata without downloading the body (FR-003).
func (b *Store) Head(ctx context.Context, key Key) (ObjectInfo, error) {
	if err := b.checkClosed(); err != nil {
		return ObjectInfo{}, err
	}
	if err := ctxErrCheck(ctx); err != nil {
		return ObjectInfo{}, err
	}
	if _, err := NewKey(string(key)); err != nil {
		return ObjectInfo{}, err
	}
	var info ObjectInfo
	_, err := b.run(ctx, "head", key, func(ctx context.Context) error {
		var perr error
		info, perr = b.adapter.HeadObject(ctx, string(key))
		return perr
	})
	if err != nil {
		return ObjectInfo{}, err
	}
	return info, nil
}

// Exists reports whether key is present (FR-003).
func (b *Store) Exists(ctx context.Context, key Key) (bool, error) {
	_, err := b.Head(ctx, key)
	if err == nil {
		return true, nil
	}
	var se *Error
	if errors.As(err, &se) && se.Kind == ErrorKindNotFound {
		return false, nil
	}
	return false, err
}

// List returns a bounded page matching prefix (FR-003 / BR-006).
func (b *Store) List(ctx context.Context, prefix Prefix, opts ListOptions) (ListPage, error) {
	if err := b.checkClosed(); err != nil {
		return ListPage{}, err
	}
	if err := ctxErrCheck(ctx); err != nil {
		return ListPage{}, err
	}
	max := opts.MaxKeys
	if max <= 0 || max > 1000 {
		max = 1000 // BR-006 bounded page size default cap.
	}
	var page ListPage
	_, err := b.run(ctx, "list", Key(prefix), func(ctx context.Context) error {
		var perr error
		page, perr = b.adapter.ListObjects(ctx, string(prefix), max, opts.ContinuationToken)
		return perr
	})
	if err != nil {
		return ListPage{}, err
	}
	return page, nil
}

// Multipart starts a multipart upload session (FR-005).
func (b *Store) Multipart(ctx context.Context) (MultipartSession, error) {
	if err := b.checkClosed(); err != nil {
		return nil, err
	}
	if err := ctxErrCheck(ctx); err != nil {
		return nil, err
	}
	return &multipartSession{store: b}, nil
}

// Health reports readiness, distinguishing config / unreachable / degraded (FR-010).
func (b *Store) Health(ctx context.Context) HealthReport {
	now := time.Now()
	if err := b.checkClosed(); err != nil {
		return HealthReport{Ready: false, ProviderStatus: "closed", Error: "blobstore closed", LastCheckedAt: now.Unix()}
	}
	herr := b.lifecycle.Health(ctx)
	if herr == nil {
		return HealthReport{Ready: true, ProviderStatus: b.adapter.Name(), LastCheckedAt: now.Unix()}
	}
	kind := errorKind(herr)
	status := "degraded"
	ready := false
	switch kind {
	case ErrorKindConnection, ErrorKindUnavailable, ErrorKindTimeout:
		status = "unreachable"
	case ErrorKindAuth, ErrorKindConfig:
		status = "config_error"
	}
	b.hooks.Metrics.SetGauge(metricHealthStatus, gaugeForStatus(status), map[string]string{"provider": b.adapter.Name()})
	return HealthReport{Ready: ready, ProviderStatus: status, Error: herr.Error(), LastCheckedAt: now.Unix()}
}

// Close releases resources; idempotent (FR-010).
func (b *Store) Close(ctx context.Context) error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil // idempotent
	}
	b.closed = true
	b.mu.Unlock()
	return b.lifecycle.Close(ctx)
}

// --- helpers (kept here for visibility; some exported for adapters) ---

func ctxErrCheck(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		if errors.Is(err, context.Canceled) {
			return wrapError(ErrorKindCanceled, "", "operation cancelled", err)
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return wrapError(ErrorKindTimeout, "", "operation deadline exceeded", err)
		}
		return wrapError(ErrorKindCanceled, "", "context error", err)
	}
	return nil
}

func gaugeForStatus(status string) float64 {
	switch status {
	case "ready", "ok":
		return 1
	case "degraded":
		return 0.5
	default:
		return 0
	}
}

// SortedKeys returns ObjectInfo items sorted by Key — used by adapters to
// produce stable List output (BR-006). Exported for adapter implementations.
func SortedKeys(items []ObjectInfo) []ObjectInfo {
	return sortedObjectInfos(items)
}

// HasPrefix reports whether key starts with prefix. Exported for adapters.
func HasPrefix(key, prefix string) bool { return hasPrefix(key, prefix) }

// hasPrefix is the internal helper.
func hasPrefix(key, prefix string) bool {
	return len(key) >= len(prefix) && key[:len(prefix)] == prefix
}
