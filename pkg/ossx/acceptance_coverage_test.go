package ossx

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestAcceptanceConfigFromEnvAndSanitize(t *testing.T) {
	t.Setenv(envPrefix+"ENDPOINT", "oss-cn.example.aliyuncs.com")
	t.Setenv(envPrefix+"REGION", "cn-test")
	t.Setenv(envPrefix+"BUCKET", "foundationx")
	t.Setenv(envPrefix+"ACCESS_KEY", "AK123456YZ")
	t.Setenv(envPrefix+"SECRET_KEY", "sk12")
	t.Setenv(envPrefix+"USE_SSL", "false")
	t.Setenv(envPrefix+"CNAME", "cdn.example.com")
	t.Setenv(envPrefix+"OPERATION_TIMEOUT", "2s")
	t.Setenv(envPrefix+"CONNECT_TIMEOUT", "not-a-duration")
	t.Setenv(envPrefix+"PRESIGN_MAX_TTL", "4m")
	t.Setenv(envPrefix+"MULTIPART_MIN_PART", "1048576")
	t.Setenv(envPrefix+"MULTIPART_MAX_PARTS", "9")

	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("ConfigFromEnv: %v", err)
	}
	if cfg.Endpoint != "oss-cn.example.aliyuncs.com" || cfg.Region != "cn-test" || cfg.Bucket != "foundationx" {
		t.Fatalf("unexpected identity: %#v", cfg)
	}
	if cfg.UseSSL {
		t.Fatal("expected USE_SSL=false to be parsed")
	}
	if cfg.CNAME != "cdn.example.com" {
		t.Fatalf("unexpected cname: %q", cfg.CNAME)
	}
	if cfg.Timeouts.Operation != 2*time.Second {
		t.Fatalf("unexpected operation timeout: %s", cfg.Timeouts.Operation)
	}
	if cfg.Timeouts.Connect != 5*time.Second {
		t.Fatalf("invalid connect timeout should fall back to default, got %s", cfg.Timeouts.Connect)
	}
	if cfg.Presign.MaxTTL != 4*time.Minute {
		t.Fatalf("unexpected presign max ttl: %s", cfg.Presign.MaxTTL)
	}
	if cfg.Multipart.MinPartSize != 1048576 || cfg.Multipart.MaxParts != 9 {
		t.Fatalf("unexpected multipart policy: %#v", cfg.Multipart)
	}
	if got := cfg.Sanitize(); got.AccessKey != "AK***YZ" || got.SecretKey != "***" {
		t.Fatalf("unexpected sanitized secrets: access=%q secret=%q", got.AccessKey, got.SecretKey)
	}
}

func TestAcceptanceConfigFromEnvMissingRequired(t *testing.T) {
	for _, key := range []string{"ENDPOINT", "REGION", "BUCKET", "ACCESS_KEY", "SECRET_KEY"} {
		t.Setenv(envPrefix+key, "")
	}

	_, err := ConfigFromEnv()
	if err == nil {
		t.Fatal("expected missing env error")
	}
	if errorKind(err) != ErrorKindConfig {
		t.Fatalf("expected config error, got %s: %v", errorKind(err), err)
	}
	msg := err.Error()
	for _, want := range []string{envPrefix + "ENDPOINT", envPrefix + "SECRET_KEY"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("missing %s in error: %v", want, err)
		}
	}
}

func TestAcceptanceConfigValidationEdges(t *testing.T) {
	cases := map[string]func(*Config){
		"operation timeout": func(c *Config) { c.Timeouts.Operation = -time.Second },
		"connect timeout":   func(c *Config) { c.Timeouts.Connect = -time.Second },
		"multipart min":     func(c *Config) { c.Multipart.MinPartSize = -1 },
		"multipart max":     func(c *Config) { c.Multipart.MaxParts = -1 },
		"multipart stale":   func(c *Config) { c.Multipart.StaleTTL = -time.Second },
		"presign too long":  func(c *Config) { c.Presign.MaxTTL = MaxAllowedPresignTTL + time.Second },
		"presign negative":  func(c *Config) { c.Presign.MaxTTL = -time.Second },
		"checksum algo":     func(c *Config) { c.Checksum.Algorithms = []ChecksumAlgorithm{"sha1"} },
		"lifecycle days": func(c *Config) {
			c.Policy.Lifecycle.Enabled = true
			c.Policy.Lifecycle.MinDays = -1
			c.Policy.Lifecycle.StorageClass = "IA"
		},
		"lifecycle class": func(c *Config) {
			c.Policy.Lifecycle.Enabled = true
			c.Policy.Lifecycle.MinDays = 1
		},
		"retention mode": func(c *Config) { c.Policy.Retention.Mode = "forever" },
		"retention days": func(c *Config) {
			c.Policy.Retention.Mode = RetentionModeGovernance
			c.Policy.Retention.MaxDays = -1
		},
		"permission overlap": func(c *Config) {
			c.Policy.Permission.AllowedPrefixes = []string{"tenant-a/"}
			c.Policy.Permission.DeniedPrefixes = []string{"tenant-a/"}
		},
		"retry attempts":   func(c *Config) { c.Retry.MaxAttempts = -1 },
		"retry multiplier": func(c *Config) { c.Retry.Multiplier = -1 },
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := validConfig()
			mutate(&cfg)
			if err := cfg.Validate(); err == nil || errorKind(err) != ErrorKindConfig {
				t.Fatalf("expected config error, got %v", err)
			}
		})
	}
}

func TestAcceptanceErrorWrappersAndContext(t *testing.T) {
	cause := errors.New("provider")
	wrapped := WrapExport(ErrorKindTimeout, "get", "timed out", cause)
	if !errors.Is(wrapped, cause) {
		t.Fatalf("expected wrapped provider cause")
	}
	if !isRetryable(wrapped) || classifyError(wrapped) != retryClassRetryable {
		t.Fatalf("timeout should be retryable")
	}

	validation := NewExport(ErrorKindValidation, "validate", "bad input")
	if isRetryable(validation) || classifyError(validation) != retryClassNonRetryable {
		t.Fatalf("validation should be non-retryable")
	}
	if !errors.Is(NewExport(ErrorKindConfig, "config", "bad"), ErrInvalidConfig) {
		t.Fatalf("config error should match sentinel kind")
	}
	if got := ctxCancelledError(context.Canceled); errorKind(got) != ErrorKindCanceled || !errors.Is(got, context.Canceled) {
		t.Fatalf("unexpected canceled error: %v", got)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ctxErrCheck(cancelled); errorKind(err) != ErrorKindCanceled {
		t.Fatalf("cancelled context should be canceled kind, got %v", err)
	}
	deadline, cancelDeadline := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancelDeadline()
	if err := ctxErrCheck(deadline); errorKind(err) != ErrorKindTimeout {
		t.Fatalf("deadline context should be timeout kind, got %v", err)
	}
	if errorKind(errors.New("plain")) != ErrorKindInternal || errorKind(nil) != ErrorKindInternal {
		t.Fatal("plain and nil errors should classify as internal")
	}
}

func TestAcceptanceChecksumVerifier(t *testing.T) {
	const payload = "checksum me"
	sum := computeChecksum(ChecksumSHA256, []byte(payload))

	reader := &ObjectReader{
		ReadCloser: io.NopCloser(strings.NewReader(payload)),
		Info: ObjectInfo{
			ChecksumAlgo: ChecksumSHA256,
			ChecksumHex:  sum,
		},
	}
	wrapped := wrapChecksumVerifier(reader, reader.Info)
	got, err := io.ReadAll(wrapped)
	if err != nil {
		t.Fatalf("ReadAll verified: %v", err)
	}
	if string(got) != payload || !wrapped.ChecksumVerified {
		t.Fatalf("checksum should verify, got body=%q verified=%v", got, wrapped.ChecksumVerified)
	}
	if err := wrapped.Close(); err != nil {
		t.Fatalf("close verified reader: %v", err)
	}

	badReader := &ObjectReader{
		ReadCloser: io.NopCloser(strings.NewReader(payload)),
		Info: ObjectInfo{
			ChecksumAlgo: ChecksumSHA256,
			ChecksumHex:  "deadbeef",
		},
	}
	bad := wrapChecksumVerifier(badReader, badReader.Info)
	if _, err := io.ReadAll(bad); err == nil || errorKind(err) != ErrorKindChecksum {
		t.Fatalf("expected checksum mismatch, got %v", err)
	}
	if bad.ChecksumVerified {
		t.Fatal("mismatched checksum must not mark verified")
	}

	unsupported := &ObjectReader{ReadCloser: io.NopCloser(strings.NewReader(payload))}
	if got := wrapChecksumVerifier(unsupported, ObjectInfo{ChecksumAlgo: "sha1", ChecksumHex: "x"}); got != unsupported {
		t.Fatal("unsupported checksum verifier should return original reader")
	}
	if computeChecksum(ChecksumMD5, []byte(payload)) == "" || computeChecksum(ChecksumCRC32, []byte(payload)) == "" {
		t.Fatal("md5 and crc32 checksum helpers should produce values")
	}
	if computeChecksum("sha1", []byte(payload)) != "" || newHasher("sha1") != nil {
		t.Fatal("unsupported checksum helper should return empty/nil")
	}
}

func TestAcceptanceMultipartValidationAndAbort(t *testing.T) {
	ctx := context.Background()
	store := mustStoreConfig(t, validConfigWith(t, func(c *Config) {
		c.Multipart.MaxParts = 3
	}))
	session, err := store.Multipart(ctx)
	if err != nil {
		t.Fatalf("Multipart: %v", err)
	}
	key, _ := NewKey("multipart/object")
	id, err := session.Initiate(ctx, key, PutOptions{})
	if err != nil {
		t.Fatalf("Initiate: %v", err)
	}

	if _, err := session.UploadPart(ctx, id, 0, strings.NewReader("bad"), 3); err == nil || errorKind(err) != ErrorKindValidation {
		t.Fatalf("expected invalid part number, got %v", err)
	}
	part1, err := session.UploadPart(ctx, id, 1, strings.NewReader("one"), 3)
	if err != nil {
		t.Fatalf("UploadPart 1: %v", err)
	}
	part2, err := session.UploadPart(ctx, id, 2, strings.NewReader("two"), 3)
	if err != nil {
		t.Fatalf("UploadPart 2: %v", err)
	}
	parts, err := session.ListParts(ctx, id)
	if err != nil || len(parts) != 2 || parts[0].PartNumber != 1 || parts[1].PartNumber != 2 {
		t.Fatalf("unexpected parts: parts=%#v err=%v", parts, err)
	}
	if _, err := session.Complete(ctx, id, nil); err == nil || errorKind(err) != ErrorKindValidation {
		t.Fatalf("expected empty complete validation, got %v", err)
	}
	if _, err := session.Complete(ctx, id, []PartETag{part2}); err == nil || errorKind(err) != ErrorKindValidation {
		t.Fatalf("expected non-contiguous complete validation, got %v", err)
	}
	partNoETag := part1
	partNoETag.ETag = ""
	if _, err := session.Complete(ctx, id, []PartETag{partNoETag, part2}); err == nil || errorKind(err) != ErrorKindValidation {
		t.Fatalf("expected missing etag validation, got %v", err)
	}
	info, err := session.Complete(ctx, id, []PartETag{part1, part2})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if info.Size != part1.Size+part2.Size {
		t.Fatalf("unexpected completed size: %d", info.Size)
	}
	if _, err := session.Complete(ctx, id, []PartETag{part1, part2}); err == nil || errorKind(err) != ErrorKindConflict {
		t.Fatalf("expected second complete conflict, got %v", err)
	}
	if err := session.Abort(ctx, id); err != nil {
		t.Fatalf("abort after complete should be idempotent: %v", err)
	}
	if err := session.Abort(ctx, UploadID("missing")); err != nil {
		t.Fatalf("abort missing should be idempotent: %v", err)
	}

	limited := mustStoreConfig(t, validConfigWith(t, func(c *Config) {
		c.Multipart.MaxParts = 1
	}))
	limitedSession, err := limited.Multipart(ctx)
	if err != nil {
		t.Fatalf("limited Multipart: %v", err)
	}
	limitedID, err := limitedSession.Initiate(ctx, key, PutOptions{})
	if err != nil {
		t.Fatalf("limited Initiate: %v", err)
	}
	if _, err := limitedSession.UploadPart(ctx, limitedID, 2, strings.NewReader("two"), 3); err == nil || errorKind(err) != ErrorKindValidation {
		t.Fatalf("expected max part upload validation, got %v", err)
	}
	if _, err := limitedSession.Complete(ctx, limitedID, []PartETag{part1, part2}); err == nil || errorKind(err) != ErrorKindValidation {
		t.Fatalf("expected max part complete validation, got %v", err)
	}
}

func TestAcceptanceRetryAndCircuitBreaker(t *testing.T) {
	ctx := context.Background()
	policy := retryPolicy{MaxAttempts: 3, InitialWait: time.Nanosecond, MaxWait: time.Nanosecond, Multiplier: 1}

	attempts := 0
	err := policy.withRetry(ctx, "put", func(context.Context) error {
		attempts++
		if attempts == 1 {
			return newError(ErrorKindUnavailable, "put", "temporary")
		}
		return nil
	})
	if err != nil || attempts != 2 {
		t.Fatalf("expected retry success on second attempt, attempts=%d err=%v", attempts, err)
	}

	attempts = 0
	err = policy.withRetry(ctx, "put", func(context.Context) error {
		attempts++
		return newError(ErrorKindValidation, "put", "bad")
	})
	if err == nil || attempts != 1 {
		t.Fatalf("non-retryable error should not retry, attempts=%d err=%v", attempts, err)
	}

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	attempts = 0
	if err := policy.withRetry(cancelled, "put", func(context.Context) error {
		attempts++
		return nil
	}); err == nil || errorKind(err) != ErrorKindCanceled || attempts != 0 {
		t.Fatalf("cancelled retry should stop before fn, attempts=%d err=%v", attempts, err)
	}
	if policy.delay(0) != 0 || policy.delay(1) != 0 || policy.delay(3) > policy.MaxWait {
		t.Fatalf("unexpected retry delays")
	}
	if got := retryPolicyFromConfig(RetryConfig{}); got.MaxAttempts != 3 || got.InitialWait <= 0 || got.MaxWait <= 0 || got.Multiplier != 2 {
		t.Fatalf("unexpected default retry policy: %#v", got)
	}

	cb := newCircuitBreaker(2, time.Hour)
	fail := func(context.Context) error { return newError(ErrorKindUnavailable, "health", "down") }
	if err := cb.do(ctx, "health", retryPolicy{MaxAttempts: 1, InitialWait: time.Nanosecond, MaxWait: time.Nanosecond, Multiplier: 1}, fail); err == nil {
		t.Fatal("first breaker failure should return provider error")
	}
	if err := cb.do(ctx, "health", retryPolicy{MaxAttempts: 1, InitialWait: time.Nanosecond, MaxWait: time.Nanosecond, Multiplier: 1}, fail); err == nil {
		t.Fatal("second breaker failure should return provider error")
	}
	called := false
	err = cb.do(ctx, "health", policy, func(context.Context) error {
		called = true
		return nil
	})
	if err == nil || !isErrCircuitOpen(err) || called {
		t.Fatalf("expected open circuit without call, called=%v err=%v", called, err)
	}
	cb.openedAt = time.Now().Add(-time.Hour)
	if err := cb.do(ctx, "health", policy, func(context.Context) error { return nil }); err != nil {
		t.Fatalf("half-open success should close breaker: %v", err)
	}
	if cb.state != breakerClosed {
		t.Fatalf("breaker should be closed after success, got %v", cb.state)
	}
}

func TestAcceptancePublicHelpersAndNoopHooks(t *testing.T) {
	items := []ObjectInfo{{Key: "b"}, {Key: "a"}}
	sorted := SortedKeys(items)
	if sorted[0].Key != "a" || sorted[1].Key != "b" {
		t.Fatalf("items not sorted: %#v", sorted)
	}
	if items[0].Key != "b" {
		t.Fatal("SortedKeys should not mutate input")
	}
	if !HasPrefix("tenant-a/object", "tenant-a/") || HasPrefix("tenant-b/object", "tenant-a/") {
		t.Fatal("HasPrefix returned unexpected result")
	}
	if gaugeForStatus("ready") != 1 || gaugeForStatus("ok") != 1 || gaugeForStatus("degraded") != 0.5 || gaugeForStatus("down") != 0 {
		t.Fatal("unexpected health gauge mapping")
	}

	hooks := Hooks{}.withDefaults()
	hooks.Metrics.IncCounter("counter", nil)
	hooks.Metrics.AddCounter("counter", 1, nil)
	hooks.Metrics.ObserveHistogram("histogram", 1, nil)
	hooks.Metrics.SetGauge("gauge", 1, nil)
	ctx, span := hooks.Tracer.Start(context.Background(), "span", Field{Key: "k", Value: "v"})
	span.SetField(Field{Key: "x", Value: 1})
	span.AddEvent("event", Field{Key: "secret", Value: "redacted", Secret: true})
	span.End()
	hooks.Logger.Debug(ctx, "debug")
	hooks.Logger.Info(ctx, "info")
	hooks.Logger.Warn(ctx, "warn")
	hooks.Logger.Error(ctx, "error")
	hooks.emit("put", "ok", "tenant-a/", 5, time.Millisecond)
	hooks.emitAudit(ctx, AuditEvent{
		Operation:     "presign",
		Result:        "ok",
		KeyScope:      "tenant-a/",
		TTLSeconds:    60,
		Method:        "GET",
		ActorFields:   map[string]string{"actor": "test"},
		CorrelationID: "corr",
		OccurredAt:    time.Now(),
	})
	(*Hooks)(nil).emit("put", "ok", "", 0, 0)
	(*Hooks)(nil).emitAudit(ctx, AuditEvent{})
}

func TestAcceptanceInMemoryAdapterEdges(t *testing.T) {
	ctx := context.Background()
	adapter := NewInMemoryAdapter()
	if adapter.Name() != "in-memory" {
		t.Fatalf("unexpected adapter name: %s", adapter.Name())
	}
	if err := adapter.Health(ctx); err != nil {
		t.Fatalf("health before close: %v", err)
	}

	key := "tenant-a/object"
	info, err := adapter.PutObject(ctx, key, strings.NewReader("payload"), int64(len("payload")), PutAdapterOptions{
		ContentType:  "text/plain",
		Metadata:     map[string]string{"m": "v"},
		Tags:         map[string]string{"t": "v"},
		ChecksumAlgo: ChecksumMD5,
	})
	if err != nil {
		t.Fatalf("PutObject: %v", err)
	}
	if info.ChecksumAlgo != ChecksumMD5 || info.ChecksumHex == "" {
		t.Fatalf("expected md5 checksum info: %#v", info)
	}
	if inferChecksumAlgo("deadbeef") != ChecksumCRC32 || inferChecksumAlgo(strings.Repeat("a", 32)) != ChecksumMD5 || inferChecksumAlgo(strings.Repeat("a", 64)) != ChecksumSHA256 || inferChecksumAlgo("123") != "" {
		t.Fatal("unexpected checksum inference")
	}

	reader, gotInfo, err := adapter.GetObject(ctx, key)
	if err != nil {
		t.Fatalf("GetObject: %v", err)
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read object: %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("close object reader: %v", err)
	}
	if string(body) != "payload" || gotInfo.Key != Key(key) {
		t.Fatalf("unexpected object: body=%q info=%#v", body, gotInfo)
	}
	if _, err := adapter.HeadObject(ctx, key); err != nil {
		t.Fatalf("HeadObject: %v", err)
	}
	if _, err := adapter.CopyObject(ctx, "missing", "tenant-a/copy", CopyAdapterOptions{}); err == nil || errorKind(err) != ErrorKindNotFound {
		t.Fatalf("expected missing copy source, got %v", err)
	}
	if _, err := adapter.CopyObject(ctx, key, "tenant-a/copy", CopyAdapterOptions{ContentType: "application/json"}); err != nil {
		t.Fatalf("CopyObject: %v", err)
	}
	if err := adapter.DeleteObject(ctx, "missing", false); err != nil {
		t.Fatalf("non-strict delete should be idempotent: %v", err)
	}
	if err := adapter.DeleteObject(ctx, "missing", true); err == nil || errorKind(err) != ErrorKindNotFound {
		t.Fatalf("strict delete should return not found, got %v", err)
	}
	page, err := adapter.ListObjects(ctx, "tenant-a/", 1, "unknown-token")
	if err != nil {
		t.Fatalf("ListObjects unknown token: %v", err)
	}
	if len(page.Items) != 1 || !page.IsTruncated || page.NextContinuation == "" {
		t.Fatalf("unexpected paged list: %#v", page)
	}
	presigned, err := adapter.PresignURL(ctx, key, PresignGet, 60, PresignAdapterOptions{})
	if err != nil || presigned.URL == "" || presigned.Method != string(PresignGet) {
		t.Fatalf("unexpected presign: url=%#v err=%v", presigned, err)
	}

	uploadID, err := adapter.InitiateMultipart(ctx, "tenant-a/multipart", PutAdapterOptions{})
	if err != nil {
		t.Fatalf("InitiateMultipart: %v", err)
	}
	part2, err := adapter.UploadPart(ctx, uploadID, 2, bytes.NewReader([]byte("two")), 3)
	if err != nil {
		t.Fatalf("UploadPart 2: %v", err)
	}
	part1, err := adapter.UploadPart(ctx, uploadID, 1, bytes.NewReader([]byte("one")), 3)
	if err != nil {
		t.Fatalf("UploadPart 1: %v", err)
	}
	unsorted := []PartETag{part2, part1}
	sortParts(unsorted)
	if unsorted[0].PartNumber != 1 || unsorted[1].PartNumber != 2 {
		t.Fatalf("parts not sorted: %#v", unsorted)
	}
	if _, err := adapter.ListParts(ctx, UploadID("missing")); err == nil || errorKind(err) != ErrorKindNotFound {
		t.Fatalf("expected missing list parts error, got %v", err)
	}
	completed, err := adapter.CompleteMultipart(ctx, uploadID, []PartETag{part1, part2})
	if err != nil {
		t.Fatalf("CompleteMultipart: %v", err)
	}
	if completed.Size != 6 {
		t.Fatalf("unexpected completed size: %d", completed.Size)
	}
	if err := adapter.AbortMultipart(ctx, UploadID("missing")); err != nil {
		t.Fatalf("AbortMultipart missing should be idempotent: %v", err)
	}

	if err := adapter.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := adapter.Health(ctx); err == nil || errorKind(err) != ErrorKindClosed {
		t.Fatalf("closed health should fail, got %v", err)
	}
	if _, err := adapter.HeadObject(ctx, key); err == nil || errorKind(err) != ErrorKindClosed {
		t.Fatalf("closed head should fail, got %v", err)
	}
	if err := adapter.AbortMultipart(ctx, UploadID("anything")); err != nil {
		t.Fatalf("closed abort remains idempotent: %v", err)
	}
}

type acceptanceScriptedAdapter struct {
	name string

	putErr       error
	getErr       error
	headErr      error
	deleteErr    error
	copyErr      error
	listErr      error
	initErr      error
	uploadErr    error
	listPartsErr error
	completeErr  error
	abortErr     error
	presignErr   error
	healthErr    error
	closeErr     error

	putInfo      ObjectInfo
	getInfo      ObjectInfo
	headInfo     ObjectInfo
	copyInfo     ObjectInfo
	completeInfo ObjectInfo
	listPage     ListPage
	uploadID     UploadID
	uploadPart   PartETag
	presigned    PresignedURL

	listMax    int
	closeCalls int
}

func (a *acceptanceScriptedAdapter) Name() string {
	if a.name != "" {
		return a.name
	}
	return "scripted"
}

func (a *acceptanceScriptedAdapter) PutObject(context.Context, string, io.Reader, int64, PutAdapterOptions) (ObjectInfo, error) {
	return a.putInfo, a.putErr
}

func (a *acceptanceScriptedAdapter) GetObject(context.Context, string) (io.ReadCloser, ObjectInfo, error) {
	if a.getErr != nil {
		return nil, ObjectInfo{}, a.getErr
	}
	return io.NopCloser(strings.NewReader("body")), a.getInfo, nil
}

func (a *acceptanceScriptedAdapter) HeadObject(context.Context, string) (ObjectInfo, error) {
	return a.headInfo, a.headErr
}

func (a *acceptanceScriptedAdapter) DeleteObject(context.Context, string, bool) error {
	return a.deleteErr
}

func (a *acceptanceScriptedAdapter) CopyObject(context.Context, string, string, CopyAdapterOptions) (ObjectInfo, error) {
	return a.copyInfo, a.copyErr
}

func (a *acceptanceScriptedAdapter) ListObjects(_ context.Context, _ string, max int, _ string) (ListPage, error) {
	a.listMax = max
	return a.listPage, a.listErr
}

func (a *acceptanceScriptedAdapter) InitiateMultipart(context.Context, string, PutAdapterOptions) (UploadID, error) {
	return a.uploadID, a.initErr
}

func (a *acceptanceScriptedAdapter) UploadPart(context.Context, UploadID, int, io.Reader, int64) (PartETag, error) {
	return a.uploadPart, a.uploadErr
}

func (a *acceptanceScriptedAdapter) ListParts(context.Context, UploadID) ([]PartETag, error) {
	return []PartETag{a.uploadPart}, a.listPartsErr
}

func (a *acceptanceScriptedAdapter) CompleteMultipart(context.Context, UploadID, []PartETag) (ObjectInfo, error) {
	return a.completeInfo, a.completeErr
}

func (a *acceptanceScriptedAdapter) AbortMultipart(context.Context, UploadID) error {
	return a.abortErr
}

func (a *acceptanceScriptedAdapter) PresignURL(context.Context, string, PresignOperation, int64, PresignAdapterOptions) (PresignedURL, error) {
	return a.presigned, a.presignErr
}

func (a *acceptanceScriptedAdapter) Health(context.Context) error {
	return a.healthErr
}

func (a *acceptanceScriptedAdapter) Close(context.Context) error {
	a.closeCalls++
	return a.closeErr
}

func acceptanceSingleAttemptConfig() Config {
	cfg := validConfig()
	cfg.Retry = RetryConfig{MaxAttempts: 1, InitialWait: time.Millisecond, MaxWait: time.Millisecond, Multiplier: 1}
	return cfg
}

func TestAcceptanceNewBlobStoreRejectsInvalidConstruction(t *testing.T) {
	if _, err := NewBlobStore(validConfig(), nil, Hooks{}); err == nil || errorKind(err) != ErrorKindConfig {
		t.Fatalf("nil adapter should be a config error, got %v", err)
	}

	cfg := validConfig()
	cfg.Endpoint = ""
	if _, err := NewBlobStore(cfg, NewInMemoryAdapter(), Hooks{}); err == nil || errorKind(err) != ErrorKindConfig {
		t.Fatalf("invalid config should be rejected, got %v", err)
	}
}

func TestAcceptanceBlobStoreValidatesPutInputs(t *testing.T) {
	cfg := acceptanceSingleAttemptConfig()
	cfg.Checksum.Algorithms = []ChecksumAlgorithm{ChecksumSHA256}
	cfg.Policy.Permission.DeniedPrefixes = []string{"blocked/"}
	store, err := NewBlobStore(cfg, NewInMemoryAdapter(), Hooks{})
	if err != nil {
		t.Fatalf("NewBlobStore: %v", err)
	}

	tests := []struct {
		name string
		key  string
		opts PutOptions
	}{
		{name: "rejects empty key", key: ""},
		{name: "rejects invalid metadata", key: "tenant/key", opts: PutOptions{Metadata: map[string]string{"": "value"}}},
		{name: "rejects unsupported checksum", key: "tenant/key", opts: PutOptions{ChecksumAlgo: ChecksumMD5}},
		{name: "rejects denied prefix", key: "blocked/key"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := store.Put(context.Background(), Key(tt.key), strings.NewReader("body"), tt.opts); err == nil {
				t.Fatalf("expected validation error")
			}
		})
	}
}

func TestAcceptanceBlobStoreReturnsAdapterErrors(t *testing.T) {
	ctx := context.Background()
	adapterErr := newError(ErrorKindUnavailable, "adapter", "down")
	storeWith := func(a *acceptanceScriptedAdapter) BlobStore {
		t.Helper()
		store, err := NewBlobStore(acceptanceSingleAttemptConfig(), a, Hooks{})
		if err != nil {
			t.Fatalf("NewBlobStore: %v", err)
		}
		return store
	}

	if _, err := storeWith(&acceptanceScriptedAdapter{putErr: adapterErr}).Put(ctx, Key("tenant/key"), strings.NewReader("body"), PutOptions{}); err == nil || errorKind(err) != ErrorKindUnavailable {
		t.Fatalf("Put should return adapter error, got %v", err)
	}
	if reader, err := storeWith(&acceptanceScriptedAdapter{getErr: adapterErr}).Get(ctx, Key("tenant/key"), GetOptions{}); err == nil || reader != nil || errorKind(err) != ErrorKindUnavailable {
		t.Fatalf("Get should return adapter error, reader=%v err=%v", reader, err)
	}
	if _, err := storeWith(&acceptanceScriptedAdapter{copyErr: adapterErr}).Copy(ctx, Key("tenant/source"), Key("tenant/target"), CopyOptions{}); err == nil || errorKind(err) != ErrorKindUnavailable {
		t.Fatalf("Copy should return adapter error, got %v", err)
	}
	if _, err := storeWith(&acceptanceScriptedAdapter{headErr: adapterErr}).Head(ctx, Key("tenant/key")); err == nil || errorKind(err) != ErrorKindUnavailable {
		t.Fatalf("Head should return adapter error, got %v", err)
	}
	if _, err := storeWith(&acceptanceScriptedAdapter{listErr: adapterErr}).List(ctx, Prefix(""), ListOptions{}); err == nil || errorKind(err) != ErrorKindUnavailable {
		t.Fatalf("List should return adapter error, got %v", err)
	}
}

func TestAcceptanceBlobStoreDeleteAndExistsErrorMappings(t *testing.T) {
	ctx := context.Background()
	notFound := newError(ErrorKindNotFound, "adapter", "missing")
	store, err := NewBlobStore(acceptanceSingleAttemptConfig(), &acceptanceScriptedAdapter{deleteErr: notFound}, Hooks{})
	if err != nil {
		t.Fatalf("NewBlobStore: %v", err)
	}
	if err := store.Delete(ctx, Key("tenant/key"), DeleteOptions{}); err != nil {
		t.Fatalf("non-strict delete maps not found to nil: %v", err)
	}
	if err := store.Delete(ctx, Key("tenant/key"), DeleteOptions{StrictNotFound: true}); err == nil || errorKind(err) != ErrorKindNotFound {
		t.Fatalf("strict delete should preserve not found, got %v", err)
	}

	existsStore, err := NewBlobStore(acceptanceSingleAttemptConfig(), &acceptanceScriptedAdapter{headErr: notFound}, Hooks{})
	if err != nil {
		t.Fatalf("NewBlobStore exists not found: %v", err)
	}
	exists, err := existsStore.Exists(ctx, Key("tenant/key"))
	if err != nil || exists {
		t.Fatalf("not found should map to exists=false nil, exists=%v err=%v", exists, err)
	}

	adapterErr := newError(ErrorKindUnavailable, "adapter", "down")
	existsStore, err = NewBlobStore(acceptanceSingleAttemptConfig(), &acceptanceScriptedAdapter{headErr: adapterErr}, Hooks{})
	if err != nil {
		t.Fatalf("NewBlobStore exists error: %v", err)
	}
	exists, err = existsStore.Exists(ctx, Key("tenant/key"))
	if err == nil || exists || errorKind(err) != ErrorKindUnavailable {
		t.Fatalf("adapter error should map to exists=false err, exists=%v err=%v", exists, err)
	}

	existsStore, err = NewBlobStore(acceptanceSingleAttemptConfig(), &acceptanceScriptedAdapter{}, Hooks{})
	if err != nil {
		t.Fatalf("NewBlobStore exists success: %v", err)
	}
	exists, err = existsStore.Exists(ctx, Key("tenant/key"))
	if err != nil || !exists {
		t.Fatalf("successful head should map to exists=true nil, exists=%v err=%v", exists, err)
	}
}

func TestAcceptanceBlobStoreHealthListAndCloseBranches(t *testing.T) {
	ctx := context.Background()
	for _, tt := range []struct {
		name   string
		err    error
		status string
	}{
		{name: "connection failure is unreachable", err: newError(ErrorKindConnection, "health", "network"), status: "unreachable"},
		{name: "auth failure is config error", err: newError(ErrorKindAuth, "health", "auth"), status: "config_error"},
		{name: "other failure is degraded", err: newError(ErrorKindInternal, "health", "internal"), status: "degraded"},
		{name: "success is ready", status: "scripted"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store, err := NewBlobStore(acceptanceSingleAttemptConfig(), &acceptanceScriptedAdapter{healthErr: tt.err}, Hooks{})
			if err != nil {
				t.Fatalf("NewBlobStore: %v", err)
			}
			health := store.Health(ctx)
			if health.ProviderStatus != tt.status {
				t.Fatalf("unexpected status: got %q want %q", health.ProviderStatus, tt.status)
			}
		})
	}

	adapter := &acceptanceScriptedAdapter{}
	store, err := NewBlobStore(acceptanceSingleAttemptConfig(), adapter, Hooks{})
	if err != nil {
		t.Fatalf("NewBlobStore list close: %v", err)
	}
	if _, err := store.List(ctx, Prefix(""), ListOptions{}); err != nil || adapter.listMax != 1000 {
		t.Fatalf("default list max mismatch max=%d err=%v", adapter.listMax, err)
	}
	if _, err := store.List(ctx, Prefix(""), ListOptions{MaxKeys: 2001}); err != nil || adapter.listMax != 1000 {
		t.Fatalf("oversized list max mismatch max=%d err=%v", adapter.listMax, err)
	}
	if _, err := store.List(ctx, Prefix(""), ListOptions{MaxKeys: 3}); err != nil || adapter.listMax != 3 {
		t.Fatalf("explicit list max mismatch max=%d err=%v", adapter.listMax, err)
	}
	if err := store.Close(ctx); err != nil {
		t.Fatalf("Close first call: %v", err)
	}
	if err := store.Close(ctx); err != nil {
		t.Fatalf("Close second call: %v", err)
	}
	if adapter.closeCalls != 1 {
		t.Fatalf("adapter close should be called once, got %d", adapter.closeCalls)
	}
	if health := store.Health(ctx); health.ProviderStatus != "closed" {
		t.Fatalf("closed store health status = %q", health.ProviderStatus)
	}
	if _, err := store.Put(ctx, Key("tenant/key"), strings.NewReader("body"), PutOptions{}); err == nil || errorKind(err) != ErrorKindClosed {
		t.Fatalf("closed Put should fail, got %v", err)
	}
}

func TestAcceptancePresignValidatesInputsAndAdapterErrors(t *testing.T) {
	cfg := acceptanceSingleAttemptConfig()
	cfg.Policy.Permission.DeniedPrefixes = []string{"blocked/"}
	store, err := NewBlobStore(cfg, &acceptanceScriptedAdapter{
		presigned: PresignedURL{URL: "https://example.invalid/object", Method: "GET", ExpiresAt: time.Now().Add(time.Minute).Unix()},
	}, Hooks{})
	if err != nil {
		t.Fatalf("NewBlobStore: %v", err)
	}
	ctx := context.Background()

	tests := []struct {
		name string
		key  string
		op   PresignOperation
		ttl  time.Duration
	}{
		{name: "rejects invalid key", key: "", op: PresignGet, ttl: time.Minute},
		{name: "rejects unsupported operation", key: "tenant/key", op: PresignOperation("TRACE"), ttl: time.Minute},
		{name: "rejects non-positive ttl", key: "tenant/key", op: PresignGet, ttl: 0},
		{name: "rejects ttl above maximum", key: "tenant/key", op: PresignGet, ttl: 16 * time.Minute},
		{name: "rejects denied key", key: "blocked/key", op: PresignGet, ttl: time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := store.Presign(ctx, Key(tt.key), tt.op, PresignOptions{TTL: int64(tt.ttl / time.Second)}); err == nil {
				t.Fatalf("expected presign validation error")
			}
		})
	}

	adapterErr := newError(ErrorKindUnavailable, "presign", "down")
	store, err = NewBlobStore(cfg, &acceptanceScriptedAdapter{presignErr: adapterErr}, Hooks{})
	if err != nil {
		t.Fatalf("NewBlobStore adapter error: %v", err)
	}
	if _, err := store.Presign(ctx, Key("tenant/key"), PresignGet, PresignOptions{TTL: int64(time.Minute / time.Second)}); err == nil || errorKind(err) != ErrorKindUnavailable {
		t.Fatalf("Presign should return adapter error, got %v", err)
	}
}

func TestAcceptanceMultipartSessionValidatesLifecycleBranches(t *testing.T) {
	ctx := context.Background()
	cfg := acceptanceSingleAttemptConfig()
	cfg.Multipart.MaxParts = 2
	adapter := &acceptanceScriptedAdapter{
		uploadID:   UploadID("upload-1"),
		uploadPart: PartETag{PartNumber: 1, ETag: "etag-1", Size: 4},
	}
	store, err := NewBlobStore(cfg, adapter, Hooks{})
	if err != nil {
		t.Fatalf("NewBlobStore: %v", err)
	}
	session, err := store.Multipart(ctx)
	if err != nil {
		t.Fatalf("Multipart: %v", err)
	}
	if _, err := session.Initiate(ctx, Key(""), PutOptions{}); err == nil {
		t.Fatalf("Initiate should reject invalid key")
	}
	if _, err := session.Initiate(ctx, Key("tenant/key"), PutOptions{Metadata: map[string]string{"": "value"}}); err == nil {
		t.Fatalf("Initiate should reject invalid metadata")
	}

	adapter.initErr = newError(ErrorKindUnavailable, "init", "down")
	if _, err := session.Initiate(ctx, Key("tenant/key"), PutOptions{}); err == nil || errorKind(err) != ErrorKindUnavailable {
		t.Fatalf("Initiate should return adapter error, got %v", err)
	}
	adapter.initErr = nil
	uploadID, err := session.Initiate(ctx, Key("tenant/key"), PutOptions{})
	if err != nil || uploadID != "upload-1" {
		t.Fatalf("Initiate success uploadID=%q err=%v", uploadID, err)
	}

	if _, err := session.UploadPart(ctx, uploadID, 0, strings.NewReader("body"), 4); err == nil {
		t.Fatalf("UploadPart should reject part number below one")
	}
	if _, err := session.UploadPart(ctx, uploadID, 3, strings.NewReader("body"), 4); err == nil {
		t.Fatalf("UploadPart should reject part number above max")
	}
	adapter.uploadErr = newError(ErrorKindUnavailable, "upload", "down")
	if _, err := session.UploadPart(ctx, uploadID, 1, strings.NewReader("body"), 4); err == nil || errorKind(err) != ErrorKindUnavailable {
		t.Fatalf("UploadPart should return adapter error, got %v", err)
	}
	adapter.uploadErr = nil
	adapter.uploadPart = PartETag{PartNumber: 2, ETag: "etag-2", Size: 4}
	if _, err := session.UploadPart(ctx, uploadID, 1, strings.NewReader("body"), 4); err == nil {
		t.Fatalf("UploadPart should reject adapter part number mismatch")
	}
	adapter.uploadPart = PartETag{PartNumber: 1}
	if _, err := session.UploadPart(ctx, uploadID, 1, strings.NewReader("body"), 4); err == nil {
		t.Fatalf("UploadPart should reject empty ETag")
	}
	adapter.uploadPart = PartETag{PartNumber: 1, ETag: "etag-1", Size: 4}
	if _, err := session.UploadPart(ctx, uploadID, 1, strings.NewReader("body"), 4); err != nil {
		t.Fatalf("UploadPart success: %v", err)
	}

	adapter.listPartsErr = newError(ErrorKindUnavailable, "listparts", "down")
	if _, err := session.ListParts(ctx, uploadID); err == nil || errorKind(err) != ErrorKindUnavailable {
		t.Fatalf("ListParts should return adapter error, got %v", err)
	}
	adapter.listPartsErr = nil
	if parts, err := session.ListParts(ctx, uploadID); err != nil || len(parts) != 1 {
		t.Fatalf("ListParts success parts=%#v err=%v", parts, err)
	}

	if _, err := session.Complete(ctx, uploadID, nil); err == nil {
		t.Fatalf("Complete should reject empty parts")
	}
	if _, err := session.Complete(ctx, uploadID, []PartETag{{PartNumber: 2, ETag: "etag-2"}}); err == nil {
		t.Fatalf("Complete should reject non-contiguous parts")
	}
	if _, err := session.Complete(ctx, uploadID, []PartETag{{PartNumber: 1}}); err == nil {
		t.Fatalf("Complete should reject empty ETag")
	}
	if _, err := session.Complete(ctx, uploadID, []PartETag{{PartNumber: 1, ETag: "etag-1"}, {PartNumber: 2, ETag: "etag-2"}, {PartNumber: 3, ETag: "etag-3"}}); err == nil {
		t.Fatalf("Complete should reject too many parts")
	}
	adapter.completeErr = newError(ErrorKindUnavailable, "complete", "down")
	if _, err := session.Complete(ctx, uploadID, []PartETag{{PartNumber: 1, ETag: "etag-1"}}); err == nil || errorKind(err) != ErrorKindUnavailable {
		t.Fatalf("Complete should return adapter error, got %v", err)
	}
	adapter.completeErr = nil
	adapter.completeInfo = ObjectInfo{Key: Key("tenant/key")}
	if _, err := session.Complete(ctx, uploadID, []PartETag{{PartNumber: 1, ETag: "etag-1"}}); err != nil {
		t.Fatalf("Complete retry after adapter error should succeed: %v", err)
	}
	if _, err := session.Complete(ctx, uploadID, []PartETag{{PartNumber: 1, ETag: "etag-1"}}); err == nil || errorKind(err) != ErrorKindConflict {
		t.Fatalf("second Complete should conflict, got %v", err)
	}

	adapter.abortErr = newError(ErrorKindNotFound, "abort", "missing")
	if err := session.Abort(ctx, UploadID("missing")); err != nil {
		t.Fatalf("Abort should map not found to nil: %v", err)
	}
	adapter.abortErr = newError(ErrorKindUnavailable, "abort", "down")
	if err := session.Abort(ctx, UploadID("other")); err == nil || errorKind(err) != ErrorKindUnavailable {
		t.Fatalf("Abort should return adapter error, got %v", err)
	}
}

func TestAcceptanceDirectHelperBranches(t *testing.T) {
	if Key("tenant/key").String() != "tenant/key" {
		t.Fatalf("Key.String returned unexpected value")
	}
	if _, err := NewKey(strings.Repeat("a", 1025)); err == nil {
		t.Fatalf("NewKey should reject keys over max length")
	}
	if _, err := NewKey("tenant//key"); err == nil {
		t.Fatalf("NewKey should reject empty path segment")
	}

	metadata := make(map[string]string, MaxMetadataKeys+1)
	for i := 0; i <= MaxMetadataKeys; i++ {
		metadata[Key("k"+strings.Repeat("x", i)).String()] = "v"
	}
	if err := validateMetadata(metadata); err == nil {
		t.Fatalf("validateMetadata should reject too many keys")
	}
	if err := validateMetadata(map[string]string{"key": strings.Repeat("v", MaxMetadataValueLen+1)}); err == nil {
		t.Fatalf("validateMetadata should reject too long value")
	}

	if err := validateChecksumAlgo("", nil); err == nil || errorKind(err) != ErrorKindConfig {
		t.Fatalf("empty checksum algo should be rejected as unsupported, got %v", err)
	}
	if err := validateChecksumAlgo(ChecksumMD5, nil); err != nil {
		t.Fatalf("nil allowed checksum list should allow md5: %v", err)
	}
	if err := validateChecksumAlgo(ChecksumMD5, []ChecksumAlgorithm{ChecksumSHA256}); err == nil {
		t.Fatalf("explicit checksum allow-list should reject md5")
	}
	if newHasher(ChecksumMD5) == nil || newHasher(ChecksumCRC32) == nil || newHasher(ChecksumAlgorithm("sha1")) != nil {
		t.Fatalf("unexpected hasher support")
	}

	if err := validateRetentionDelete(RetentionPolicy{}, ObjectInfo{}, time.Now()); err != nil {
		t.Fatalf("empty retention should allow delete: %v", err)
	}
	if err := validateRetentionDelete(RetentionPolicy{Mode: RetentionModeGovernance, MaxDays: 0}, ObjectInfo{CreatedAt: time.Now()}, time.Now()); err != nil {
		t.Fatalf("zero retention window should allow delete: %v", err)
	}
	if err := validateRetentionDelete(RetentionPolicy{Mode: RetentionModeGovernance, MaxDays: 7}, ObjectInfo{}, time.Now()); err != nil {
		t.Fatalf("zero creation time should allow delete: %v", err)
	}
	if err := validateRetentionDelete(RetentionPolicy{Mode: RetentionModeGovernance, MaxDays: 7}, ObjectInfo{CreatedAt: time.Now()}, time.Now()); err == nil {
		t.Fatalf("young retained object should reject delete")
	}

	if gaugeForStatus("ready") != 1 || gaugeForStatus("ok") != 1 || gaugeForStatus("degraded") != 0.5 || gaugeForStatus("other") != 0 {
		t.Fatalf("unexpected health gauge mapping")
	}
	if err := ctxErrCheck(context.Background()); err != nil {
		t.Fatalf("active context should not fail: %v", err)
	}
	deadlineCtx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	if err := ctxErrCheck(deadlineCtx); err == nil || errorKind(err) != ErrorKindTimeout {
		t.Fatalf("deadline context should map to timeout, got %v", err)
	}

	var nilErr *Error
	if nilErr.Error() != "<nil>" || nilErr.IsKind(ErrorKindNotFound) {
		t.Fatalf("nil Error receiver helpers returned unexpected values")
	}
	if !wrapError(ErrorKindNotFound, "head", "missing", ErrNotFound).Is(ErrNotFound) {
		t.Fatalf("Error.Is should match wrapped sentinel")
	}
	if newError(ErrorKindInternal, "", "message").Error() != "ossx: message" {
		t.Fatalf("Error.Error without op returned unexpected string")
	}
	if classifyError(nil) != retryClassNonRetryable || classifyError(errors.New("plain")) != retryClassNonRetryable {
		t.Fatalf("classifyError fallback mismatch")
	}
	if kindToClass(ErrorKindCanceled) != retryClassFatal {
		t.Fatalf("kindToClass fallback mismatch")
	}
	if isRetryable(nil) || isErrCircuitOpen(nil) || isErrCircuitOpen(errors.New("plain")) {
		t.Fatalf("error helper fallback mismatch")
	}

	if boolEnvDefault("OSSX_ACCEPTANCE_BOOL_UNSET", true) != true {
		t.Fatalf("boolEnvDefault should use default for unset env")
	}
	t.Setenv("OSSX_ACCEPTANCE_BOOL_INVALID", "not-bool")
	if boolEnvDefault("OSSX_ACCEPTANCE_BOOL_INVALID", true) != true {
		t.Fatalf("boolEnvDefault should use default for invalid env")
	}
	if durationEnvDefault("OSSX_ACCEPTANCE_DURATION_UNSET", time.Second) != time.Second {
		t.Fatalf("durationEnvDefault should use default for unset env")
	}
	t.Setenv("OSSX_ACCEPTANCE_DURATION_INVALID", "bad")
	if durationEnvDefault("OSSX_ACCEPTANCE_DURATION_INVALID", time.Second) != time.Second {
		t.Fatalf("durationEnvDefault should use default for invalid env")
	}
	if int64EnvDefault("OSSX_ACCEPTANCE_INT_UNSET", 42) != 42 {
		t.Fatalf("int64EnvDefault should use default for unset env")
	}
	t.Setenv("OSSX_ACCEPTANCE_INT_INVALID", "bad")
	if int64EnvDefault("OSSX_ACCEPTANCE_INT_INVALID", 42) != 42 {
		t.Fatalf("int64EnvDefault should use default for invalid env")
	}

	cfg := validConfig()
	cfg.Retry = RetryConfig{MaxAttempts: 1}
	cfg.Multipart.MaxParts = 5
	cfg.Multipart.MaxConcurrency = 2
	cfg.Presign.MaxTTL = time.Minute
	defaulted := cfg.withDefaults()
	if defaulted.Retry.MaxAttempts != 1 || defaulted.Multipart.MaxParts != 5 || defaulted.Multipart.MaxConcurrency != 2 || defaulted.Presign.MaxTTL != time.Minute {
		t.Fatalf("withDefaults should preserve explicit values: %#v", defaulted)
	}

	policy := retryPolicy{InitialWait: 10 * time.Millisecond, MaxWait: 15 * time.Millisecond, Multiplier: 10}
	if got := policy.delay(3); got != 15*time.Millisecond {
		t.Fatalf("retry delay should cap at max wait, got %v", got)
	}
	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := (retryPolicy{MaxAttempts: 1}).withRetry(cancelCtx, "retry", func(context.Context) error { return nil }); err == nil || errorKind(err) != ErrorKindCanceled {
		t.Fatalf("withRetry should fail before attempt on cancelled context, got %v", err)
	}
	cancelDuringBackoff, cancelBackoff := context.WithCancel(context.Background())
	err := (retryPolicy{MaxAttempts: 2, InitialWait: time.Minute, MaxWait: time.Minute, Multiplier: 1}).withRetry(cancelDuringBackoff, "retry", func(context.Context) error {
		cancelBackoff()
		return newError(ErrorKindUnavailable, "retry", "temporary")
	})
	if err == nil || errorKind(err) != ErrorKindCanceled {
		t.Fatalf("withRetry should map cancelled backoff, got %v", err)
	}

	breaker := newCircuitBreaker(0, 0)
	breaker.state = breakerOpen
	breaker.openedAt = time.Now()
	if breaker.allow() {
		t.Fatalf("open breaker before cooldown should reject")
	}
	breaker.openedAt = time.Now().Add(-2 * breaker.cooldown)
	if !breaker.allow() {
		t.Fatalf("open breaker after cooldown should half-open")
	}
	breaker.state = breakerState(99)
	if !breaker.allow() {
		t.Fatalf("unknown breaker state should allow")
	}
}

type acceptanceErrReader struct{}

func (acceptanceErrReader) Read([]byte) (int, error) {
	return 0, errors.New("read failed")
}

type acceptanceErrContext struct {
	context.Context
	err error
}

func (c acceptanceErrContext) Err() error {
	return c.err
}

func TestAcceptanceConcreteNoopHooksAcceptCalls(t *testing.T) {
	ctx := context.Background()

	span := noopSpan{}
	span.SetField(Field{Key: "scope", Value: "tenant"})
	span.AddEvent("event", Field{Key: "attempt", Value: 1})
	span.End(Field{Key: "result", Value: "ok"})

	metrics := NoopMetrics{}
	labels := map[string]string{"provider": "memory"}
	metrics.IncCounter("counter", labels)
	metrics.AddCounter("counter", 2, labels)
	metrics.ObserveHistogram("histogram", 1.5, labels)
	metrics.SetGauge("gauge", 3, labels)

	logger := NoopLogger{}
	logger.Debug(ctx, "debug", Field{Key: "k", Value: "v"})
	logger.Info(ctx, "info", Field{Key: "k", Value: "v"})
	logger.Warn(ctx, "warn", Field{Key: "k", Value: "v"})
	logger.Error(ctx, "error", Field{Key: "k", Value: "v"})
}

func TestAcceptanceConfigDefaultsAndEnvValidationBranches(t *testing.T) {
	cfg := Config{Endpoint: "endpoint", Region: "region", Bucket: "bucket", AccessKey: "access", SecretKey: "secret"}
	defaulted := cfg.withDefaults()
	if defaulted.Retry.MaxAttempts != 3 || defaulted.Retry.InitialWait != 100*time.Millisecond || defaulted.Multipart.MaxParts != 10000 || defaulted.Multipart.MaxConcurrency != 4 || defaulted.Presign.MaxTTL != MaxAllowedPresignTTL {
		t.Fatalf("unexpected defaulted config: %#v", defaulted)
	}

	t.Setenv(envPrefix+"ENDPOINT", "endpoint")
	t.Setenv(envPrefix+"REGION", "region")
	t.Setenv(envPrefix+"BUCKET", "bucket")
	t.Setenv(envPrefix+"ACCESS_KEY", "access")
	t.Setenv(envPrefix+"SECRET_KEY", "secret")
	t.Setenv(envPrefix+"PRESIGN_MAX_TTL", (MaxAllowedPresignTTL + time.Second).String())
	if _, err := ConfigFromEnv(); err == nil || errorKind(err) != ErrorKindConfig {
		t.Fatalf("expected config error for excessive env presign TTL, got %v", err)
	}

	t.Setenv(envPrefix+"BOOL_INVALID", "not-bool")
	if boolEnvDefault("BOOL_INVALID", true) != true {
		t.Fatal("boolEnvDefault should use default for invalid prefixed env")
	}
	t.Setenv(envPrefix+"INT_INVALID", "bad")
	if int64EnvDefault("INT_INVALID", 42) != 42 {
		t.Fatal("int64EnvDefault should use default for invalid prefixed env")
	}
}

func TestAcceptanceBlobStoreSuccessAndPolicyBranches(t *testing.T) {
	ctx := context.Background()
	key := Key("tenant/key")
	sum := computeChecksum(ChecksumSHA256, []byte("body"))
	adapter := &acceptanceScriptedAdapter{
		getInfo: ObjectInfo{
			Key:          key,
			ChecksumAlgo: ChecksumSHA256,
			ChecksumHex:  sum,
		},
		headInfo: ObjectInfo{Key: key},
		copyInfo: ObjectInfo{Key: "tenant/copy"},
		listPage: ListPage{Items: []ObjectInfo{{Key: key}}},
		presigned: PresignedURL{
			URL:       "https://example.invalid/signed",
			Method:    "GET",
			ExpiresAt: time.Now().Add(time.Minute).Unix(),
		},
	}
	cfg := acceptanceSingleAttemptConfig()
	cfg.Presign.AllowedOperations = []PresignOperation{PresignGet}
	store, err := NewBlobStore(cfg, adapter, Hooks{})
	if err != nil {
		t.Fatalf("NewBlobStore: %v", err)
	}

	reader, err := store.Get(ctx, key, GetOptions{VerifyChecksum: true})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read verified object: %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("close verified object: %v", err)
	}
	if string(data) != "body" || !reader.ChecksumVerified {
		t.Fatalf("unexpected verified object data=%q verified=%v", string(data), reader.ChecksumVerified)
	}
	if _, err := store.Head(ctx, key); err != nil {
		t.Fatalf("Head: %v", err)
	}
	if _, err := store.Copy(ctx, key, "tenant/copy", CopyOptions{}); err != nil {
		t.Fatalf("Copy: %v", err)
	}
	page, err := store.List(ctx, "tenant/", ListOptions{MaxKeys: 2001})
	if err != nil || len(page.Items) != 1 || adapter.listMax != 1000 {
		t.Fatalf("List should cap max keys, page=%#v listMax=%d err=%v", page, adapter.listMax, err)
	}
	if _, err := store.Presign(ctx, key, PresignGet, PresignOptions{TTL: 60}); err != nil {
		t.Fatalf("Presign: %v", err)
	}

	retainedAdapter := &acceptanceScriptedAdapter{
		headInfo: ObjectInfo{Key: key, CreatedAt: time.Now()},
	}
	retainedCfg := acceptanceSingleAttemptConfig()
	retainedCfg.Policy.Retention = RetentionPolicy{Mode: RetentionModeGovernance, MaxDays: 7}
	retainedStore, err := NewBlobStore(retainedCfg, retainedAdapter, Hooks{})
	if err != nil {
		t.Fatalf("NewBlobStore retained: %v", err)
	}
	if err := retainedStore.Delete(ctx, key, DeleteOptions{}); err == nil || errorKind(err) != ErrorKindValidation {
		t.Fatalf("expected retention validation, got %v", err)
	}
}

func TestAcceptanceBlobStoreClosedAndContextBranches(t *testing.T) {
	ctx := context.Background()
	store, err := NewBlobStore(acceptanceSingleAttemptConfig(), NewInMemoryAdapter(), Hooks{})
	if err != nil {
		t.Fatalf("NewBlobStore: %v", err)
	}
	if err := store.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}

	key := Key("tenant/key")
	if _, err := store.Get(ctx, key, GetOptions{}); err != ErrClosed {
		t.Fatalf("Get on closed store got %v", err)
	}
	if err := store.Delete(ctx, key, DeleteOptions{}); err != ErrClosed {
		t.Fatalf("Delete on closed store got %v", err)
	}
	if _, err := store.Copy(ctx, key, "tenant/copy", CopyOptions{}); err != ErrClosed {
		t.Fatalf("Copy on closed store got %v", err)
	}
	if _, err := store.Head(ctx, key); err != ErrClosed {
		t.Fatalf("Head on closed store got %v", err)
	}
	if _, err := store.List(ctx, "tenant/", ListOptions{}); err != ErrClosed {
		t.Fatalf("List on closed store got %v", err)
	}
	if _, err := store.Multipart(ctx); err != ErrClosed {
		t.Fatalf("Multipart on closed store got %v", err)
	}
	if _, err := store.Presign(ctx, key, PresignGet, PresignOptions{TTL: 1}); err != ErrClosed {
		t.Fatalf("Presign on closed store got %v", err)
	}

	customCtx := acceptanceErrContext{Context: context.Background(), err: errors.New("custom context")}
	if err := ctxErrCheck(customCtx); err == nil || errorKind(err) != ErrorKindCanceled || !strings.Contains(err.Error(), "context error") {
		t.Fatalf("expected generic context cancellation, got %v", err)
	}
}

func TestAcceptanceMultipartSessionClosedAndContextBranches(t *testing.T) {
	ctx := context.Background()
	adapter := &acceptanceScriptedAdapter{
		uploadID:   "upload-1",
		uploadPart: PartETag{PartNumber: 1, ETag: "etag", Size: 4},
	}
	store, err := NewBlobStore(acceptanceSingleAttemptConfig(), adapter, Hooks{})
	if err != nil {
		t.Fatalf("NewBlobStore: %v", err)
	}
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := store.Multipart(cancelCtx); err == nil || errorKind(err) != ErrorKindCanceled {
		t.Fatalf("Multipart with cancelled context got %v", err)
	}

	session, err := store.Multipart(ctx)
	if err != nil {
		t.Fatalf("Multipart: %v", err)
	}
	if _, err := session.Initiate(cancelCtx, "tenant/key", PutOptions{}); err == nil || errorKind(err) != ErrorKindCanceled {
		t.Fatalf("Initiate with cancelled context got %v", err)
	}
	if _, err := session.UploadPart(cancelCtx, "upload-1", 1, strings.NewReader("part"), 4); err == nil || errorKind(err) != ErrorKindCanceled {
		t.Fatalf("UploadPart with cancelled context got %v", err)
	}
	if _, err := session.ListParts(cancelCtx, "upload-1"); err == nil || errorKind(err) != ErrorKindCanceled {
		t.Fatalf("ListParts with cancelled context got %v", err)
	}
	if _, err := session.Complete(cancelCtx, "upload-1", []PartETag{{PartNumber: 1, ETag: "etag", Size: 4}}); err == nil || errorKind(err) != ErrorKindCanceled {
		t.Fatalf("Complete with cancelled context got %v", err)
	}
	if err := session.Abort(cancelCtx, "upload-1"); err == nil || errorKind(err) != ErrorKindCanceled {
		t.Fatalf("Abort with cancelled context got %v", err)
	}

	if err := store.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := session.Initiate(ctx, "tenant/key", PutOptions{}); err != ErrClosed {
		t.Fatalf("Initiate on closed store got %v", err)
	}
	if _, err := session.UploadPart(ctx, "upload-1", 1, strings.NewReader("part"), 4); err != ErrClosed {
		t.Fatalf("UploadPart on closed store got %v", err)
	}
	if _, err := session.ListParts(ctx, "upload-1"); err != ErrClosed {
		t.Fatalf("ListParts on closed store got %v", err)
	}
	if _, err := session.Complete(ctx, "upload-1", []PartETag{{PartNumber: 1, ETag: "etag", Size: 4}}); err != ErrClosed {
		t.Fatalf("Complete on closed store got %v", err)
	}
	if err := session.Abort(ctx, "upload-1"); err != ErrClosed {
		t.Fatalf("Abort on closed store got %v", err)
	}
}

func TestAcceptanceInMemoryAdapterRemainingBranches(t *testing.T) {
	ctx := context.Background()
	adapter := NewInMemoryAdapter()

	if _, err := adapter.PutObject(ctx, "tenant/read-error", acceptanceErrReader{}, -1, PutAdapterOptions{}); err == nil || errorKind(err) != ErrorKindUnavailable {
		t.Fatalf("expected read error to become unavailable, got %v", err)
	}
	if _, _, err := adapter.GetObject(ctx, "tenant/missing"); err != ErrNotFound {
		t.Fatalf("GetObject missing got %v", err)
	}
	if _, err := adapter.HeadObject(ctx, "tenant/missing"); err != ErrNotFound {
		t.Fatalf("HeadObject missing got %v", err)
	}
	if err := adapter.DeleteObject(ctx, "tenant/missing", false); err != nil {
		t.Fatalf("DeleteObject missing non-strict should be nil, got %v", err)
	}
	if err := adapter.DeleteObject(ctx, "tenant/missing", true); err != ErrNotFound {
		t.Fatalf("DeleteObject missing strict got %v", err)
	}
	if _, err := adapter.CopyObject(ctx, "tenant/missing", "tenant/copy", CopyAdapterOptions{}); err != ErrNotFound {
		t.Fatalf("CopyObject missing got %v", err)
	}

	if _, err := adapter.PutObject(ctx, "tenant/a", strings.NewReader("aaa"), -1, PutAdapterOptions{
		ContentType:  "text/plain",
		Metadata:     map[string]string{"m": "v"},
		Tags:         map[string]string{"tag": "v"},
		ChecksumAlgo: ChecksumSHA256,
	}); err != nil {
		t.Fatalf("PutObject tenant/a: %v", err)
	}
	if _, err := adapter.PutObject(ctx, "tenant/b", strings.NewReader("bbb"), -1, PutAdapterOptions{}); err != nil {
		t.Fatalf("PutObject tenant/b: %v", err)
	}
	if _, err := adapter.PutObject(ctx, "other/c", strings.NewReader("ccc"), -1, PutAdapterOptions{}); err != nil {
		t.Fatalf("PutObject other/c: %v", err)
	}
	copied, err := adapter.CopyObject(ctx, "tenant/a", "tenant/copied", CopyAdapterOptions{})
	if err != nil {
		t.Fatalf("CopyObject inherited fields: %v", err)
	}
	if copied.ContentType != "text/plain" || copied.Metadata["m"] != "v" {
		t.Fatalf("CopyObject should inherit content type and metadata, got %#v", copied)
	}
	page, err := adapter.ListObjects(ctx, "tenant/", 1, "")
	if err != nil || len(page.Items) != 1 || !page.IsTruncated || page.NextContinuation == "" {
		t.Fatalf("ListObjects first page = %#v err=%v", page, err)
	}
	nextPage, err := adapter.ListObjects(ctx, "tenant/", 10, page.NextContinuation)
	if err != nil || len(nextPage.Items) == 0 || nextPage.IsTruncated {
		t.Fatalf("ListObjects next page = %#v err=%v", nextPage, err)
	}
	emptyPage, err := adapter.ListObjects(ctx, "none/", 0, "")
	if err != nil || len(emptyPage.Items) != 0 {
		t.Fatalf("ListObjects empty page = %#v err=%v", emptyPage, err)
	}

	uploadID, err := adapter.InitiateMultipart(ctx, "tenant/multipart", PutAdapterOptions{})
	if err != nil {
		t.Fatalf("InitiateMultipart: %v", err)
	}
	if _, err := adapter.UploadPart(ctx, "missing", 1, strings.NewReader("part"), -1); err == nil || errorKind(err) != ErrorKindNotFound {
		t.Fatalf("UploadPart missing got %v", err)
	}
	if _, err := adapter.UploadPart(ctx, uploadID, 1, acceptanceErrReader{}, -1); err == nil || errorKind(err) != ErrorKindUnavailable {
		t.Fatalf("UploadPart read error got %v", err)
	}
	part, err := adapter.UploadPart(ctx, uploadID, 1, strings.NewReader("part"), -1)
	if err != nil {
		t.Fatalf("UploadPart: %v", err)
	}
	parts, err := adapter.ListParts(ctx, uploadID)
	if err != nil || len(parts) != 1 || parts[0].ETag != part.ETag {
		t.Fatalf("ListParts = %#v err=%v", parts, err)
	}
	if _, err := adapter.ListParts(ctx, "missing"); err == nil || errorKind(err) != ErrorKindNotFound {
		t.Fatalf("ListParts missing got %v", err)
	}
	if _, err := adapter.CompleteMultipart(ctx, "missing", []PartETag{part}); err == nil || errorKind(err) != ErrorKindNotFound {
		t.Fatalf("CompleteMultipart missing got %v", err)
	}
	if _, err := adapter.CompleteMultipart(ctx, uploadID, []PartETag{part}); err != nil {
		t.Fatalf("CompleteMultipart: %v", err)
	}
	closedUploadID, err := adapter.InitiateMultipart(ctx, "tenant/closed", PutAdapterOptions{})
	if err != nil {
		t.Fatalf("InitiateMultipart before close: %v", err)
	}

	if err := adapter.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := adapter.Close(ctx); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if _, err := adapter.PutObject(ctx, "tenant/closed", strings.NewReader("x"), -1, PutAdapterOptions{}); err != ErrClosed {
		t.Fatalf("PutObject on closed adapter got %v", err)
	}
	if _, _, err := adapter.GetObject(ctx, "tenant/a"); err != ErrClosed {
		t.Fatalf("GetObject on closed adapter got %v", err)
	}
	if err := adapter.DeleteObject(ctx, "tenant/a", false); err != ErrClosed {
		t.Fatalf("DeleteObject on closed adapter got %v", err)
	}
	if _, err := adapter.CopyObject(ctx, "tenant/a", "tenant/copy2", CopyAdapterOptions{}); err != ErrClosed {
		t.Fatalf("CopyObject on closed adapter got %v", err)
	}
	if _, err := adapter.ListObjects(ctx, "tenant/", 1, ""); err != ErrClosed {
		t.Fatalf("ListObjects on closed adapter got %v", err)
	}
	if _, err := adapter.InitiateMultipart(ctx, "tenant/new", PutAdapterOptions{}); err != ErrClosed {
		t.Fatalf("InitiateMultipart on closed adapter got %v", err)
	}
	if _, err := adapter.UploadPart(ctx, closedUploadID, 1, strings.NewReader("part"), -1); err != ErrClosed {
		t.Fatalf("UploadPart on closed adapter got %v", err)
	}
	if _, err := adapter.ListParts(ctx, closedUploadID); err != ErrClosed {
		t.Fatalf("ListParts on closed adapter got %v", err)
	}
	if _, err := adapter.CompleteMultipart(ctx, closedUploadID, []PartETag{part}); err != ErrClosed {
		t.Fatalf("CompleteMultipart on closed adapter got %v", err)
	}
	if _, err := adapter.PresignURL(ctx, "tenant/a", PresignGet, 60, PresignAdapterOptions{}); err != ErrClosed {
		t.Fatalf("PresignURL on closed adapter got %v", err)
	}
	if err := adapter.Health(ctx); err != ErrClosed {
		t.Fatalf("Health on closed adapter got %v", err)
	}
}

func TestAcceptanceRemainingDirectHelperBranches(t *testing.T) {
	if _, err := NewKey(string([]byte{0xff, 'a'})); err == nil {
		t.Fatal("NewKey should reject invalid UTF-8")
	}
	for _, raw := range []string{"/tenant/key", "tenant/./key", "tenant/../key"} {
		if _, err := NewKey(raw); err == nil {
			t.Fatalf("NewKey should reject %q", raw)
		}
	}
	if err := validateChecksumAlgo(ChecksumSHA256, []ChecksumAlgorithm{ChecksumSHA256}); err != nil {
		t.Fatalf("validateChecksumAlgo should accept allowed checksum: %v", err)
	}
	if err := validateRetentionDelete(RetentionPolicy{Mode: RetentionModeGovernance, MaxDays: 7}, ObjectInfo{CreatedAt: time.Now().Add(-8 * 24 * time.Hour)}, time.Now()); err != nil {
		t.Fatalf("old retained object should be deletable: %v", err)
	}

	wrapped := wrapError(ErrorKindUnavailable, "get", "down", errors.New("provider"))
	if !strings.Contains(wrapped.Error(), "get: down: provider") {
		t.Fatalf("wrapped error string missing cause: %v", wrapped)
	}
	if newError(ErrorKindInternal, "op", "message").Is(errors.New("plain")) {
		t.Fatal("Error.Is should not match unrelated plain error")
	}
	if !isRetryable(newError(ErrorKindUnavailable, "op", "temporary")) {
		t.Fatal("unavailable error should be retryable")
	}
}

func TestAcceptanceBlobStoreRejectsCancelledContextBeforeReadOperations(t *testing.T) {
	ctx := context.Background()
	store, err := NewBlobStore(acceptanceSingleAttemptConfig(), NewInMemoryAdapter(), Hooks{})
	if err != nil {
		t.Fatalf("NewBlobStore: %v", err)
	}
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()

	key := Key("tenant/key")
	if _, err := store.Get(cancelCtx, key, GetOptions{}); err == nil || errorKind(err) != ErrorKindCanceled {
		t.Fatalf("Get with cancelled context got %v", err)
	}
	if err := store.Delete(cancelCtx, key, DeleteOptions{}); err == nil || errorKind(err) != ErrorKindCanceled {
		t.Fatalf("Delete with cancelled context got %v", err)
	}
	if _, err := store.Copy(cancelCtx, key, "tenant/copy", CopyOptions{}); err == nil || errorKind(err) != ErrorKindCanceled {
		t.Fatalf("Copy with cancelled context got %v", err)
	}
	if _, err := store.Head(cancelCtx, key); err == nil || errorKind(err) != ErrorKindCanceled {
		t.Fatalf("Head with cancelled context got %v", err)
	}
	if _, err := store.List(cancelCtx, "tenant/", ListOptions{}); err == nil || errorKind(err) != ErrorKindCanceled {
		t.Fatalf("List with cancelled context got %v", err)
	}
	if _, err := store.Presign(cancelCtx, key, PresignGet, PresignOptions{TTL: 1}); err == nil || errorKind(err) != ErrorKindCanceled {
		t.Fatalf("Presign with cancelled context got %v", err)
	}
}

func TestAcceptanceBlobStoreRejectsInvalidReadOperationKeys(t *testing.T) {
	ctx := context.Background()
	store, err := NewBlobStore(acceptanceSingleAttemptConfig(), NewInMemoryAdapter(), Hooks{})
	if err != nil {
		t.Fatalf("NewBlobStore: %v", err)
	}

	if _, err := store.Get(ctx, Key(""), GetOptions{}); err == nil || errorKind(err) != ErrorKindValidation {
		t.Fatalf("Get with invalid key got %v", err)
	}
	if err := store.Delete(ctx, Key(""), DeleteOptions{}); err == nil || errorKind(err) != ErrorKindValidation {
		t.Fatalf("Delete with invalid key got %v", err)
	}
	if _, err := store.Copy(ctx, Key(""), "tenant/copy", CopyOptions{}); err == nil || errorKind(err) != ErrorKindValidation {
		t.Fatalf("Copy with invalid source key got %v", err)
	}
	if _, err := store.Copy(ctx, "tenant/key", Key(""), CopyOptions{}); err == nil || errorKind(err) != ErrorKindValidation {
		t.Fatalf("Copy with invalid target key got %v", err)
	}
	if _, err := store.Head(ctx, Key("")); err == nil || errorKind(err) != ErrorKindValidation {
		t.Fatalf("Head with invalid key got %v", err)
	}
}

func TestAcceptanceBlobStoreRejectsDeniedCopyTarget(t *testing.T) {
	ctx := context.Background()
	cfg := acceptanceSingleAttemptConfig()
	cfg.Policy.Permission.DeniedPrefixes = []string{"blocked/"}
	store, err := NewBlobStore(cfg, NewInMemoryAdapter(), Hooks{})
	if err != nil {
		t.Fatalf("NewBlobStore: %v", err)
	}

	if _, err := store.Copy(ctx, "tenant/key", "blocked/key", CopyOptions{}); err == nil || errorKind(err) != ErrorKindValidation {
		t.Fatalf("Copy with denied target got %v", err)
	}
}

func TestAcceptancePresignUsesGlobalMaximumWhenPolicyMaxTTLIsZero(t *testing.T) {
	ctx := context.Background()
	adapter := &acceptanceScriptedAdapter{
		presigned: PresignedURL{
			URL:       "https://example.invalid/signed",
			Method:    "GET",
			ExpiresAt: time.Now().Add(time.Minute).Unix(),
		},
	}
	store, err := NewBlobStore(acceptanceSingleAttemptConfig(), adapter, Hooks{})
	if err != nil {
		t.Fatalf("NewBlobStore: %v", err)
	}
	concrete := store.(*blobStore)
	concrete.cfg.Presign.MaxTTL = 0

	if _, err := store.Presign(ctx, "tenant/key", PresignGet, PresignOptions{TTL: int64(MaxAllowedPresignTTL.Seconds())}); err != nil {
		t.Fatalf("Presign should fall back to global max TTL: %v", err)
	}
}

func TestAcceptanceMultipartInitiateRejectsDeniedKey(t *testing.T) {
	ctx := context.Background()
	cfg := acceptanceSingleAttemptConfig()
	cfg.Policy.Permission.DeniedPrefixes = []string{"blocked/"}
	store, err := NewBlobStore(cfg, NewInMemoryAdapter(), Hooks{})
	if err != nil {
		t.Fatalf("NewBlobStore: %v", err)
	}
	session, err := store.Multipart(ctx)
	if err != nil {
		t.Fatalf("Multipart: %v", err)
	}

	if _, err := session.Initiate(ctx, "blocked/key", PutOptions{}); err == nil || errorKind(err) != ErrorKindValidation {
		t.Fatalf("Initiate with denied key got %v", err)
	}
}

func TestAcceptanceChecksumVerifierReportsVerificationStateAfterRead(t *testing.T) {
	body := "verified"
	reader := &ObjectReader{ReadCloser: io.NopCloser(strings.NewReader(body))}
	wrapped := wrapChecksumVerifier(reader, ObjectInfo{
		ChecksumAlgo: ChecksumSHA256,
		ChecksumHex:  computeChecksum(ChecksumSHA256, []byte(body)),
	})
	checksum, ok := wrapped.ReadCloser.(*checksumReader)
	if !ok {
		t.Fatalf("expected checksumReader, got %T", wrapped.ReadCloser)
	}
	if checksum.verify() {
		t.Fatal("checksum should not be verified before EOF")
	}

	data, err := io.ReadAll(wrapped)
	if err != nil {
		t.Fatalf("read verified stream: %v", err)
	}
	if string(data) != body || !checksum.verify() || !wrapped.ChecksumVerified {
		t.Fatalf("unexpected checksum verification result data=%q verify=%v reader=%v", string(data), checksum.verify(), wrapped.ChecksumVerified)
	}
}

func TestAcceptanceErrorNilReceiverDoesNotMatchSentinel(t *testing.T) {
	var typedNil *Error
	if typedNil.Is(ErrInvalidConfig) {
		t.Fatal("nil *Error receiver should not match sentinel")
	}
}

func TestAcceptancePlainErrorsAreNotRetryable(t *testing.T) {
	if isRetryable(errors.New("plain")) {
		t.Fatal("plain errors should not be retryable")
	}
}

func TestAcceptanceRetryPolicyReturnsFatalErrorsWithoutRetry(t *testing.T) {
	attempts := 0
	fatal := newError(ErrorKindCanceled, "retry", "fatal")
	err := (retryPolicy{
		MaxAttempts: 3,
		InitialWait: time.Nanosecond,
		MaxWait:     time.Nanosecond,
		Multiplier:  1,
	}).withRetry(context.Background(), "retry", func(context.Context) error {
		attempts++
		return fatal
	})
	if err != fatal || attempts != 1 {
		t.Fatalf("fatal error should return immediately, attempts=%d err=%v", attempts, err)
	}
}

func TestAcceptanceCircuitBreakerAllowsHalfOpenAttempt(t *testing.T) {
	breaker := newCircuitBreaker(1, time.Minute)
	breaker.state = breakerHalfOpen
	if !breaker.allow() {
		t.Fatal("half-open breaker should allow one trial attempt")
	}
}
