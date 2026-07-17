param(
    [string]$EnvFile = (Join-Path $PSScriptRoot "..\.env")
)

$ErrorActionPreference = "Stop"
$testbedRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$envPath = (Resolve-Path $EnvFile).Path

$values = @{}
foreach ($line in Get-Content -LiteralPath $envPath) {
    $trimmed = $line.Trim()
    if ($trimmed -eq "" -or $trimmed.StartsWith("#")) {
        continue
    }
    $name, $value = $trimmed.Split("=", 2)
    if ($name -and $value) {
        $values[$name.Trim()] = $value.Trim()
    }
}

$required = @("TURN_REALM", "TURN_USERNAME", "TURN_PASSWORD", "TURN_ALLOWED_PEER_IP", "TURN_CERT_DIR")
foreach ($name in $required) {
    if (-not $values.ContainsKey($name) -or [string]::IsNullOrWhiteSpace($values[$name]) -or $values[$name] -eq "CHANGE_ME") {
        throw "Missing or placeholder required value $name in $envPath"
    }
}

$certDirectory = $values["TURN_CERT_DIR"]
if (-not [IO.Path]::IsPathRooted($certDirectory)) {
    $certDirectory = Join-Path $testbedRoot $certDirectory
}
foreach ($name in @("turn-cert.pem", "turn-key.pem")) {
    $path = Join-Path $certDirectory $name
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Missing TURN certificate file: $path"
    }
}

foreach ($path in @("private\mitmproxy", "..\captures\mitmproxy")) {
    $directory = Join-Path $testbedRoot $path
    if (-not (Test-Path -LiteralPath $directory)) {
        New-Item -ItemType Directory -Path $directory | Out-Null
    }
}

Push-Location $testbedRoot
try {
    & docker compose --env-file $envPath -f compose.yaml config --quiet
    if ($LASTEXITCODE -ne 0) {
        throw "docker compose configuration validation failed"
    }
}
finally {
    Pop-Location
}
Write-Host "Part 2 Compose configuration, secrets, and certificate paths are valid."
