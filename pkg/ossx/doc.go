// Package ossx provides a stable BlobStore API, object metadata model,
// streaming semantics, and adapter SPI for object storage.
//
// The public API depends only on the Go standard library. Provider-specific
// SDKs (e.g., Aliyun OSS, AWS S3, MinIO) are encapsulated in the
// adapters/* packages and never appear in public types.
//
// Status: v1.0.2-alpha — In-memory adapter is fully implemented; multipart
// and presign return ErrNotImplemented. S3/Aliyun adapters scheduled for
// v1.1.0. See module/ossx/SPEC.md FR-001..FR-010 for the full contract.
package ossx
