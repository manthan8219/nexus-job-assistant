# install-hook.ps1 - installs the local pre-push git hook that blocks pushes
# unless the full verify chain (AGENTS.md section 4) passes. CI remains the
# authoritative gate; this is a convenience so broken code never leaves the machine.
#
# Usage:  pwsh ./scripts/install-hook.ps1
#         pwsh ./scripts/install-hook.ps1 -Uninstall
param([switch]$Uninstall)
$ErrorActionPreference = "Stop"

$repo = (git rev-parse --show-toplevel).Trim()
$hooksDir = Join-Path $repo ".git\hooks"
$dest = Join-Path $hooksDir "pre-push"

if ($Uninstall) {
    if (Test-Path $dest) { Remove-Item $dest; Write-Host "removed $dest" -ForegroundColor Yellow }
    else { Write-Host "no pre-push hook installed" -ForegroundColor DarkGray }
    exit 0
}

if (-not (Test-Path $hooksDir)) { New-Item -ItemType Directory -Path $hooksDir | Out-Null }
Copy-Item (Join-Path $repo "scripts\hooks\pre-push") $dest -Force

# Git for Windows runs hooks via bash; the file just needs to be readable.
# On POSIX we would chmod +x; that is a no-op on Windows but harmless in docs.
Write-Host "installed pre-push hook -> $dest" -ForegroundColor Green
Write-Host "verify with a test push; bypass with: git push --no-verify" -ForegroundColor Cyan
