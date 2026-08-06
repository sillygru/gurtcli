#!/usr/bin/env pwsh
# gurtcli — Windows install script
# Run: irm https://github.com/sillygru/gurtcli/releases/latest/download/install.ps1 | iex

$Repo = "sillygru/gurtcli"
$BinaryName = "gurtcli.exe"
$InstallDir = "$env:USERPROFILE\.local\bin"

# ---- helpers ----

function Die($msg) {
    Write-Error "error: $msg"
    exit 1
}

# ---- platform detection ----

$Arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    "AMD64" { "amd64" }
    "ARM64" { "arm64" }
    default { Die "unsupported architecture: $env:PROCESSOR_ARCHITECTURE" }
}

# ---- fetch latest release tag ----

Write-Host "Fetching latest release..."

$ApiUrl = "https://api.github.com/repos/$Repo/releases/latest"
$Release = Invoke-RestMethod -Uri $ApiUrl -UseBasicParsing
$Tag = $Release.tag_name -replace "^v", ""
$Version = $Tag

if (-not $Version) {
    Die "could not determine latest release version"
}

Write-Host "Found gurtcli v$Version"

# ---- download ----

$ArchiveName = "gurtcli_${Version}_windows_${Arch}.zip"
$BaseUrl = "https://github.com/$Repo/releases/download/v$Version"
$ArchiveUrl = "$BaseUrl/$ArchiveName"
$ChecksumsUrl = "$BaseUrl/checksums.txt"

$TmpDir = Join-Path $env:TEMP "gurtcli-$(Get-Random)"
$ArchivePath = Join-Path $TmpDir $ArchiveName
$ChecksumsPath = Join-Path $TmpDir "checksums.txt"

New-Item -ItemType Directory -Force -Path $TmpDir | Out-Null

Write-Host "Downloading $ArchiveName..."
$Attempt = 0
do {
    $Attempt++
    try {
        Invoke-WebRequest -Uri $ArchiveUrl -OutFile $ArchivePath -UseBasicParsing -ErrorAction Stop
        break
    } catch {
        if ($Attempt -ge 3) { throw }
        Write-Host "  Download failed (attempt $Attempt/3), retrying in $([math]::Pow(2, $Attempt - 1))s..."
        Start-Sleep -Seconds ([math]::Pow(2, $Attempt - 1))
    }
} while ($true)

Write-Host "Verifying checksum..."
try {
    Invoke-WebRequest -Uri $ChecksumsUrl -OutFile $ChecksumsPath -UseBasicParsing -ErrorAction Stop
    $Checksums = Get-Content $ChecksumsPath
    $ExpectedLine = $Checksums | Where-Object { $_ -match [regex]::Escape($ArchiveName) }
    if ($ExpectedLine) {
        $ExpectedHash = ($ExpectedLine -split "\s+")[0]
        $ActualHash = (Get-FileHash -Path $ArchivePath -Algorithm SHA256).Hash.ToLower()
        if ($ExpectedHash -ne $ActualHash) {
            Die "checksum mismatch for $ArchiveName"
        }
        Write-Host "  Checksum verified"
    }
} catch {
    Write-Host "  (no checksum file to verify against)"
}

# ---- extract ----

Write-Host "Extracting..."
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null

$ExtractDir = Join-Path $TmpDir "extracted"
New-Item -ItemType Directory -Force -Path $ExtractDir | Out-Null

try {
    Expand-Archive -Path $ArchivePath -DestinationPath $ExtractDir -Force
} catch {
    # Fallback: try tar if Expand-Archive fails
    tar -xf $ArchivePath -C $ExtractDir 2>$null
}

# Find the binary (may be in a subdirectory inside the archive)
$BinPath = Get-ChildItem -Path $ExtractDir -Recurse -Filter $BinaryName | Select-Object -First 1 -ExpandProperty FullName
if (-not $BinPath) {
    Die "binary not found in archive"
}

$Installed = Join-Path $InstallDir $BinaryName
Move-Item -Path $BinPath -Destination $Installed -Force

Write-Host "gurtcli v$Version installed to $Installed"

# ---- PATH check ----

$UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($UserPath -notlike "*$InstallDir*") {
    $NewPath = "$InstallDir;$UserPath"
    [Environment]::SetEnvironmentVariable("Path", $NewPath, "User")
    # Update current session too
    $env:Path = "$InstallDir;$env:Path"
    Write-Host ""
    Write-Host "  Added $InstallDir to your PATH."
    Write-Host "  Restart your terminal or run: `$env:Path = `"$InstallDir;`$env:Path`""
    Write-Host ""
}

Write-Host "Run 'gurtcli' to start."