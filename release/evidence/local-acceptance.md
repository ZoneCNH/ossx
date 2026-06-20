# ossx local acceptance evidence

Date: 2026-06-19

Status: local-production-candidate

Version: v1.2.1

Local evidence:

- `GOWORK=off go test -race ./... -count=1`
- `GOWORK=off go test ./pkg/ossx -run 'TestAcceptanceBlobStore(StrictDeletePreflightsIdempotentAdapter|NonStrictDeleteKeepsIdempotentAdapter|DeleteAndExistsErrorMappings)$' -count=1`
- `GOWORK=off go test ./pkg/ossx -run 'TestPublicInterfacesStayWithinGovernanceLimit|TestNewBlobStoreRejectsMissingAdapterCapabilities|TestSPISurface' -count=1`
- `GOWORK=off go test -race ./adapters/aliyun -run TestAdapterCloseStateIsRaceSafe -count=1`
- `GOWORK=off go test -tags integration ./adapters/aliyun -run TestIntegrationDeleteStrictMissing -count=1` (compiles and skips without live credentials)
- `/home/ZoneCNH/sre/secrets/env/dev.md` loaded in-process as `FOUNDATIONX_OSSX_*`, then `OSSX_LIVE_INTEGRATION=1 GOWORK=off go test -tags integration ./adapters/aliyun -count=1 -timeout 180s` => PASS; credential values withheld
- `GOWORK=off go test ./pkg/ossx -count=1 -covermode=atomic -coverprofile=/tmp/ossx-pkg.cover`
- `go tool cover -func=/tmp/ossx-pkg.cover` => `total: (statements) 100.0%`
- `golangci-lint run ./...`
- `GOWORK=off go vet ./...`
- `GOWORK=off go build ./...`
- `scripts/secret-scope-check.sh`
- `git diff --check`

Local production-readiness fixes validated:

- Strict delete now preflights idempotent object-store adapters and returns `ErrNotFound` for missing objects when `StrictNotFound` is enabled.
- Aliyun adapter close state is race-safe under concurrent `Close` and operation preflight checks.
- Public API governance is locally resolved: `BlobStore`/`StoreAdapter` were split into bounded capability interfaces and covered by an interface-size regression test.

Pending external evidence:

- Aliyun OSS live integration passed locally with `dev.md`; the release-tag CI artifact is still not archived in this workspace.
- External release-tag CI, Gitleaks, and xlibgate artifacts are not available in this local workspace.
- Downstream adoption and production soak evidence are not available in this local workspace.
