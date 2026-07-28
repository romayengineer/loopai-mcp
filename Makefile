.PHONY: all build test-unit test-integration test-all test fmt vet install-hooks clean

all: build test-all

build:
	go build ./cmd/...

fmt:
	gofmt -s -l .
	@echo "OK"

vet:
	go vet ./internal/... ./cmd/...

test-unit:
	go test -race ./internal/... -count=1 -timeout 30s

test-integration:
	go test -tags=integration ./internal/... -count=1 -timeout 30s
	go test -tags=integration ./cmd/... -count=1 -timeout 120s

test-all: test-unit test-integration

test:
	go test -tags=integration ./internal/... -count=1 -timeout 30s

install-hooks:
	git config core.hooksPath .githooks

clean:
	rm -f loopai loopai-backend
	go clean ./cmd/...
