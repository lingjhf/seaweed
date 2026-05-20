.PHONY: lint test test-race vet integration coverage bench ci check check-full release-check

WEED_BINARY ?= ./weed
COVER_MIN ?= 93.0

lint:
	golangci-lint run ./...

test:
	go test -count=1 ./...

test-race:
	go test -count=1 -race ./...

vet:
	go vet ./...

integration:
	WEED_BINARY=$(WEED_BINARY) go test -count=1 -tags=integration ./...

coverage:
	WEED_BINARY=$(WEED_BINARY) COVER_MIN=$(COVER_MIN) ./scripts/coverage.sh

bench:
	go test -run '^$$' -bench=. -benchmem ./...

ci: lint test test-race vet

check: ci integration

check-full: check coverage

release-check: check-full
