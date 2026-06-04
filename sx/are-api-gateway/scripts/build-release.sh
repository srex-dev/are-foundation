#!/usr/bin/env bash
set -euo pipefail

go mod tidy
go test ./...
golangci-lint run
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/are-api-gateway ./cmd/are-api-gateway
docker build -t are/api-gateway:local .
