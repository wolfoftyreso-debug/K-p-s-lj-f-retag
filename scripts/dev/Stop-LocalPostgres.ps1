[CmdletBinding()]
param()
$ErrorActionPreference = 'Stop'
$repo = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../..'))
$dataDir = Join-Path $repo '.tools/local-postgres/data'
$pgCtl = Join-Path $repo '.tools/pgsql/bin/pg_ctl.exe'
if (!(Test-Path -LiteralPath $pgCtl)) { throw 'Local PostgreSQL binaries are not installed.' }
if (!(Test-Path -LiteralPath (Join-Path $dataDir 'PG_VERSION'))) { throw 'Local PostgreSQL data has not been initialized.' }
& $pgCtl status -D $dataDir *> $null
if ($LASTEXITCODE -eq 0) {
    & $pgCtl stop -D $dataDir -m fast -w -t 30
    if ($LASTEXITCODE -ne 0) { throw 'Local PostgreSQL shutdown failed.' }
} else { Write-Output 'Local PostgreSQL is already stopped.' }
