// Package ossx provides an object storage SDK with a verified Aliyun OSS adapter.
//
// S3, MinIO, Azure, and GCS provider constants are reserved for compatibility,
// but v1.0.1 rejects them at client construction until provider-specific
// integration evidence exists.
//
// This package follows xlib-standard conventions: Config, Validate, Sanitize,
// New, Close, HealthCheck, Error model, Metrics hooks, and structured errors.
package ossx
