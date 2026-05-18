.PHONY: test test-race vet integration coverage bench ci check check-full

WEED_BINARY ?= ./weed
COVER_MIN ?= 90.0

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

ci: test test-race vet

check: test test-race vet integration

check-full: check coverage
