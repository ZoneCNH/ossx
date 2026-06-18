// Package ossx is the Aliyun OSS adapter — a stable BlobStore API, object
// metadata model, streaming semantics, multipart lifecycle, presigned URL
// policy, and observability hooks over Aliyun OSS (SPEC §1, single provider).
//
// The public API depends only on the Go standard library. The Aliyun OSS SDK
// (github.com/aliyun/aliyun-oss-go-sdk/oss) is encapsulated in adapters/aliyun
// and never appears in public types (BR-011). ossx does NOT import configx
// (BR-002); configuration is supplied as a module-owned Config struct (populated
// by the composition root, e.g., via ConfigFromEnv reading FOUNDATIONX_OSSX_*
// secrets).
//
// # Quickstart
//
//	cfg, err := ossx.ConfigFromEnv() // reads FOUNDATIONX_OSSX_*
//	if err != nil { return err }
//	adapter, err := aliyun.NewAdapter(ctx, cfg)
//	if err != nil { return err }
//	store, err := ossx.NewBlobStore(cfg, adapter, ossx.Hooks{})
//	if err != nil { return err }
//	defer store.Close(ctx)
//	info, err := store.Put(ctx, key, body, ossx.PutOptions{ContentType: "text/plain"})
//
// For tests and examples, use ossx.NewInMemoryAdapter() — no SDK required.
//
// Status: v1.1.0 — real Aliyun adapter, streaming Put/Get, full multipart
// lifecycle, real presigned URLs, lifecycle/retention/permission policy
// validation, retry/circuit, and observex-compatible hooks. See
// module/ossx/SPEC.md FR-001..FR-010 for the full contract.
package ossx
