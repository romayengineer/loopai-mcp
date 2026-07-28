GOLANGCI_LINT_VERSION := v2.12.2
GOLANGCI_LINT := $(shell go env GOPATH)/bin/golangci-lint

.PHONY: all build test-unit test-integration test-all test cover fmt vet lint install-lint install-hooks clean

all: build lint test-all

build:
	go build ./cmd/...

fmt:
	gofmt -s -l .
	@echo "OK"

vet:
	go vet ./internal/... ./cmd/...

install-lint:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	@echo "Installed golangci-lint $(GOLANGCI_LINT_VERSION)"

lint:
	@if [ -x "$(GOLANGCI_LINT)" ]; then \
		actual=$$("$(GOLANGCI_LINT)" version 2>/dev/null | head -1); \
		if ! echo "$$actual" | grep -qE "v?2\.12\.2"; then \
			echo "WARNING: expected golangci-lint $(GOLANGCI_LINT_VERSION), got: $$actual"; \
			echo "  Run 'make install-lint' to install the pinned version matching CI."; \
		fi; \
		"$(GOLANGCI_LINT)" run ./...; \
	elif command -v golangci-lint >/dev/null 2>&1; then \
		actual=$$(golangci-lint version 2>/dev/null | head -1); \
		echo "WARNING: using golangci-lint from PATH (not GOBIN). Version: $$actual"; \
		echo "  Run 'make install-lint' to install the pinned version matching CI."; \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not found. Run 'make install-lint' first."; \
		exit 1; \
	fi

test-unit:
	go test -race ./internal/... -count=1 -timeout 30s
	go test -race ./cmd/loopai ./cmd/loopai-capture -count=1 -timeout 30s

test-integration:
	go test -tags=integration ./internal/... -count=1 -timeout 30s
	go test -tags=integration ./cmd/... -count=1 -timeout 120s

test-all: test-unit test-integration

test:
	go test -tags=integration ./internal/... -count=1 -timeout 30s

cover:
	@go test -coverprofile=/tmp/loopai-cover-unit.out ./internal/... -count=1 -timeout 30s > /dev/null 2>&1
	@go test -coverprofile=/tmp/loopai-cover-all.out -coverpkg=./internal/... -tags=integration ./internal/... -count=1 -timeout 60s > /dev/null 2>&1
	@echo "=== Internal unit test coverage ==="
	@go tool cover -func=/tmp/loopai-cover-unit.out | grep total | awk '{print $$3}'
	@echo "=== Internal combined (unit + integration) ==="
	@go tool cover -func=/tmp/loopai-cover-all.out | grep total | awk '{print $$3}'

install-hooks:
	git config core.hooksPath .githooks

clean:
	rm -f loopai loopai-backend
	go clean ./cmd/...
