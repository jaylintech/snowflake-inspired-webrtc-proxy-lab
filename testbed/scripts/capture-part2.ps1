param(
    [string]$Interface,
    [ValidateRange(10, 3600)]
    [int]$DurationSeconds = 120,
    [ValidatePattern('^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$')]
    [string]$RunId = ("p2-" + (Get-Date).ToUniversalTime().ToString("yyyyMMddTHHmmssZ")),
    [string]$OutputRoot = "captures",
    [string]$CaptureFilter = "udp port 3478 or tcp port 3478 or tcp port 443 or udp portrange 49160-49200 or tcp port 8080 or tcp port 8081",
    [switch]$ListInterfaces
)

$ErrorActionPreference = "Stop"

$dumpcap = Get-Command "dumpcap" -ErrorAction SilentlyContinue
if (-not $dumpcap) {
    throw "dumpcap was not found. Install Wireshark with Npcap, then reopen the terminal."
}

if ($ListInterfaces) {
    & $dumpcap.Source -D
    exit $LASTEXITCODE
}
if ([string]::IsNullOrWhiteSpace($Interface)) {
    throw "-Interface is required. Use -ListInterfaces to enumerate capture interfaces."
}

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
if (-not [IO.Path]::IsPathRooted($OutputRoot)) {
    $OutputRoot = Join-Path $repoRoot $OutputRoot
}
$runDirectory = Join-Path $OutputRoot $RunId
if (Test-Path $runDirectory) {
    throw "Run directory already exists: $runDirectory"
}
New-Item -ItemType Directory -Path $runDirectory | Out-Null

$capturePath = Join-Path $runDirectory "$RunId.pcapng"
$startedAt = (Get-Date).ToUniversalTime()
$commit = (git -C $repoRoot rev-parse HEAD).Trim()
$dumpcapVersion = (& $dumpcap.Source --version | Select-Object -First 1)

Write-Host "Capturing run $RunId on interface $Interface for $DurationSeconds seconds"
& $dumpcap.Source -i $Interface -f $CaptureFilter -a "duration:$DurationSeconds" -w $capturePath
if ($LASTEXITCODE -ne 0) {
    throw "dumpcap failed with exit code $LASTEXITCODE"
}

$endedAt = (Get-Date).ToUniversalTime()
$hash = Get-FileHash -Algorithm SHA256 -LiteralPath $capturePath
$metadata = [ordered]@{
    schema_version = 1
    run_id = $RunId
    git_commit = $commit
    started_at_utc = $startedAt.ToString("o")
    ended_at_utc = $endedAt.ToString("o")
    duration_seconds = $DurationSeconds
    interface = $Interface
    capture_filter = $CaptureFilter
    capture_file = [IO.Path]::GetFileName($capturePath)
    capture_sha256 = $hash.Hash.ToLowerInvariant()
    capture_bytes = (Get-Item -LiteralPath $capturePath).Length
    dumpcap_version = $dumpcapVersion
}
$metadataPath = Join-Path $runDirectory "capture-manifest.json"
$metadata | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath $metadataPath -Encoding UTF8

Write-Host "Capture: $capturePath"
Write-Host "SHA-256: $($metadata.capture_sha256)"
Write-Host "Manifest: $metadataPath"
