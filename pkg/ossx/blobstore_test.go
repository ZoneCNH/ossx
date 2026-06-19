package ossx

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

// TC-002: Config validation
func TestConfigValidate(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		ok   bool
	}{
		{"valid minimal", Config{Endpoint: "https://x", Region: "cn", Bucket: "b"}, true},
		{"empty endpoint", Config{Region: "cn", Bucket: "b"}, false},
		{"empty bucket", Config{Endpoint: "x", Region: "cn"}, false},
		{"empty region", Config{Endpoint: "x", Bucket: "b"}, false},
		{"presign too long", Config{Endpoint: "x", Region: "cn", Bucket: "b", Presign: PresignPolicy{MaxTTL: 30 * time.Minute}}, false},
		{"unknown checksum", Config{Endpoint: "x", Region: "cn", Bucket: "b", Checksum: ChecksumPolicy{Algorithms: []ChecksumAlgorithm{"sha999"}}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.ok && err != nil {
				t.Fatalf("want ok, got %v", err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("want error, got nil")
			}
			if !tc.ok && !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("want ErrInvalidConfig, got %v", err)
			}
		})
	}
}

// TC-003: Key validation
func TestNewKey(t *testing.T) {
	cases := []struct {
		raw string
		ok  bool
	}{
		{"a/b.txt", true},
		{"deep/path/object", true},
		{"", false},
		{"/abs", false},
		{"a/../b", false},
		{"a/./b", false},
		{strings.Repeat("x", 2000), false},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			k, err := NewKey(tc.raw)
			if tc.ok && err != nil {
				t.Fatalf("want ok, got %v", err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("want error for %q, got %s", tc.raw, k)
			}
		})
	}
}

func TestSanitizedScope(t *testing.T) {
	long := Key(strings.Repeat("a", 50))
	if got := long.SanitizedScope(); !strings.HasSuffix(got, "…") {
		t.Fatalf("want ellipsis suffix, got %q", got)
	}
	short := Key("hello")
	if got := short.SanitizedScope(); got != "hello" {
		t.Fatalf("want unchanged, got %q", got)
	}
}

// TC-004: Basic object operations
func TestBlobStoreCRUD(t *testing.T) {
	ctx := context.Background()
	bs := mustStore(t)

	key, _ := NewKey("a/b.txt")
	body := []byte("hello world")

	info, err := bs.Put(ctx, key, bytes.NewReader(body), PutOptions{ContentType: "text/plain", ChecksumAlgo: ChecksumSHA256})
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if info.Size != int64(len(body)) {
		t.Fatalf("size mismatch: %d", info.Size)
	}
	if info.ChecksumHex == "" {
		t.Fatalf("expected checksum")
	}

	rdr, err := bs.Get(ctx, key, GetOptions{})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	got, err := io.ReadAll(rdr)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if err := rdr.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("body mismatch")
	}

	exists, err := bs.Exists(ctx, key)
	if err != nil || !exists {
		t.Fatalf("exists: %v %v", exists, err)
	}

	if _, err := bs.Head(ctx, key); err != nil {
		t.Fatalf("head: %v", err)
	}

	if err := bs.Delete(ctx, key, DeleteOptions{}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := bs.Get(ctx, key, GetOptions{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}

	// Idempotent delete
	if err := bs.Delete(ctx, key, DeleteOptions{}); err != nil {
		t.Fatalf("idempotent delete should succeed, got %v", err)
	}
	// Strict delete reports not found
	if err := bs.Delete(ctx, key, DeleteOptions{StrictNotFound: true}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("strict delete should report ErrNotFound, got %v", err)
	}
}

func TestPutChecksumAlgoValidation(t *testing.T) {
	ctx := context.Background()
	bs := mustStore(t)
	key, _ := NewKey("checked.bin")
	if _, err := bs.Put(ctx, key, bytes.NewReader([]byte("abc")), PutOptions{ChecksumAlgo: ChecksumSHA256}); err != nil {
		t.Fatalf("put: %v", err)
	}
	if _, err := bs.Put(ctx, key, bytes.NewReader([]byte("abc")), PutOptions{ChecksumAlgo: "md9"}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("invalid algo expected, got %v", err)
	}
}

func TestList(t *testing.T) {
	ctx := context.Background()
	bs := mustStore(t)

	for _, name := range []string{"a/1", "a/2", "a/3", "b/1"} {
		k, _ := NewKey(name)
		if _, err := bs.Put(ctx, k, strings.NewReader("x"), PutOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	page, err := bs.List(ctx, Prefix("a/"), ListOptions{MaxKeys: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("want 2, got %d", len(page.Items))
	}
	if !page.IsTruncated {
		t.Fatalf("expected truncated")
	}
	page2, err := bs.List(ctx, Prefix("a/"), ListOptions{MaxKeys: 2, ContinuationToken: page.NextContinuation})
	if err != nil {
		t.Fatal(err)
	}
	if len(page2.Items) != 1 {
		t.Fatalf("want 1 remaining, got %d", len(page2.Items))
	}
}

func TestCopy(t *testing.T) {
	ctx := context.Background()
	bs := mustStore(t)
	src, _ := NewKey("src/a")
	dst, _ := NewKey("dst/a")
	if _, err := bs.Put(ctx, src, strings.NewReader("payload"), PutOptions{ContentType: "text/plain"}); err != nil {
		t.Fatal(err)
	}
	info, err := bs.Copy(ctx, src, dst, CopyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if info.Key != dst {
		t.Fatalf("want %s, got %s", dst, info.Key)
	}
	exists, _ := bs.Exists(ctx, dst)
	if !exists {
		t.Fatalf("dst missing")
	}
}

// TC-006: Multipart full lifecycle (v1.1.0 — was stub in v1.0.2-alpha).
func TestMultipartNotImplemented(t *testing.T) {
	ctx := context.Background()
	bs := mustStore(t)
	sess, err := bs.Multipart(ctx)
	if err != nil {
		t.Fatalf("Multipart: %v", err)
	}
	id, err := sess.Initiate(ctx, Key("mp/k"), PutOptions{ContentType: "text/plain"})
	if err != nil {
		t.Fatalf("Initiate: %v", err)
	}
	part, err := sess.UploadPart(ctx, id, 1, strings.NewReader("hello"), 5)
	if err != nil {
		t.Fatalf("UploadPart: %v", err)
	}
	if _, err := sess.Complete(ctx, id, []PartETag{part}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
}

// TC-007: Presign enforces TTL + allowlist (v1.1.0 — signing now real).
func TestPresign(t *testing.T) {
	ctx := context.Background()
	cfg := validConfig()
	cfg.Presign = PresignPolicy{
		MaxTTL:            10 * time.Minute,
		AllowedOperations: []PresignOperation{PresignGet},
	}
	bs, err := NewBlobStore(cfg, NewInMemoryAdapter(), Hooks{})
	if err != nil {
		t.Fatal(err)
	}
	// PUT not in allowlist → auth error.
	if _, err := bs.Presign(ctx, Key("k"), PresignPut, PresignOptions{TTL: 60}); errorKind(err) != ErrorKindAuth {
		t.Fatalf("want ErrorKindAuth for PUT, got %v", err)
	}
	// GET within TTL + allowlist → success.
	url, err := bs.Presign(ctx, Key("k"), PresignGet, PresignOptions{TTL: 60})
	if err != nil {
		t.Fatalf("GET presign: %v", err)
	}
	if url.URL == "" || url.Method != "GET" {
		t.Fatalf("bad presign url: %+v", url)
	}
	// Over-TTL → validation error.
	if _, err := bs.Presign(ctx, Key("k"), PresignGet, PresignOptions{TTL: 9999}); errorKind(err) != ErrorKindValidation {
		t.Fatalf("want ErrorKindValidation for over-TTL, got %v", err)
	}
}

// TC-012: Health & idempotent close
func TestHealthAndClose(t *testing.T) {
	ctx := context.Background()
	bs := mustStore(t)
	rep := bs.Health(ctx)
	if !rep.Ready {
		t.Fatalf("expected ready")
	}
	if err := bs.Close(ctx); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := bs.Close(ctx); err != nil {
		t.Fatalf("idempotent close failed: %v", err)
	}
	if _, err := bs.Put(ctx, Key("x"), strings.NewReader("y"), PutOptions{}); !errors.Is(err, ErrClosed) {
		t.Fatalf("want ErrClosed, got %v", err)
	}
}

// TC-009: SPI does not leak SDK types — public API uses only stdlib + ossx types.
// The public BlobStore interface is implemented; the SPI (StoreAdapter) is
// exported so external adapters can implement it explicitly.
func TestSPISurface(t *testing.T) {
	var _ BlobStore = (*Store)(nil)
	var _ MultipartStarter = (*Store)(nil)
	var _ Presigner = (*Store)(nil)
	var _ HealthChecker = (*Store)(nil)
	var _ StoreCloser = (*Store)(nil)
	var _ StoreAdapter = (*InMemoryAdapter)(nil)
	var _ MultipartAdapter = (*InMemoryAdapter)(nil)
	var _ PresignAdapter = (*InMemoryAdapter)(nil)
	var _ AdapterLifecycle = (*InMemoryAdapter)(nil)
}

func TestPublicInterfacesStayWithinGovernanceLimit(t *testing.T) {
	limit := 7
	interfaces := []struct {
		name string
		typ  reflect.Type
	}{
		{"BlobStore", reflect.TypeOf((*BlobStore)(nil)).Elem()},
		{"MultipartStarter", reflect.TypeOf((*MultipartStarter)(nil)).Elem()},
		{"Presigner", reflect.TypeOf((*Presigner)(nil)).Elem()},
		{"HealthChecker", reflect.TypeOf((*HealthChecker)(nil)).Elem()},
		{"StoreCloser", reflect.TypeOf((*StoreCloser)(nil)).Elem()},
		{"StoreAdapter", reflect.TypeOf((*StoreAdapter)(nil)).Elem()},
		{"MultipartAdapter", reflect.TypeOf((*MultipartAdapter)(nil)).Elem()},
		{"PresignAdapter", reflect.TypeOf((*PresignAdapter)(nil)).Elem()},
		{"AdapterLifecycle", reflect.TypeOf((*AdapterLifecycle)(nil)).Elem()},
		{"MultipartSession", reflect.TypeOf((*MultipartSession)(nil)).Elem()},
		{"Metrics", reflect.TypeOf((*Metrics)(nil)).Elem()},
		{"Tracer", reflect.TypeOf((*Tracer)(nil)).Elem()},
		{"Span", reflect.TypeOf((*Span)(nil)).Elem()},
		{"Logger", reflect.TypeOf((*Logger)(nil)).Elem()},
	}

	for _, iface := range interfaces {
		t.Run(iface.name, func(t *testing.T) {
			if got := iface.typ.NumMethod(); got > limit {
				t.Fatalf("%s has %d methods; governance limit is %d", iface.name, got, limit)
			}
		})
	}
}

func TestNewBlobStoreRejectsMissingAdapterCapabilities(t *testing.T) {
	cases := []struct {
		name    string
		adapter StoreAdapter
		want    string
	}{
		{name: "missing multipart", adapter: capabilityCoreAdapter{}, want: "multipart"},
		{name: "missing presign", adapter: capabilityMultipartAdapter{}, want: "presign"},
		{name: "missing lifecycle", adapter: capabilityPresignAdapter{}, want: "lifecycle"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewBlobStore(validConfig(), tc.adapter, Hooks{})
			if err == nil {
				t.Fatalf("want capability error")
			}
			if errorKind(err) != ErrorKindConfig {
				t.Fatalf("want config error, got %v", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
}

// TC-011: Hooks emit non-nil and survive nil callbacks.
func TestHooks(t *testing.T) {
	ctx := context.Background()
	mm := newMemoryMetrics()
	cfg := validConfig()
	bs, err := NewBlobStore(cfg, NewInMemoryAdapter(), Hooks{Metrics: mm})
	if err != nil {
		t.Fatal(err)
	}
	k, _ := NewKey("hooked")
	if _, err := bs.Put(ctx, k, strings.NewReader("data"), PutOptions{}); err != nil {
		t.Fatal(err)
	}
	if len(mm.counters) == 0 {
		t.Fatalf("expected hook to fire")
	}
}

// memoryMetrics is a test Metrics implementation that records counters.
type memoryMetrics struct {
	mu       sync.Mutex
	counters map[string]float64
	histos   map[string][]float64
}

func newMemoryMetrics() *memoryMetrics {
	return &memoryMetrics{counters: map[string]float64{}, histos: map[string][]float64{}}
}

func (m *memoryMetrics) IncCounter(name string, _ map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counters[name]++
}
func (m *memoryMetrics) AddCounter(name string, delta float64, _ map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counters[name] += delta
}
func (m *memoryMetrics) ObserveHistogram(name string, value float64, _ map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.histos[name] = append(m.histos[name], value)
}
func (m *memoryMetrics) SetGauge(string, float64, map[string]string) {}

// helpers
func validConfig() Config {
	return Config{
		Endpoint:  "https://internal.example",
		Region:    "cn-hangzhou",
		Bucket:    "test-bucket",
		Timeouts:  Timeouts{Operation: 30 * time.Second},
		Multipart: MultipartPolicy{MinPartSize: 8 << 20, MaxParts: 10000},
		Presign:   PresignPolicy{MaxTTL: 5 * time.Minute, AllowedOperations: []PresignOperation{PresignGet, PresignPut}},
	}
}

func mustStore(t *testing.T) *Store {
	t.Helper()
	bs, err := NewBlobStore(validConfig(), NewInMemoryAdapter(), Hooks{})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	return bs
}

type capabilityCoreAdapter struct{}

func (capabilityCoreAdapter) Name() string { return "core-only" }

func (capabilityCoreAdapter) PutObject(context.Context, string, io.Reader, int64, PutAdapterOptions) (ObjectInfo, error) {
	return ObjectInfo{}, nil
}

func (capabilityCoreAdapter) GetObject(_ context.Context, key string) (io.ReadCloser, ObjectInfo, error) {
	return io.NopCloser(strings.NewReader("")), ObjectInfo{Key: Key(key)}, nil
}

func (capabilityCoreAdapter) HeadObject(_ context.Context, key string) (ObjectInfo, error) {
	return ObjectInfo{Key: Key(key)}, nil
}

func (capabilityCoreAdapter) DeleteObject(context.Context, string, bool) error {
	return nil
}

func (capabilityCoreAdapter) CopyObject(_ context.Context, _ string, target string, _ CopyAdapterOptions) (ObjectInfo, error) {
	return ObjectInfo{Key: Key(target)}, nil
}

func (capabilityCoreAdapter) ListObjects(context.Context, string, int, string) (ListPage, error) {
	return ListPage{}, nil
}

type capabilityMultipartAdapter struct {
	capabilityCoreAdapter
}

func (capabilityMultipartAdapter) InitiateMultipart(context.Context, string, PutAdapterOptions) (UploadID, error) {
	return "upload", nil
}

func (capabilityMultipartAdapter) UploadPart(_ context.Context, _ UploadID, partNumber int, _ io.Reader, size int64) (PartETag, error) {
	return PartETag{PartNumber: partNumber, ETag: "etag", Size: size}, nil
}

func (capabilityMultipartAdapter) ListParts(context.Context, UploadID) ([]PartETag, error) {
	return nil, nil
}

func (capabilityMultipartAdapter) CompleteMultipart(context.Context, UploadID, []PartETag) (ObjectInfo, error) {
	return ObjectInfo{}, nil
}

func (capabilityMultipartAdapter) AbortMultipart(context.Context, UploadID) error {
	return nil
}

type capabilityPresignAdapter struct {
	capabilityMultipartAdapter
}

func (capabilityPresignAdapter) PresignURL(_ context.Context, _ string, op PresignOperation, ttlSeconds int64, _ PresignAdapterOptions) (PresignedURL, error) {
	return PresignedURL{
		URL:       "https://example.test/object",
		Method:    string(op),
		ExpiresAt: time.Now().Unix() + ttlSeconds,
	}, nil
}
