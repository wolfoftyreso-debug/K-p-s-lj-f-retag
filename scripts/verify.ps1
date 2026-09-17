param()
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

# Local toolchains, credentials and test data are never repository source.
# Check the index before any other gate so force-added ignored paths fail too.
$taskTrackedTools = @(& git ls-files --cached -- '.tools' ':(glob)**/.tools' ':(glob)**/.tools/**')
if ($LASTEXITCODE -ne 0) { throw 'Cannot enumerate tracked paths for the local-tooling exclusion gate' }
if ($taskTrackedTools.Count -gt 0) { throw 'Local .tools paths must not be tracked or staged in Git' }
Write-Output 'PASS no tracked local tooling or data paths'

foreach ($taskTool in @('go', 'gofmt', 'govulncheck', 'gosec', 'gitleaks', 'python', 'openapi-spec-validator')) {
    if (-not (Get-Command $taskTool -ErrorAction SilentlyContinue)) { throw "Required verification tool missing: $taskTool" }
}
foreach ($taskVariable in @('TEST_DATABASE_URL', 'TEST_API_DATABASE_URL', 'TEST_WORKER_DATABASE_URL')) {
    if (-not [Environment]::GetEnvironmentVariable($taskVariable)) { throw "Required integration configuration missing: $taskVariable" }
}
$env:REQUIRE_INTEGRATION = '1'
$env:GOTOOLCHAIN = 'local'
$taskEvidence = Join-Path $PSScriptRoot '../.tools/evidence'
New-Item -ItemType Directory -Path $taskEvidence -Force | Out-Null

& python scripts/check-docs.py
if ($LASTEXITCODE -ne 0) { throw 'documentation link check failed' }
& openapi-spec-validator api/openapi/package-a.json
if ($LASTEXITCODE -ne 0) { throw 'OpenAPI contract validation failed' }

$taskSources = @(git ls-files --cached --others --exclude-standard -- '*.go' | Sort-Object -Unique)
if ($LASTEXITCODE -ne 0 -or $taskSources.Count -eq 0) { throw 'Cannot enumerate Go source files' }
$taskUnformatted = @(& gofmt -l $taskSources)
if ($LASTEXITCODE -ne 0 -or $taskUnformatted.Count -gt 0) { throw ('gofmt failed: ' + ($taskUnformatted -join ', ')) }
Write-Output 'PASS gofmt'

& go mod verify
if ($LASTEXITCODE -ne 0) { throw 'go mod verify failed' }
& go vet ./...
if ($LASTEXITCODE -ne 0) { throw 'go vet failed' }
Write-Output 'PASS go vet'
foreach ($taskCommand in @('api', 'worker', 'migrate')) {
    & go build -o (Join-Path $taskEvidence ($taskCommand + '.bin')) ('./cmd/' + $taskCommand)
    if ($LASTEXITCODE -ne 0) { throw ('Independent build failed: ' + $taskCommand) }
    Write-Output ('PASS independent build ' + $taskCommand)
}
& go test -json -count=1 ./... > (Join-Path $taskEvidence 'tests.jsonl')
if ($LASTEXITCODE -ne 0) { throw 'go test/integration failed' }
Write-Output 'PASS go test with required PostgreSQL integration'
& go test -race -json -count=1 ./... > (Join-Path $taskEvidence 'race.jsonl')
if ($LASTEXITCODE -ne 0) { throw 'race detector failed' }
Write-Output 'PASS race detector with required PostgreSQL integration'
& govulncheck -json ./... > (Join-Path $taskEvidence 'govulncheck.jsonl')
if ($LASTEXITCODE -ne 0) { throw 'dependency vulnerability analysis failed' }
# JSON mode is a data stream and returns zero even for reachable findings.
# Convert that SAME analysis through the official text handler, whose exit code
# is 3 for a reachable vulnerability. Never treat JSON generation as a pass.
Get-Content -LiteralPath (Join-Path $taskEvidence 'govulncheck.jsonl') -Raw | & govulncheck -mode=convert > (Join-Path $taskEvidence 'govulncheck.txt')
if ($LASTEXITCODE -ne 0) { throw 'dependency vulnerability gate failed; inspect govulncheck.txt' }
Write-Output 'PASS dependency vulnerability gate'
$taskPackages = @(& go list -f '{{.Dir}}' ./...)
if ($LASTEXITCODE -ne 0 -or $taskPackages.Count -eq 0) { throw 'Cannot enumerate application packages for SAST' }
& gosec -concurrency=2 -fmt=json -out (Join-Path $taskEvidence 'gosec.json') $taskPackages
if ($LASTEXITCODE -ne 0) { throw 'SAST failed' }
& gitleaks dir . --config .gitleaks.toml --redact --report-format json --report-path (Join-Path $taskEvidence 'secrets-worktree.json')
if ($LASTEXITCODE -ne 0) { throw 'working-tree secret scan failed' }
& gitleaks git . --config .gitleaks-history.toml --redact --report-format json --report-path (Join-Path $taskEvidence 'secrets-history.json')
if ($LASTEXITCODE -ne 0) { throw 'Git history secret scan failed' }
& git diff --check
if ($LASTEXITCODE -ne 0) { throw 'diff whitespace check failed' }
Write-Output 'PASS all Package A verification commands'
