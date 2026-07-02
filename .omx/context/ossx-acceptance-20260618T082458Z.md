# ossx acceptance context

Task: execute the code/test side of the ossx acceptance repair for `/home/ossx`.

Target result:
- Local code gates pass.
- `pkg/ossx` coverage reaches the documented acceptance target where feasible.
- Context cancellation and integration-test adequacy are either fixed or reported with concrete SDK/runtime limits.
- Remaining live Aliyun gates are reported with exact prerequisites and commands.

Constraints:
- Keep changes scoped to `/home/ossx`.
- Do not add dependencies.
- Keep diffs small and reversible.
- The related documentation repository `/home/ZoneCNH` is dirty; do not edit it from this team run.

Known evidence:
- Previous local gates passed: `GOWORK=off go test ./... -race -count=1`, `GOWORK=off go vet ./...`, `GOWORK=off go build ./...`, and `bash scripts/secret-scope-check.sh`.
- `GOWORK=off go test -cover ./pkg/ossx/` previously reported 61.2%.
- Integration tests skip unless `OSSX_LIVE_INTEGRATION=1` and Aliyun OSS credentials are present.

Suggested lanes:
- Worker 1: inspect `pkg/ossx` coverage gaps and add focused tests.
- Worker 2: inspect Aliyun adapter context propagation and presign integration-test behavior.
- Worker 3: run verification gates and summarize unavailable external prerequisites.

Stop condition:
- Local gates pass after changes, or a required live Aliyun credential/service gate blocks further local progress with exact evidence.
