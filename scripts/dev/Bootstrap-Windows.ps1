# Workspace-local tools only; no machine PATH changes, installers, or services.
# Requires PowerShell 7 and Windows tar.exe. See TOOLING.md for provenance.
[CmdletBinding()]
param([switch]$SkipRaceCompiler)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$repo = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../..'))
$toolsDir = Join-Path $repo '.tools'
$downloads = Join-Path $toolsDir 'downloads'
New-Item -ItemType Directory -Force -Path $downloads | Out-Null
function Get-CheckedArchive([string]$Name, [string]$Uri, [string]$Sha256) {
    $path = Join-Path $downloads $Name
    if (!(Test-Path -LiteralPath $path)) {
        Invoke-WebRequest -Uri $Uri -OutFile $path -TimeoutSec 600
    }
    $actual = (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -ne $Sha256) { throw "Checksum mismatch for $Name; inspect/remove the cached archive manually." }
    return $path
}
function Expand-CheckedArchive([string]$Path, [string[]]$Members = @()) {
    & tar.exe -xf $Path -C $toolsDir @Members
    if ($LASTEXITCODE -ne 0) { throw "Archive extraction failed: $Path" }
}
function Install-CheckedTool(
    [string]$Name, [string]$Archive, [string]$Sha256,
    [string]$Directory, [string]$Executable, [string]$ExpectedVersion,
    [string[]]$VersionArguments, [string[]]$Members = @()
) {
    $markerDir = Join-Path $toolsDir 'install-markers'
    $marker = Join-Path $markerDir ($Name + '.json')
    $installPath = Join-Path $toolsDir $Directory
    $binary = Join-Path $toolsDir $Executable
    if (Test-Path -LiteralPath $installPath) {
        if (!(Test-Path -LiteralPath $marker)) { throw "$Name exists without a completed-install marker. Inspect and explicitly repair the local installation; bootstrap will not overwrite potentially running tools." }
        $record = Get-Content -LiteralPath $marker -Raw | ConvertFrom-Json
        if ($record.ArchiveSHA256 -ne $Sha256 -or $record.Version -ne $ExpectedVersion -or !(Test-Path -LiteralPath $binary)) { throw "$Name installation marker does not match the required tool; explicit local repair is required." }
        if ((Get-FileHash -LiteralPath $binary -Algorithm SHA256).Hash -ne $record.BinarySHA256) { throw "$Name executable differs from its completed installation; explicit local repair is required." }
    } else {
        if (Test-Path -LiteralPath $marker) { throw "$Name has a stale installation marker; explicit local repair is required." }
        Expand-CheckedArchive $Archive $Members
    }
    $version = @(& $binary @VersionArguments)
    if ($LASTEXITCODE -ne 0 -or $version[0] -ne $ExpectedVersion) { throw "$Name version verification failed; explicit local repair is required." }
    if (!(Test-Path -LiteralPath $marker)) {
        New-Item -ItemType Directory -Force -Path $markerDir | Out-Null
        @{ ArchiveSHA256 = $Sha256; BinarySHA256 = (Get-FileHash -LiteralPath $binary -Algorithm SHA256).Hash; Version = $ExpectedVersion } | ConvertTo-Json | Set-Content -LiteralPath $marker -Encoding utf8
    }
    Write-Output "$Name verified; completed installation available."
}
$goArchive = Get-CheckedArchive 'go1.27.1.windows-amd64.zip' 'https://go.dev/dl/go1.27.1.windows-amd64.zip' 'a3911b5e0e1b1053f25ed0675f4c1c6aad1e2bfcf253df2b9be4caabd2edd95d'
Install-CheckedTool 'go1.27.1' $goArchive 'a3911b5e0e1b1053f25ed0675f4c1c6aad1e2bfcf253df2b9be4caabd2edd95d' 'go' 'go/bin/go.exe' 'go version go1.27.1 windows/amd64' @('version')
$pgArchive = Get-CheckedArchive 'postgresql-18.6-windows-x64.zip' 'https://get.enterprisedb.com/postgresql/postgresql-18.6-3-windows-x64-binaries.zip' '59f8ce701c63c2ed623c665a5e51b3ef6f2e37ccf837b68ffeed0742d0ae6abd'
Install-CheckedTool 'postgresql18.6' $pgArchive '59f8ce701c63c2ed623c665a5e51b3ef6f2e37ccf837b68ffeed0742d0ae6abd' 'pgsql' 'pgsql/bin/postgres.exe' 'postgres (PostgreSQL) 18.6' @('--version') @('pgsql/bin', 'pgsql/lib', 'pgsql/share')
if (!$SkipRaceCompiler) {
    $gccArchive = Get-CheckedArchive 'mingw64.7z' 'https://github.com/niXman/mingw-builds-binaries/releases/download/16.2.0-rt_v14-rev1/x86_64-16.2.0-release-posix-seh-ucrt-rt_v14-rev1.7z' '26788998d615856a915e1eeacb0854fce87bc608fa9a0c16f96bcbfdcf3415c0'
    Install-CheckedTool 'gcc16.2.0' $gccArchive '26788998d615856a915e1eeacb0854fce87bc608fa9a0c16f96bcbfdcf3415c0' 'mingw64' 'mingw64/bin/gcc.exe' 'gcc.exe (x86_64-posix-seh-rev1, Built by MinGW-Builds project) 16.2.0' @('--version')
}
