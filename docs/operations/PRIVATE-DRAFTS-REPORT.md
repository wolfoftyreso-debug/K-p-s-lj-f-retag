# Private draft slice — implementation and verification report

Date: 2026-10-07. Approved scope: the six decisions in [the original proposal](../architecture/DRAFT-SLICE-PROPOSAL.md), explicitly accepted by the owner with “Godkänn det föreslagna paketet”. This is a private backend slice verified with synthetic data; it is not a production rollout or a connected seller interface.

## Implementation and repository changes

Separate workspace-owned Business and Listing Draft records can be created, read, replaced and privately previewed through the existing authenticated Go API. Incomplete drafts are permitted; supplied values remain typed, normalized and bounded. Only DRAFT exists. Business and Listing retain distinct IDs and versions. API, worker and migration commands build independently.

The implementation commit is `df44086c0b0772ef515149d40b152a875a95d847`, based on `fe48fe99f54900250b46b3dfa640d30018570e6b`. A subsequent test-fixture correction is `d013a55393db286f35a62319425af201dd7b9efd`; production behavior is unchanged by that correction. Source is published on `codex/private-listing-drafts` in [draft PR 1](https://github.com/wolfoftyreso-debug/K-p-s-lj-f-retag/pull/1), targeting the previously verified foundation branch. Main is not merged. Prior isolated design files remain preserved separately in the local working tree and are not in this backend PR.

The reviewed source adds `internal/listings`, draft transaction code in `internal/store`, protected transport/strict Unicode handling in `internal/httpapi`, the additive migration and real PostgreSQL tests. It extends only the current permission catalogue, runtime schema/readiness guard and finite request body default. New documentation comprises the accepted proposal, [ADR 0008](../adr/0008-private-listing-drafts.md), [operations runbook](PRIVATE-DRAFTS.md) and [scoped threat review](../security/PRIVATE-DRAFTS-THREATS.md). The versioned [OpenAPI contract](../../api/openapi/listing-drafts.json) is authoritative for the five protected method/path combinations. CI now also triggers on approved codex branches; action/image pins and architecture are unchanged.

## Database and migration integrity

Migration 000003 introduces `listings.businesses`, `listings.drafts`, `listings.command_receipts`, `organizations.personal_drafting_assignments` and `audit.aggregate_events`. Typed fields, positive versions, finite enum/state checks, scoped uniqueness, composite ownership/resource/session foreign keys, column grants, forced RLS and revision/audit constraints enforce durable integrity.

Checksums remain:

- 000001: `7875c3fb0dbd59222d80237df65037bbd82a9577162f7c539a7a7f2aedbe1b8e`.
- 000002: `b568cbadcda7c16ed3ec6c5fc45840b39216803302ca51f5513088f8adc40ec3`.
- 000003: `8d5052f4e11a7725ec6833a052368fedc40f4301b256539a0cf9b40434e2f09a`.

Empty-database apply/repeat/drift tests, historical upgrade, seeded 000002→000003 upgrade and non-superuser/no-BYPASSRLS migration-owner helper tests use real PostgreSQL. Existing ownership and grants are preserved; migration grants no existing User implicit listing authority. Migration is operator-controlled, never API startup behavior. Reversal requires a reviewed forward repair preserving records/receipts/evidence, not destructive automatic down migration.

## Authentication and authorization

P0.4 remains the authentication boundary: current HUMAN account/session, derived actor, server request context, cookie/Origin/CSRF enforcement and application-owned workspace grants. There is no new identity vendor, developer identity or visible sign-in screen. Requested IDs are selectors, never authorization facts. Read/Preview require `listing.read_private`; create/update additionally require `listing.create`/`listing.update`. Existing Reader/Editor templates are not expanded.

Explicit first personal drafting creates one default assignment, a PERSON-owned Workspace, active membership and exactly `workspace.read`, `workspace.update`, `listing.read_private`, `listing.create`, `listing.update` atomically. Assignment reuse never restores revoked grants or membership. Owning a workspace, organization affiliation and seller authority remain different concepts. Organization affiliation removal alone does not revoke independently granted workspace access; a future offboarding workflow must manage its intended grants explicitly. No SellerMandate, verified claim or organization-wide access is created.

## Audit, retries and concurrency

Five finite new audit actions identify their typed workspace/resource target, derived HUMAN actor/session, server request/correlation IDs, database timestamp, SUCCESS and previous/current versions. Metadata contains only those versions. Runtime ledger mutation is forbidden. Reverse/deferred checks prevent fabricated revision evidence and committing resource revisions without matching audit. Audit failure rolls back resources, bootstrap grants and creation receipt together. Privileged operators remain outside this append-only runtime guarantee.

Creation receipts are durable and scoped to User/context/workspace/operation/command identity, with a normalized finite request fingerprint. Identical authorized replay returns original IDs/initial versions and `replayed=true`; changed content conflicts. Replay rechecks current account/session/grants, so receipts cannot restore access. Tests exercise duplicate/concurrent creation against real PostgreSQL. COMMIT-return failure is injected at the pgx transaction boundary both after an actual commit and before commit, then durable state and safe retry are inspected. Physical network-partition testing remains outside this evidence. No exactly-once delivery claim is made.

PUT is complete replacement requiring both expected versions. Changed resources alone advance; no-op changes do not create material audit. Stale versions conflict. On an uncertain PUT result, GET and reconcile before changing again. Lock order and current-clock checks after waits preserve revocation/expiry boundaries. Live cancellation during an observed advisory lock wait persists no state, audit, receipt or idle activity; the same command can succeed after release.

## Outbox and worker boundary

Protected Preview is a synchronous authoritative allowlist. No real asynchronous draft responsibility is present, so this slice adds no draft event, projection or consumer. Existing WorkspaceNameChanged outbox, audit and worker semantics remain intact and are included in the full normal/race suites. The worker has no listing/bootstrap access and never impersonates a human. A later real effect requires its own approved atomic event and idempotent consumer.

## Local verification

`scripts/verify.ps1` passed on Windows/amd64 with Go 1.27.1, real PostgreSQL 18.6 and `REQUIRE_INTEGRATION=1`. All required commands ran; no relevant gate was skipped. The [durable evidence summary](private-drafts-evidence.json) records counts, package times, raw-log digests and exact source/CI identity. Raw local output remains ignored under `.tools/evidence`.

| Gate | Exact result |
| --- | --- |
| Format / vet / module integrity | gofmt empty; `go vet ./...` and `go mod verify` passed. |
| Independent builds | API, worker and migration commands passed. |
| `go test -json -count=1 ./...` | 9 tested packages; 88 top-level tests and 326 subtests passed; 0 failed/skipped tests. |
| `go test -race -json -count=1 ./...` | Same 9/88/326 passed; 0 failed/skipped tests and no race report. |
| Migrations / integration | All migration and restricted-role PostgreSQL suites ran, including upgrade, rollback, replay, concurrency, revocation, live cancellation and owner-helper scenarios. |
| Contracts / docs / whitespace | All 3 OpenAPI contracts validated; repository-local links and `git diff --check` passed. Clean-checkout documentation check independently passed 211 local links before the evidence addendum. |
| Dependency vulnerability | govulncheck 1.8.0 analysis plus conversion gate: “No vulnerabilities found.” No application dependency was added; locks/licenses remain the reviewed baseline. |
| Static security | gosec 2.29.0: 20 production files, 3369 lines, 0 findings, 0 errors, 0 nosec suppressions. |
| Secrets | gitleaks 8.30.1 worktree and full Git history: 0 findings each. |

No deployed image, Terraform, provider tenant, production restore, load/SLO or connected frontend exists in this scope; those gates are not applicable here or remain explicit later acceptance, not passing controls.

## Committed clean-checkout CI

[Run 37628527165](https://github.com/wolfoftyreso-debug/K-p-s-lj-f-retag/actions/runs/37628527165), attempt 1, job 112816455226 passed on Ubuntu 24.04 from exact clean source `d013a55393db286f35a62319425af201dd7b9efd`, with pinned Go 1.27.1 and PostgreSQL 18.6 service image. All migration, independent build and required verification steps passed. Normal and race outputs each contain 9 passing tested packages, 88 top-level tests, 326 subtests and 0 failed/skipped tests. Secret findings are 0/0; vulnerability output reports none; SAST has 20 files/3369 lines/0 findings and 0 suppressions. The workflow installs gosec module v2.29.0; its built binary reports the unstamped label `dev`, which is retained honestly in the raw artifact.

Downloaded artifact 11484574619 matched GitHub's SHA-256 `20cd4a0f74017fd6009beb595039b5a2913919374766e95f67018d1d7e1c05d4`. `commit.txt`, `run.txt`, complete test JSON, SAST, dependency and secret results were inspected. Artifact expiry is 2026-10-21; the sanitized evidence summary is durable repository source. Pull-request artifact names use GitHub's merge-context SHA, so the inspected `commit.txt`—not the filename—is the checkout identity.

The initial implementation `df44086…` separately passed run 37627376828; this later inspected result includes the fixture repair. This evidence/status-only addendum will itself receive clean-checkout CI. Its containing commit SHA cannot be embedded here before creation; the final delivery response and CI artifact's `commit.txt` identify that final SHA. No main merge or deployment is performed.

## Failure discovered and corrected

One final Windows race attempt failed `TestOIDCCancellationAndConcurrentExchange` with dependency-unavailable timeout; all draft tests in that attempt passed. The fixture held one mutex while producing twelve RSA signatures, serializing token and key responses under race instrumentation. Failed JSON/log evidence is retained locally under `.tools/evidence/private-drafts-failed-race` rather than relabelled as passed.

The narrow correction pre-signs the immutable valid test token once. Twelve actual simultaneous TLS exchanges, current key retrieval and signature verification remain; exact twelve-token/twelve-JWKS and canonical facts assertions strengthen the test. Production five-second dependency timeout is unchanged; no check is skipped or suppressed. Targeted normal ×3 passed in 7.065s, targeted race ×3 in 8.144s, and the complete identity race suite in 10.649s. The subsequent full-source and CI results above are separate executed evidence.

## Design study and research

This backend package introduces no visible authentication or seller interface. The separately authorized study received fresh Mobbin form-recovery research (`docs/research/ux/2026-10-07-form-recovery.md` in the local study) before keyboard/error refinements. That research and the study remain local design work outside this backend commit. Local study verification passed 17 checks, including 81 JavaScript tests and 25 verification-helper tests, with zero failures/skips and observed keyboard/320px reflow scenarios; its configured workflow has not run in hosted CI. These results do not attest a connected API journey, full accessibility compliance, iPhone hardware or production frontend.

## Deviations and limits

The original Phase 0 plan's later P0.7 is not implemented wholesale. The owner's subsequent narrow approval resolves only the private drafting subset. PUT replaces the proposal's candidate PATCH to make omitted/cleared fields explicit; ADR 0008 documents that reversible transport choice. The finite new ledger supports HUMAN actions actually present; existing machine attribution remains in the prior worker/security architecture. No unused SERVICE draft workflow or generalized event engine is fabricated.

Country validation establishes only two-letter shape, not ISO membership, launch eligibility or jurisdiction. Free text may identify a business; private Preview is not an anonymity certification. Protected no-store/noindex responses do not establish public disclosure safety. No price, financial facts, report freshness rule, upload, publication, buyer access, search, billing, mandate verification, account/team management, production AWS or real-user admission is introduced.

No retention/deletion/quota workflow or approved workload/SLO is supplied. Receipt retention must preserve retry guarantees. Provider conformance, provider-event propagation, privacy/purpose/retention, secure ingress, operating ownership, restore and independent security/accessibility acceptance remain open. Synthetic protocol/SQL tests do not close those gates. Repository licensing, named reviewers/code owners and enforced branch protection remain unresolved governance decisions. Automated security scans are evidence, not compliance certification.

## Next package recommendation

Finish the real-provider synthetic conformance and privacy/retention decisions, then approve connection of the seller interface to these actual private draft contracts with honest save/conflict/recovery behavior. Professional delegation/offboarding needs an understandable product model over explicit grants. Publication and financial/document policy require separate decisions. No recommended next package begins as part of this delivery.
