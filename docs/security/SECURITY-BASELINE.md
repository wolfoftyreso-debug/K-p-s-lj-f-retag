# Phase 0 security baseline

Date: 2026-09-16
Status: target security baseline; Package A implements workspace authorization/RLS, bounded transactional audit, idempotent delivery and safe runtime boundaries described in the [runbook](../operations/PACKAGE-A.md). Remaining controls are requirements, not deployed capabilities. No claim of compliance or production readiness.
Accountable roles: engineering lead, security owner, product owner and privacy/legal owner; named assignments pending

The master directive mandates server-side authorization, explicit tenant isolation, protected document handling, durable audit, transactionally committed outbox events, safe payment processing and an OWASP ASVS-based security program. This document specifies the evidence expected from implementation and surfaces decisions that must not be silently chosen. See the [threat model](THREAT-MODEL.md) for abuse cases and failure behavior.

## Standards and evidence

Use **OWASP ASVS 5.0.0** as the version-pinned control catalogue. OWASP identifies 5.0.0 as its stable release on the source checked on 2026-09-16. The proposed assurance target is Level 2 with additional selected controls justified by document sensitivity and privileged operations; the security owner must approve that target and scope before a compliance claim or launch assessment. Record applicable requirement IDs with their version, implementation owner, evidence, test and exception. This document is not a completed ASVS assessment. [OWASP ASVS project](https://owasp.github.io/www-project-application-security-verification-standard/)

The directive's accessibility target is **WCAG 2.2 AA**. The verified W3C Recommendation is dated 12 December 2024. Authentication, session expiry, permission errors, upload progress and abuse challenges are part of complete accessible journeys. Automated checks alone cannot establish conformance; use manual keyboard, focus, screen-reader and authentication-flow review. [W3C WCAG 2.2](https://www.w3.org/TR/WCAG22/)

## Decisions requiring explicit approval

All recommendations below are **proposed**, not adopted policy. The [central decision register](../architecture/DECISIONS.md) owns approval status; this table supplies security-specific review topics, not a second set of approvals. No affected production capability may be enabled while its necessary policy remains undefined. Documentation may proceed now; executable contracts and synthetic tests proceed within their approved implementation package.

| Topic | Decision and recommendation | Accountable roles | Must precede |
| --- | --- | --- | --- |
| Tenant isolation | Use workspace isolation with scoped memberships and composite ownership constraints; evaluate RLS as additional defense | Product, engineering, security | Persistent tenant schema and authorization implementation |
| Authentication | Identity provider, browser session/token transport, recovery, MFA/step-up and session revocation; retain a vendor-independent application boundary | Product, security, engineering | Authentication implementation |
| Permissions | Role/permission matrix, invitations, professional client delegation, support access and emergency access; default deny | Product, security | Protected workspace workflows |
| Seller authority | SellerMandate evidence, scope, review, expiry, revocation and interaction with published listings; keep separate from identity | Product, privacy/legal, security | Publication or authority claims |
| Disclosure | Field-level disclosure, public identity redaction, protected-document grant rules, revocation and cache policy; unsupported NDA access stays unavailable | Product, privacy/legal, security | Public listing contract and protected sharing |
| Document access | Private document storage isolation, scanner, allowed formats/sizes, inspection exceptions, signed access lifetime and immediate-revocation needs | Security, engineering, product | Upload/download implementation |
| Infrastructure | EU region(s), data residency, encryption/key ownership, backup/restore design, recovery objectives and supplier transfers | Infrastructure, security, privacy/legal | Cloud provisioning or real personal/commercial data |
| Retention and evidence | Per-class retention, legal holds, erasure, audit minimization, privileged tamper evidence and backup deletion reconciliation | Privacy/legal, security, product | Real data collection or immutable retention configuration |
| Payment policy | Payment entitlement, refund/cancellation/reversal and publication dependencies; Stripe is for platform revenue only | Product, finance, engineering | Payment state machine and checkout |
| Assurance | ASVS assurance scope, vulnerability gates, exception authority and incident/abuse ownership | Security, engineering | Release gate adoption and production release |

No session timeout, URL lifetime, upload policy, retention interval, service objective, budget, risk threshold or deletion deadline is established here. Material policy values need the relevant decision, an accountable owner, rationale, configurable representation and boundary tests. Ordinary reversible engineering limits within an approved scope, such as bounded local test resources, can be chosen conventionally and documented without a separate approval for each number.

## Identity, authorization and tenancy

Authentication resolves a stable application `User` using a verified external identity; it does not resolve seller authority or workspace permissions. Roles are bundles of application permissions, and commands must evaluate permission plus resource ownership, state and any applicable grant conditions. Tenant IDs, actor IDs and role names supplied by a client cannot be authoritative.

Before protected APIs exist, specify the policy table for each operation, actor class and resource state. Cover reads, writes, list/count, batch actions, exports, search, document access, invitations, background jobs and operator tools. Apply policy in the Go application layer regardless of the calling UI. Default deny unknown actions or unsupported disclosure modes. Do not substitute an opaque UUID for an ownership check.

Use optimistic concurrency or appropriate locking for membership, permission and material resource transitions. Determine the acceptable revocation behavior and consistency boundary explicitly. Cache authorization only if its invalidation and residual lifetime are approved and testable. Proposed privileged access requires a named actor, reason, narrow scope, expiration and audit; no permanent invisible support superuser.

If RLS is accepted, migrations must include policies and role grants alongside schema changes. Runtime connections must not own protected tables or use bypass privileges. Missing context denies access. Maintenance, migration, public projection and worker roles require separate, documented capabilities. Prove isolation with actual PostgreSQL roles and connection reuse, not mocks alone.

## Public disclosure and browser security

Define a separately typed, allowlisted public listing representation. Never serialize a private aggregate and remove a few known fields. Public rendering, API, search, SEO metadata, translations, notifications and media derivatives consume explicitly approved fields. Free text and uploaded imagery require a disclosure review policy because structure alone cannot prevent a seller from naming a confidential business.

Next.js caches and CDN rules must distinguish public pages from personalized/protected responses. Test HTML, hydration payloads, error pages, redirect locations, query strings, structured data, alternate languages, filenames and image metadata. A confidentiality change requires an authoritative visibility check/suppression path plus projection and cache invalidation. `robots.txt` and `noindex` are discoverability controls, never confidentiality controls.

Implement safe output encoding, a reviewed rich-text allowlist if rich text is required, parameterized SQL and allowlisted sort/filter expressions. Restrict remote resource fetching and outbound targets where needed; do not introduce URL import as an incidental convenience. Configure request/body/time limits, rate limits, abuse signals and bounded pagination. Values remain policy decisions.

The authentication transport decision determines the CSRF design. For cookie sessions, require secure cookie attributes, CSRF/origin protections on state changes and explicit cross-origin policy. Configure CSP, framing restrictions, content-type protections, referrer policy and TLS/HSTS according to actual deployment domains. Test protection on error responses as well as success. Do not disable framework protections to make an integration work.

## Document subsystem

Use private object storage for quarantine and protected documents, with Block Public Access and least-privilege roles. Separate public-media delivery from document access policy. The concrete bucket/account/KMS arrangement is proposed architecture pending the document-access and infrastructure decisions. Prefer private S3 origins even when approved derivatives are delivered publicly through CloudFront.

Every version needs ownership, classification, storage identity, checksum, actual format/size, validation status, scan evidence, creator, timestamps and retention/deletion state. Use non-identifying keys; retain original filenames only as protected metadata if needed. Immutable version binding prevents replacing scanned bytes with unscanned bytes. Worker input and scanner output are validated like other external input.

Quarantine is mandatory until required validation and scanning succeed. Processing failures cannot imply approval. Parse untrusted documents in constrained workers with minimal permissions and bounded resources. Generate public derivatives intentionally; remove metadata where appropriate and inspect residual image/content disclosures. Do not index protected document content into public search.

Authorize each download capability against current application policy and exact version. Record authorization/issuance durably before issuing access. Keep signed URLs out of logs, analytics, traces, referers and emails except through an explicitly approved sharing design. The chosen access mechanism must describe reusable links, expiration, residual revocation window and retrieval-evidence limits. Detailed sequence and tests are in T10-T12 of the threat model.

## Audit and privacy

Proposed audit envelope: event ID/schema version, occurred/recorded UTC timestamps, actor type and stable actor reference, action, target type/reference, workspace when applicable, outcome/reason code, request/trace correlation, policy/version context and an allowlisted change summary. Retain original actor context for delegated work; record the service that executes it. Any IP/device fields require a documented security purpose and retention policy.

Record successful material state changes and their audit event in one database transaction. Include associated outbox records in that same transaction. Denied operations need separately bounded security-event recording because their business transaction is not committed. Audit storage failure must prevent a material mutation from being reported as successful; durable access authorization must precede issuance of a protected capability.

Runtime application roles should insert audit records without update/delete permission. That is a proposed protection against application mistakes, not a guarantee against a privileged database operator. Tamper-evident export or immutable storage, retention locks and separation of duties need the retention-and-evidence decision; an irreversible retention setting must not be enabled casually.


Do not log secrets, access tokens, cookies, signed URLs, document bodies, raw payment payloads, full request bodies or unrestricted before/after snapshots. Structured telemetry uses bounded fields and redaction tests. Audit readers are authorized and their access is itself traceable. Metrics avoid personal identifiers and unbounded tenant/user labels. Request/trace identifiers must not carry identity data.

The following are **questions for the accountable privacy/legal owner**, not legal determinations: controller/processor roles; purposes and lawful bases; processing of sole-trader and employee information; jurisdiction-specific seller/insolvency records; retention and legal holds; access/erasure/restriction requests; subprocessors and international transfers; and whether a DPIA or specific public disclosure notices are required. Engineering must supply a data inventory and flow map so these can be answered. No GDPR compliance claim is made. [GDPR official text](https://eur-lex.europa.eu/eli/reg/2016/679/oj/eng)

Erasure design must reconcile live rows, documents/versions, search, CDN caches, analytics, suppliers, pending messages and backups. A restored environment must reapply deletion/revocation state before serving users. An append-only audit requirement does not justify unlimited personal-data retention. Decide whether actor references can be separated from identifying attributes while preserving necessary evidence; do not automatically call pseudonymous data anonymous.

## Payment and asynchronous integrity

Stripe pays for platform listings, promotions or subscriptions only. No business purchase settlement or escrow is in scope. Contract tests must verify request size limits, the endpoint secret, exact signed bytes, accepted account/environment, and authenticated event parsing. Stripe events may be duplicated or delivered out of order, so transport verification alone cannot grant an entitlement. [Stripe webhook guidance](https://docs.stripe.com/webhooks)

Proposed processing: authenticate the delivery, persist a uniquely keyed provider-event inbox entry, and acknowledge only after durable acceptance. A worker applies the approved business transition, entitlement uniqueness, audit and outbox atomically. A repeated event becomes a safe no-op; different event IDs representing the same entitlement cannot duplicate it. Scope uniqueness to the relevant provider account/environment. Reuse an outbound idempotency key only for the same logical operation. Validate amounts/currency and internal purchase identity against server-owned records. Refunds, reversals and cancellation behavior require the payment-policy decision.

Event envelopes carry ID, type/schema version, occurred UTC timestamp, aggregate reference/version, ownership scope and correlation. Minimize personal data. Outbox publication is at least once: a crash between send and acknowledgment will duplicate delivery. Consumers need durable deduplication and business uniqueness, transactionally coupled to their local effects. External side effects require provider idempotency or a reconciliation strategy. Replays must re-evaluate current grant/visibility where delayed disclosure could leak data.

Bound retries, configure monitored DLQs and expose outbox age, processing lag and terminal failures. Do not quietly discard poison events or assume arrival order. A dead-letter replay is a privileged, audited operation. Audit retention, event retention and replay windows must be designed together so old messages cannot silently recreate erased information or expired entitlements.

## Infrastructure, delivery and operations

No production secrets belong in Git or frontend bundles. Prefer workload identity and short-lived CI credentials with restricted trust conditions. Separate application, worker, migration and deployment roles; constrain storage/queue access by purpose. Terraform state is sensitive and needs access control, encryption and locking. Provision approved infrastructure through reviewed code, including security group policy, private data services, backup settings and logs.

Each release must identify its commit, dependency lockfiles, build artifacts/images and migration version. Pin build toolchains and third-party actions; produce and retain dependency/image inventory as release evidence. CI must include secret/dependency scanning, Go and TypeScript static checks, SAST, container and IaC scanning as applicable. Tool configuration, applicability, severity gates and exception owners must be explicit; a green job with an empty scan scope is not evidence.

Run database integration tests against the selected PostgreSQL version. Verify transaction rollback, duplicate/reordered events, migration permissions and restore behavior. Choose expand/contract migrations for compatibility; rehearse restoration rather than claiming every migration is automatically reversible. Deployments cannot assume schema rollback can undo data loss.

Instrument authentication failures, authorization denials, privilege changes, scan backlog/failures, signed-access issuance, webhook failures/duplicates, outbox age, DLQs and unusual exports with redacted structured logs, metrics and traces. Assign responders and alert routing before production. Backup retention, restore frequency, RPO/RTO, incident escalation and breach response decisions need named owners and approved values. Multi-AZ does not demonstrate recoverability.

## Release evidence and exceptions

| Gate | Required evidence before the affected production capability |
| --- | --- |
| Design | Approved relevant entries in the central decision register, ownership/permission matrix, reviewed data flow and threat model |
| Authorization | T01-T06 pass through real application/database boundaries; revoked access and connection reuse covered |
| Confidentiality | T07-T12 pass, including all public rendering/projection surfaces and document version binding |
| Integrity | T15-T18 pass with concurrent duplicates and crash boundaries; monetary values and currency validated safely |
| Browser/accessibility | XSS/CSRF/header checks, keyboard/screen-reader/focus review and accessible authentication evidence |
| Supply chain/infrastructure | Reproducible scanned artifact, reviewed Terraform plan, bounded roles and secrets checks |
| Operations | Restored backup exercise, deletion/revocation reconciliation, usable incident runbook, tested alerts and rollback plan |
| Assurance | Versioned ASVS applicability/evidence register, independent review proportional to risk, accepted residual risks |

An exception must name the control, affected scope, concrete risk, compensating measure, accountable approver, expiration/review date and remediation task. Exceptions cannot silently override the directive's tenant isolation, secret handling or public document restrictions. A future implementation must report controls as passed, failed, not run or explicitly not applicable; planned work is never reported as verified.
