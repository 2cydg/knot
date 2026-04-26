param(
    [string]$Version = $env:KNOT_VERSION,
    [string]$InstallDir = $env:KNOT_INSTALL_DIR,
    [string]$BaseUrl = $env:KNOT_BASE_URL,
    [string]$ManifestUrl = $env:KNOT_MANIFEST_URL
)

$ErrorActionPreference = "Stop"
$DefaultBaseUrl = "https://knot.clay.li/i"

function Fail($Message) {
    [Console]::Error.WriteLine("knot install: $Message")
    exit 1
}

if ([string]::IsNullOrWhiteSpace($InstallDir)) {
    $InstallDir = Join-Path $HOME ".local\bin"
}

if ([string]::IsNullOrWhiteSpace($BaseUrl)) {
    $BaseUrl = $DefaultBaseUrl
}

$arch = $env:PROCESSOR_ARCHITEW6432
if ([string]::IsNullOrWhiteSpace($arch)) {
    $arch = $env:PROCESSOR_ARCHITECTURE
}

switch ($arch.ToUpperInvariant()) {
    "AMD64" { $AssetKey = "windows_amd64" }
    "ARM64" { $AssetKey = "windows_arm64" }
    default { Fail "unsupported CPU architecture: $arch" }
}

$BaseUrl = $BaseUrl.TrimEnd("/")
if ([string]::IsNullOrWhiteSpace($ManifestUrl)) {
    if ([string]::IsNullOrWhiteSpace($Version)) {
        $ManifestUrl = "$BaseUrl/latest.json"
    } else {
        $ManifestUrl = "$BaseUrl/releases/$Version/manifest.json"
    }
}

$TempDir = Join-Path ([System.IO.Path]::GetTempPath()) ("knot-install-" + [System.Guid]::NewGuid().ToString("N"))
$ArchivePath = Join-Path $TempDir "knot.zip"
$ExtractDir = Join-Path $TempDir "extract"

try {
    New-Item -ItemType Directory -Path $TempDir, $ExtractDir -Force | Out-Null

    Write-Host "Downloading manifest: $ManifestUrl"
    $ManifestPath = Join-Path $TempDir "manifest.json"
    Invoke-WebRequest -Uri $ManifestUrl -OutFile $ManifestPath -UseBasicParsing
    $Manifest = Get-Content -Raw -Path $ManifestPath | ConvertFrom-Json

    $AssetProperty = $Manifest.assets.PSObject.Properties[$AssetKey]
    if ($null -eq $AssetProperty) {
        Fail "manifest does not contain asset: $AssetKey"
    }

    $Asset = $AssetProperty.Value
    if ([string]::IsNullOrWhiteSpace($Asset.url)) {
        Fail "manifest does not contain URL for: $AssetKey"
    }
    if ([string]::IsNullOrWhiteSpace($Asset.sha256)) {
        Fail "manifest does not contain sha256 for: $AssetKey"
    }

    Write-Host "Downloading package for $AssetKey"
    Invoke-WebRequest -Uri $Asset.url -OutFile $ArchivePath -UseBasicParsing

    $ActualHash = (Get-FileHash -Algorithm SHA256 -Path $ArchivePath).Hash.ToLowerInvariant()
    $ExpectedHash = $Asset.sha256.ToLowerInvariant()
    if ($ActualHash -ne $ExpectedHash) {
        Fail "checksum mismatch for $AssetKey"
    }

    Expand-Archive -Path $ArchivePath -DestinationPath $ExtractDir -Force
    $SourceExe = Join-Path $ExtractDir "knot.exe"
    if (!(Test-Path $SourceExe)) {
        Fail "package did not contain knot.exe"
    }

    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    $TempExe = Join-Path $InstallDir (".knot.tmp.{0}.exe" -f $PID)
    $TargetExe = Join-Path $InstallDir "knot.exe"

    Copy-Item -Path $SourceExe -Destination $TempExe -Force
    Move-Item -Path $TempExe -Destination $TargetExe -Force

    Write-Host "knot installed to $TargetExe"

    $PathEntries = ($env:PATH -split ";") | Where-Object { $_ -ne "" }
    if ($PathEntries -notcontains $InstallDir) {
        Write-Host "Add $InstallDir to PATH if knot is not found by your shell."
    }
} catch {
    Fail $_.Exception.Message
} finally {
    Remove-Item -Path $TempDir -Recurse -Force -ErrorAction SilentlyContinue
}
