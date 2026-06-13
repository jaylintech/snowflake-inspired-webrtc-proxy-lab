param(
    [ValidateSet("local", "relay-local", "proxy-local", "browser-local", "broker", "listener", "client", "target", "relay", "proxy", "webclient", "browserui", "build", "test")]
    [string]$Role = "local",

    [string]$Session = "lab-demo",
    [string]$BrokerUrl = "http://127.0.0.1:8080",
    [string]$Listen = ":8080",
    [string]$TargetListen = ":9090",
    [string]$TargetUrl = "http://127.0.0.1:9090",
    [string]$UiListen = "127.0.0.1:7777",
    [string]$Stun = "stun:stun.l.google.com:19302",
    [switch]$NoStun,
    [int]$MaxBody = 262144,

    [string]$Interval = "8s",
    [int]$Jitter = 35,
    [int]$Count = 8,
    [string]$HostId = "Host_ID_8842_Active",

    [int]$TaskEvery = 2,
    [string]$TaskSequence = "sleep,inventory,synthetic-upload",
    [int]$SyntheticBytes = 8192,
    [int]$ChunkBytes = 1024,
    [string]$TaskDelay = "1s",
    [string]$ChunkDelay = "250ms",

    [string]$Paths = "/,/healthz,/article-proof?via=webrtc",
    [ValidateSet("GET", "POST")]
    [string]$Method = "GET",
    [string]$Body = "synthetic proxy lab body"
)

$ErrorActionPreference = "Stop"

function Get-RepoRoot {
    return (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
}

function Get-StunArg {
    if ($NoStun) {
        return ""
    }
    return $Stun
}

function Get-StunNativeArgs {
    if ($NoStun) {
        return @("-stun=")
    }
    return @("-stun", $Stun)
}

function Test-CommandAvailable {
    param([string]$Name)
    return $null -ne (Get-Command $Name -ErrorAction SilentlyContinue)
}

function Get-GoCommand {
    $cmd = Get-Command "go" -ErrorAction SilentlyContinue
    if ($cmd) {
        return $cmd.Source
    }

    $defaultGo = "C:\Program Files\Go\bin\go.exe"
    if (Test-Path $defaultGo) {
        return $defaultGo
    }

    return $null
}

function Invoke-GoOrBinary {
    param(
        [Parameter(Mandatory = $true)]
        [ValidateSet("broker", "listener", "client", "target", "relay", "webclient", "browserui")]
        [string]$CommandName,

        [string[]]$CommandArgs
    )

    $repoRoot = Get-RepoRoot
    $exePath = Join-Path $repoRoot "bin\$CommandName.exe"

    if (Test-Path $exePath) {
        & $exePath @CommandArgs
        return
    }

    $go = Get-GoCommand
    if ($go) {
        Push-Location $repoRoot
        try {
            & $go run ".\cmd\$CommandName" @CommandArgs
        }
        finally {
            Pop-Location
        }
        return
    }

    throw "No $CommandName.exe found in bin and Go is not installed. Run: .\scripts\run-lab.ps1 -Role build"
}

function Invoke-Broker {
    Invoke-GoOrBinary -CommandName "broker" -CommandArgs @("-listen", $Listen)
}

function Invoke-Target {
    Invoke-GoOrBinary -CommandName "target" -CommandArgs @("-listen", $TargetListen)
}

function Invoke-Listener {
    $args = @(
        "-broker", $BrokerUrl,
        "-session", $Session,
        "-task-every", "$TaskEvery",
        "-task-sequence", $TaskSequence,
        "-synthetic-bytes", "$SyntheticBytes",
        "-chunk-bytes", "$ChunkBytes"
    )
    $args += Get-StunNativeArgs
    Invoke-GoOrBinary -CommandName "listener" -CommandArgs $args
}

function Invoke-Client {
    $args = @(
        "-broker", $BrokerUrl,
        "-session", $Session,
        "-interval", $Interval,
        "-jitter", "$Jitter",
        "-count", "$Count",
        "-host-id", $HostId,
        "-task-delay", $TaskDelay,
        "-chunk-delay", $ChunkDelay
    )
    $args += Get-StunNativeArgs
    Invoke-GoOrBinary -CommandName "client" -CommandArgs $args
}

function Invoke-Relay {
    $args = @(
        "-broker", $BrokerUrl,
        "-session", $Session,
        "-target", $TargetUrl,
        "-max-body", "$MaxBody"
    )
    $args += Get-StunNativeArgs
    Invoke-GoOrBinary -CommandName "relay" -CommandArgs $args
}

function Invoke-WebClient {
    $args = @(
        "-broker", $BrokerUrl,
        "-session", $Session,
        "-paths", $Paths,
        "-method", $Method,
        "-body", $Body,
        "-interval", $Interval
    )
    $args += Get-StunNativeArgs
    Invoke-GoOrBinary -CommandName "webclient" -CommandArgs $args
}

function Invoke-BrowserUI {
    $args = @(
        "-listen", $UiListen,
        "-broker", $BrokerUrl,
        "-session", $Session
    )
    $args += Get-StunNativeArgs
    Invoke-GoOrBinary -CommandName "browserui" -CommandArgs $args
}

function Start-LabWindow {
    param(
        [string]$Title,
        [string]$WindowRole
    )

    $argList = @(
        "-NoExit",
        "-ExecutionPolicy", "Bypass",
        "-File", $PSCommandPath,
        "-Role", $WindowRole,
        "-Session", $Session,
        "-BrokerUrl", $BrokerUrl,
        "-Listen", $Listen,
        "-TargetListen", $TargetListen,
        "-TargetUrl", $TargetUrl,
        "-UiListen", $UiListen,
        "-Interval", $Interval,
        "-Jitter", "$Jitter",
        "-Count", "$Count",
        "-HostId", $HostId,
        "-TaskEvery", "$TaskEvery",
        "-TaskSequence", $TaskSequence,
        "-SyntheticBytes", "$SyntheticBytes",
        "-ChunkBytes", "$ChunkBytes",
        "-TaskDelay", $TaskDelay,
        "-ChunkDelay", $ChunkDelay,
        "-Paths", $Paths,
        "-Method", $Method,
        "-Body", $Body,
        "-MaxBody", "$MaxBody"
    )

    if ($NoStun) {
        $argList += "-NoStun"
    }
    else {
        $argList += @("-Stun", $Stun)
    }

    Start-Process -FilePath "powershell.exe" -ArgumentList $argList -WindowStyle Normal
    Write-Host "Started $Title window"
}

function Invoke-LocalLab {
    Write-Host "Starting safe WebRTC lab session '$Session'"
    Write-Host "This wrapper emits synthetic lab traffic only; it does not execute commands, collect files, or install persistence."

    Start-LabWindow -Title "broker" -WindowRole "broker"
    Start-Sleep -Seconds 1
    Start-LabWindow -Title "listener" -WindowRole "listener"
    Start-Sleep -Seconds 1
    Start-LabWindow -Title "client" -WindowRole "client"

    Write-Host ""
    Write-Host "Watch the three PowerShell windows for LAB_HELLO, LAB_BEACON, LAB_TASK, LAB_RESULT, and LAB_CHUNK messages."
}

function Invoke-RelayLocalLab {
    Write-Host "Starting bounded WebRTC proxy lab session '$Session'"
    Write-Host "The proxy server only connects to the configured target URL: $TargetUrl"

    Start-LabWindow -Title "broker" -WindowRole "broker"
    Start-Sleep -Seconds 1
    Start-LabWindow -Title "target" -WindowRole "target"
    Start-Sleep -Seconds 1
    Start-LabWindow -Title "proxy" -WindowRole "proxy"
    Start-Sleep -Seconds 1
    Start-LabWindow -Title "webclient" -WindowRole "webclient"

    Write-Host ""
    Write-Host "Watch for proxy request/response logs and target hit logs."
}

function Invoke-BrowserLocalLab {
    Write-Host "Starting bounded WebRTC browser viewer lab session '$Session'"
    Write-Host "The proxy server only connects to the configured target URL: $TargetUrl"

    Start-LabWindow -Title "broker" -WindowRole "broker"
    Start-Sleep -Seconds 1
    Start-LabWindow -Title "target" -WindowRole "target"
    Start-Sleep -Seconds 1
    Start-LabWindow -Title "proxy" -WindowRole "proxy"
    Start-Sleep -Seconds 1
    Start-LabWindow -Title "browserui" -WindowRole "browserui"

    Write-Host ""
    Write-Host "Open http://$UiListen in a browser on this machine."
    Write-Host "Watch for DataChannel logs, proxy request logs, and target hit logs."
}

function Invoke-Build {
    $repoRoot = Get-RepoRoot
    Push-Location $repoRoot
    try {
        $go = Get-GoCommand
        if (-not $go) {
            throw "Go 1.22+ is required. Install it from https://go.dev/dl/, restart PowerShell, then rerun this command."
        }

        New-Item -ItemType Directory -Force -Path "bin" | Out-Null
        & $go mod tidy
        & $go test ./...
        & $go build -o bin ./cmd/...
    }
    finally {
        Pop-Location
    }
}

function Invoke-Test {
    $repoRoot = Get-RepoRoot
    Push-Location $repoRoot
    try {
        $go = Get-GoCommand
        if (-not $go) {
            throw "Go 1.22+ is required. Install it from https://go.dev/dl/, restart PowerShell, then rerun this command."
        }

        & $go test ./...
    }
    finally {
        Pop-Location
    }
}

switch ($Role) {
    "local" { Invoke-LocalLab }
    "relay-local" { Invoke-RelayLocalLab }
    "proxy-local" { Invoke-RelayLocalLab }
    "browser-local" { Invoke-BrowserLocalLab }
    "broker" { Invoke-Broker }
    "listener" { Invoke-Listener }
    "client" { Invoke-Client }
    "target" { Invoke-Target }
    "relay" { Invoke-Relay }
    "proxy" { Invoke-Relay }
    "webclient" { Invoke-WebClient }
    "browserui" { Invoke-BrowserUI }
    "build" { Invoke-Build }
    "test" { Invoke-Test }
}
