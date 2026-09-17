# Package A dependency inventory

This records application dependencies and verification tools, not a legal opinion or a production artifact SBOM. Vulnerability results belong to the dated delivery evidence. No new dependency is justified by hypothetical future product functionality.

| Application module | Locked version | Purpose | Upstream license reviewed |
| --- | --- | --- | --- |
| Go standard library/toolchain | 1.27.1 | HTTP, configuration, logging, context, crypto identifiers, processes | BSD-style |
| github.com/jackc/pgx/v5 | v5.11.0 | Explicit PostgreSQL queries, transactions and pools | MIT |
| github.com/jackc/pgpassfile | v1.0.0 | pgx credential-file support | MIT |
| github.com/jackc/pgservicefile | v0.0.0-20240606120523-5a60cdf6a761 | pgx connection-service parsing | MIT |
| github.com/jackc/puddle/v2 | v2.2.2 | pgx pool synchronization | MIT |
| golang.org/x/sync | v0.21.0 | Transitive concurrency primitives | BSD-3-Clause |
| golang.org/x/text | v0.39.0 | Transitive encoding support; security-patched floor | BSD-3-Clause |

`go.mod` fixes selected versions, `go.sum` supplies module integrity hashes, and `go mod verify` checks the downloaded module cache. SQL is written explicitly; no ORM, application framework, external queue client or auth vendor SDK is introduced. Third-party notices must accompany future distributable artifacts where licenses require them. The repository's own license and publication policy remain D13.

The initial scan found reachable [GO-2026-5970 / CVE-2026-56852](https://pkg.go.dev/vuln/GO-2026-5970) in the initial indirect `x/text v0.29.0`. It was raised to the fixed `v0.39.0`; module resolution also raised `x/sync` to `v0.21.0`. Verification must rerun after that change. The scan gate converts govulncheck's JSON stream through its official text handler because successful JSON generation alone does not indicate that no vulnerabilities were found.

Verification tools are separate from runtime dependencies: govulncheck v1.8.0 (Go vulnerability database, BSD-style), gosec v2.29.0 (local static analysis, Apache-2.0), gitleaks v8.30.1 (secret scanning, MIT), and openapi-spec-validator 0.7.2 (OpenAPI 3.1 contract validation, Apache-2.0). Scanner use does not imply certification or prove absence of vulnerabilities. No scanner AI mode or upload of application source is used.

See [Windows tooling provenance](../../scripts/dev/TOOLING.md) for Go/PostgreSQL/GCC distribution checks. Scanning application Go dependencies does not scan a PostgreSQL server distribution, operating system, or nonexistent container image. Those scopes must remain distinguishable in release evidence.
