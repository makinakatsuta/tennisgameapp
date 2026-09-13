$ErrorActionPreference = 'Stop'

$projectRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
$webDirectory = Join-Path $projectRoot 'web'
$env:GOOS = 'js'
$env:GOARCH = 'wasm'

Push-Location $projectRoot
try {
    go build -o (Join-Path $webDirectory 'tennis-go.wasm') .
} finally {
    Pop-Location
}

Write-Host "WASM build complete: $webDirectory\tennis-go.wasm"
