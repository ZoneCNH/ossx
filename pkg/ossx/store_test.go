package ossx

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

func TestNewObjectStoreProviderRouting(t *testing.T) {
	bucket := &fakeBucket{exists: true}
	withAliyunBucket(t, bucket, nil)
	store, err := newObjectStore(validConfig())
	if err != nil {
		t.Fatalf("expected OSS store, got %v", err)
	}
	if _, ok := store.(*aliyunStore); !ok {
		t.Fatalf("unexpected store type: %T", store)
	}

	for _, provider := range []Provider{ProviderS3, ProviderMinIO, ProviderAzure, ProviderGCS} {
		t.Run(string(provider), func(t *testing.T) {
			cfg := validConfig()
			cfg.Provider = provider
			_, err := newObjectStore(cfg)
			assertErrorKind(t, err, ErrorKindConfig)
			if !strings.Contains(err.Error(), "reserved but not implemented") {
				t.Fatalf("unexpected reserved provider error: %v", err)
			}
		})
	}

	cfg := validConfig()
	cfg.Provider = Provider("unknown")
	_, err = newObjectStore(cfg)
	assertErrorKind(t, err, ErrorKindConfig)
}

func TestNewAliyunStore(t *testing.T) {
	t.Run("bucket error is mapped", func(t *testing.T) {
		withAliyunBucket(t, nil, errors.New("dial failed"))
		_, err := newAliyunStore(validConfig())
		assertErrorKind(t, err, ErrorKindTransfer)
	})

	t.Run("bucket success", func(t *testing.T) {
		bucket := &fakeBucket{exists: true}
		withAliyunBucket(t, bucket, nil)
		store, err := newAliyunStore(validConfig())
		if err != nil {
			t.Fatalf("newAliyunStore failed: %v", err)
		}
		if store.(*aliyunStore).bucket != bucket {
			t.Fatalf("unexpected bucket: %#v", store)
		}
	})
}

func TestNewSDKAliyunBucket(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		bucket, err := newSDKAliyunBucket(validConfig())
		if err != nil {
			t.Fatalf("newSDKAliyunBucket failed: %v", err)
		}
		if bucket == nil {
			t.Fatal("expected bucket adapter")
		}
	})

	t.Run("invalid endpoint", func(t *testing.T) {
		cfg := validConfig()
		cfg.Endpoint = "%zz"
		_, err := newSDKAliyunBucket(cfg)
		if err == nil {
			t.Fatal("expected SDK constructor to reject invalid endpoint")
		}
	})

	t.Run("invalid bucket", func(t *testing.T) {
		cfg := validConfig()
		cfg.Bucket = "Invalid_Bucket"
		_, err := newSDKAliyunBucket(cfg)
		if err == nil {
			t.Fatal("expected SDK constructor to reject invalid bucket name")
		}
	})
}

func TestAliyunStoreObjectOperations(t *testing.T) {
	t.Run("put without content type", func(t *testing.T) {
		bucket := &fakeBucket{}
		store := &aliyunStore{bucket: bucket}
		if err := store.putObject("a.txt", strings.NewReader("payload"), ""); err != nil {
			t.Fatalf("putObject failed: %v", err)
		}
		if bucket.putKey != "a.txt" || bucket.putBody != "payload" || bucket.putOptions != 0 {
			t.Fatalf("unexpected put capture: %#v", bucket)
		}
	})

	t.Run("put with content type", func(t *testing.T) {
		bucket := &fakeBucket{}
		store := &aliyunStore{bucket: bucket}
		if err := store.putObject("a.txt", strings.NewReader("payload"), "text/plain"); err != nil {
			t.Fatalf("putObject failed: %v", err)
		}
		if bucket.putOptions != 1 {
			t.Fatalf("content type option not applied: %#v", bucket)
		}
	})

	t.Run("put error", func(t *testing.T) {
		store := &aliyunStore{bucket: &fakeBucket{putErr: errors.New("put failed")}}
		if err := store.putObject("a.txt", strings.NewReader("payload"), ""); err == nil {
			t.Fatal("expected put error")
		}
	})

	t.Run("get success", func(t *testing.T) {
		body := newTrackingReadCloser("stored")
		store := &aliyunStore{bucket: &fakeBucket{body: body}}
		object, err := store.getObject("a.txt")
		if err != nil {
			t.Fatalf("getObject failed: %v", err)
		}
		if object.Info.Key != "a.txt" || object.Info.Size != 6 || object.Info.ETag != "etag-value" || object.Body != body {
			t.Fatalf("unexpected object: %#v", object)
		}
	})

	t.Run("get object error", func(t *testing.T) {
		store := &aliyunStore{bucket: &fakeBucket{getErr: errors.New("get failed")}}
		_, err := store.getObject("a.txt")
		if err == nil {
			t.Fatal("expected get error")
		}
	})

	t.Run("get metadata error closes body", func(t *testing.T) {
		body := newTrackingReadCloser("stored")
		store := &aliyunStore{bucket: &fakeBucket{body: body, metaErr: errors.New("meta failed")}}
		_, err := store.getObject("a.txt")
		if err == nil {
			t.Fatal("expected metadata error")
		}
		if !body.closed {
			t.Fatal("body was not closed after metadata error")
		}
	})

	t.Run("delete success and error", func(t *testing.T) {
		bucket := &fakeBucket{}
		store := &aliyunStore{bucket: bucket}
		if err := store.deleteObject("a.txt"); err != nil {
			t.Fatalf("deleteObject failed: %v", err)
		}
		if bucket.deleteKey != "a.txt" {
			t.Fatalf("unexpected delete key: %q", bucket.deleteKey)
		}
		store = &aliyunStore{bucket: &fakeBucket{deleteErr: errors.New("delete failed")}}
		if err := store.deleteObject("a.txt"); err == nil {
			t.Fatal("expected delete error")
		}
	})

	t.Run("list success and error", func(t *testing.T) {
		bucket := &fakeBucket{listResult: oss.ListObjectsResult{
			Objects: []oss.ObjectProperties{{
				Key:          "prefix/a.txt",
				Size:         12,
				ETag:         `"etag"`,
				LastModified: time.Date(2026, 6, 13, 1, 2, 3, 0, time.FixedZone("JST", 9*60*60)),
			}},
			IsTruncated: true,
			NextMarker:  "prefix/a.txt",
		}}
		store := &aliyunStore{bucket: bucket}
		list, err := store.listObjects(listRequest{Prefix: "prefix/", Marker: "prefix/0.txt", MaxKeys: 1})
		if err != nil {
			t.Fatalf("listObjects failed: %v", err)
		}
		if bucket.listOptions != 3 || len(list.Objects) != 1 || list.Objects[0].LastModified != "2026-06-12T16:02:03Z" || !list.IsTruncated || list.NextMarker == "" {
			t.Fatalf("unexpected list result: bucket=%#v list=%#v", bucket, list)
		}

		store = &aliyunStore{bucket: &fakeBucket{listErr: errors.New("list failed")}}
		if _, err := store.listObjects(listRequest{}); err == nil {
			t.Fatal("expected list error")
		}
	})

	t.Run("check states", func(t *testing.T) {
		store := &aliyunStore{bucket: &fakeBucket{exists: true}}
		if err := store.check(); err != nil {
			t.Fatalf("check failed: %v", err)
		}
		store = &aliyunStore{bucket: &fakeBucket{existsErr: errors.New("exists failed")}}
		if err := store.check(); err == nil {
			t.Fatal("expected exists error")
		}
		store = &aliyunStore{bucket: &fakeBucket{exists: false}}
		assertErrorKind(t, store.check(), ErrorKindBucketNotFound)
	})
}

func TestObjectInfoHelpers(t *testing.T) {
	info := objectInfoFromHeaders("empty", http.Header{})
	if info.Key != "empty" || info.Size != 0 {
		t.Fatalf("unexpected empty headers info: %#v", info)
	}
	if trimETag(`"quoted"`) != "quoted" || trimETag("plain") != "plain" {
		t.Fatal("trimETag did not trim only wrapping quotes")
	}
}

func TestMapStoreError(t *testing.T) {
	existing := NewError(ErrorKindConflict, "op", "already mapped", false)
	if mapStoreError("op", nil) != nil {
		t.Fatal("nil error should map to nil")
	}
	if mapStoreError("op", existing) != existing {
		t.Fatal("existing *Error should be returned unchanged")
	}

	canceled := mapStoreError("op", context.Canceled)
	assertErrorKind(t, canceled, ErrorKindUnavailable)
	deadline := mapStoreError("op", context.DeadlineExceeded)
	assertErrorKind(t, deadline, ErrorKindTimeout)

	timeout := mapStoreError("op", timeoutError{})
	assertErrorKind(t, timeout, ErrorKindTimeout)

	serviceValue := mapStoreError("op", oss.ServiceError{StatusCode: http.StatusForbidden, Message: "forbidden"})
	assertErrorKind(t, serviceValue, ErrorKindAuth)
	servicePointer := mapStoreError("op", &oss.ServiceError{StatusCode: http.StatusTooManyRequests, Code: "TooManyRequests"})
	assertErrorKind(t, servicePointer, ErrorKindRateLimit)

	var nilService *oss.ServiceError
	generic := mapStoreError("op", nilService)
	assertErrorKind(t, generic, ErrorKindTransfer)
	generic = mapStoreError("op", errors.New("plain failure"))
	assertErrorKind(t, generic, ErrorKindTransfer)
}

func TestMapServiceErrorKinds(t *testing.T) {
	tests := []struct {
		name      string
		err       oss.ServiceError
		wantKind  ErrorKind
		retryable bool
		message   string
	}{
		{name: "bucket", err: oss.ServiceError{Code: "NoSuchBucket"}, wantKind: ErrorKindBucketNotFound, message: "NoSuchBucket"},
		{name: "key", err: oss.ServiceError{Code: "NoSuchKey"}, wantKind: ErrorKindNotFound, message: "NoSuchKey"},
		{name: "object", err: oss.ServiceError{Code: "NoSuchObject"}, wantKind: ErrorKindNotFound, message: "NoSuchObject"},
		{name: "forbidden", err: oss.ServiceError{StatusCode: http.StatusForbidden, Message: "forbidden"}, wantKind: ErrorKindAuth, message: "forbidden"},
		{name: "not found", err: oss.ServiceError{StatusCode: http.StatusNotFound}, wantKind: ErrorKindNotFound, message: "object storage service error"},
		{name: "conflict", err: oss.ServiceError{StatusCode: http.StatusConflict}, wantKind: ErrorKindConflict, message: "object storage service error"},
		{name: "large", err: oss.ServiceError{StatusCode: http.StatusRequestEntityTooLarge}, wantKind: ErrorKindObjectTooLarge, message: "object storage service error"},
		{name: "rate", err: oss.ServiceError{StatusCode: http.StatusTooManyRequests}, wantKind: ErrorKindRateLimit, retryable: true, message: "object storage service error"},
		{name: "timeout", err: oss.ServiceError{StatusCode: http.StatusRequestTimeout}, wantKind: ErrorKindTimeout, retryable: true, message: "object storage service error"},
		{name: "gateway timeout", err: oss.ServiceError{StatusCode: http.StatusGatewayTimeout}, wantKind: ErrorKindTimeout, retryable: true, message: "object storage service error"},
		{name: "server", err: oss.ServiceError{StatusCode: http.StatusInternalServerError}, wantKind: ErrorKindUnavailable, retryable: true, message: "object storage service error"},
		{name: "bad request", err: oss.ServiceError{StatusCode: http.StatusBadRequest}, wantKind: ErrorKindValidation, message: "object storage service error"},
		{name: "default", err: oss.ServiceError{StatusCode: http.StatusUnauthorized, Code: "Unauthorized"}, wantKind: ErrorKindTransfer, message: "Unauthorized"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := mapServiceError("op", tt.err, tt.err)
			assertErrorKind(t, err, tt.wantKind)
			var mapped *Error
			if !errors.As(err, &mapped) {
				t.Fatalf("expected *Error, got %T", err)
			}
			if mapped.Retryable != tt.retryable {
				t.Fatalf("unexpected retryable value: %#v", mapped)
			}
			if mapped.Message != tt.message {
				t.Fatalf("expected message %q, got %#v", tt.message, mapped)
			}
		})
	}
}

func TestSDKAliyunBucketPanicsOnNilBucket(t *testing.T) {
	var bucket sdkAliyunBucket
	assertPanic(t, func() { _ = bucket.PutObject("a", strings.NewReader("x")) })
	assertPanic(t, func() { _, _ = bucket.GetObject("a") })
	assertPanic(t, func() { _, _ = bucket.GetObjectDetailedMeta("a") })
	assertPanic(t, func() { _ = bucket.DeleteObject("a") })
	assertPanic(t, func() { _, _ = bucket.ListObjects() })
	assertPanic(t, func() { _, _ = bucket.IsBucketExist() })
}

type timeoutError struct{}

func (timeoutError) Error() string   { return "timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

var _ net.Error = timeoutError{}
var _ io.Reader = strings.NewReader("")
