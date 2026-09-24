# Phase 0 implementation plan

Original plan: 2026-09-16. Approval/implementation: 2026-09-17. Evidence update: 2026-09-24. Status: **P0.1–P0.3 committed baseline passed clean-checkout CI; P0.4 decision proposal ready, identity implementation awaiting D03. Later packages are not approved.** The original proposal below is retained as planning history. The [completion report](../operations/PACKAGE-A-REPORT.md) records local implementation and evidence; the [release-gate record](../operations/PACKAGE-A-RELEASE-GATE.md) records reproducibility proof and separately verified follow-ups. The concrete [P0.4 proposal](P0.4-IDENTITY-DESIGN.md) supersedes no business/security decision without owner acceptance.

## Approval addendum — 2026-09-17

The user approved only API/worker foundation, PERSON/ORGANIZATION workspace isolation, and atomic audit/outbox with idempotent processing. The later explicit instruction removes the originally proposed Next.js shell from package A. No UI, listing capability, production AWS or unrelated product features are included. Real PostgreSQL tests and exact gate results are mandatory. Historical “pending approval” and “next requested approval” language below describes the original 2026-09-16 proposal; the central decision register records the current approved scope. Implementation ADRs and the eventual Package A verification report distinguish implemented behavior from this historical plan. No later package has been started.

## Outcome and current scope

The result of Phase 0 is a reproducible, testable engineering foundation with demonstrated ownership, authorization, transaction integrity and operational controls. It is not a public marketplace release. No quantity of scaffolding substitutes for proving these boundaries.

The starting repository contains only a README at `e048a121492aef8a4c1aa43a004a2fe6550ffac8`; [inspection evidence](REPOSITORY-INSPECTION.md) records every tracked file, history, tooling and remote-state limitation. This delivery completes the inspection and design package. It does not claim that application, security, infrastructure or test implementation already exists.

## Review package delivered now

- Repository doctrine: [AGENTS.md](../../AGENTS.md), README review map, editor conventions and secret/build ignore patterns.
- Accepted directive constraints: [ADR 0001](../adr/0001-directive-baseline.md).
- [System architecture](SYSTEM.md), [canonical glossary](../domain/GLOSSARY.md), [conceptual model](../domain/MODEL.md), [decision register](DECISIONS.md).
- [Threat model](../security/THREAT-MODEL.md), [security baseline](../security/SECURITY-BASELINE.md), [quality gates](../operations/QUALITY-GATES.md), [operations readiness](../operations/READINESS.md).
- [Experience foundation](../product/EXPERIENCE-FOUNDATION.md), actual [Mobbin search research](../research/ux/2026-09-16-marketplace-search.md), and [requirement traceability](REQUIREMENTS-TRACEABILITY.md).

All control language in these documents describes requirements or proposals unless explicitly marked verified. No production readiness or compliance certification is asserted.

## Approval boundary

The next requested approval is **foundation package A: P0.1–P0.3**, including D01 (module/runtime shape), D02 (Workspace tenancy and database defenses) and D04 (atomic state/audit/outbox and idempotent processing). Approval authorizes local implementation, CI workflow configuration and local equivalent checks with synthetic data. Running CI remotely and publishing source require D13; CI status remains “not run remotely” until then. These choices are detailed, with alternatives, in the decision register.

This first package does not select an identity vendor, enforce a real business publication policy, take payments, process sensitive uploads, launch public journeys or provision AWS. Unresolved decisions remain open. The user explicitly required “Then implement the approved foundation” and to surface material architectural decisions; approval is therefore a required step from the user's directive, not an added skill or tool restriction.

## Sequenced work packages

| Package | Implementable deliverable | Dependencies / owner role | Acceptance evidence |
| --- | --- | --- | --- |
| P0.1 Reproducible process foundation | Go module; API and separately runnable worker; typed configuration, safe startup validation, liveness/readiness, graceful shutdown, bounded HTTP handling, structured logs and trace correlation; minimal Next.js TypeScript shell with English catalog boundary and no public product journey; local developer commands; build containers; initial CI | Package A approval and D01; engineering | Fresh checkout builds with pinned tools, unit/static checks pass, malformed config fails safely, process shutdown and unhealthy dependency behavior demonstrated; no invented UI/data |
| P0.2 Ownership and contract foundation | Minimal User/Organization/Workspace/Membership persistence needed for approved D02, including exactly one typed User-or-Organization managing owner; explicit authorization interface using synthetic test principals only; migration runner, runtime/migration roles, composite keys and proposed RLS; first OpenAPI boundary; safe money/time primitives | P0.1, D02; security + backend | Real PostgreSQL tests prove workspace A cannot read/write/attach B resources, absent/double owner rejected, missing tenant context denies, pool reuse does not leak context, revoked membership denies, owner/organization affiliation grants no bypass, cross-workspace FK fails; exact money/UTC/ambiguous time tests; no production authentication adapter |
| P0.3 Atomic audit and asynchronous foundation | Bounded audit schema, transactional outbox, relay/consumer ports, leased work, durable inbox/consumer deduplication, event schema/version handling, worker retry/DLQ contract and telemetry. Use synthetic ownership-scoped test commands to prove boundaries; do not implement unapproved listing lifecycle | P0.2, D04; backend + security | Fault injection proves state/audit/outbox all-or-nothing; crash after send repeats safely; concurrent duplicates cannot duplicate effect; out-of-order versions do not regress projection; poison payload does not block healthy work; no exactly-once claims |
| P0.4 Authentication and authorization adapter | Approved OIDC/session integration, internal identity mapping, permission catalogue, revocation/MFA/recovery boundary, CSRF protection, abuse limits and secure headers. Mobbin research before account UX | P0.2 plus D03/D08/D12; identity/security | Forged actor/tenant headers denied, issuer/audience checks, session fixation/CSRF/revocation tests, explicit role matrix, no provider-role authority, keyboard-accessible account journey if UI is included |
| P0.5 Document security subsystem | Private upload intent, exact object/version binding, size/type checks, quarantine/scanner adapter, classification, approved download grants, durable issuance audit and deletion hooks; synthetic files only until privacy gates | P0.3/P0.4 plus D05/D08/D15 and D06 for AWS; security + documents | Spoofed type, oversized input, malware, replaced version, scanner outage, expiry and revoked/cross-workspace grant denied; unsigned storage access denied; residual signed-link window documented and accepted |
| P0.6 Infrastructure and delivery proof | Costed Terraform design followed by approved nonproduction AWS environment, least-privilege GitHub OIDC, ECS API/web/worker, RDS, private S3 origins, SQS/DLQs, edge controls, secrets, telemetry, alarms, encrypted locked state, image promotion and restore runbook | P0.1/P0.3; D06/D08/D11/D13; infrastructure + budget | Reviewed plan matches applied state, no public database/protected bucket, scoped identities, commit/digest traceability, health/rollback evidence, alert drill and measured restore; production Multi-AZ design reviewed but no automatic production apply |
| P0.7 Listing and discovery contracts | Approved normalized listing schema with four sale axes, mandate boundary, immutable versions/public allowlist, explicit transition rules, SearchPort plus deterministic PostgreSQL adapter, normalized SavedSearch schema. Public UX only after journey research and product decision | P0.2/P0.3; D07/D10/D15/D16, D03/D08 for real users; product + backend | Invalid transitions rejected, concurrency audited, public leak fixtures checked across HTTP/SSR/metadata/search, deterministic filter/order tests, exact currency handling, protected content excluded. Search works without OpenSearch |
| P0.8 Foundation acceptance | Consolidated ASVS applicability/evidence register, contract/security/integration results, performance baseline, deployment/restore evidence, ownership and unresolved risk review | All applicable packages and resolved decisions; accountable engineering/security/operations/product owners | Required gates in QUALITY-GATES.md satisfied; deviations explicit; no pending blocker hidden behind a green pipeline. Public launch is a separate release decision |

An authentication test principal is a test fixture, not a “development login” enabled by an environment toggle in production. P0.2 must fail closed without a real adapter; fixtures are limited to test composition. Test the normal API composition as well: supplied actor/role/tenant headers cannot establish a principal, and protected operations remain inaccessible without a real adapter. P0.3 proofs live in tests and owning persistence boundaries, not public fake business endpoints. P0.1 shell is an internal build/i18n check, not a design-approved marketplace landing page.

## Planned delivery order and review points

1. Accept or amend package A decisions and record evidence in DECISIONS.md.
2. Deliver P0.1, then P0.2, then P0.3 as small reviewable changes. Each package must pass its applicable gates before the next depends on it.
3. Demonstrate package A with runnable local commands and recorded negative/fault-test results. Request remaining decisions only when their concrete design is reviewable.
4. P0.4 and P0.6 planning can then proceed in parallel if their decisions are resolved; neither is implied by package A approval.
5. Build P0.5/P0.7 only against approved policy and contracts. Public UX research is per journey, not satisfied wholesale by the initial search study.
6. P0.8 evaluates the actual implementation and environment. A documentation-only phase cannot pass implementation or operational acceptance.

Estimates are deliberately omitted until staffing, budget, account readiness and the blocking decisions are known. Sequence and acceptance are the planning commitments; fabricated dates would hide uncertainty.

## Explicit scope limits

Stripe platform-payment architecture, promotion separation, notification contracts and insolvency/offer evidence are designed here but do not authorize live checkout, ad delivery or offer processing in package A. Their future implementation requires D09/D14/D16 and feature-specific acceptance criteria. No escrow, auction engine, legally effective NDA flow, misconduct detection, public financial claims or fabricated metrics are included.

The platform's eventual modules are not a checklist of directories to generate. For example, `payments` is created with a real responsibility and tests when its approved integration is implemented, not as an empty placeholder now.

## Definition of Phase 0 completion

Phase 0 implementation is complete only when the scoped executable foundation is built, its invariants and failure modes have evidence, its nonproduction operation is reproducible, and the accountable owners accept the unresolved/deferred boundary. This documentation package is **complete for review**; Phase 0 implementation is **pending approval and work**.

Production release additionally requires named operators, approved privacy/retention and jurisdiction policies, verified real authentication and protected access, supply-chain/security assurance, measured restoration and approved SLO/RPO/RTO/cost. Repository documentation and a deployed container do not alone make a production-grade system.

## Verification of this documentation delivery

Checked 2026-09-16: 16 Markdown documents; 98 relative document links resolve; all 28 numbered directive sections have traceability rows; all 28 requested glossary terms plus Membership, Role, Permission and SaleSubject are present; 22 unique threat cases are recorded. Domain, security and UX cross-reviews resolved ownership, buyer grant, CI-scope and journey-scope inconsistencies. Whitespace/encoding checks and `git diff --check` passed. These checks verify the documentation package, not application behavior, security conformance, cloud operation or production readiness. No application test, cloud apply or remote CI run occurred.
