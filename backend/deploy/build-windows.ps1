param(
  [string]$OutputDir = "dist"
)

$ErrorActionPreference = "Stop"

$root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $root

New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $OutputDir "configs") | Out-Null

go mod tidy
go build -trimpath -ldflags "-s -w" -o (Join-Path $OutputDir "edge-backend.exe") ./cmd/edge-backend

Copy-Item -Force "configs/config.example.json" (Join-Path $OutputDir "configs/config.example.json")
if (Test-Path "configs/config.json") {
  Copy-Item -Force "configs/config.json" (Join-Path $OutputDir "configs/config.json")
}
Copy-Item -Force "deploy/schema.sql" (Join-Path $OutputDir "schema.sql")

Write-Host "Built $OutputDir/edge-backend.exe"
