# Phase 0 Package A delivery report

Date: 2026-09-17. **Status: Package A complete within its approved local scope. All applicable local verification commands passed.** Scope is P0.1–P0.3 only; no later package has started. See the [runbook](PACKAGE-A.md) for reproducible commands and operational limits, and [verification evidence](PACKAGE-A-EVIDENCE.json) for source/report hashes and machine-readable result counts. This is not a production release or a claim of Linux deployment verification.

## 1. Exact implementation

Independent Go API and worker processes, shared explicit configuration, separate database credentials, health/readiness, structured logs, bounded requests and graceful cancellation. A separate migration command applies reviewed SQL. Protected internal workspace reads and version-checked name updates use actual PostgreSQL authorization. Updates atomically append audit and outbox records. The independent worker durably projects workspace revisions and records idempotency receipts.

No public product endpoints, UI, listings, authentication vendor, production AWS or later package was implemented.

## 2. Repository changes

```text
.github/workflows/package-a.yml
.gitleaks.toml
go.mod / go.sum
cmd/{api,worker,migrate}/
internal/{config,httpapi,lifecycle,store}/
db/migrations/{000001_package_a.sql,embed.go,*_test.go}
api/openapi/package-a.json
scripts/{verify.ps1,check-docs.py,requirements-checks.txt}
scripts/dev/                         Windows tools, PostgreSQL lifecycle/test roles
docs/adr/0002-* through 0005-*
docs/operations/{PACKAGE-A,PACKAGE-A-REPORT,DEPENDENCIES}.md
```

README, repository doctrine and Phase 0 architecture/security/quality/decision documents distinguish current implementation from historical proposals. Runtime tests live beside the code. `.tools/` contains ignored local tools, synthetic database data and raw execution evidence; it is not application source.

## 3. Database schema

One versioned application migration creates ten tables: `identity.users`; `organizations.organizations`, `organization_memberships`, `workspaces`, `workspace_memberships`, `workspace_permissions`; `audit.events`; `eventing.outbox`, `consumer_receipts`, `workspace_revisions`. The migration runner adds `foundation_schema.migrations` with version, filename, checksum and application time.

PostgreSQL enforces typed owner XOR and foreign keys, scoped membership/permission keys, positive versions, bounded names/event payloads, permitted states, lease/delivery consistency, strict audit metadata and composite workspace/audit/version/event references. Two NOLOGIN capability roles separate API from worker; separately created runtime logins inherit the respective capability. Runtime identities cannot create schema or administer memberships.

## 4. Authorization

Default deny. An active user must hold active membership and the exact permission for the requested workspace. Ownership and organization membership are not bypasses. Updates also require read permission. Application commands set actor/scope transaction-locally; forced RLS and a narrow fixed-search-path authorization function enforce scope and lock authorization rows against concurrent revocation. Missing, foreign and guessed workspace IDs produce the same not-found result.

The normal HTTP composition denies protected requests and ignores spoofed identity/tenant headers. No authentication adapter is being represented as implemented. Database runtime startup rejects privileged/mixed roles and identity masking through role/session switching. RLS is defense in depth, not protection against a privileged database operator or a compromised trusted application identity boundary.

## 5. Audit

The durable application ledger records actor, action, workspace, Workspace target, database timestamp, request/correlation IDs, result and previous/current resource versions. Metadata is constrained to those versions. It stores no workspace name, arbitrary request body, credentials or document content. The API has insert-only audit privileges; the worker has no audit-write capability. This package records successful material workspace changes, not a general login/denied-action ledger or administrator-proof archival system.

## 6. Outbox

State, audit and outbox commit in one PostgreSQL transaction. The worker polls durable rows using `SKIP LOCKED`, unique lease tokens, database-clock expiry and bounded retries. Unsupported/exhausted events remain visible as `DEAD`. Recovery handles expired leases and a lowered retry budget. Queue/dead-letter counts are measured from PostgreSQL. SQS transport and managed DLQs are not implemented; no in-memory substitute or exactly-once claim exists.

## 7. Idempotency

A unique `(consumer,event_id)` receipt and monotonic revision projection commit together. Delivery acknowledgement follows in a separate step. Duplicate delivery after a lost acknowledgement finds the durable receipt and cannot repeat the projection effect; older events cannot regress its version. Recovery reads receipts in a fresh Read Committed snapshot after locking exhausted events. This guarantee covers this local transactional effect, not future external services.

## 8. Tests and exact results

The final `scripts/verify.ps1` run returned **exit 0** on Windows/amd64 with Go **1.27.1**, PostgreSQL **18.6**, GCC **16.2.0** and `REQUIRE_INTEGRATION=1`. Both normal and race runs used `-count=1`, so cached test results could not satisfy the gate.

| Check | Exact result |
| --- | --- |
| `gofmt -l` over all 24 Go source/test files | Passed; no unformatted paths |
| `go mod verify` | Passed; all selected modules verified |
| `go vet ./...` | Passed; exit 0 |
| Independent `go build` of API, worker and migrate | All three passed; exit 0 each |
| `go test -json -count=1 ./...` | 7 packages passed; 43 top-level tests and 54 subtests passed; 0 failed, 0 skipped tests |
| `go test -race -json -count=1 ./...` | Same 7 packages, 43 top-level tests and 54 subtests passed; 0 failed, 0 skipped tests; no race report |
| Real PostgreSQL integration | 5 top-level integration tests passed in both runs, including 20 store scenarios plus real API/worker lifecycle and migration suites |
| Migration command | Initial application and repeat succeeded; ledger contains exactly version 1 with the expected filename/checksum |
| Migration failure behavior | Real empty-database, repeat, applied-checksum drift rejection, failed SQL/history rollback and retry tests passed |
| Operational/schema checks | PostgreSQL start/stop/restart, invalid-password denial, restricted role checks and ten-table DDL preflight passed; 0 residual disposable test databases |
| OpenAPI | openapi-spec-validator 0.7.2 accepted the 3.1 health contract |
| Repository tooling | Six PowerShell files and workflow YAML parsed; local Markdown links and `git diff --check` passed |

`cmd/migrate` has no Go test files, so Go emits one package-level “no test files” skip. It is not a skipped integration test: the command compiled and ran against PostgreSQL, and the migration package's unit/integration tests passed. The test counts above separate top-level tests from subtests rather than presenting their sum as independent scenarios.

The store scenarios cover ownership constraints; cross-workspace reads/mutations and guessed identifiers; organization/owner non-bypass; permission and membership revocation; pooled-context cleanup; forbidden scope/audit/envelope changes; composite FKs; atomic success/conflict; audit/outbox failure rollback; concurrent updates; duplicate, reordered and crashed delivery; final-attempt lost acknowledgement; poison isolation; projection rollback/retry; attempt exhaustion and lowered retry budgets; cancellation and concurrent claims.

Migration SHA-256, matching the durable ledger: `7875c3fb0dbd59222d80237df65037bbd82a9577162f7c539a7a7f2aedbe1b8e`.

Initial attempts during incomplete tool installation are not counted as passes. The first full suite exposed malformed encoding in a new security test fixture: pgx treats `+` literally in a URI, so the fixture needed `%20` for spaces. That fixture was corrected without weakening the required `ErrUnsafeRole` assertion; the targeted regression and final normal/race suites then passed.

## 9. Security checks

| Security check | Exact final result |
| --- | --- |
| govulncheck v1.8.0 | JSON analysis plus official text conversion gate exited 0: “No vulnerabilities found.” Database last modified 2026-09-15 18:39:25 UTC |
| gosec v2.29.0 | All 8 application packages; 12 production Go files, 1,380 lines; 0 findings, 0 `nosec` suppressions; exit 0 |
| gitleaks v8.30.1, working tree | No leaks; exit 0; only ignored local tools/data/evidence excluded |
| gitleaks v8.30.1, Git history | 1 commit scanned; no leaks; exit 0 |
| Dependency integrity/licenses | `go mod verify` passed; selected application module license files reviewed and recorded in [DEPENDENCIES.md](DEPENDENCIES.md) |

The initial dependency scan found reachable [GO-2026-5970 / CVE-2026-56852](https://pkg.go.dev/vuln/GO-2026-5970). `golang.org/x/text` was updated from v0.29.0 to the fixed v0.39.0; dependency resolution raised `x/sync` to v0.21.0. All final checks ran after that update. The gate now converts the JSON stream through govulncheck's official text handler: the original vulnerable report demonstrably returned exit 3 on conversion. JSON generation's exit 0 is never treated as security acceptance.

An initially mis-scoped SAST invocation was stopped after it traversed ignored toolchain/cache source. The final scanner derives the exact application package directories from `go list`; all eight application packages are scanned without rule suppression. Go/GCC published distribution checksums were verified. The PostgreSQL archive hash is observed from the official-linked distribution, not publisher signature verification. See [tooling provenance](../../scripts/dev/TOOLING.md). These checks do not certify the Windows host, PostgreSQL distribution, or verification-tool transitive dependencies as vulnerability-free.

## 10. Known limitations

This is a locally verified backend foundation, not a production deployment or assurance certification. Real authentication, membership/ownership lifecycle, broader role catalogue, privacy/retention policy, privileged audit archival, SQS, external-effect idempotency, distributed tracing/metrics export, alert routing, capacity/restore drills and release artifacts remain future work. No real personal or commercial data is required or approved for these tests. Runtime append-only privileges do not prevent a privileged administrator from changing the database.

Linux execution was **not run**: Docker is unavailable and the Windows WSL launcher reports that WSL is not installed. Hosted GitHub Actions was **not run** because source publication/remote execution still awaits D13. Local workflow YAML parsing is not a hosted CI pass. The Windows race result does not replace a required Linux release gate before deployment. Frontend/accessibility, document/payment workflows, container scanning and Terraform checks are **not applicable to the artifacts delivered here**; those capabilities do not exist in Package A. Deployment, restore, production TLS and alert drills remain **not run** because no deployment is part of this approval.

Network failure during commit can leave a client uncertain. Workspace commands use optimistic concurrency, not request-ID deduplication; reread authoritative version before retrying. Migration recovery must inspect durable migration history. Retention/replay are not silently automated because removing receipts or replaying effects can change semantics.

## 11. Differences from the original plan

The 2026-09-17 approval supersedes the proposed frontend shell: no frontend was created. Unused money/deadline abstractions were not added. The asynchronous implementation is explicit PostgreSQL dispatch with a real local revision effect; SQS transport is deferred to approved infrastructure work. Container builds/scans, OpenTelemetry export and production operating proofs remain later delivery work. Hosted CI is configured but not published/run; D13 remains unresolved. These omissions are explicit and are not passed release gates.

## 12. ADRs

- [0002: runtime and authentication boundary](../adr/0002-package-a-runtime-boundaries.md)
- [0003: workspace ownership and isolation](../adr/0003-workspace-ownership-and-isolation.md)
- [0004: durable outbox and delivery](../adr/0004-durable-outbox-and-delivery.md)
- [0005: reviewed migrations](../adr/0005-reviewed-migrations.md)

## 13. Owner decisions

No new business rule or production infrastructure choice is assumed approved. D01/D02/D04 have the user's Package A approval. Authentication/session/vendor/role/bootstrap policy (D03), privacy and retention (D08), governance/publication (D13), and later infrastructure/release decisions remain open. The process defaults are engineering bounds, not approved SLOs. No future decision is required to run the synthetic local verification.

## 14. Recommended next package and repository state

Recommend P0.4 only after the identity/session/role/ownership-lifecycle decisions are concretely reviewed and approved. A fresh targeted Mobbin pass is required before introducing any account journey. P0.4 has not started.

No commit, push or deployment has been made. The existing base commit is `e048a121492aef8a4c1aa43a004a2fe6550ffac8`; it is the original README baseline, not the implementation SHA. The implementation is an uncommitted working-tree change on `docs/phase-0-foundation`.
