#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p bin
GOFLAGS="${GOFLAGS:-}" go build -o bin/megavpn-api ./cmd/api
GOFLAGS="${GOFLAGS:-}" go build -o bin/megavpn-migrate ./cmd/migrate
GOFLAGS="${GOFLAGS:-}" go build -o bin/megavpn-worker ./cmd/worker
GOFLAGS="${GOFLAGS:-}" go build -o bin/megavpn-agent ./cmd/agent
GOFLAGS="${GOFLAGS:-}" go build -o bin/megavpn-admin ./cmd/admin
