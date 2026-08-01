#!/usr/bin/env bash
# Runs the full verification chain required by AGENTS.md section 4 before any change is done.
# Exits non-zero on the first failing step so it works as a CI/pre-push gate.
#
# Usage:
#   ./scripts/verify.sh                       # build + vet + test(-race) + coverage
#   ./scripts/verify.sh --coverage-floor 60  # also enforce 60% on changed pkgs
set -euo pipefail

COVERAGE_FLOOR=0
BASE_REF="origin/main"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --coverage-floor) COVERAGE_FLOOR="$2"; shift 2 ;;
    --base-ref) BASE_REF="$2"; shift 2 ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done

echo "==> gofmt -l . (must be empty)"
fmt="$(gofmt -l .)"
if [[ -n "$fmt" ]]; then
  echo "FAIL: gofmt found unformatted files:" >&2
  echo "$fmt" >&2
  exit 1
fi

echo "==> go vet ./..."
go vet ./...

echo "==> go build ./..."
go build ./...

echo "==> go test -race -cover (per-package)"
go test -race -count=1 -coverprofile=coverage.out -covermode=atomic ./...
go tool cover -func=coverage.out | tail -1

if [[ "$COVERAGE_FLOOR" -gt 0 ]]; then
  pwsh ./scripts/coverage-floor.ps1 -FloorPct "$COVERAGE_FLOOR" -BaseRef "$BASE_REF" -CoverProfile coverage.out
fi

rm -f coverage.out
echo "verify: all checks passed"
