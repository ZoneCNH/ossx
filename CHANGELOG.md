# Changelog

## v1.0.2-alpha — 2026-06-18

### Added
- **Public API surface**: `BlobStore` interface with 11 methods per SPEC §8.
- **Type system**: `Config`, `Key`, `Prefix`, `ObjectInfo`, `PutOptions`, `GetOptions`, `DeleteOptions`, `CopyOptions`, `ListOptions`, `ListPage`, `ObjectReader`, `MultipartSession`, `UploadID`, `PartETag`, `PresignedURL`, `PresignOptions`, `HealthReport`, `Hooks`, `ObjectStorageAdapter`.
- **Typed errors** (`errors.go`): `ErrInvalidConfig`, `ErrNotFound`, `ErrConflict`, `ErrPermission`, `ErrChecksumMismatch`, `ErrTimeout`, `ErrCancelled`, `ErrProviderFailure`, `ErrClosed`, `ErrInvalidKey`, `ErrInvalidMetadata`, `ErrNotImplemented`.
- **`InMemoryAdapter`**: Full in-memory `ObjectStorageAdapter` for tests and stub integration.
- **Validators**: `Config.Validate`, `NewKey`, `validateMetadata`, `validateChecksumAlgo`.
- **Checksum**: SHA-256 / MD5 / CRC32 (`computeChecksum`).
- **Hooks**: `OnOperation(name, key, latencyNs, sizeBytes, errorClass)` with nil-safe no-op.
- **Tests**: 10 test cases covering TC-002/003/004/006/007/009/011/012 (`go test -race -count=1` passes in 1.012s).

### Deferred to v1.1.0
- `Multipart()` session methods return `ErrNotImplemented`.
- `Presign()` returns `ErrNotImplemented` for signing (TTL/allowlist validation works).
- `adapters/s3`, `adapters/aliyun` packages.
- Streaming partial-failure error surfacing beyond `io.ReadAll`.

### Notes
- Module path: `github.com/ZoneCNH/ossx`.
- Go 1.23+; stdlib-only dependencies.
- BR-002 enforced: no `configx` import.
- SPEC: `https://github.com/ZoneCNH/ZoneCNH/blob/main/module/ossx/SPEC.md`.

## v1.0.1 — 2026-06-13

- Repository scaffolding: `.repo-contract.yaml`, `.env.example`, `LICENSE`, CI workflow.
- No `pkg/ossx` source code present (release tag without import-able code).

## v1.0.0 — 2026-06-12

- Initial commit; identity declaration.
