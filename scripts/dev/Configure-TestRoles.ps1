# Run after applying migrations to the dedicated synthetic test database.
[CmdletBinding()]
param()
$ErrorActionPreference = 'Stop'
$repo = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../..'))
$settings = Get-Content -LiteralPath (Join-Path $repo '.tools/local-postgres/connection.json') -Raw | ConvertFrom-Json
$psql = Join-Path $repo '.tools/pgsql/bin/psql.exe'
$sql = @"
SELECT 'CREATE ROLE packagea_test_api LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS' WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'packagea_test_api')
\gexec
SELECT 'CREATE ROLE packagea_test_worker LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS' WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'packagea_test_worker')
\gexec
ALTER ROLE packagea_test_api PASSWORD '$($settings.ApiPassword)';
ALTER ROLE packagea_test_worker PASSWORD '$($settings.WorkerPassword)';
GRANT foundation_api TO packagea_test_api;
GRANT foundation_worker TO packagea_test_worker;
"@
$previousPassword = $env:PGPASSWORD
try {
    $env:PGPASSWORD = $settings.AdminPassword
    $sql | & $psql -X -q -h 127.0.0.1 -p $settings.Port -U packagea_admin -d foundation_test -v ON_ERROR_STOP=1
    if ($LASTEXITCODE -ne 0) { throw 'Restricted test role setup failed.' }
} finally {
    $env:PGPASSWORD = $previousPassword
    Remove-Variable sql,settings
}
Write-Output 'Restricted local API and worker logins configured.'
