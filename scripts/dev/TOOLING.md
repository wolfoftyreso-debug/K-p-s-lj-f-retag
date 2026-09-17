# Local Windows toolchain

These scripts support synthetic tests on this Windows workstation. They do not select a production database version, deploy AWS, install a service, or change machine configuration. PowerShell 7 and Windows `tar.exe` are prerequisites. `.tools/` is ignored by Git; local database credentials and data receive an ACL limited to the current Windows user.

Bootstrap verifies cached/downloaded archive hashes on every run. A completed installation marker records its archive hash, executable hash and exact version; matching installations are reused without extraction. A partial installation, missing/stale marker, changed executable or version mismatch fails explicitly and requires local inspection/repair. Do not run initial installation while tools from that installation are active.

## Pinned download provenance (2026-09-17)

| Tool | Source and checksum evidence | License / purpose |
| --- | --- | --- |
| Go 1.27.1 windows/amd64 | [Official download metadata](https://go.dev/dl/?mode=json), SHA-256 `a3911b5e0e1b1053f25ed0675f4c1c6aad1e2bfcf253df2b9be4caabd2edd95d` | BSD-style Go license; current supported stable compiler/toolchain |
| PostgreSQL 18.6, EDB Windows build 3 | [PostgreSQL Windows page](https://www.postgresql.org/download/windows/) links [EDB archives](https://www.enterprisedb.com/download-postgresql-binaries); its Windows link `https://sbp.enterprisedb.com/getfile.jsp?fileid=1260488` redirected to `https://get.enterprisedb.com/postgresql/postgresql-18.6-3-windows-x64-binaries.zip`. Observed SHA-256 `59f8ce701c63c2ed623c665a5e51b3ef6f2e37ccf837b68ffeed0742d0ae6abd` | PostgreSQL license; real local database semantics. Only bin/lib/share are extracted by bootstrap |
| MinGW-Builds GCC 16.2.0, POSIX/SEH/UCRT, mingw runtime 14 revision 1 | [Maintainer release](https://github.com/niXman/mingw-builds-binaries/releases/tag/16.2.0-rt_v14-rev1), GitHub release-asset SHA-256 `26788998d615856a915e1eeacb0854fce87bc608fa9a0c16f96bcbfdcf3415c0` | GCC GPL with runtime exception; supporting binutils/runtime licenses included upstream. Local C compiler for Go's Windows race detector; not an application dependency |

The PostgreSQL hash pins the archive actually retrieved via the official site's HTTPS-linked EDB distribution; no independently published checksum was available at the attempted `.sha256` URL (HTTP 403), and `postgres.exe` was not Authenticode signed. Do not represent this observed hash as publisher signature verification. Go and GCC archive hashes were compared with their respective published metadata before use. These bootstrap tools are not included in production application images. Vulnerability evidence for application dependencies is reported separately; download verification is not a vulnerability scan.

The initial workstation installation also compared extracted Go file hashes with archive contents through entry 14,000, covering every file in `go/bin`, `go/pkg`, `go/lib` and `go/src`. The remaining entries are upstream `go/test` fixtures; their individual rehash was stopped because they are not inputs to this application's build. The complete downloaded archive's published SHA-256 was verified.

[Go race-detector documentation](https://go.dev/doc/articles/race_detector#Requirements) requires cgo and a compatible C compiler on Windows. This compiler resolves `libsynchronization.a` and contains mingw runtime 14; actual repository race results are recorded in the Package A evidence.

## Commands

From the repository root, in PowerShell 7:

```powershell
./scripts/dev/Bootstrap-Windows.ps1
./scripts/dev/Start-LocalPostgres.ps1
. ./scripts/dev/Set-LocalTestEnvironment.ps1
$env:MIGRATION_DATABASE_URL = $env:TEST_DATABASE_URL
go run ./cmd/migrate apply
./scripts/dev/Configure-TestRoles.ps1
go test -count=1 ./...
go test -race -count=1 ./...
./scripts/dev/Stop-LocalPostgres.ps1
```

Confirm the migration command's configuration contract in its source/runbook if it changes. Runtime commands must use the corresponding restricted API/worker DSN; the administrator DSN is for migrations and test setup only. `Set-LocalTestEnvironment.ps1` only sets variables in the calling shell when dot-sourced. It sets `REQUIRE_INTEGRATION=1` so absent integration configuration fails instead of passing via skipped tests. `sslmode=disable` is only for the loopback synthetic database. PostgreSQL listens only on `127.0.0.1`, uses SCRAM passwords, and retains the local database between runs. Never use these credentials, administrator role, or settings in a deployed environment.

`Stop-LocalPostgres.ps1` performs a fast graceful PostgreSQL shutdown and retains data. No reset/delete command is supplied: deleting a database must be an explicit, separately reviewed operation against a confirmed local target. Backup/restore and production operational proof remain outside this development helper.
