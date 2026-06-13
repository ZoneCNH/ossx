GO ?= go
COVERPROFILE ?= coverage.out
INTEGRATION_RUN ?= TestIntegrationAliyunOSS

.PHONY: build test test-unit test-race test-integration coverage lint vet fmt release-check clean

build:
	$(GO) build ./...

test: test-race

test-unit:
	$(GO) test ./...

test-race:
	$(GO) test -race -count=1 ./...

test-integration:
	@test "$$OSSX_INTEGRATION" = "1"
	@test -n "$$OSSX_ENDPOINT"
	@test -n "$$OSSX_REGION"
	@test -n "$$OSSX_BUCKET"
	@test -n "$$OSSX_ACCESS_KEY_ID"
	@test -n "$$OSSX_SECRET_ACCESS_KEY"
	$(GO) test -tags=integration ./... -run $(INTEGRATION_RUN) -count=1

coverage:
	$(GO) test -tags=integration ./... -covermode=atomic -coverprofile=$(COVERPROFILE)
	$(GO) tool cover -func=$(COVERPROFILE)

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then golangci-lint run ./...; else echo "golangci-lint not installed; skipping"; fi

vet:
	$(GO) vet ./...

fmt:
	gofmt -s -w .

release-check: test-unit test-race vet build

clean:
	rm -f coverage.out unit.out
