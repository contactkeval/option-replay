.PHONY: test build

test:
	go test ./... -v

build:
	go build ./cmd/option-replay
