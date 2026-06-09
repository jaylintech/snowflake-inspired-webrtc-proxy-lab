param(
    [ValidateSet("local", "broker", "listener", "client", "build", "test")]
    [string]$Role = "local",

    [string]$Session = "lab-demo",
    [string]$BrokerUrl = "http://127.0.0.1:8080",
    [string]$Listen = ":8080",
    [string]$Stun = "stun:stun.l.google.com:19302",
    [switch]$NoStun,

    [string]$Interval = "8s",
    [int]$Jitter = 35,
    [int]$Count = 8,
    [string]$HostId = "Host_ID_8842_Active",

    [int]$TaskEvery = 2,
    [string]$TaskSequence = "sleep,inventory,synthetic-upload",
    [int]$SyntheticBytes = 8192,
    [int]$ChunkBytes = 1024,
    [string]$TaskDelay = "1s",
    [string]$ChunkDelay = "250ms"
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

function Test-CommandAvailable {
    param([string]$Name)
    return $null -ne (Get-Command $Name -ErrorAction SilentlyContinue)
}

function Invoke-GoOrBinary {
    param(
        [Parameter(Mandatory = $true)]
        [ValidateSet("broker", "listener", "client")]
        [string]$CommandName,

        [string[]]$CommandArgs
    )

    $repoRoot = Get-RepoRoot
    $exePath = Join-Path $repoRoot "bin\$CommandName.exe"

    if (Test-Path $exePath) {
        & $exePath @CommandArgs
        return
    }

    if (Test-CommandAvailable "go") {
        Push-Location $repoRoot
        try {
            & go run ".\cmd\$CommandName" @CommandArgs
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

function Invoke-Listener {
    Invoke-GoOrBinary -CommandName "listener" -CommandArgs @(
        "-broker", $BrokerUrl,
        "-session", $Session,
        "-stun", (Get-StunArg),
        "-task-every", "$TaskEvery",
        "-task-sequence", $TaskSequence,
        "-synthetic-bytes", "$SyntheticBytes",
        "-chunk-bytes", "$ChunkBytes"
    )
}

function Invoke-Client {
    Invoke-GoOrBinary -CommandName "client" -CommandArgs @(
        "-broker", $BrokerUrl,
        "-session", $Session,
        "-stun", (Get-StunArg),
        "-interval", $Interval,
        "-jitter", "$Jitter",
        "-count", "$Count",
        "-host-id", $HostId,
        "-task-delay", $TaskDelay,
        "-chunk-delay", $ChunkDelay
    )
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
        "-Stun", (Get-StunArg),
        "-Interval", $Interval,
        "-Jitter", "$Jitter",
        "-Count", "$Count",
        "-HostId", $HostId,
        "-TaskEvery", "$TaskEvery",
        "-TaskSequence", $TaskSequence,
        "-SyntheticBytes", "$SyntheticBytes",
        "-ChunkBytes", "$ChunkBytes",
        "-TaskDelay", $TaskDelay,
        "-ChunkDelay", $ChunkDelay
    )

    if ($NoStun) {
        $argList += "-NoStun"
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

function Invoke-Build {
    $repoRoot = Get-RepoRoot
    Push-Location $repoRoot
    try {
        if (Test-CommandAvailable "go") {
            go mod tidy
            go test ./...
            go build -o bin ./cmd/...
            return
        }

        if (Test-CommandAvailable "docker") {
            docker run --rm -v "${repoRoot}:/src" -w /src golang:1.22 go test ./...
            docker run --rm -e GOOS=windows -e GOARCH=amd64 -e CGO_ENABLED=0 -v "${repoRoot}:/src" -w /src golang:1.22 go build -o bin ./cmd/...
            return
        }

        throw "Install Go 1.22+ or Docker Desktop, then rerun this build command."
    }
    finally {
        Pop-Location
    }
}

function Invoke-Test {
    $repoRoot = Get-RepoRoot
    Push-Location $repoRoot
    try {
        if (Test-CommandAvailable "go") {
            go test ./...
            return
        }

        if (Test-CommandAvailable "docker") {
            docker run --rm -v "${repoRoot}:/src" -w /src golang:1.22 go test ./...
            return
        }

        throw "Install Go 1.22+ or Docker Desktop to run tests."
    }
    finally {
        Pop-Location
    }
}

switch ($Role) {
    "local" { Invoke-LocalLab }
    "broker" { Invoke-Broker }
    "listener" { Invoke-Listener }
    "client" { Invoke-Client }
    "build" { Invoke-Build }
    "test" { Invoke-Test }
}
