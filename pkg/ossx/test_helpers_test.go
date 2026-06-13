package ossx

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

func validConfig() Config {
	return Config{
		Name:            "ossx-test",
		Provider:        ProviderOSS,
		Endpoint:        "oss-ap-northeast-1.aliyuncs.com",
		Region:          "ap-northeast-1",
		Bucket:          "ossx-test-bucket",
		AccessKeyID:     "test-access-key",
		SecretAccessKey: "test-secret-key",
		UseSSL:          true,
		Timeout:         time.Second,
	}
}

type fakeStore struct {
	putFn    func(string, io.Reader, string) error
	getFn    func(string) (storedObject, error)
	deleteFn func(string) error
	listFn   func(listRequest) (storedList, error)
	checkFn  func() error
}

func (s fakeStore) putObject(key string, body io.Reader, contentType string) error {
	if s.putFn != nil {
		return s.putFn(key, body, contentType)
	}
	return nil
}

func (s fakeStore) getObject(key string) (storedObject, error) {
	if s.getFn != nil {
		return s.getFn(key)
	}
	return storedObject{
		Info: ObjectInfo{Key: key, Size: 4, ContentType: "text/plain", ETag: "etag"},
		Body: io.NopCloser(strings.NewReader("body")),
	}, nil
}

func (s fakeStore) deleteObject(key string) error {
	if s.deleteFn != nil {
		return s.deleteFn(key)
	}
	return nil
}

func (s fakeStore) listObjects(input listRequest) (storedList, error) {
	if s.listFn != nil {
		return s.listFn(input)
	}
	return storedList{Objects: []ObjectInfo{{Key: input.Prefix + "one"}}}, nil
}

func (s fakeStore) check() error {
	if s.checkFn != nil {
		return s.checkFn()
	}
	return nil
}

func withObjectStore(t *testing.T, store objectStore) {
	t.Helper()
	old := newObjectStoreFunc
	newObjectStoreFunc = func(Config) (objectStore, error) {
		if store == nil {
			return nil, errors.New("store factory failed")
		}
		return store, nil
	}
	t.Cleanup(func() {
		newObjectStoreFunc = old
	})
}

func withAliyunBucket(t *testing.T, bucket aliyunBucket, err error) {
	t.Helper()
	old := newAliyunBucketFunc
	newAliyunBucketFunc = func(Config) (aliyunBucket, error) {
		if err != nil {
			return nil, err
		}
		return bucket, nil
	}
	t.Cleanup(func() {
		newAliyunBucketFunc = old
	})
}

func newTestClient(t *testing.T, store objectStore, metrics Metrics) *Client {
	t.Helper()
	withObjectStore(t, store)
	if metrics == nil {
		metrics = NoopMetrics{}
	}
	client, err := New(t.Context(), validConfig(), WithMetrics(metrics))
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	return client
}

type metricEvent struct {
	name   string
	value  float64
	labels map[string]string
}

type testMetrics struct {
	mu         sync.Mutex
	counters   []metricEvent
	histograms []metricEvent
	gauges     []metricEvent
}

func (m *testMetrics) IncCounter(name string, labels map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counters = append(m.counters, metricEvent{name: name, labels: cloneLabels(labels)})
}

func (m *testMetrics) ObserveHistogram(name string, value float64, labels map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.histograms = append(m.histograms, metricEvent{name: name, value: value, labels: cloneLabels(labels)})
}

func (m *testMetrics) SetGauge(name string, value float64, labels map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.gauges = append(m.gauges, metricEvent{name: name, value: value, labels: cloneLabels(labels)})
}

func (m *testMetrics) countCounter(name string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, event := range m.counters {
		if event.name == name {
			count++
		}
	}
	return count
}

func cloneLabels(labels map[string]string) map[string]string {
	cloned := make(map[string]string, len(labels))
	for key, value := range labels {
		cloned[key] = value
	}
	return cloned
}

type fakeBucket struct {
	putErr       error
	getErr       error
	metaErr      error
	deleteErr    error
	listErr      error
	existsErr    error
	exists       bool
	putOptions   int
	putBody      string
	putKey       string
	deleteKey    string
	listOptions  int
	body         io.ReadCloser
	headers      http.Header
	listResult   oss.ListObjectsResult
	closedBodies []*trackingReadCloser
}

func (b *fakeBucket) PutObject(objectKey string, reader io.Reader, options ...oss.Option) error {
	b.putKey = objectKey
	b.putOptions = len(options)
	if reader != nil {
		data, _ := io.ReadAll(reader)
		b.putBody = string(data)
	}
	return b.putErr
}

func (b *fakeBucket) GetObject(objectKey string, options ...oss.Option) (io.ReadCloser, error) {
	if b.getErr != nil {
		return nil, b.getErr
	}
	if b.body != nil {
		return b.body, nil
	}
	return io.NopCloser(strings.NewReader("stored")), nil
}

func (b *fakeBucket) GetObjectDetailedMeta(objectKey string, options ...oss.Option) (http.Header, error) {
	if b.metaErr != nil {
		return nil, b.metaErr
	}
	if b.headers != nil {
		return b.headers, nil
	}
	headers := http.Header{}
	headers.Set("Content-Length", "6")
	headers.Set("Content-Type", "text/plain")
	headers.Set("ETag", `"etag-value"`)
	headers.Set("Last-Modified", "Sat, 13 Jun 2026 00:00:00 GMT")
	return headers, nil
}

func (b *fakeBucket) DeleteObject(objectKey string, options ...oss.Option) error {
	b.deleteKey = objectKey
	return b.deleteErr
}

func (b *fakeBucket) ListObjects(options ...oss.Option) (oss.ListObjectsResult, error) {
	b.listOptions = len(options)
	if b.listErr != nil {
		return oss.ListObjectsResult{}, b.listErr
	}
	return b.listResult, nil
}

func (b *fakeBucket) IsBucketExist() (bool, error) {
	if b.existsErr != nil {
		return false, b.existsErr
	}
	return b.exists, nil
}

type trackingReadCloser struct {
	*bytes.Reader
	closed bool
}

func newTrackingReadCloser(value string) *trackingReadCloser {
	return &trackingReadCloser{Reader: bytes.NewReader([]byte(value))}
}

func (r *trackingReadCloser) Close() error {
	r.closed = true
	return nil
}

func assertErrorKind(t *testing.T, err error, kind ErrorKind) {
	t.Helper()
	if !IsKind(err, kind) {
		t.Fatalf("expected %s error, got %v", kind, err)
	}
}

func assertPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	fn()
}

func sortedKeys(labels map[string]string) []string {
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
