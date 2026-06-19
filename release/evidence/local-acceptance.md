# ossx local acceptance evidence

Date: 2026-06-18

Local evidence:

- `GOWORK=off go test ./... -race -count=1`
- `GOWORK=off go test -count=1 -coverprofile=/tmp/ossx_pkg.cover ./pkg/ossx`
- `golangci-lint run ./...`
- `go vet ./...`
- `go build ./...`

Pending external evidence:

- Aliyun OSS live integration requires a private cloud test account.
- External CI artifact is not available in this local workspace.
- Downstream adoption and production soak evidence are not available in this local workspace.
