param(
    [Parameter(Mandatory = $true)]
    [string]$Pcap,
    [string]$OutputRoot = "captures\analysis",
    [string]$SuricataImage = $(if ($env:SURICATA_IMAGE) { $env:SURICATA_IMAGE } else { "jasonish/suricata:8.0.5" }),
    [string]$ZeekImage = $(if ($env:ZEEK_IMAGE) { $env:ZEEK_IMAGE } else { "zeek/zeek:8.0.8" }),
    [string]$SuricataRules
)

$ErrorActionPreference = "Stop"

function Get-DockerImageEvidence {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Reference
    )

    $rawMetadata = & docker image inspect $Reference
    if ($LASTEXITCODE -ne 0) {
        throw "Could not inspect Docker image $Reference"
    }
    $metadataItems = @($rawMetadata | ConvertFrom-Json)
    if ($metadataItems.Count -eq 0) {
        throw "Docker returned no metadata for image $Reference"
    }
    $metadata = $metadataItems[0]
    return [ordered]@{
        reference = $Reference
        id = [string]$metadata.Id
        repo_digests = @($metadata.RepoDigests)
    }
}

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
$pcapPath = (Resolve-Path $Pcap).Path
if (-not [IO.Path]::IsPathRooted($OutputRoot)) {
    $OutputRoot = Join-Path $repoRoot $OutputRoot
}
$runName = [IO.Path]::GetFileNameWithoutExtension($pcapPath)
$outputDirectory = Join-Path $OutputRoot $runName
if (Test-Path $outputDirectory) {
    throw "Analysis output already exists: $outputDirectory"
}
$suricataOutput = Join-Path $outputDirectory "suricata"
$zeekOutput = Join-Path $outputDirectory "zeek"
New-Item -ItemType Directory -Path $suricataOutput,$zeekOutput | Out-Null

$pcapDirectory = Split-Path $pcapPath -Parent
$pcapName = Split-Path $pcapPath -Leaf
$suricataArgs = @(
    "run", "--rm", "--network", "none",
    "--mount", "type=bind,source=$pcapDirectory,target=/input,readonly",
    "--mount", "type=bind,source=$suricataOutput,target=/output",
    $SuricataImage,
    "-r", "/input/$pcapName", "-l", "/output"
)
if (-not [string]::IsNullOrWhiteSpace($SuricataRules)) {
    $rulesPath = (Resolve-Path $SuricataRules).Path
    $rulesDirectory = Split-Path $rulesPath -Parent
    $rulesName = Split-Path $rulesPath -Leaf
    $imageIndex = [Array]::IndexOf($suricataArgs, $SuricataImage)
    $beforeImage = $suricataArgs[0..($imageIndex-1)]
    $afterImage = $suricataArgs[$imageIndex..($suricataArgs.Count-1)]
    $suricataArgs = @($beforeImage + @("--mount", "type=bind,source=$rulesDirectory,target=/rules,readonly") + $afterImage + @("-S", "/rules/$rulesName"))
}

Write-Host "Running Suricata offline analysis with $SuricataImage"
& docker @suricataArgs
if ($LASTEXITCODE -ne 0) {
    throw "Suricata analysis failed with exit code $LASTEXITCODE"
}

Write-Host "Running Zeek offline analysis with $ZeekImage"
& docker run --rm --network none `
    --mount "type=bind,source=$pcapDirectory,target=/input,readonly" `
    --mount "type=bind,source=$zeekOutput,target=/output" `
    --workdir /output `
    $ZeekImage zeek -C -r "/input/$pcapName" LogAscii::use_json=T
if ($LASTEXITCODE -ne 0) {
    throw "Zeek analysis failed with exit code $LASTEXITCODE"
}

$suricataImageEvidence = Get-DockerImageEvidence -Reference $SuricataImage
$zeekImageEvidence = Get-DockerImageEvidence -Reference $ZeekImage
$manifest = [ordered]@{
    schema_version = 2
    analyzed_at_utc = (Get-Date).ToUniversalTime().ToString("o")
    git_commit = (git -C $repoRoot rev-parse HEAD).Trim()
    pcap_file = $pcapName
    pcap_sha256 = (Get-FileHash -Algorithm SHA256 -LiteralPath $pcapPath).Hash.ToLowerInvariant()
    suricata_image = $suricataImageEvidence
    zeek_image = $zeekImageEvidence
    custom_suricata_rules = if ($SuricataRules) { (Split-Path $SuricataRules -Leaf) } else { $null }
}
$manifest | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath (Join-Path $outputDirectory "analysis-manifest.json") -Encoding UTF8
Write-Host "Analysis output: $outputDirectory"
