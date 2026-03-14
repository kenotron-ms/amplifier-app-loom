# install.ps1 — download and install agent-daemon on Windows
# Usage: irm https://raw.githubusercontent.com/kenotron-ms/agent-daemon/main/install.ps1 | iex
#   or with a custom install dir:
#   $env:INSTALL_DIR="C:\tools"; irm .../install.ps1 | iex
[CmdletBinding()]
param(
    [string]$InstallDir = "$env:LOCALAPPDATA\Programs\agent-daemon"
)

$ErrorActionPreference = "Stop"
$Repo = "kenotron-ms/agent-daemon"
$Bin = "agent-daemon.exe"
$Asset = "agent-daemon-windows-amd64.exe"

# ── Resolve latest release ─────────────────────────────────────────────────
Write-Host "Fetching latest release..."
$release = Invoke-RestMethod "https://api.github.com/repos/$Repo/releases/latest"
$asset = $release.assets | Where-Object { $_.name -eq $Asset } | Select-Object -First 1

if (-not $asset) {
    Write-Error "Could not find release asset '$Asset'. Check https://github.com/$Repo/releases"
    exit 1
}

# ── Download ───────────────────────────────────────────────────────────────
if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir | Out-Null
}

$dest = Join-Path $InstallDir $Bin
Write-Host "Downloading $Asset..."
Invoke-WebRequest -Uri $asset.browser_download_url -OutFile $dest

Write-Host ""
Write-Host "Installed: $dest"

# ── Add to PATH (user scope) ───────────────────────────────────────────────
$userPath = [Environment]::GetEnvironmentVariable("PATH", "User")
if ($userPath -notlike "*$InstallDir*") {
    [Environment]::SetEnvironmentVariable("PATH", "$InstallDir;$userPath", "User")
    $env:PATH = "$InstallDir;$env:PATH"
    Write-Host "Added $InstallDir to your PATH (user scope)."
    Write-Host "Restart your terminal for the change to take effect."
} else {
    Write-Host "$InstallDir is already in your PATH."
}

Write-Host ""
Write-Host "Next steps:"
Write-Host "  agent-daemon install   # register as a user-level service"
Write-Host "  agent-daemon start     # start the daemon"
Write-Host "  start http://localhost:7700"
