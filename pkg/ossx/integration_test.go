//go:build integration

package ossx

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestIntegrationAliyunOSS(t *testing.T) {
	if os.Getenv("OSSX_INTEGRATION") != "1" {
		t.Skip("OSSX_INTEGRATION=1 is required")
	}
	cfg := Config{
		Name:            "ossx-integration",
		Provider:        ProviderOSS,
		Endpoint:        envRequired(t, "OSSX_ENDPOINT"),
		Region:          os.Getenv("OSSX_REGION"),
		Bucket:          envRequired(t, "OSSX_BUCKET"),
		AccessKeyID:     envRequired(t, "OSSX_ACCESS_KEY_ID"),
		SecretAccessKey: envRequired(t, "OSSX_SECRET_ACCESS_KEY"),
		UseSSL:          true,
		Timeout:         30 * time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	client, err := New(ctx, cfg, WithMetrics(NoopMetrics{}))
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer func() {
		if err := client.Close(context.Background()); err != nil {
			t.Fatalf("Close failed: %v", err)
		}
	}()

	prefix := fmt.Sprintf("ossx/integration/%d/", time.Now().UnixNano())
	key := prefix + "object-a.txt"
	secondKey := prefix + "object-b.txt"
	payload := "ossx v1.0.1 integration payload"
	secondPayload := payload + " second page"

	info, err := client.PutObject(ctx, PutInput{Key: key, Body: strings.NewReader(payload), Size: int64(len(payload)), ContentType: "text/plain"})
	if err != nil {
		t.Fatalf("PutObject failed: %v", err)
	}
	if info.Key != key || info.Size != int64(len(payload)) || info.ContentType != "text/plain" {
		t.Fatalf("unexpected put info: %#v", info)
	}
	defer func() {
		for _, cleanupKey := range []string{key, secondKey} {
			if err := client.DeleteObject(context.Background(), cleanupKey); err != nil && !IsKind(err, ErrorKindNotFound) {
				t.Fatalf("cleanup DeleteObject failed: %v", err)
			}
		}
	}()
	secondInfo, err := client.PutObject(ctx, PutInput{Key: secondKey, Body: strings.NewReader(secondPayload), Size: int64(len(secondPayload)), ContentType: "text/plain"})
	if err != nil {
		t.Fatalf("second PutObject failed: %v", err)
	}
	if secondInfo.Key != secondKey || secondInfo.Size != int64(len(secondPayload)) {
		t.Fatalf("unexpected second put info: %#v", secondInfo)
	}

	output, err := client.GetObject(ctx, key)
	if err != nil {
		t.Fatalf("GetObject failed: %v", err)
	}
	data, err := io.ReadAll(output.Body)
	if closeErr := output.Body.Close(); closeErr != nil {
		t.Fatalf("body close failed: %v", closeErr)
	}
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if string(data) != payload || output.Size != int64(len(payload)) || output.ContentType != "text/plain" {
		t.Fatalf("unexpected get output: size=%d type=%q body=%q", output.Size, output.ContentType, string(data))
	}
	if output.ETag == "" || output.Info.ETag == "" || output.Info.LastModified == "" {
		t.Fatalf("expected populated metadata, got %#v", output.Info)
	}

	listed, err := client.ListObjects(ctx, ListInput{Prefix: prefix, MaxKeys: 1})
	if err != nil {
		t.Fatalf("ListObjects failed: %v", err)
	}
	if len(listed.Objects) != 1 || listed.Objects[0].Key != key || !listed.IsTruncated || listed.NextMarker == "" {
		t.Fatalf("expected first paginated key %q, got %#v", key, listed)
	}
	nextPage, err := client.ListObjects(ctx, ListInput{Prefix: prefix, Marker: listed.NextMarker, MaxKeys: 1})
	if err != nil {
		t.Fatalf("second ListObjects failed: %v", err)
	}
	if len(nextPage.Objects) != 1 || nextPage.Objects[0].Key != secondKey {
		t.Fatalf("expected second paginated key %q, got %#v", secondKey, nextPage)
	}

	status := client.HealthCheck(ctx)
	if status.Status != HealthHealthy {
		t.Fatalf("expected healthy status, got %#v", status)
	}

	cancelled, cancelPut := context.WithCancel(context.Background())
	cancelPut()
	_, err = client.PutObject(cancelled, PutInput{Key: prefix + "cancelled.txt", Body: strings.NewReader("cancelled"), Size: 9})
	if !IsKind(err, ErrorKindUnavailable) {
		t.Fatalf("expected cancelled context error, got %v", err)
	}

	if err := client.DeleteObject(ctx, key); err != nil {
		t.Fatalf("DeleteObject failed: %v", err)
	}
	_, err = client.GetObject(ctx, key)
	if !IsKind(err, ErrorKindNotFound) {
		t.Fatalf("expected not found after delete, got %v", err)
	}

	_, err = client.PutObject(ctx, PutInput{Key: "/invalid", Body: strings.NewReader("bad"), Size: 3})
	if !IsKind(err, ErrorKindValidation) {
		t.Fatalf("expected invalid key validation error, got %v", err)
	}
}

func envRequired(t *testing.T, name string) string {
	t.Helper()
	value := os.Getenv(name)
	if value == "" {
		t.Fatalf("%s is required", name)
	}
	return value
}
