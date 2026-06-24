APP_NAME=megavpn
BIN_DIR=bin

.PHONY: build test race vet fmt clean release-gate self-test run-api run-agent run-worker

build:
	mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/megavpn-api ./cmd/api
	go build -o $(BIN_DIR)/megavpn-agent ./cmd/agent
	go build -o $(BIN_DIR)/megavpn-worker ./cmd/worker

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -w ./cmd ./internal

clean:
	rm -rf $(BIN_DIR)

release-gate:
	scripts/release-gate.sh

self-test:
	scripts/self-test.sh

run-api:
	go run ./cmd/api

run-agent:
	go run ./cmd/agent

run-worker:
	go run ./cmd/worker
