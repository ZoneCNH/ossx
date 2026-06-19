# Changelog

## v1.2.0 — 2026-06-19

### Added — Release-grade test hardening + CI/CD pipeline + live integration evidence
- **`pkg/ossx` 100.0% statement coverage**: `acceptance_coverage_test.go` closes every remaining branch (config-from-env edges, error wrappers/context mapping, checksum verifier states, multipart lifecycle validation, retry + circuit-breaker half-open/cooldown, public helpers + nil-safe hooks, InMemoryAdapter closed-state paths, presign global-max fallback). CI `test` job enforces `100.0%` as a hard gate (`pkg-ossx.coverage.out`).
- **Full CI/CD pipeline on `sre/` machine pools** (`.github/workflows/ci.yml`): `ci-preflight` → `build` / `test` (100% coverage gate) / `lint` (vet + golangci-lint) / `boundary` (xlibgate trust: identity, release-consistency, template-residue, testkit-prod-import, secret-redaction) / `integration` (env-gated live Aliyun; required on release tags) / `secret-scan` (gitleaks) / `evidence` → `release-preflight` (sre/deploy, VERSION↔tag↔manifest↔repo-contract↔CHANGELOG consistency) → `release-publish` (gh release) → `post-release-smoke`; plus `rollback-drill` on workflow_dispatch.
- **`release/` artifacts**: `manifest/latest.json` + `evidence/local-acceptance.md` record reproducible acceptance commands and pending-public-evidence gaps.
- **`VERSION` file + `.repo-contract.yaml` maturity block**: explicit version source-of-truth and maturity matrix (live_integration_complete tracked).

### Tests
- Verified locally 2026-06-19 against real bucket `x-go` (ap-northeast-1): 5/5 integration PASS (Health 1.13s, PutGetDelete 4.66s, List 3.78s, Multipart 7.66s, Presign 2.80s).
- `pkg/ossx` statement coverage: **100.0%** (was 83.9% at v1.1.0). All-package coverage non-blocking.
- Reproducible local gate: `go test ./... -race`, `go vet ./...`, `go build ./...` all green.

### Changed
- `adapters/aliyun/oss.go`, `pkg/ossx/{blobstore,errors,helpers,presign,retry}.go`: refactor to eliminate uncovered branches (no public API change; signatures stable).
- CI integration gate keys now source `FOUNDATIONX_OSSX_*` secrets; skips cleanly when credentials absent (except release tags).

### Notes
- Identity unchanged: Aliyun OSS single-provider adapter (SPEC v1.2.0). Still NOT S3-compatible / generic.
- `factory=false` remains until public evidence archive, external CI artifacts, downstream adoption, and production-soak records are present (BLK-008 governance track).
- SPEC: `https://github.com/ZoneCNH/ZoneCNH/blob/main/module/ossx/SPEC.md`.

## v1.1.0

### Added — Real Aliyun OSS adapter + complete OSS functionality
- **`adapters/aliyun/`**: Real Aliyun OSS adapter (`github.com/aliyun/aliyun-oss-go-sdk/oss` v3.0.2) implementing `ossx.StoreAdapter`. SDK isolated to this package (FR-008 / BR-011); provider errors translated to typed `*Error` at the boundary (SPEC §11).
- **Streaming SPI redesign**: `StoreAdapter` (exported) is now streaming-first (`PutObject(ctx, key, body io.Reader, ...)` / `GetObject(ctx, key) (io.ReadCloser, ...)`). The v1.0.2-alpha `[]byte`-buffering SPI is gone. `Put`/`Get` no longer buffer whole objects (FR-004 / §16).
- **Complete Multipart lifecycle** (FR-005): `Initiate`/`UploadPart`/`ListParts`/`Complete`/`Abort` fully implemented with idempotency guard (BR-007), part-number/ETag validation, and part-count cap.
- **Real Presign** (FR-006): `bucket.SignURL` delegation; TTL ≤15min + operation allowlist + checksum/permission gates enforced before signing; `AuditEvent` emitted with sanitized scope (signed URL never logged, BR-009).
- **Policy validation** (FR-007): `LifecyclePolicy` / `RetentionPolicy` / `PermissionPolicy` in `Config`, validated at `Config.Validate()` and enforced before write/presign/delete (AC-OSS-007).
- **Typed error taxonomy** (`*Error` + `ErrorKind`): 15 kinds mirroring kernel `errx`; `Is(target)` matches by kind so `errors.Is(err, ErrInvalidConfig)` works across instances. `Retryable` flag drives retry classification.
- **Retry + Circuit Breaker** (`retry.go`): `retryPolicy.withRetry` + per-operation `circuitBreaker` (local, resiliencx-semantics-aligned, no base import — mirrors sibling adapters). Retries `connection`/`unavailable`/`timeout`/`rate_limit`; fatal on cancel.
- **observex-compatible Hooks** (`observability.go`): `Metrics`/`Tracer`/`Logger` interfaces (signature-compatible with observex) with `Noop*` defaults; `AuditEvent` type; sanitized metric labels (BR-009).
- **Three-state Health** (`blobstore.go Health`): distinguishes `ready`/`unreachable`/`config_error`/`degraded`/`closed` (AC-OSS-010).
- **`ConfigFromEnv()`** (`env.go`): reads `FOUNDATIONX_OSSX_*` (mirror of natsx `ConfigFromEnv`); composition-root convenience.
- **Checksum streaming verification** (`helpers.go wrapChecksumVerifier`): Get with `VerifyChecksum` tees through a hasher without buffering (FR-004 / BR-010).

### Tests
- Local reproducible gate passes: `GOWORK=off go test ./... -race -count=1`, `go vet ./...`, `go build ./...`, `golangci-lint run ./...`, secret scan, and import-boundary scan.
- `pkg/ossx` coverage reports 83.9% statement coverage via `GOWORK=off go test -count=1 -coverprofile=/tmp/ossx_pkg.cover ./pkg/ossx`.
- 5 Aliyun integration tests are build-tag and environment gated; the local no-credential gate passes with 5 intentional skips. Live OSS pass evidence remains pending a credentialed evidence archive.

### Changed
- `doc.go` / `README.md`: converged to Aliyun-only identity; removed S3/MinIO/multi-provider wording (was identity residue).
- `NewBlobStore(cfg, adapter StoreAdapter, hooks Hooks)`: adapter is now a `StoreAdapter` (was the old `ObjectStorageAdapter`).
- `Multipart(ctx)` now returns `(MultipartSession, error)` (was a bare session, ignored ctx).
- `go.mod`: adds `github.com/aliyun/aliyun-oss-go-sdk v3.0.2+incompatible` (direct). Still no `configx` (BR-002). Go 1.25.

### Removed
- `ObjectStorageAdapter` (old public `[]byte`-based SPI) — replaced by streaming `StoreAdapter`.
- `notImplementedSession` (multipart stub) — replaced by real `multipartSession`.
- Presign `ErrNotImplemented` after validation — signing is now real.

### Notes
- Identity: Aliyun OSS single-provider adapter (SPEC v1.1.1+ §1). NOT S3-compatible / generic.
- `factory=false` remains until the public evidence archive, live Aliyun pass evidence, external CI artifacts, downstream adoption, and production-soak records are present.
- SPEC: `https://github.com/ZoneCNH/ZoneCNH/blob/main/module/ossx/SPEC.md`.

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
