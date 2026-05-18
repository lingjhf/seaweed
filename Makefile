.PHONY: test test-race vet integration check

WEED_BINARY ?= ./weed

test:
	go test -count=1 ./...

test-race:
	go test -count=1 -race ./...

vet:
	go vet ./...

integration:
	WEED_BINARY=$(WEED_BINARY) go test -count=1 -tags=integration ./...

check: test test-race vet integration
