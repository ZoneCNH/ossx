package ossx

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	metrics := &testMetrics{}
	withObjectStore(t, fakeStore{})
	client, err := New(t.Context(), validConfig(), WithMetrics(metrics))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if client.cfg.Name != validConfig().Name || !client.initialized || client.closed {
		t.Fatalf("client not initialized correctly: %#v", client)
	}
	if metrics.countCounter(MetricClientCreatedTotal) != 1 {
		t.Fatal("client creation metric not recorded")
	}
}

func TestNewErrors(t *testing.T) {
	t.Run("nil context", func(t *testing.T) {
		_, err := New(nil, validConfig(), WithMetrics(&testMetrics{}))
		assertErrorKind(t, err, ErrorKindValidation)
	})

	t.Run("canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, err := New(ctx, validConfig(), WithMetrics(&testMetrics{}))
		assertErrorKind(t, err, ErrorKindUnavailable)
	})

	t.Run("invalid config", func(t *testing.T) {
		cfg := validConfig()
		cfg.Name = ""
		_, err := New(t.Context(), cfg, WithMetrics(&testMetrics{}))
		assertErrorKind(t, err, ErrorKindValidation)
	})

	t.Run("store factory error", func(t *testing.T) {
		withObjectStore(t, nil)
		_, err := New(t.Context(), validConfig(), WithMetrics(&testMetrics{}))
		if err == nil {
			t.Fatal("expected store factory error")
		}
	})
}

func TestClose(t *testing.T) {
	t.Run("nil client", func(t *testing.T) {
		var client *Client
		assertErrorKind(t, client.Close(t.Context()), ErrorKindValidation)
	})

	t.Run("nil context", func(t *testing.T) {
		metrics := &testMetrics{}
		client := newTestClient(t, fakeStore{}, metrics)
		assertErrorKind(t, client.Close(nil), ErrorKindValidation)
		if metrics.countCounter(MetricClientErrorsTotal) == 0 {
			t.Fatal("close error metric not recorded")
		}
	})

	t.Run("canceled context", func(t *testing.T) {
		client := newTestClient(t, fakeStore{}, &testMetrics{})
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		assertErrorKind(t, client.Close(ctx), ErrorKindUnavailable)
	})

	t.Run("uninitialized", func(t *testing.T) {
		client := &Client{metrics: &testMetrics{}}
		assertErrorKind(t, client.Close(t.Context()), ErrorKindValidation)
	})

	t.Run("idempotent", func(t *testing.T) {
		metrics := &testMetrics{}
		client := newTestClient(t, fakeStore{}, metrics)
		if err := client.Close(t.Context()); err != nil {
			t.Fatalf("Close failed: %v", err)
		}
		if err := client.Close(t.Context()); err != nil {
			t.Fatalf("second Close failed: %v", err)
		}
		if metrics.countCounter(MetricClientClosedTotal) != 1 {
			t.Fatalf("expected one close metric, got %d", metrics.countCounter(MetricClientClosedTotal))
		}
	})
}

func TestReadyStates(t *testing.T) {
	var nilClient *Client
	_, _, err := nilClient.ready("ready")
	assertErrorKind(t, err, ErrorKindValidation)

	_, _, err = (&Client{}).ready("ready")
	assertErrorKind(t, err, ErrorKindValidation)

	_, _, err = (&Client{initialized: true, closed: true}).ready("ready")
	assertErrorKind(t, err, ErrorKindValidation)

	_, _, err = (&Client{initialized: true}).ready("ready")
	assertErrorKind(t, err, ErrorKindValidation)

	store, _, err := (&Client{initialized: true, store: fakeStore{}}).ready("ready")
	if err != nil || store == nil {
		t.Fatalf("ready returned store=%v err=%v", store, err)
	}
}

func TestOperationsReturnReadyErrors(t *testing.T) {
	client := &Client{}

	_, err := client.GetObject(t.Context(), "key")
	assertErrorKind(t, err, ErrorKindValidation)

	err = client.DeleteObject(t.Context(), "key")
	assertErrorKind(t, err, ErrorKindValidation)

	_, err = client.ListObjects(t.Context(), ListInput{Prefix: "prefix"})
	assertErrorKind(t, err, ErrorKindValidation)
}

func TestPutObject(t *testing.T) {
	metrics := &testMetrics{}
	var sawKey, sawContentType, sawBody string
	client := newTestClient(t, fakeStore{putFn: func(key string, body io.Reader, contentType string) error {
		sawKey = key
		sawContentType = contentType
		data, err := io.ReadAll(body)
		if err != nil {
			t.Fatalf("ReadAll failed: %v", err)
		}
		sawBody = string(data)
		return nil
	}}, metrics)

	info, err := client.PutObject(t.Context(), PutInput{
		Key:         "objects/a.txt",
		Body:        strings.NewReader("payload"),
		Size:        7,
		ContentType: "text/plain",
	})
	if err != nil {
		t.Fatalf("PutObject failed: %v", err)
	}
	if info.Key != "objects/a.txt" || info.Size != 7 || info.ContentType != "text/plain" {
		t.Fatalf("unexpected info: %#v", info)
	}
	if sawKey != "objects/a.txt" || sawBody != "payload" || sawContentType != "text/plain" {
		t.Fatalf("store received key=%q body=%q type=%q", sawKey, sawBody, sawContentType)
	}
	if metrics.countCounter(MetricOSSUploadsTotal) != 1 || metrics.countCounter(MetricClientRequestsTotal) == 0 {
		t.Fatal("put metrics not recorded")
	}
}

func TestPutObjectErrors(t *testing.T) {
	tests := []struct {
		name  string
		setup func() *Client
		ctx   context.Context
		input PutInput
		kind  ErrorKind
	}{
		{name: "not ready", setup: func() *Client { return &Client{} }, ctx: t.Context(), input: PutInput{Key: "a", Body: strings.NewReader("x")}, kind: ErrorKindValidation},
		{name: "nil context", setup: func() *Client { return newTestClient(t, fakeStore{}, &testMetrics{}) }, input: PutInput{Key: "a", Body: strings.NewReader("x")}, kind: ErrorKindValidation},
		{name: "bad context", setup: func() *Client { return newTestClient(t, fakeStore{}, &testMetrics{}) }, ctx: canceledContext(t), input: PutInput{Key: "a", Body: strings.NewReader("x")}, kind: ErrorKindUnavailable},
		{name: "bad key", setup: func() *Client { return newTestClient(t, fakeStore{}, &testMetrics{}) }, ctx: t.Context(), input: PutInput{Key: "/a", Body: strings.NewReader("x")}, kind: ErrorKindValidation},
		{name: "nil body", setup: func() *Client { return newTestClient(t, fakeStore{}, &testMetrics{}) }, ctx: t.Context(), input: PutInput{Key: "a"}, kind: ErrorKindValidation},
		{name: "negative size", setup: func() *Client { return newTestClient(t, fakeStore{}, &testMetrics{}) }, ctx: t.Context(), input: PutInput{Key: "a", Body: strings.NewReader("x"), Size: -1}, kind: ErrorKindValidation},
		{name: "store error", setup: func() *Client {
			return newTestClient(t, fakeStore{putFn: func(string, io.Reader, string) error {
				return errors.New("write failed")
			}}, &testMetrics{})
		}, ctx: t.Context(), input: PutInput{Key: "a", Body: strings.NewReader("x"), Size: 1}, kind: ErrorKindTransfer},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.setup().PutObject(tt.ctx, tt.input)
			assertErrorKind(t, err, tt.kind)
		})
	}
}

func TestGetObject(t *testing.T) {
	metrics := &testMetrics{}
	body := newTrackingReadCloser("stored-body")
	client := newTestClient(t, fakeStore{getFn: func(key string) (storedObject, error) {
		return storedObject{
			Info: ObjectInfo{Key: key, Size: 11, ContentType: "text/plain", ETag: "etag"},
			Body: body,
		}, nil
	}}, metrics)

	output, err := client.GetObject(t.Context(), "objects/a.txt")
	if err != nil {
		t.Fatalf("GetObject failed: %v", err)
	}
	if output.Info.Key != "objects/a.txt" || output.Size != 11 || output.ContentType != "text/plain" || output.ETag != "etag" || output.Body != body {
		t.Fatalf("unexpected get output: %#v", output)
	}
	if metrics.countCounter(MetricOSSDownloadsTotal) != 1 {
		t.Fatal("download metric not recorded")
	}
}

func TestGetObjectErrors(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
		key  string
		fn   func(string) (storedObject, error)
		kind ErrorKind
	}{
		{name: "nil context", key: "a", kind: ErrorKindValidation},
		{name: "bad context", ctx: canceledContext(t), key: "a", kind: ErrorKindUnavailable},
		{name: "bad key", ctx: t.Context(), key: "../a", kind: ErrorKindValidation},
		{name: "store error", ctx: t.Context(), key: "a", fn: func(string) (storedObject, error) {
			return storedObject{}, NewError(ErrorKindNotFound, "get", "missing", false)
		}, kind: ErrorKindNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestClient(t, fakeStore{getFn: tt.fn}, &testMetrics{})
			_, err := client.GetObject(tt.ctx, tt.key)
			assertErrorKind(t, err, tt.kind)
		})
	}
}

func TestDeleteObject(t *testing.T) {
	var deleted string
	client := newTestClient(t, fakeStore{deleteFn: func(key string) error {
		deleted = key
		return nil
	}}, &testMetrics{})
	if err := client.DeleteObject(t.Context(), "objects/a.txt"); err != nil {
		t.Fatalf("DeleteObject failed: %v", err)
	}
	if deleted != "objects/a.txt" {
		t.Fatalf("unexpected deleted key: %q", deleted)
	}
}

func TestDeleteObjectErrors(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
		key  string
		fn   func(string) error
		kind ErrorKind
	}{
		{name: "nil context", key: "a", kind: ErrorKindValidation},
		{name: "bad context", ctx: canceledContext(t), key: "a", kind: ErrorKindUnavailable},
		{name: "bad key", ctx: t.Context(), key: "/a", kind: ErrorKindValidation},
		{name: "store error", ctx: t.Context(), key: "a", fn: func(string) error {
			return errors.New("delete failed")
		}, kind: ErrorKindTransfer},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestClient(t, fakeStore{deleteFn: tt.fn}, &testMetrics{})
			err := client.DeleteObject(tt.ctx, tt.key)
			assertErrorKind(t, err, tt.kind)
		})
	}
}

func TestListObjects(t *testing.T) {
	var saw listRequest
	client := newTestClient(t, fakeStore{listFn: func(input listRequest) (storedList, error) {
		saw = input
		return storedList{
			Objects:     []ObjectInfo{{Key: "prefix/a.txt"}},
			IsTruncated: true,
			NextMarker:  "prefix/a.txt",
		}, nil
	}}, &testMetrics{})

	output, err := client.ListObjects(t.Context(), ListInput{Prefix: "prefix/", Marker: "prefix/0.txt", MaxKeys: 10})
	if err != nil {
		t.Fatalf("ListObjects failed: %v", err)
	}
	if saw.Prefix != "prefix/" || saw.Marker != "prefix/0.txt" || saw.MaxKeys != 10 {
		t.Fatalf("unexpected list request: %#v", saw)
	}
	if len(output.Objects) != 1 || !output.IsTruncated || output.NextMarker != "prefix/a.txt" {
		t.Fatalf("unexpected list output: %#v", output)
	}
}

func TestListObjectsErrors(t *testing.T) {
	tests := []struct {
		name  string
		ctx   context.Context
		input ListInput
		fn    func(listRequest) (storedList, error)
		kind  ErrorKind
	}{
		{name: "nil context", input: ListInput{}, kind: ErrorKindValidation},
		{name: "bad context", ctx: canceledContext(t), input: ListInput{}, kind: ErrorKindUnavailable},
		{name: "bad prefix", ctx: t.Context(), input: ListInput{Prefix: "/prefix"}, kind: ErrorKindValidation},
		{name: "bad marker", ctx: t.Context(), input: ListInput{Marker: "/marker"}, kind: ErrorKindValidation},
		{name: "negative max keys", ctx: t.Context(), input: ListInput{MaxKeys: -1}, kind: ErrorKindValidation},
		{name: "store error", ctx: t.Context(), input: ListInput{}, fn: func(listRequest) (storedList, error) {
			return storedList{}, errors.New("list failed")
		}, kind: ErrorKindTransfer},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestClient(t, fakeStore{listFn: tt.fn}, &testMetrics{})
			_, err := client.ListObjects(tt.ctx, tt.input)
			assertErrorKind(t, err, tt.kind)
		})
	}
}

func TestMetricHelpersAcceptNilMetrics(t *testing.T) {
	recordErrorMetric(nil, "op", errors.New("err"))
	recordOperationMetric(nil, "op", nowForTest(), nil)
	recordInflight(nil, "op", 1)
	recordTransferMetric(nil, "other", 1)

	metrics := &testMetrics{}
	recordTransferMetric(metrics, "other", 5)
	if len(metrics.histograms) != 1 || metrics.histograms[0].name != MetricOSSBytesTransferred {
		t.Fatalf("unexpected transfer metric events: %#v", metrics.histograms)
	}
}

func canceledContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	return ctx
}

func nowForTest() time.Time {
	return time.Now()
}
