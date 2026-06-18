// Package aliyun implements the real Aliyun OSS adapter for ossx.
//
// The Aliyun OSS SDK (github.com/aliyun/aliyun-oss-go-sdk/oss) is imported ONLY
// in this package (FR-008 / BR-011). It implements ossx.StoreAdapter (the
// exported SPI), so no SDK type ever crosses into the public ossx API. Provider
// errors are translated to ossx typed *Error at every method exit (SPEC §11).
//
// Construction: NewAdapter(ctx, cfg) builds an SDK-backed Adapter from an
// ossx.Config. Credentials come from cfg (populated by the composition root,
// typically via ossx.ConfigFromEnv reading FOUNDATIONX_OSSX_* secrets). This
// package never reads secrets itself.
package aliyun

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	oss "github.com/aliyun/aliyun-oss-go-sdk/oss"

	"github.com/ZoneCNH/ossx/pkg/ossx"
)

// Adapter is the Aliyun OSS StoreAdapter implementation.
type Adapter struct {
	client    *oss.Client
	bucket    *oss.Bucket
	cfg       ossx.Config
	closeOnce sync.Once
	closed    bool

	// uploads maps ossx UploadID → SDK InitiateMultipartUploadResult handle.
	// Required because the SDK threads the imur struct through UploadPart/
	// Complete/Abort rather than a bare id string.
	mu      sync.Mutex
	uploads map[string]oss.InitiateMultipartUploadResult
}

// compile-time guarantee: *Adapter satisfies ossx.StoreAdapter.
var _ ossx.StoreAdapter = (*Adapter)(nil)

// NewAdapter constructs an Aliyun OSS Adapter from an ossx.Config.
//
// cfg is validated by ossx.NewBlobStore before this is called, but we re-check
// credentials here since the adapter is unusable without them.
func NewAdapter(_ context.Context, cfg ossx.Config) (*Adapter, error) {
	if cfg.AccessKey == "" || cfg.SecretKey == "" {
		return nil, ossx.ErrInvalidConfig
	}
	endpoint := strings.TrimPrefix(strings.TrimPrefix(cfg.Endpoint, "https://"), "http://")
	client, err := oss.New(endpoint, cfg.AccessKey, cfg.SecretKey)
	if err != nil {
		return nil, ossx.WrapExport(ossx.ErrorKindConfig, "aliyun.New", "oss client init", err)
	}
	bucket, err := client.Bucket(cfg.Bucket)
	if err != nil {
		return nil, ossx.WrapExport(ossx.ErrorKindConfig, "aliyun.New", "bucket bind", err)
	}
	return &Adapter{
		client:  client,
		bucket:  bucket,
		cfg:     cfg,
		uploads: map[string]oss.InitiateMultipartUploadResult{},
	}, nil
}

// Name returns the adapter identifier.
func (a *Adapter) Name() string { return "aliyun-oss" }

// PutObject streams body to key (FR-004 — no whole-object buffering; the SDK
// reads the io.Reader directly).
func (a *Adapter) PutObject(ctx context.Context, key string, body io.Reader, _ int64, opts ossx.PutAdapterOptions) (ossx.ObjectInfo, error) {
	if a.closed {
		return ossx.ObjectInfo{}, ossx.ErrClosed
	}
	options := buildPutOptions(opts)
	if err := a.bucket.PutObject(key, body, options...); err != nil {
		return ossx.ObjectInfo{}, translateError("PutObject", err)
	}
	// Fetch metadata to populate ObjectInfo (etag/size/modified).
	info, herr := a.headInfo(ctx, key)
	if herr != nil {
		// Put succeeded but Head failed — return minimal info.
		return ossx.ObjectInfo{
			Key:         ossx.Key(key),
			ContentType: opts.ContentType,
			Metadata:    opts.Metadata,
		}, nil
	}
	return info, nil
}

// GetObject returns a streaming reader (FR-004).
func (a *Adapter) GetObject(ctx context.Context, key string) (io.ReadCloser, ossx.ObjectInfo, error) {
	if a.closed {
		return nil, ossx.ObjectInfo{}, ossx.ErrClosed
	}
	body, err := a.bucket.GetObject(key)
	if err != nil {
		return nil, ossx.ObjectInfo{}, translateError("GetObject", err)
	}
	info, _ := a.headInfo(ctx, key)
	return body, info, nil
}

// HeadObject returns metadata via GetObjectMeta.
func (a *Adapter) HeadObject(_ context.Context, key string) (ossx.ObjectInfo, error) {
	if a.closed {
		return ossx.ObjectInfo{}, ossx.ErrClosed
	}
	return a.headInfo(context.Background(), key)
}

// headInfo builds ObjectInfo from OSS object meta headers.
func (a *Adapter) headInfo(_ context.Context, key string) (ossx.ObjectInfo, error) {
	header, err := a.bucket.GetObjectMeta(key)
	if err != nil {
		return ossx.ObjectInfo{}, translateError("HeadObject", err)
	}
	info := ossx.ObjectInfo{Key: ossx.Key(key)}
	if v := header.Get("Content-Length"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			info.Size = n
		}
	}
	info.ContentType = header.Get("Content-Type")
	info.ETag = strings.Trim(header.Get("ETag"), "\"")
	if v := header.Get("Last-Modified"); v != "" {
		if t, err := http.ParseTime(v); err == nil {
			info.ModifiedAt = t
			info.CreatedAt = t
		}
	}
	info.StorageClass = header.Get("x-oss-storage-class")
	return info, nil
}

// DeleteObject removes key. Aliyun OSS DeleteObject is idempotent for missing
// objects; strict semantics are enforced at the BlobStore layer.
func (a *Adapter) DeleteObject(_ context.Context, key string, _ bool) error {
	if a.closed {
		return ossx.ErrClosed
	}
	if err := a.bucket.DeleteObject(key); err != nil {
		return translateError("DeleteObject", err)
	}
	return nil
}

// CopyObject duplicates source to target server-side.
func (a *Adapter) CopyObject(_ context.Context, source, target string, _ ossx.CopyAdapterOptions) (ossx.ObjectInfo, error) {
	if a.closed {
		return ossx.ObjectInfo{}, ossx.ErrClosed
	}
	result, err := a.bucket.CopyObject(source, target)
	if err != nil {
		return ossx.ObjectInfo{}, translateError("CopyObject", err)
	}
	now := time.Now().UTC()
	return ossx.ObjectInfo{
		Key:        ossx.Key(target),
		ETag:       result.ETag,
		CreatedAt:  now,
		ModifiedAt: now,
	}, nil
}

// ListObjects returns a bounded page (BR-006).
func (a *Adapter) ListObjects(_ context.Context, prefix string, max int, continuation string) (ossx.ListPage, error) {
	if a.closed {
		return ossx.ListPage{}, ossx.ErrClosed
	}
	if max <= 0 || max > 1000 {
		max = 1000
	}
	opts := []oss.Option{oss.Prefix(prefix), oss.MaxKeys(max)}
	if continuation != "" {
		opts = append(opts, oss.Marker(continuation))
	}
	result, err := a.bucket.ListObjects(opts...)
	if err != nil {
		return ossx.ListPage{}, translateError("ListObjects", err)
	}
	items := make([]ossx.ObjectInfo, 0, len(result.Objects))
	for _, op := range result.Objects {
		items = append(items, ossx.ObjectInfo{
			Key:          ossx.Key(op.Key),
			Size:         op.Size,
			ETag:         strings.Trim(op.ETag, "\""),
			StorageClass: op.StorageClass,
			ModifiedAt:   op.LastModified,
			CreatedAt:    op.LastModified,
		})
	}
	next := ""
	if result.IsTruncated && len(items) > 0 {
		next = string(items[len(items)-1].Key)
	}
	return ossx.ListPage{
		Items:            items,
		NextContinuation: next,
		IsTruncated:      result.IsTruncated,
	}, nil
}

// --- Multipart ---

// InitiateMultipart starts a multipart upload.
func (a *Adapter) InitiateMultipart(_ context.Context, key string, opts ossx.PutAdapterOptions) (ossx.UploadID, error) {
	if a.closed {
		return "", ossx.ErrClosed
	}
	imur, err := a.bucket.InitiateMultipartUpload(key, buildPutOptions(opts)...)
	if err != nil {
		return "", translateError("InitiateMultipart", err)
	}
	id := ossx.UploadID(imur.UploadID)
	a.mu.Lock()
	a.uploads[imur.UploadID] = imur
	a.mu.Unlock()
	return id, nil
}

// UploadPart stages a part.
func (a *Adapter) UploadPart(_ context.Context, id ossx.UploadID, partNumber int, body io.Reader, size int64) (ossx.PartETag, error) {
	if a.closed {
		return ossx.PartETag{}, ossx.ErrClosed
	}
	a.mu.Lock()
	imur, ok := a.uploads[string(id)]
	a.mu.Unlock()
	if !ok {
		return ossx.PartETag{}, ossx.WrapExport(ossx.ErrorKindNotFound, "UploadPart", "upload id not found", nil)
	}
	partSize := size
	if partSize < 0 {
		partSize = int64(-1)
	}
	result, err := a.bucket.UploadPart(imur, body, partSize, partNumber, nil)
	if err != nil {
		return ossx.PartETag{}, translateError("UploadPart", err)
	}
	return ossx.PartETag{
		PartNumber: partNumber,
		ETag:       strings.Trim(result.ETag, "\""),
		Size:       size,
	}, nil
}

// ListParts returns uploaded parts.
func (a *Adapter) ListParts(_ context.Context, id ossx.UploadID) ([]ossx.PartETag, error) {
	if a.closed {
		return nil, ossx.ErrClosed
	}
	a.mu.Lock()
	imur, ok := a.uploads[string(id)]
	a.mu.Unlock()
	if !ok {
		return nil, ossx.WrapExport(ossx.ErrorKindNotFound, "ListParts", "upload id not found", nil)
	}
	lpr, err := a.bucket.ListUploadedParts(imur)
	if err != nil {
		return nil, translateError("ListParts", err)
	}
	parts := make([]ossx.PartETag, 0, len(lpr.UploadedParts))
	for _, up := range lpr.UploadedParts {
		parts = append(parts, ossx.PartETag{
			PartNumber: up.PartNumber,
			ETag:       strings.Trim(up.ETag, "\""),
			Size:       int64(up.Size),
		})
	}
	return parts, nil
}

// CompleteMultipart finalizes the upload.
func (a *Adapter) CompleteMultipart(_ context.Context, id ossx.UploadID, parts []ossx.PartETag) (ossx.ObjectInfo, error) {
	if a.closed {
		return ossx.ObjectInfo{}, ossx.ErrClosed
	}
	a.mu.Lock()
	imur, ok := a.uploads[string(id)]
	a.mu.Unlock()
	if !ok {
		return ossx.ObjectInfo{}, ossx.WrapExport(ossx.ErrorKindNotFound, "CompleteMultipart", "upload id not found", nil)
	}
	sdkParts := make([]oss.UploadPart, 0, len(parts))
	for _, p := range parts {
		sdkParts = append(sdkParts, oss.UploadPart{PartNumber: p.PartNumber, ETag: p.ETag})
	}
	result, err := a.bucket.CompleteMultipartUpload(imur, sdkParts)
	if err != nil {
		return ossx.ObjectInfo{}, translateError("CompleteMultipart", err)
	}
	a.mu.Lock()
	delete(a.uploads, string(id))
	a.mu.Unlock()
	var total int64
	for _, p := range parts {
		total += p.Size
	}
	now := time.Now().UTC()
	return ossx.ObjectInfo{
		Key:        ossx.Key(imur.Key),
		Size:       total,
		ETag:       result.ETag,
		Location:   result.Location,
		CreatedAt:  now,
		ModifiedAt: now,
	}, nil
}

// AbortMultipart cancels the upload. Idempotent.
func (a *Adapter) AbortMultipart(_ context.Context, id ossx.UploadID) error {
	if a.closed {
		return nil
	}
	a.mu.Lock()
	imur, ok := a.uploads[string(id)]
	if ok {
		delete(a.uploads, string(id))
	}
	a.mu.Unlock()
	if !ok {
		return nil // idempotent: unknown id already aborted
	}
	if err := a.bucket.AbortMultipartUpload(imur); err != nil {
		return translateError("AbortMultipart", err)
	}
	return nil
}

// --- Presign ---

// PresignURL signs a URL via bucket.SignURL (FR-006).
func (a *Adapter) PresignURL(_ context.Context, key string, op ossx.PresignOperation, ttlSeconds int64, opts ossx.PresignAdapterOptions) (ossx.PresignedURL, error) {
	if a.closed {
		return ossx.PresignedURL{}, ossx.ErrClosed
	}
	method := oss.HTTPGet
	switch op {
	case ossx.PresignPut:
		method = oss.HTTPPut
	case ossx.PresignGet:
		method = oss.HTTPGet
	default:
		return ossx.PresignedURL{}, ossx.WrapExport(ossx.ErrorKindValidation, "PresignURL", "unsupported operation", nil)
	}
	options := []oss.Option{}
	if opts.ContentType != "" {
		options = append(options, oss.ContentType(opts.ContentType))
	}
	url, err := a.bucket.SignURL(key, method, ttlSeconds, options...)
	if err != nil {
		return ossx.PresignedURL{}, translateError("PresignURL", err)
	}
	return ossx.PresignedURL{
		URL:       url,
		Method:    string(method),
		ExpiresAt: time.Now().Unix() + ttlSeconds,
	}, nil
}

// --- Lifecycle ---

// Health probes the bucket via a lightweight GetBucketInfo (client-level call).
func (a *Adapter) Health(_ context.Context) error {
	if a.closed {
		return ossx.ErrClosed
	}
	if _, err := a.client.GetBucketInfo(a.cfg.Bucket); err != nil {
		return translateError("Health", err)
	}
	return nil
}

// Close releases resources. Idempotent.
func (a *Adapter) Close(_ context.Context) error {
	a.closeOnce.Do(func() {
		a.closed = true
	})
	return nil
}

// --- helpers ---

// buildPutOptions translates ossx put options to SDK options.
func buildPutOptions(opts ossx.PutAdapterOptions) []oss.Option {
	out := []oss.Option{}
	if opts.ContentType != "" {
		out = append(out, oss.ContentType(opts.ContentType))
	}
	for k, v := range opts.Metadata {
		out = append(out, oss.Meta(k, v))
	}
	return out
}

// translateError maps an Aliyun OSS SDK error to an ossx typed *Error at the
// adapter boundary (SPEC §11 / AC-OSS-008).
func translateError(op string, err error) error {
	if err == nil {
		return nil
	}
	// ServiceError: structured OSS error with a Code.
	var se oss.ServiceError
	if asServiceError(err, &se) {
		return mapServiceError(op, se)
	}
	// Network / connection errors.
	msg := err.Error()
	switch {
	case strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline"):
		return ossx.WrapExport(ossx.ErrorKindTimeout, op, msg, err)
	case strings.Contains(msg, "connection refused") || strings.Contains(msg, "no such host") || strings.Contains(msg, "dial"):
		return ossx.WrapExport(ossx.ErrorKindConnection, op, msg, err)
	case strings.Contains(msg, "canceled"):
		return ossx.WrapExport(ossx.ErrorKindCanceled, op, msg, err)
	}
	return ossx.WrapExport(ossx.ErrorKindUnavailable, op, msg, err)
}

// mapServiceError converts an OSS ServiceError code to an ossx *Error kind.
func mapServiceError(op string, se oss.ServiceError) error {
	msg := fmt.Sprintf("%s: %s", se.Code, se.Message)
	switch se.Code {
	case "NoSuchKey", "NoSuchUpload", "NoSuchBucket":
		return ossx.WrapExport(ossx.ErrorKindNotFound, op, msg, nil)
	case "AccessDenied", "SignatureDoesNotMatch":
		return ossx.WrapExport(ossx.ErrorKindAuth, op, msg, nil)
	case "BucketAlreadyExists", "ObjectAlreadyExists":
		return ossx.WrapExport(ossx.ErrorKindAlreadyExist, op, msg, nil)
	case "Conflict":
		return ossx.WrapExport(ossx.ErrorKindConflict, op, msg, nil)
	case "InvalidArgument", "InvalidObjectName", "MalformedXML":
		return ossx.WrapExport(ossx.ErrorKindValidation, op, msg, nil)
	case "RequestTimeout":
		return ossx.WrapExport(ossx.ErrorKindTimeout, op, msg, nil)
	case "Throttling", "TooManyRequests", "RequestTimeTooSkewed":
		return ossx.WrapExport(ossx.ErrorKindRateLimit, op, msg, nil)
	case "ServiceUnavailable", "InternalError":
		return ossx.WrapExport(ossx.ErrorKindUnavailable, op, msg, nil)
	}
	return ossx.WrapExport(ossx.ErrorKindInternal, op, msg, nil)
}

// asServiceError extracts an oss.ServiceError from the SDK error chain.
func asServiceError(err error, target *oss.ServiceError) bool {
	if err == nil {
		return false
	}
	if se, ok := err.(oss.ServiceError); ok {
		*target = se
		return true
	}
	if sep, ok := err.(*oss.ServiceError); ok && sep != nil {
		*target = *sep
		return true
	}
	return false
}
