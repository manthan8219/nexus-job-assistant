# Runs the full verification chain required by AGENTS.md section 4 before any change is done.
# Exits non-zero on the first failing step so it works as a CI/pre-push gate.
#
# Usage:
#   ./scripts/verify.ps1                      # build + vet + test(-race) + coverage
#   ./scripts/verify.ps1 -CoverageFloor 60    # also enforce 60% on changed pkgs
param(
    [switch]$CoverageFloor,
    [int]$FloorPct = 60,
    [string]$BaseRef = "origin/main"
)
$ErrorActionPreference = "Continue"

function Step($name, $script) {
    Write-Host "==> $name" -ForegroundColor Cyan
    & $script
    if ($LASTEXITCODE -ne 0) { Write-Host "FAIL: $name" -ForegroundColor Red; exit $LASTEXITCODE }
}

# gofmt -l lists unformatted files but exits 0, so check output explicitly.
Write-Host "==> gofmt -l . (must be empty)" -ForegroundColor Cyan
$fmt = gofmt -l .
if ($fmt) { Write-Host "FAIL: gofmt found unformatted files:" -ForegroundColor Red; $fmt; exit 1 }

Step "go vet ./..." { go vet ./... }

Step "go build ./..." { go build ./... }

# go test with -race (catches data races) and per-package coverage.
# -race requires CGO/gcc; fall back to plain tests if unavailable (e.g. Windows without gcc).
Write-Host "==> go test -race -cover (per-package)" -ForegroundColor Cyan
$raceFlag = "-race"
go env CGO_ENABLED | Out-Null
$gccAvailable = $true
try {
    $null = Get-Command gcc -ErrorAction Stop
} catch {
    $gccAvailable = $false
}
if (-not $gccAvailable) {
    Write-Host "WARNING: gcc not found; running tests without -race" -ForegroundColor Yellow
    $raceFlag = ""
}
go test $raceFlag -count=1 -timeout 120s -coverprofile coverage.out -covermode=atomic ./...
if ($LASTEXITCODE -ne 0) {
    Write-Host "FAIL: go test -race ./..." -ForegroundColor Red
    Remove-Item coverage.out -ErrorAction SilentlyContinue
    exit $LASTEXITCODE
}
Write-Host "per-package coverage:" -ForegroundColor Cyan
go tool cover -func coverage.out | Where-Object { $_ -match '\.go:' } | ForEach-Object {
    if ($_ -match '\s+0\.0%$') { Write-Host $_ -ForegroundColor Yellow } else { Write-Host $_ }
}
$summary = (go tool cover -func coverage.out | Select-Object -Last 1)
Write-Host "TOTAL: $summary" -ForegroundColor Green

if ($CoverageFloor) {
    & "$PSScriptRoot/coverage-floor.ps1" -FloorPct $FloorPct -BaseRef $BaseRef -CoverProfile coverage.out
    if ($LASTEXITCODE -ne 0) { Remove-Item coverage.out -ErrorAction SilentlyContinue; exit $LASTEXITCODE }
}

Remove-Item coverage.out -ErrorAction SilentlyContinue
Write-Host "verify: all checks passed" -ForegroundColor Green
