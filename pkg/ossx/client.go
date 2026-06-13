package ossx

import (
	"context"
	"io"
	"sync"
	"time"
)

// ObjectInfo holds metadata about a stored object.
type ObjectInfo struct {
	Key          string
	Size         int64
	ETag         string
	ContentType  string
	LastModified string
}

// PutInput holds parameters for PutObject.
type PutInput struct {
	Key         string
	Body        io.Reader
	Size        int64
	ContentType string
}

// GetOutput holds the result of GetObject.
type GetOutput struct {
	Info        ObjectInfo
	Body        io.ReadCloser
	Size        int64
	ContentType string
	ETag        string
}

// ListInput holds parameters for ListObjects.
type ListInput struct {
	Prefix  string
	Marker  string
	MaxKeys int
}

// ListOutput holds the result of ListObjects.
type ListOutput struct {
	Objects     []ObjectInfo
	IsTruncated bool
	NextMarker  string
}

// Client manages connections and operations against an object storage service.
type Client struct {
	cfg         Config
	metrics     Metrics
	store       objectStore
	mu          sync.Mutex
	initialized bool
	closed      bool
}

var newObjectStoreFunc = newObjectStore

// New creates a new Client. It validates the config and applies options.
func New(ctx context.Context, cfg Config, opts ...Option) (*Client, error) {
	const op = "ossx.New"
	options := defaultOptions()
	for _, opt := range opts {
		opt(&options)
	}

	if ctx == nil {
		err := validationError(op, "context is required", nil)
		recordErrorMetric(options.metrics, "new", err)
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		wrapped := contextError(op, err)
		recordErrorMetric(options.metrics, "new", wrapped)
		return nil, wrapped
	}
	if err := cfg.Validate(); err != nil {
		recordErrorMetric(options.metrics, "new", err)
		return nil, err
	}
	store, err := newObjectStoreFunc(cfg)
	if err != nil {
		recordErrorMetric(options.metrics, "new", err)
		return nil, err
	}

	options.metrics.IncCounter(MetricClientCreatedTotal, map[string]string{"name": cfg.Name})
	return &Client{cfg: cfg, metrics: options.metrics, store: store, initialized: true}, nil
}

// Close releases resources held by the client.
func (c *Client) Close(ctx context.Context) error {
	const op = "ossx.Close"
	if c == nil {
		return validationError(op, "client is nil", nil)
	}
	if ctx == nil {
		err := validationError(op, "context is required", nil)
		recordErrorMetric(c.metrics, "close", err)
		return err
	}
	if err := ctx.Err(); err != nil {
		wrapped := contextError(op, err)
		recordErrorMetric(c.metrics, "close", wrapped)
		return wrapped
	}

	c.mu.Lock()
	if !c.initialized {
		c.mu.Unlock()
		err := validationError(op, "client is not initialized", nil)
		recordErrorMetric(c.metrics, "close", err)
		return err
	}
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	name := c.cfg.Name
	metrics := c.metrics
	c.mu.Unlock()

	if metrics != nil {
		metrics.IncCounter(MetricClientClosedTotal, map[string]string{"name": name})
	}
	return nil
}

// PutObject uploads an object to the storage service.
func (c *Client) PutObject(ctx context.Context, input PutInput) (*ObjectInfo, error) {
	const op = "ossx.PutObject"
	store, metrics, err := c.ready(op)
	if err != nil {
		recordErrorMetric(metrics, "put", err)
		return nil, err
	}
	if ctx == nil {
		err := validationError(op, "context is required", nil)
		recordErrorMetric(metrics, "put", err)
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		wrapped := contextError(op, err)
		recordErrorMetric(metrics, "put", wrapped)
		return nil, wrapped
	}
	if err := validateObjectKey(op, input.Key); err != nil {
		recordErrorMetric(metrics, "put", err)
		return nil, err
	}
	if input.Body == nil {
		err := validationError(op, "body is required", nil)
		recordErrorMetric(metrics, "put", err)
		return nil, err
	}
	if input.Size < 0 {
		err := validationError(op, "size must not be negative", nil)
		recordErrorMetric(metrics, "put", err)
		return nil, err
	}

	start := time.Now()
	recordInflight(metrics, "put", 1)
	err = store.putObject(input.Key, input.Body, input.ContentType)
	recordInflight(metrics, "put", 0)
	if err != nil {
		mapped := mapStoreError(op, err)
		recordOperationMetric(metrics, "put", start, mapped)
		recordErrorMetric(metrics, "put", mapped)
		return nil, mapped
	}
	recordOperationMetric(metrics, "put", start, nil)
	recordTransferMetric(metrics, "upload", input.Size)
	return &ObjectInfo{Key: input.Key, Size: input.Size, ContentType: input.ContentType}, nil
}

// GetObject downloads an object from the storage service.
func (c *Client) GetObject(ctx context.Context, key string) (*GetOutput, error) {
	const op = "ossx.GetObject"
	store, metrics, err := c.ready(op)
	if err != nil {
		recordErrorMetric(metrics, "get", err)
		return nil, err
	}
	if ctx == nil {
		err := validationError(op, "context is required", nil)
		recordErrorMetric(metrics, "get", err)
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		wrapped := contextError(op, err)
		recordErrorMetric(metrics, "get", wrapped)
		return nil, wrapped
	}
	if err := validateObjectKey(op, key); err != nil {
		recordErrorMetric(metrics, "get", err)
		return nil, err
	}

	start := time.Now()
	recordInflight(metrics, "get", 1)
	stored, err := store.getObject(key)
	recordInflight(metrics, "get", 0)
	if err != nil {
		mapped := mapStoreError(op, err)
		recordOperationMetric(metrics, "get", start, mapped)
		recordErrorMetric(metrics, "get", mapped)
		return nil, mapped
	}
	recordOperationMetric(metrics, "get", start, nil)
	recordTransferMetric(metrics, "download", stored.Info.Size)
	return &GetOutput{
		Info:        stored.Info,
		Body:        stored.Body,
		Size:        stored.Info.Size,
		ContentType: stored.Info.ContentType,
		ETag:        stored.Info.ETag,
	}, nil
}

// DeleteObject removes an object from the storage service.
func (c *Client) DeleteObject(ctx context.Context, key string) error {
	const op = "ossx.DeleteObject"
	store, metrics, err := c.ready(op)
	if err != nil {
		recordErrorMetric(metrics, "delete", err)
		return err
	}
	if ctx == nil {
		err := validationError(op, "context is required", nil)
		recordErrorMetric(metrics, "delete", err)
		return err
	}
	if err := ctx.Err(); err != nil {
		wrapped := contextError(op, err)
		recordErrorMetric(metrics, "delete", wrapped)
		return wrapped
	}
	if err := validateObjectKey(op, key); err != nil {
		recordErrorMetric(metrics, "delete", err)
		return err
	}

	start := time.Now()
	recordInflight(metrics, "delete", 1)
	err = store.deleteObject(key)
	recordInflight(metrics, "delete", 0)
	if err != nil {
		mapped := mapStoreError(op, err)
		recordOperationMetric(metrics, "delete", start, mapped)
		recordErrorMetric(metrics, "delete", mapped)
		return mapped
	}
	recordOperationMetric(metrics, "delete", start, nil)
	return nil
}

// ListObjects lists objects in the storage service with the given prefix.
func (c *Client) ListObjects(ctx context.Context, input ListInput) (*ListOutput, error) {
	const op = "ossx.ListObjects"
	store, metrics, err := c.ready(op)
	if err != nil {
		recordErrorMetric(metrics, "list", err)
		return nil, err
	}
	if ctx == nil {
		err := validationError(op, "context is required", nil)
		recordErrorMetric(metrics, "list", err)
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		wrapped := contextError(op, err)
		recordErrorMetric(metrics, "list", wrapped)
		return nil, wrapped
	}
	if err := validateObjectPrefix(op, input.Prefix); err != nil {
		recordErrorMetric(metrics, "list", err)
		return nil, err
	}
	if input.Marker != "" {
		if err := validateObjectKey(op, input.Marker); err != nil {
			recordErrorMetric(metrics, "list", err)
			return nil, err
		}
	}
	if input.MaxKeys < 0 {
		err := validationError(op, "max_keys must not be negative", nil)
		recordErrorMetric(metrics, "list", err)
		return nil, err
	}

	start := time.Now()
	recordInflight(metrics, "list", 1)
	stored, err := store.listObjects(listRequest{
		Prefix:  input.Prefix,
		Marker:  input.Marker,
		MaxKeys: input.MaxKeys,
	})
	recordInflight(metrics, "list", 0)
	if err != nil {
		mapped := mapStoreError(op, err)
		recordOperationMetric(metrics, "list", start, mapped)
		recordErrorMetric(metrics, "list", mapped)
		return nil, mapped
	}
	recordOperationMetric(metrics, "list", start, nil)
	return &ListOutput{
		Objects:     stored.Objects,
		IsTruncated: stored.IsTruncated,
		NextMarker:  stored.NextMarker,
	}, nil
}

func (c *Client) ready(op string) (objectStore, Metrics, error) {
	if c == nil {
		return nil, nil, validationError(op, "client is nil", nil)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.initialized {
		return nil, c.metrics, validationError(op, "client is not initialized", nil)
	}
	if c.closed {
		return nil, c.metrics, validationError(op, "client is closed", nil)
	}
	if c.store == nil {
		return nil, c.metrics, validationError(op, "object store is not initialized", nil)
	}
	return c.store, c.metrics, nil
}

func recordErrorMetric(metrics Metrics, op string, err error) {
	if metrics == nil {
		return
	}
	metrics.IncCounter(MetricClientErrorsTotal, map[string]string{
		"op":   op,
		"kind": string(errorKind(err)),
	})
}

func recordOperationMetric(metrics Metrics, op string, start time.Time, err error) {
	if metrics == nil {
		return
	}
	status := "ok"
	if err != nil {
		status = "error"
	}
	labels := map[string]string{"op": op, "status": status}
	metrics.IncCounter(MetricClientRequestsTotal, labels)
	metrics.ObserveHistogram(MetricClientRequestDurationSeconds, time.Since(start).Seconds(), labels)
}

func recordInflight(metrics Metrics, op string, value float64) {
	if metrics == nil {
		return
	}
	metrics.SetGauge(MetricClientInflight, value, map[string]string{"op": op})
}

func recordTransferMetric(metrics Metrics, direction string, bytes int64) {
	if metrics == nil {
		return
	}
	if direction == "upload" {
		metrics.IncCounter(MetricOSSUploadsTotal, map[string]string{"direction": direction})
	}
	if direction == "download" {
		metrics.IncCounter(MetricOSSDownloadsTotal, map[string]string{"direction": direction})
	}
	metrics.ObserveHistogram(MetricOSSBytesTransferred, float64(bytes), map[string]string{"direction": direction})
}
