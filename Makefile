.PHONY: all build test test-unit test-integration test-all clean

all: build test-all

build:
	go build ./cmd/...

test-unit:
	go test ./internal/... -count=1 -timeout 30s

test-integration:
	go test -tags=integration ./internal/... -count=1 -timeout 30s

test-all: test-unit test-integration

test:
	go test -tags=integration ./internal/... -count=1 -timeout 30s

clean:
	rm -f loopai loopai-backend
	go clean ./cmd/...
