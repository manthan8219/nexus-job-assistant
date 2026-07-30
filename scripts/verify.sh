#!/usr/bin/env bash
# Runs the full verification chain required by AGENTS.md §4 before any change is considered done.
set -euo pipefail

gofmt -w .
go vet ./...
go build ./...
go test ./...

echo "verify: all checks passed"
