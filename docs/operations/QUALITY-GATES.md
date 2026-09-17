# Quality gates and evidence

Created: 2026-09-16. Updated: 2026-09-17. Package A installs Go/unit/integration/race, migrations, SAST, dependency and secret checks in `scripts/verify.ps1` and a pinned GitHub Actions workflow. Local execution evidence must be recorded separately; workflow configuration is not a hosted CI result. The remaining gates activate with their respective capabilities. Branch protection is unverified, not assumed active.

## Evidence standard

Each change names affected requirements, decisions, tests run and residual risk. Each check result is `passed`, `failed`, `not run` or `not applicable` with a reason and affected scope. A job that skips all tests cannot satisfy a required check. CI evidence identifies the commit, tool versions and artifacts. Test fixtures are synthetic; no sensitive documents or production databases are copied into CI.

The table specifies proposed stable check names and expected behavior for implementation. Exact tooling versions are chosen and pinned in P0.1. A release cannot relabel a relevant failing check as not applicable.

Package A local results are in the [completion report](PACKAGE-A-REPORT.md). Go tests and race detection ran on Windows/amd64 against PostgreSQL 18.6 with required integration enabled. This proves that verification platform; Linux hosted CI and release-artifact verification remain unexecuted and must pass before a Linux deployment. Gates for absent frontend, document, payment, container and Terraform capabilities are not applicable to this package, not passed controls.

| Gate / proposed check | Required evidence | Owner role / activation |
| --- | --- | --- |
| `docs-policy` | Valid local links, no malformed patch/whitespace errors, glossary/decision consistency, honest status; manual review of meaningful changes | Engineering; documentation package |
| `go-quality` | gofmt, go vet, unit tests and Linux race detector where applicable; domain invariants and table-driven negative tests | Backend; first Go package |
| `web-quality` | Locked dependency install, lint, TypeScript strict check, production build, catalog/placeholder checks | Web; first web package |
| `contracts` | OpenAPI validation, compatibility review and generated-client drift check; unknown/oversized/invalid input tests | API owner; first contract |
| `database-integrity` | Empty-database migration and supported previous-schema upgrade against selected real PostgreSQL, checksums, runtime role permissions, FK/unique/check constraints, rollback/fault behavior | Database owner; first migration |
| `tenant-isolation` | Cross-workspace read/write/list/attach denied through API and actual DB roles; absent scope, ID substitution, revoked membership and reused pool connections covered | Security; first protected object |
| `event-integrity` | Atomic state/audit/outbox, repeated/concurrent/reordered delivery, crashed relay/consumer, expired lease, poisoned payload, unique effects | Backend; first asynchronous effect |
| `public-disclosure` | Canary secrets absent from public HTML, React serialization, API JSON, metadata/JSON-LD, index, image metadata/names, logs, caches and notifications; current withdrawal policy demonstrated | Security + web; first public projection |
| `document-security` | Quarantine, content/size limits, type spoofing, malware/failed scan, immutable version binding, cross-workspace grant, expiration/revocation and durable issuance audit | Security; first file capability |
| `payment-integrity` | Signature on exact body, event/account/environment replay, concurrent duplicate/business-key uniqueness, changed/out-of-order payment and entitlement reconciliation | Billing; first platform-payment integration |
| `accessibility` | Automated accessibility scans plus manual keyboard, focus, screen-reader, zoom/reflow, contrast, reduced-motion and form-error evidence for each affected journey | Web/product; first user journey |
| `security-analysis` | Secret scan; dependency/license review; Go vulnerability scan; SAST for Go/TypeScript; explicit scan scope; reviewed findings and exceptions | Security; as source/dependencies exist |
| `artifact-security` | Container vulnerability/configuration scans, non-root runtime, minimal images, SBOM, immutable digest and provenance; no embedded secrets | Platform; first images |
| `terraform-security` | fmt/validate, pinned provider lock, IaC scan, policy checks and reviewed environment-specific plan; state/security review | Infrastructure; first Terraform |
| `release-readiness` | Tests on release artifact, integration smoke, migration compatibility, restore and rollback evidence, tested alert routing, approved deployment scope | Operations; first deployed environment |

## Mandatory scenario matrix

The [threat model](../security/THREAT-MODEL.md) contains detailed adversarial cases. At minimum, implementation must demonstrate these product invariants:

- A member of workspace A cannot read or mutate workspace B, even with a valid B ID, foreign-key attachment attempt, cursor, search filter or background event.
- Authentication does not grant a seller mandate, a mandate does not grant every document permission, and an organization email address is not evidence of transfer authority.
- Money round-trips without float conversion or JavaScript truncation; unknown price differs from zero; currency is never inferred from locale. Deadline instants do not change across browser timezone or daylight-saving transitions.
- Two concurrent writes against one version cannot silently lose an update. Audit failure cannot leave an unaudited successful material write.
- Publication fails if any required gate is unsatisfied. Unsupported NDA or auction conditions remain unavailable; status labels never bypass policy.
- Duplicate Stripe event IDs and different events for the same purchase cannot duplicate entitlements. Queue replay cannot duplicate a business effect or resurrect revoked access.
- Scanner failure never releases a file. Scan approval applies to the exact immutable stored version delivered to the requester.
- Search sorting and cursors are deterministic on the same dataset, public filtering cannot return private values, and paid placement cannot reorder the organic collection invisibly.
- Saved searches persist normalized versioned criteria; notification pause and access revocation are rechecked before sending. Metrics are measured events with definitions, not fabricated numbers.

Use real PostgreSQL for transaction/constraint/isolation tests. Mocks are appropriate for external-provider fault scenarios, not as sole proof of SQL integrity. In-memory adapters cannot demonstrate RLS. AWS/Stripe sandbox contract tests supplement local tests when integration is approved; they do not test live commercial policy automatically.

## CI and repository protection

P0.1 installs the applicable checks, with least-privilege workflow permissions, pinned actions, bounded runtimes and concurrency. Untrusted PR code receives no deployment secrets and cannot publish trusted artifacts. Avoid executing untrusted changes in privileged `pull_request_target` contexts. Production deploy identities use narrowly scoped OIDC trust tied to the repository and approved environment.

P0.6 builds once from an identified commit, scans and records the artifact digest, then promotes that exact artifact. Migrations run as a separate controlled job with their own role. Review the Terraform plan produced for the target account/environment before apply. Drift and manual emergency changes are reconciled into code.

Required reviews, CODEOWNERS identities and branch rules are owner decisions D13. Do not populate fictional GitHub team handles. Configure checks as required only after jobs exist and are proven to report correctly; record an actual settings read-back. A documentation statement that main is protected is not enforcement.

## Acceptance and exceptions

Security-critical isolation, confidential-file exposure, secrets and broken transactional invariants block release. Other findings need risk assessment rather than blind reliance on a scanner score. A time-bounded exception names the control, evidence, risk, compensating measure, accountable approver and remediation date. No undocumented ignore rules or blanket scan suppressions.

Performance acceptance uses an approved workload and SLO (D11), recorded data distribution, query plans and p95/p99 latency/error/resource results. Do not use an arbitrary percentage of test coverage as proof of correctness. Add tests for boundaries and actual risks; avoid tests that merely restate implementation.

Definition of done: acceptance criteria met, applicable checks passed or explicitly accepted exceptions, current docs, decision traceability, rollback/recovery path and inspected evidence. Production readiness adds the [operational gates](READINESS.md).
