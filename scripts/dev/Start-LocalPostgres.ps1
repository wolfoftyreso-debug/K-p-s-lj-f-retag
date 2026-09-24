# Local synthetic-data development only. Does not install a Windows service.
[CmdletBinding()]
param([ValidateRange(1024,65535)][int]$Port = 55439)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$repo = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../..'))
$toolsDir = Join-Path $repo '.tools'
$pgBin = Join-Path $toolsDir 'pgsql/bin'
$localDir = Join-Path $toolsDir 'local-postgres'
$dataDir = Join-Path $localDir 'data'
$settingsPath = Join-Path $localDir 'connection.json'

function Assert-LocalPostgresAccess([string]$Path, [Security.Principal.SecurityIdentifier]$UserSid, [switch]$ExplicitOnly) {
    $items = @(Get-Item -LiteralPath $Path -Force)
    $items += @(Get-ChildItem -LiteralPath $Path -Force -Recurse -ErrorAction Stop)
    foreach ($item in $items) {
        if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw 'Local PostgreSQL files must not contain reparse points; inspect the local directory manually.'
        }
        $acl = Get-Acl -LiteralPath $item.FullName -ErrorAction Stop
        $descriptor = [Security.AccessControl.RawSecurityDescriptor]::new($acl.GetSecurityDescriptorBinaryForm(), 0)
        if ($null -eq $descriptor.DiscretionaryAcl -or $acl.GetOwner([Security.Principal.SecurityIdentifier]).Value -ne $UserSid.Value) {
            throw 'Local PostgreSQL files require the current user as owner and an explicit access-control list.'
        }
        foreach ($rule in $acl.GetAccessRules($true, !$ExplicitOnly, [Security.Principal.SecurityIdentifier])) {
            if ($rule.AccessControlType -eq [Security.AccessControl.AccessControlType]::Allow -and $rule.IdentityReference.Value -ne $UserSid.Value) {
                throw 'Unsafe local PostgreSQL access grant found; inspect permissions manually before continuing.'
            }
        }
    }
}

if (!(Test-Path -LiteralPath (Join-Path $pgBin 'pg_ctl.exe'))) { throw 'Run Bootstrap-Windows.ps1 first.' }
$currentIdentity = [Security.Principal.WindowsIdentity]::GetCurrent()
try {
    $identity = $currentIdentity.Name
    $userSid = $currentIdentity.User
} finally { $currentIdentity.Dispose() }
# Do not preserve an unsafe explicit grant on a reused credential/data path.
if (Test-Path -LiteralPath $localDir) { Assert-LocalPostgresAccess $localDir $userSid -ExplicitOnly }
New-Item -ItemType Directory -Force -Path $localDir | Out-Null
# Credentials/data are local user-only files, in the ignored tools directory.
& icacls.exe $localDir /inheritance:r /grant:r ($identity + ':(OI)(CI)F') | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Restricting the local database directory ACL failed.' }
# Check effective grants on every existing descendant, including protected ACLs,
# before reading credentials or touching the database.
Assert-LocalPostgresAccess $localDir $userSid
if (Test-Path -LiteralPath $settingsPath) {
    $settings = Get-Content -LiteralPath $settingsPath -Raw | ConvertFrom-Json
    if ($settings.Port -ne $Port) { throw 'Existing local database uses a different port; use its configured port.' }
} else {
    if (Test-Path -LiteralPath $dataDir) { throw 'Data exists without connection settings; inspect manually instead of overwriting it.' }
    $settings = [ordered]@{
        Port = $Port
        AdminPassword = [Convert]::ToHexString([Security.Cryptography.RandomNumberGenerator]::GetBytes(32)).ToLowerInvariant()
        ApiPassword = [Convert]::ToHexString([Security.Cryptography.RandomNumberGenerator]::GetBytes(32)).ToLowerInvariant()
        WorkerPassword = [Convert]::ToHexString([Security.Cryptography.RandomNumberGenerator]::GetBytes(32)).ToLowerInvariant()
    }
    $settings | ConvertTo-Json | Set-Content -LiteralPath $settingsPath -Encoding utf8
}
if (!(Test-Path -LiteralPath (Join-Path $dataDir 'PG_VERSION'))) {
    $passwordFile = Join-Path $localDir 'init-password'
    try {
        $settings.AdminPassword | Set-Content -LiteralPath $passwordFile -Encoding utf8
        & (Join-Path $pgBin 'initdb.exe') -D $dataDir -U packagea_admin --encoding=UTF8 --locale=C --auth-host=scram-sha-256 --auth-local=scram-sha-256 --pwfile=$passwordFile
        if ($LASTEXITCODE -ne 0) { throw 'initdb failed.' }
    } finally {
        if (Test-Path -LiteralPath $passwordFile) { Remove-Item -LiteralPath $passwordFile -Force }
    }
    @"
listen_addresses = '127.0.0.1'
port = $Port
timezone = 'UTC'
password_encryption = 'scram-sha-256'
log_statement = 'none'
"@ | Add-Content -LiteralPath (Join-Path $dataDir 'postgresql.conf') -Encoding utf8
}
& (Join-Path $pgBin 'pg_ctl.exe') status -D $dataDir *> $null
$statusCode = $LASTEXITCODE
if ($statusCode -notin @(0, 3)) { throw "Could not determine local PostgreSQL status (exit $statusCode)." }
if ($statusCode -eq 3) {
    $serverLog = Join-Path $localDir 'server.log'
    $startOut = Join-Path $localDir 'start.stdout.log'
    $startErr = Join-Path $localDir 'start.stderr.log'
    $process = Start-Process -FilePath (Join-Path $pgBin 'pg_ctl.exe') -ArgumentList @('start','-D',('"' + $dataDir + '"'),'-l',('"' + $serverLog + '"'),'-w','-t','30') -WindowStyle Hidden -PassThru -RedirectStandardOutput $startOut -RedirectStandardError $startErr
    # Start-Process -Wait waits for descendant processes on Windows, including
    # the long-lived database. Wait only for pg_ctl itself.
    if (!$process.WaitForExit(40000)) { throw "PostgreSQL startup did not complete; inspect $serverLog." }
    if ($process.ExitCode -ne 0) { throw "PostgreSQL startup failed; inspect $startErr and $serverLog." }
}
$previousPassword = $env:PGPASSWORD
try {
    $env:PGPASSWORD = $settings.AdminPassword
    $exists = & (Join-Path $pgBin 'psql.exe') -X -h 127.0.0.1 -p $Port -U packagea_admin -d postgres -At -v ON_ERROR_STOP=1 -c "SELECT 1 FROM pg_database WHERE datname = 'foundation_test'"
    if ($LASTEXITCODE -ne 0) { throw 'PostgreSQL admin connection failed.' }
    if ($exists -ne '1') {
        & (Join-Path $pgBin 'createdb.exe') -h 127.0.0.1 -p $Port -U packagea_admin foundation_test
        if ($LASTEXITCODE -ne 0) { throw 'Local test database creation failed.' }
    }
} finally { $env:PGPASSWORD = $previousPassword }
Write-Output "Local PostgreSQL is ready on 127.0.0.1:$Port (foundation_test, synthetic data only)."
