//go:build integration

// Package aliyun integration tests connect to a REAL Aliyun OSS bucket.
//
// Two-layer gate (mirrors taosx):
//  1. Build tag: `//go:build integration` — run with `go test -tags integration`
//  2. Env gate:  `OSSX_LIVE_INTEGRATION=1` — must be set or the test skips.
//
// Credentials come from ossx.ConfigFromEnv() reading FOUNDATIONX_OSSX_* env
// vars loaded from the operator-owned sre/secrets/env/dev.md by the test
// harness. Never commit long-term static AK/SK, bucket-specific endpoints, or
// environment dumps.
//
// Run locally:
//
//	export FOUNDATIONX_OSSX_* from /home/ZoneCNH/sre/secrets/env/dev.md
//	OSSX_LIVE_INTEGRATION=1 go test -tags integration ./adapters/aliyun/ -v -timeout 120s
package aliyun

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ZoneCNH/ossx/pkg/ossx"
)

// integrationSettings holds the live bucket configuration.
type integrationSettings struct {
	cfg    ossx.Config
	prefix string // unique per-run key prefix to avoid collisions
}

func loadIntegrationSettings(t *testing.T) integrationSettings {
	t.Helper()
	if os.Getenv("OSSX_LIVE_INTEGRATION") != "1" {
		t.Skip("set OSSX_LIVE_INTEGRATION=1 with env loaded from sre/secrets/env/dev.md to run live Aliyun OSS integration")
	}
	cfg, err := ossx.ConfigFromEnv()
	if err != nil {
		t.Fatalf("ConfigFromEnv: %v (did you load sre/secrets/env/dev.md?)", err)
	}
	// Unique prefix per test run to avoid key collisions across runs.
	prefix := fmt.Sprintf("ossx-it/%d/", time.Now().UnixNano())
	return integrationSettings{cfg: cfg, prefix: prefix}
}

// newLiveAdapter builds a real Aliyun Adapter for the integration test.
func newLiveAdapter(t *testing.T) (*Adapter, ossx.Config, string) {
	t.Helper()
	s := loadIntegrationSettings(t)
	ctx := context.Background()
	adapter, err := NewAdapter(ctx, s.cfg)
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}
	return adapter, s.cfg, s.prefix
}

// TestIntegrationHealth verifies the adapter can reach the real bucket (FR-010).
func TestIntegrationHealth(t *testing.T) {
	adapter, _, _ := newLiveAdapter(t)
	ctx := context.Background()
	if err := adapter.Health(ctx); err != nil {
		t.Fatalf("Health: %v", err)
	}
	defer adapter.Close(ctx)
}

// TestIntegrationPutGetDelete exercises the core round-trip against the real
// bucket (FR-003 / FR-004 — streaming Put/Get).
func TestIntegrationPutGetDelete(t *testing.T) {
	adapter, _, prefix := newLiveAdapter(t)
	ctx := context.Background()
	defer adapter.Close(ctx)

	key := prefix + "round-trip.txt"
	payload := []byte("ossx integration test payload — v1.1.0")
	_, err := adapter.PutObject(ctx, key, bytes.NewReader(payload), int64(len(payload)), ossx.PutAdapterOptions{
		ContentType: "text/plain",
	})
	if err != nil {
		t.Fatalf("PutObject: %v", err)
	}
	defer adapter.DeleteObject(ctx, key, false)

	body, info, err := adapter.GetObject(ctx, key)
	if err != nil {
		t.Fatalf("GetObject: %v", err)
	}
	got, _ := io.ReadAll(body)
	body.Close()
	if !bytes.Equal(got, payload) {
		t.Fatalf("round-trip mismatch: got %q want %q", got, payload)
	}
	if info.Size != int64(len(payload)) {
		t.Fatalf("size mismatch: got %d want %d", info.Size, len(payload))
	}

	hinfo, err := adapter.HeadObject(ctx, key)
	if err != nil {
		t.Fatalf("HeadObject: %v", err)
	}
	if hinfo.ETag == "" {
		t.Fatalf("HeadObject: empty ETag")
	}
}

func TestIntegrationDeleteStrictMissing(t *testing.T) {
	adapter, _, prefix := newLiveAdapter(t)
	ctx := context.Background()
	defer adapter.Close(ctx)

	key := prefix + "strict-missing.txt"
	if err := adapter.DeleteObject(ctx, key, false); err != nil {
		t.Fatalf("non-strict DeleteObject missing: %v", err)
	}
	err := adapter.DeleteObject(ctx, key, true)
	if err == nil {
		t.Fatal("strict DeleteObject missing: expected not found")
	}
	if !errors.Is(err, ossx.ErrNotFound) {
		t.Fatalf("strict DeleteObject missing: got %v", err)
	}
}

// TestIntegrationList verifies bounded pagination (FR-003 / BR-006).
func TestIntegrationList(t *testing.T) {
	adapter, _, prefix := newLiveAdapter(t)
	ctx := context.Background()
	defer adapter.Close(ctx)

	// Seed 3 objects.
	for i := 0; i < 3; i++ {
		k := fmt.Sprintf("%slist/%d.txt", prefix, i)
		if _, err := adapter.PutObject(ctx, k, strings.NewReader("x"), 1, ossx.PutAdapterOptions{}); err != nil {
			t.Fatalf("seed PutObject %d: %v", i, err)
		}
		defer adapter.DeleteObject(ctx, k, false)
	}
	page, err := adapter.ListObjects(ctx, prefix+"list/", 2, "")
	if err != nil {
		t.Fatalf("ListObjects: %v", err)
	}
	if len(page.Items) != 2 || !page.IsTruncated {
		t.Fatalf("expected 2 items truncated, got %d items truncated=%v", len(page.Items), page.IsTruncated)
	}
}

// TestIntegrationMultipart exercises full multipart upload against the real
// bucket (FR-005).
func TestIntegrationMultipart(t *testing.T) {
	adapter, _, prefix := newLiveAdapter(t)
	ctx := context.Background()
	defer adapter.Close(ctx)

	key := prefix + "multipart.bin"
	id, err := adapter.InitiateMultipart(ctx, key, ossx.PutAdapterOptions{ContentType: "application/octet-stream"})
	if err != nil {
		t.Fatalf("InitiateMultipart: %v", err)
	}
	defer adapter.AbortMultipart(ctx, id) // safety net

	part1 := bytes.Repeat([]byte("a"), 1024*100) // 100KB
	part2 := bytes.Repeat([]byte("b"), 1024*100)

	etag1, err := adapter.UploadPart(ctx, id, 1, bytes.NewReader(part1), int64(len(part1)))
	if err != nil {
		t.Fatalf("UploadPart 1: %v", err)
	}
	etag2, err := adapter.UploadPart(ctx, id, 2, bytes.NewReader(part2), int64(len(part2)))
	if err != nil {
		t.Fatalf("UploadPart 2: %v", err)
	}

	parts, err := adapter.ListParts(ctx, id)
	if err != nil {
		t.Fatalf("ListParts: %v", err)
	}
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts listed, got %d", len(parts))
	}

	info, err := adapter.CompleteMultipart(ctx, id, []ossx.PartETag{etag1, etag2})
	if err != nil {
		t.Fatalf("CompleteMultipart: %v", err)
	}
	defer adapter.DeleteObject(ctx, key, false)

	if info.Size != int64(len(part1)+len(part2)) {
		t.Fatalf("complete size mismatch: got %d want %d", info.Size, len(part1)+len(part2))
	}
	if info.ETag == "" {
		t.Fatalf("complete: empty ETag")
	}
}

// TestIntegrationPresign generates a real presigned URL and validates it can
// fetch the object (FR-006).
func TestIntegrationPresign(t *testing.T) {
	adapter, _, prefix := newLiveAdapter(t)
	ctx := context.Background()
	defer adapter.Close(ctx)

	key := prefix + "presign.txt"
	payload := "presign-target"
	if _, err := adapter.PutObject(ctx, key, strings.NewReader(payload), int64(len(payload)), ossx.PutAdapterOptions{}); err != nil {
		t.Fatalf("PutObject: %v", err)
	}
	defer adapter.DeleteObject(ctx, key, false)

	url, err := adapter.PresignURL(ctx, key, ossx.PresignGet, 120, ossx.PresignAdapterOptions{})
	if err != nil {
		t.Fatalf("PresignURL: %v", err)
	}
	if !presignURLUsesExpectedHost(url.URL, cfgCNAME(adapter)) {
		t.Fatal("presigned URL host did not match Aliyun OSS or configured CNAME")
	}
	if url.Method != "GET" {
		t.Fatalf("method mismatch: got %q want GET", url.Method)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url.URL, nil)
	if err != nil {
		t.Fatalf("presigned GET request: %v", err)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("presigned GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		t.Fatalf("presigned GET status: got %d body=%q", resp.StatusCode, body)
	}
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("presigned GET read: %v", err)
	}
	if string(got) != payload {
		t.Fatalf("presigned GET body mismatch: got %q want %q", got, payload)
	}
}

// cfgCNAME returns the CNAME (if set) for presign URL validation.
func cfgCNAME(a *Adapter) string { return a.cfg.CNAME }

func presignURLUsesExpectedHost(rawURL, cname string) bool {
	parsed, err := neturl.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Host)
	if strings.Contains(host, "aliyuncs.com") {
		return true
	}
	cname = strings.ToLower(strings.TrimSpace(cname))
	return cname != "" && (host == cname || strings.HasSuffix(host, "."+cname))
}
