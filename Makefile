APP_NAME=megavpn
BIN_DIR=bin

.PHONY: build test race vet fmt clean release-gate self-test run-api run-agent run-worker run-migrate run-admin

build:
	mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/megavpn-api ./cmd/api
	go build -o $(BIN_DIR)/megavpn-migrate ./cmd/migrate
	go build -o $(BIN_DIR)/megavpn-worker ./cmd/worker
	go build -o $(BIN_DIR)/megavpn-agent ./cmd/agent
	go build -o $(BIN_DIR)/megavpn-admin ./cmd/admin

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
	scripts/ci/release-gate.sh

self-test:
	scripts/ci/self-test.sh

run-api:
	go run ./cmd/api

run-agent:
	go run ./cmd/agent

run-worker:
	go run ./cmd/worker

run-migrate:
	go run ./cmd/migrate

run-admin:
	go run ./cmd/admin
