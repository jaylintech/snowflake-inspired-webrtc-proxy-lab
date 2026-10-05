param(
    [Parameter(Mandatory=$true)]
    [string]$TestId,

    [string]$TurnUrls = "",
    [string]$TurnUsername = "",
    [string]$TurnCredential = "",
    [string]$IcePolicy = "all",
    [switch]$NoStun
)

$ErrorActionPreference = "Stop"

$binDir = Join-Path $PSScriptRoot "..\..\bin"
$brokerExe = Join-Path $binDir "broker.exe"
$targetExe = Join-Path $binDir "target.exe"
$relayExe = Join-Path $binDir "relay.exe"
$webclientExe = Join-Path $binDir "webclient.exe"

Write-Host "========================================="
Write-Host " Starting Part 2 Test: $TestId"
Write-Host " ICE Policy: $IcePolicy"
Write-Host " TURN URLs:  $TurnUrls"
Write-Host "========================================="

# Set environment variables for Pion WebRTC in internal/lab
$env:LAB_TURN_URLS = $TurnUrls
$env:LAB_TURN_USERNAME = $TurnUsername
$env:LAB_TURN_CREDENTIAL = $TurnCredential
$env:LAB_ICE_POLICY = $IcePolicy
$token = "secret-token-$TestId"
$env:LAB_BROKER_TOKEN = $token

$session = "session-$TestId"
$brokerPort = 18080
$targetPort = 19090

$brokerOut = [System.IO.Path]::GetTempFileName()
$brokerErr = [System.IO.Path]::GetTempFileName()
$relayOut = [System.IO.Path]::GetTempFileName()
$relayErr = [System.IO.Path]::GetTempFileName()
$clientOut = [System.IO.Path]::GetTempFileName()
$clientErr = [System.IO.Path]::GetTempFileName()

Write-Host "1. Starting Target on port $targetPort..."
$targetProc = Start-Process -FilePath $targetExe -ArgumentList "-listen", ":$targetPort" -PassThru -NoNewWindow

Write-Host "2. Starting Broker on port $brokerPort..."
$brokerProc = Start-Process -FilePath $brokerExe -ArgumentList "-listen", ":$brokerPort", "-shared-secret", $token -PassThru -NoNewWindow -RedirectStandardOutput $brokerOut -RedirectStandardError $brokerErr

Start-Sleep -Milliseconds 500

Write-Host "3. Starting Relay..."
$relayArgs = @(
    "-broker", "http://127.0.0.1:$brokerPort",
    "-session", $session,
    "-target", "http://127.0.0.1:$targetPort"
)
if ($NoStun) {
    $relayArgs += "-stun", ""
}
$relayProc = Start-Process -FilePath $relayExe -ArgumentList $relayArgs -PassThru -NoNewWindow -RedirectStandardOutput $relayOut -RedirectStandardError $relayErr

Start-Sleep -Milliseconds 500

Write-Host "4. Starting WebClient..."
$webclientArgs = @(
    "-broker", "http://127.0.0.1:$brokerPort",
    "-session", $session,
    "-paths", "/",
    "-timeout", "15s"
)
if ($NoStun) {
    $webclientArgs += "-stun", ""
}

$clientProc = Start-Process -FilePath $webclientExe -ArgumentList $webclientArgs -PassThru -NoNewWindow -RedirectStandardOutput $clientOut -RedirectStandardError $clientErr -Wait

Write-Host "`n=== Broker Log ==="
Get-Content $brokerOut, $brokerErr -ErrorAction SilentlyContinue
Write-Host "`n=== Relay Log ==="
Get-Content $relayOut, $relayErr -ErrorAction SilentlyContinue
Write-Host "`n=== Client Log ==="
Get-Content $clientOut, $clientErr -ErrorAction SilentlyContinue

Write-Host "`n=== Cleaning up processes ==="
Stop-Process -Id $relayProc.Id -Force -ErrorAction SilentlyContinue
Stop-Process -Id $brokerProc.Id -Force -ErrorAction SilentlyContinue
Stop-Process -Id $targetProc.Id -Force -ErrorAction SilentlyContinue

Remove-Item $brokerOut, $brokerErr, $relayOut, $relayErr, $clientOut, $clientErr -ErrorAction SilentlyContinue
Remove-Item Env:LAB_TURN_URLS, Env:LAB_TURN_USERNAME, Env:LAB_TURN_CREDENTIAL, Env:LAB_ICE_POLICY, Env:LAB_BROKER_TOKEN -ErrorAction SilentlyContinue

Write-Host "`n=== Test $TestId Execution Complete ==="
