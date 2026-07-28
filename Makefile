.PHONY: all build test-unit test-integration test-all test fmt vet lint install-hooks clean

all: build lint test-all

build:
	go build ./cmd/...

fmt:
	gofmt -s -l .
	@echo "OK"

vet:
	go vet ./internal/... ./cmd/...

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	elif [ -f "$$HOME/go/bin/golangci-lint" ]; then \
		$$HOME/go/bin/golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed, skipping"; \
	fi

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
