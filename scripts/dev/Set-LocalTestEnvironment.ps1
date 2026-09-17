# Dot-source in the shell that will run migrations/tests. Never prints credentials.
$ErrorActionPreference = 'Stop'
$repo = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../..'))
$settingsPath = Join-Path $repo '.tools/local-postgres/connection.json'
if (!(Test-Path -LiteralPath $settingsPath)) { throw 'Run Start-LocalPostgres.ps1 first.' }
$settings = Get-Content -LiteralPath $settingsPath -Raw | ConvertFrom-Json
$endpoint = "127.0.0.1:$($settings.Port)/foundation_test?sslmode=disable"
$env:TEST_DATABASE_URL = "postgres://packagea_admin:$($settings.AdminPassword)@$endpoint"
$env:TEST_API_DATABASE_URL = "postgres://packagea_test_api:$($settings.ApiPassword)@$endpoint"
$env:TEST_WORKER_DATABASE_URL = "postgres://packagea_test_worker:$($settings.WorkerPassword)@$endpoint"
$env:MIGRATION_DATABASE_URL = $env:TEST_DATABASE_URL
$env:REQUIRE_INTEGRATION = '1'
$env:GOTOOLCHAIN = 'local'
$env:GOCACHE = Join-Path $repo '.tools/gocache'
$env:GOPATH = Join-Path $repo '.tools/gopath'
$env:PATH = (Join-Path $repo '.tools/go/bin') + ';' + (Join-Path $repo '.tools/mingw64/bin') + ';' + $env:PATH
$env:CGO_ENABLED = '1'
Remove-Variable settings
Write-Output 'Local test environment loaded (credentials not displayed).'
