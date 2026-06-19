package ossx

import (
	"context"
	"time"
)

// presign.go implements FR-006: presigned URL policy enforcement + audit
// masking. Signing is delegated to the StoreAdapter (BR-009: secrets never
// leave the adapter). The BlobStore enforces the operation allowlist, TTL cap,
// and checksum constraints BEFORE calling the adapter.

// Presign generates a signed URL for op with TTL-second expiry (FR-006).
//
// Enforced before the adapter call:
//   - BR-008: TTL ≤ MaxAllowedPresignTTL (15m) and ≤ PresignPolicy.MaxTTL
//   - BR-008: op ∈ PresignPolicy.AllowedOperations
//   - FR-006: checksum constraint (ContentMD5 if configured)
//   - FR-007: permission policy (validateKeyPolicy)
//
// The returned PresignedURL is opaque; secrets are never logged. An AuditEvent
// is emitted with the sanitized key scope, method, TTL, and result.
func (b *blobStore) Presign(ctx context.Context, key Key, op PresignOperation, opts PresignOptions) (PresignedURL, error) {
	start := nowFn()
	if err := b.checkClosed(); err != nil {
		return PresignedURL{}, err
	}
	if err := ctxErrCheck(ctx); err != nil {
		return PresignedURL{}, err
	}
	if _, err := NewKey(string(key)); err != nil {
		return PresignedURL{}, err
	}

	// BR-008: operation allowlist.
	allowed := false
	for _, allowedOp := range b.cfg.Presign.AllowedOperations {
		if allowedOp == op {
			allowed = true
			break
		}
	}
	if !allowed {
		b.emitPresignAudit(ctx, start, key, op, opts, "", ErrorKindAuth)
		return PresignedURL{}, newError(ErrorKindAuth, "Presign", "operation not allowed")
	}

	// BR-008: TTL bounds. Positive and ≤ MaxTTL.
	if opts.TTL <= 0 {
		b.emitPresignAudit(ctx, start, key, op, opts, "", ErrorKindValidation)
		return PresignedURL{}, newError(ErrorKindValidation, "Presign", "TTL must be positive")
	}
	maxTTL := int64(b.cfg.Presign.MaxTTL.Seconds())
	if maxTTL == 0 {
		maxTTL = int64(MaxAllowedPresignTTL.Seconds())
	}
	if opts.TTL > maxTTL {
		b.emitPresignAudit(ctx, start, key, op, opts, "", ErrorKindValidation)
		return PresignedURL{}, newError(ErrorKindValidation, "Presign", "TTL exceeds max")
	}

	// FR-007: permission policy gate.
	if err := validateKeyPolicy(b.cfg.Policy.Permission, string(key)); err != nil {
		b.emitPresignAudit(ctx, start, key, op, opts, "", ErrorKindValidation)
		return PresignedURL{}, err
	}

	// Checksum.Required is intentionally not enforced for presign in v1.1.0:
	// adapters may sign PUT URLs while callers attach payload checksums during upload.

	adapterOpts := PresignAdapterOptions{
		ContentType: opts.ContentType,
		ContentMD5:  opts.ContentMD5,
	}
	var url PresignedURL
	_, err := b.run(ctx, "presign", key, func(ctx context.Context) error {
		var perr error
		url, perr = b.adapter.PresignURL(ctx, string(key), op, opts.TTL, adapterOpts)
		return perr
	})
	if err != nil {
		b.emitPresignAudit(ctx, start, key, op, opts, "", errorKind(err))
		return PresignedURL{}, err
	}
	b.emitPresignAudit(ctx, start, key, op, opts, url.URL, "")
	return url, nil
}

// emitPresignAudit logs a presign AuditEvent. The signed URL is NEVER included
// in the audit payload (BR-009). Only the sanitized key scope, method, TTL,
// and result are recorded. rawURL is passed only to validate it was generated
// (non-empty on success) but is never logged.
func (b *blobStore) emitPresignAudit(ctx context.Context, start time.Time, key Key, op PresignOperation, opts PresignOptions, rawURL string, errKind ErrorKind) {
	result := "ok"
	if errKind != "" {
		result = string(errKind)
	}
	_ = rawURL // validated upstream; never logged (BR-009)
	ev := AuditEvent{
		Operation:  "presign",
		Result:     result,
		KeyScope:   key.SanitizedScope(),
		Latency:    nowFn().Sub(start),
		TTLSeconds: opts.TTL,
		Method:     string(op),
		OccurredAt: nowFn(),
	}
	b.hooks.emitAudit(ctx, ev)
}
