package ossx

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"
)

// MultipartSession represents an in-flight multipart upload context (FR-005).
// A session is obtained via BlobStore.Multipart(ctx) and MUST be either
// Completed or Aborted to release staged parts. Complete/Abort are idempotent
// (BR-007).
type MultipartSession interface {
	// Initiate starts a multipart upload for key, returning an upload id.
	Initiate(ctx context.Context, key Key, opts PutOptions) (UploadID, error)
	// UploadPart appends part number partNumber (1-based) with body. size hints
	// total bytes when known (-1 unknown). Part numbers, sizes, ETags and
	// checksums are validated.
	UploadPart(ctx context.Context, id UploadID, partNumber int, body io.Reader, size int64) (PartETag, error)
	// ListParts returns currently uploaded parts for id.
	ListParts(ctx context.Context, id UploadID) ([]PartETag, error)
	// Complete finalizes the upload, publishing the object. parts must be the
	// full ordered set; missing or inconsistent parts are rejected.
	Complete(ctx context.Context, id UploadID, parts []PartETag) (ObjectInfo, error)
	// Abort cancels the upload and frees staged parts. Idempotent and safe
	// after partial failure (BR-007).
	Abort(ctx context.Context, id UploadID) error
}

// UploadID identifies a multipart upload.
type UploadID string

// PartETag binds a part number to its server-acknowledged ETag and size.
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
	ProviderStatus string // "ready" | "unreachable" | "config_error" | "degraded" | "closed"
	Error          string
	LastCheckedAt  int64
}

// multipartSession is the concrete session backed by a StoreAdapter.
type multipartSession struct {
	store *blobStore

	// idempotency guard against double-complete / double-abort (BR-007).
	mu   sync.Mutex
	done map[UploadID]bool
}

func (s *multipartSession) markDone(id UploadID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done == nil {
		s.done = map[UploadID]bool{}
	}
	if s.done[id] {
		return false // already done
	}
	s.done[id] = true
	return true
}

func (s *multipartSession) Initiate(ctx context.Context, key Key, opts PutOptions) (UploadID, error) {
	if err := s.store.checkClosed(); err != nil {
		return "", err
	}
	if err := ctxErrCheck(ctx); err != nil {
		return "", err
	}
	if _, err := NewKey(string(key)); err != nil {
		return "", err
	}
	if err := validateMetadata(opts.Metadata); err != nil {
		return "", err
	}
	if err := validateKeyPolicy(s.store.cfg.Policy.Permission, string(key)); err != nil {
		return "", err
	}
	adapterOpts := PutAdapterOptions{
		ContentType:  opts.ContentType,
		Metadata:     opts.Metadata,
		Tags:         opts.Tags,
		ChecksumAlgo: opts.ChecksumAlgo,
	}
	var id UploadID
	_, err := s.store.run(ctx, "multipart_initiate", key, func(ctx context.Context) error {
		var ierr error
		id, ierr = s.store.adapter.InitiateMultipart(ctx, string(key), adapterOpts)
		return ierr
	})
	return id, err
}

func (s *multipartSession) UploadPart(ctx context.Context, id UploadID, partNumber int, body io.Reader, size int64) (PartETag, error) {
	if err := s.store.checkClosed(); err != nil {
		return PartETag{}, err
	}
	if err := ctxErrCheck(ctx); err != nil {
		return PartETag{}, err
	}
	if partNumber < 1 {
		return PartETag{}, newError(ErrorKindValidation, "UploadPart", "part number must be >= 1")
	}
	maxParts := s.store.cfg.Multipart.MaxParts
	if maxParts > 0 && partNumber > maxParts {
		return PartETag{}, newError(ErrorKindValidation, "UploadPart", "part number exceeds MaxParts")
	}
	var part PartETag
	_, err := s.store.run(ctx, "multipart_upload_part", "", func(ctx context.Context) error {
		var perr error
		part, perr = s.store.adapter.UploadPart(ctx, id, partNumber, body, size)
		return perr
	})
	if err != nil {
		return PartETag{}, err
	}
	// BR-007: validate returned part metadata.
	if part.PartNumber != partNumber {
		return PartETag{}, newError(ErrorKindValidation, "UploadPart", "part number mismatch")
	}
	if part.ETag == "" {
		return PartETag{}, newError(ErrorKindValidation, "UploadPart", "empty ETag returned")
	}
	return part, nil
}

func (s *multipartSession) ListParts(ctx context.Context, id UploadID) ([]PartETag, error) {
	if err := s.store.checkClosed(); err != nil {
		return nil, err
	}
	if err := ctxErrCheck(ctx); err != nil {
		return nil, err
	}
	var parts []PartETag
	_, err := s.store.run(ctx, "multipart_list_parts", "", func(ctx context.Context) error {
		var perr error
		parts, perr = s.store.adapter.ListParts(ctx, id)
		return perr
	})
	return parts, err
}

func (s *multipartSession) Complete(ctx context.Context, id UploadID, parts []PartETag) (ObjectInfo, error) {
	if err := s.store.checkClosed(); err != nil {
		return ObjectInfo{}, err
	}
	if err := ctxErrCheck(ctx); err != nil {
		return ObjectInfo{}, err
	}
	// BR-007: validate all required parts before publishing.
	if len(parts) == 0 {
		return ObjectInfo{}, newError(ErrorKindValidation, "Complete", "no parts provided")
	}
	for i, p := range parts {
		if p.PartNumber != i+1 {
			return ObjectInfo{}, newError(ErrorKindValidation, "Complete", "part numbers must be contiguous starting at 1")
		}
		if p.ETag == "" {
			return ObjectInfo{}, newError(ErrorKindValidation, "Complete", "part missing ETag")
		}
	}
	maxParts := s.store.cfg.Multipart.MaxParts
	if maxParts > 0 && len(parts) > maxParts {
		return ObjectInfo{}, newError(ErrorKindValidation, "Complete", "part count exceeds MaxParts")
	}
	// BR-007 idempotency: double-complete is a conflict, not a re-publish.
	if !s.markDone(id) {
		return ObjectInfo{}, newError(ErrorKindConflict, "Complete", "upload already completed or aborted")
	}
	var info ObjectInfo
	_, err := s.store.run(ctx, "multipart_complete", "", func(ctx context.Context) error {
		var perr error
		info, perr = s.store.adapter.CompleteMultipart(ctx, id, parts)
		return perr
	})
	if err != nil {
		// On failure, unmark so the caller can retry.
		s.mu.Lock()
		delete(s.done, id)
		s.mu.Unlock()
		return ObjectInfo{}, err
	}
	return info, nil
}

func (s *multipartSession) Abort(ctx context.Context, id UploadID) error {
	if err := s.store.checkClosed(); err != nil {
		return err
	}
	if err := ctxErrCheck(ctx); err != nil {
		return err
	}
	// BR-007: Abort is idempotent — aborting an already-aborted/completed id is not an error.
	s.markDone(id)
	_, err := s.store.run(ctx, "multipart_abort", "", func(ctx context.Context) error {
		perr := s.store.adapter.AbortMultipart(ctx, id)
		if perr == nil {
			return nil
		}
		// Idempotent: a missing upload on abort is not an error.
		var se *Error
		if errors.As(perr, &se) && se.Kind == ErrorKindNotFound {
			return nil
		}
		return perr
	})
	return err
}

// nowFn is a package-level clock seam for tests; production uses time.Now.
var nowFn = func() time.Time { return time.Now() }
