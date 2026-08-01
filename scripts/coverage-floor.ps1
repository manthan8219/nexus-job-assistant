# coverage-floor.ps1 - enforce a minimum coverage % on packages touched by a change.
# Used by CI (and locally) to guarantee AGENTS.md section 13: changed code ships with tests.
#
# Logic:
#   1. Build the set of Go packages changed vs -BaseRef (default: HEAD~1).
#   2. Parse `go tool cover -func=<profile>`; group function coverage by package.
#   3. Fail (exit 1) if any changed package's average coverage is below -FloorPct.
# Packages whose changed files have no coverable funcs (e.g. cmd/ main) are
# skipped rather than false-failing. Pre-existing untouched code is not gated.
#
# Usage:
#   pwsh ./scripts/coverage-floor.ps1 -FloorPct 60 -BaseRef origin/main
#   pwsh ./scripts/coverage-floor.ps1 -FloorPct 60          # defaults to HEAD~1
param(
    [int]$FloorPct = 60,
    [string]$BaseRef = "HEAD~1",
    [string]$CoverProfile = "coverage.out"
)
$ErrorActionPreference = "Continue"

if (-not (Test-Path $CoverProfile)) {
    Write-Error "coverage profile '$CoverProfile' not found. Run 'go test -coverprofile $CoverProfile ./...' first."
    exit 2
}

# 1. Changed non-test .go files vs base ref, as repo-relative paths.
$changed = @(git diff --name-only "$BaseRef" -- '*.go' 2>$null | Where-Object { $_ -notmatch '_test\.go$' })
if ($changed.Count -eq 0) {
    Write-Host "coverage-floor: no non-test .go files changed vs $BaseRef - nothing to enforce." -ForegroundColor Green
    exit 0
}

# Map changed file paths -> Go package import paths.
# internal/config/config.go  ->  github.com/manthan8219/nexus-job-assistant/internal/config
$module = ((Get-Content go.mod | Where-Object { $_ -match '^module ' }) -replace '^module ', '').Trim()
$changedPkgs = New-Object System.Collections.Generic.HashSet[string]
foreach ($f in $changed) {
    $dir = (Split-Path $f -Parent) -replace '\\', '/'
    $dir = $dir -replace '^\.\/', ''
    $pkg = if ($dir -eq '') { $module } else { "$module/$dir" }
    $changedPkgs.Add($pkg) | Out-Null
}

# 2. Parse cover -func output. Each line: <pkg>/<file>.go:<line>:<tab><func><tab><pct>%
# `go tool cover -func` lines look like:
#   github.com/.../internal/geo/aliases.go:70:<tab>init<tab><tab>100.0%
$funcOut = go tool cover -func "$CoverProfile"
$pkgStats = @{} # pkg import path -> @{ sum; n }
foreach ($line in $funcOut) {
    if ($line -match '^(.+\.go):\d+:\s+(.+?)\s+([\d.]+)%$') {
        $file = $matches[1]
        $pct = [double]$matches[3]
        $idx = $file.LastIndexOf('/')
        if ($idx -lt 0) { continue }
        $pkg = $file.Substring(0, $idx)
        if (-not $pkgStats.ContainsKey($pkg)) { $pkgStats[$pkg] = @{ sum = 0.0; n = 0 } }
        $pkgStats[$pkg].sum += $pct
        $pkgStats[$pkg].n += 1
    }
}

# 3. Check floor for each changed package.
$failed = $false
foreach ($pkg in ($changedPkgs | Sort-Object)) {
    if ($pkgStats.ContainsKey($pkg)) {
        $s = $pkgStats[$pkg]
        $avg = if ($s.n -gt 0) { $s.sum / $s.n } else { 0.0 }
        $status = if ($avg -ge $FloorPct) { "PASS" } else { "FAIL" }
        $color = if ($avg -ge $FloorPct) { "Green" } else { "Red" }
        $avgStr = "{0:N1}" -f $avg
        Write-Host "$status $avgStr%  $pkg" -ForegroundColor $color
        if ($avg -lt $FloorPct) { $failed = $true }
    } else {
        Write-Host "SKIP  (no coverable funcs)  $pkg" -ForegroundColor DarkGray
    }
}

if ($failed) {
    Write-Error "coverage-floor: one or more changed packages are below $FloorPct% coverage. Add tests."
    exit 1
}
Write-Host "coverage-floor: all changed packages >= ${FloorPct}% coverage." -ForegroundColor Green
