.PHONY: test test-race vet integration check

WEED_BINARY ?= ./weed

test:
	go test ./...

test-race:
	go test -race ./...

vet:
	go vet ./...

integration:
	WEED_BINARY=$(WEED_BINARY) go test -tags=integration ./...

check: test test-race vet integration
