# Phase 0 threat model

Date: 2026-09-16
Status: design proposal; controls are not implemented or verified
Accountable roles: engineering lead and security owner; named owners pending

This model translates the master directive into threats and proposed controls. Requirements explicitly stated by the directive remain requirements. The implementation choices below require the decision approval described in [SECURITY-BASELINE.md](SECURITY-BASELINE.md). No risk score, legal conclusion, retention period, production readiness claim, or permission grant is implied by this document.

## Scope and assets

The system consists of a public Next.js client, a Go modular monolith, independently runnable Go workers, PostgreSQL, S3, SQS, and external identity and platform-payment providers. AWS infrastructure is managed through Terraform and deployed through GitHub Actions. Search is a replaceable projection of authorized listing data.

The highest-impact assets are protected business documents, confidential seller and business identities, membership and permission records, seller mandates, listing versions, payment entitlements, offer-process records when introduced, and durable audit evidence. Credentials, deployment authority, backups, encryption keys, and recovery access protect all of these assets. Public listings also require integrity: unauthorized alteration can mislead buyers even when no confidential data is exposed.

Threat actors include anonymous attackers, authenticated buyers, dishonest or compromised sellers, professionals with access to one client but not another, privileged staff, compromised third-party providers, and compromised CI dependencies. A valid account does not make its input trustworthy. A successful identity check does not establish authority to sell.

## Trust boundaries

| Boundary | Trust crossing | Required or proposed enforcement |
| --- | --- | --- |
| Internet to edge | Browser input, bots, abusive requests | TLS, WAF, request limits; application authorization remains mandatory |
| Edge to web/API | Forwarded identity context and headers | Restrict origin reachability; accept proxy headers only through known ingress; never trust browser-supplied actor or tenant headers |
| Browser/Next.js to Go application | Session or access token, command input | Explicit contract validation; authenticate behind an adapter; server-owned authorization for every protected operation |
| Identity provider to identity adapter | Subject, issuer, session evidence | Verify issuer, audience, signature and applicable lifetime; map provider identity to an application user; no provider claim grants a workspace permission by itself |
| One workspace to another | IDs, search scopes, exports, background jobs | Resolve membership and ownership on the server; workspace-scoped access; proposed composite constraints and optional defense through RLS |
| Application modules to database | Commands, audit records, outbox events | Explicit transaction ownership, constrained database roles, foreign keys, unique constraints, version checks; no unrestricted cross-module table updates |
| Database to search/cache | Approved public representation | Allowlisted projection only; authorization-safe cache keys; visibility recheck or suppression before returning stale results |
| Upload caller to quarantine | Potential malware, misleading MIME, huge files | Server-issued upload intent; isolated private storage; bounded parser/scanner; no download while unaccepted |
| Quarantine to released document/media | Scanner verdict and exact object version | Bind result to immutable bytes/version and policy version; release only after required validations complete |
| Application to S3 access grant | Bearer download capability | Current resource authorization and audit before issuance; narrow method/key/version and short lifetime; no URLs in telemetry |
| Application/outbox to worker/SQS | At-least-once messages | Versioned event envelope, provenance, validation and idempotency; recheck current state before sensitive external effects |
| Stripe to payment endpoint | Signed external payment event | Authenticate exact request bytes; durable inbox, duplicate handling, entitlement uniqueness, environment/account checks |
| Operator/CI to production | Exceptional access and release authority | Distinct deployment/runtime/migration roles, traceable access, reviewed changes, restricted secrets and recovery procedures |

Next.js server execution is a runtime boundary, not proof of permission. Server rendering, route handlers, server actions and direct API callers must converge on the same application policy. Administrative routes do not inherit authority merely by being placed under an administrative URL.

## Tenant isolation proposal

Use a workspace as the proposed tenant boundary. Organization membership alone must not give access to every client workspace. Membership binds a user to an explicitly scoped role/permission set. Define this ownership relationship before finalizing database schemas.

For a workspace-owned listing document, the candidate path is `Workspace -> Listing -> ListingDocument -> DocumentVersion`. Persist enough ownership information to enforce matching `(workspace_id, listing_id)` relationships through composite keys/foreign keys. Caller-supplied workspace IDs are selectors to validate, never authorization evidence. Resource lookup, list/count, exports, signed access, caches and jobs must preserve the same ownership path.

Buyers may receive a document-specific access grant without membership of the seller workspace. Such a grant must identify its subject, issuer, resource/version scope, conditions and revocation state. It must never create implicit access to sibling resources. The exact disclosure rules and role permission matrix are pending product/security approval.

**Proposed additional defense:** PostgreSQL row-level security on tenant-owned relations. This requires transaction-local tenant context, connection-pool cleanup tests, worker-specific access design and roles unable to bypass policy. PostgreSQL owners and privileged roles can bypass ordinary RLS; RLS is not a substitute for application authorization. [PostgreSQL row-security documentation](https://www.postgresql.org/docs/current/ddl-rowsecurity.html)

## Principal abuse cases and negative tests

These are required test scenarios for the affected capability, not assertions that tests already exist. Use synthetic workspaces A and B, separate users, revoked users and an anonymous caller. Tests must reach the real application/storage boundary where that boundary is under test.

| ID | Abuse or failure | Expected prevention and observable evidence |
| --- | --- | --- |
| T01 | User in A requests B's document, listing draft, invoice, mandate or export by known ID | Denied before data or access URL is returned; no sensitive existence disclosure; denial has bounded security telemetry |
| T02 | A creates/updates a child object using B's parent ID | Application denies; database ownership constraints independently reject mismatched relationships |
| T03 | User tampers with role, workspace, owner or verification fields in JSON/token headers | Untrusted fields cannot elevate authority; permission check uses application records |
| T04 | Membership revoked after session creation; pooled connection previously served A | Current policy denies protected requests; no tenant context survives transaction reuse |
| T05 | Broker has two clients and searches, exports or processes a job in the wrong workspace | Results, totals, keys and asynchronous effects stay in the selected authorized scope |
| T06 | Verified identity or matching company email used to publish without a mandate | Publication evaluates the separate mandate policy; identity success cannot satisfy it |
| T07 | Draft or withdrawn identity leaks through HTML, hydration data, metadata, JSON-LD, sitemap, image name, EXIF or search snippet | Public contract uses allowlisted approved fields; fixture containing distinctive secret markers never appears on public surfaces |
| T08 | Private SSR response is cached and served to anonymous user or another workspace | Protected responses are excluded from shared public caches; cross-session cache tests return no private bytes |
| T09 | Listing becomes confidential while its search projection, CDN page or notification job is stale | Authoritative visibility gate/suppression prevents fresh disclosure; purge/reconciliation runs; failure is alerted, not silently ignored |
| T10 | MIME spoof, path traversal name, script-capable content, decompression bomb, oversized upload or scanner timeout | Upload remains quarantined or is rejected; processing has resource limits; no release on timeout or unknown verdict |
| T11 | File changes between scan and signed download; old scan result is replayed | Exact object version/checksum and scan policy bind release; changed bytes need new validation |
| T12 | Unauthorized caller requests a signed URL; previous recipient reuses one after grant revocation | New issuance denied; test documents the chosen residual lifetime or gateway enforcement; no claim that ordinary presigning is single-use |
| T13 | Cookie-authenticated victim is tricked into publishing, inviting a member or granting document access | CSRF/origin defenses reject the cross-site command; safe methods do not change state |
| T14 | Listing description, translated content, filename or URL contains stored XSS/SSRF payload | Context-appropriate escaping/sanitization; controlled link schemes; remote fetch capability absent or explicitly constrained |
| T15 | Forged, duplicate, concurrent or reordered Stripe events | Invalid signatures rejected; durable duplicate/event semantics and entitlement uniqueness prevent duplicate grants; reconciliation handles ordering |
| T16 | API/worker crashes after database commit, after sending a message, or before acknowledgment | Outbox/inbox state survives; retries produce one business effect; poison message reaches monitored DLQ |
| T17 | Concurrent publish, pause, withdrawal or material amendment bypasses lifecycle checks | Expected-version transition succeeds once; audit and event commit with the winning state |
| T18 | Application tries to modify/delete audit evidence; logs receive raw document, token or signed URL | Runtime role cannot perform prohibited audit mutation; schema/redaction tests reject sensitive payloads |
| T19 | Search input or sorting fragment attempts SQL injection or unbounded queries | Parameterized values, allowlisted sort choices, bounded complexity and timeout; deterministic pagination |
| T20 | Bulk enumeration, contact spam, account recovery abuse or repeated export | Layered limits and abuse signals protect service; inaccessible challenge mechanisms are not the only recovery path |
| T21 | Backup restore resurrects deleted public data or previously revoked grants | Restore isolated from traffic; deletion/revocation reconciliation and policy validation precede reopening |
| T22 | CI dependency, image or Terraform change escalates production permissions | Pinned dependencies/actions, scanning, least-privilege deployment identity and review gates expose/reject the change |

## Document handling sequence proposal

1. Authorize an upload intent against a resource and workspace. Generate a non-identifying object key. Apply approved type/size limits; these values are undecided.
2. Upload to a private quarantine area with a narrowly scoped capability. A client completion callback is evidence to inspect, not evidence that the file is safe.
3. Verify actual size, file signature/format and immutable version/checksum. Validate archive/container behavior and parser limits. Run malware scanning. Produce sanitized media derivatives where required; original metadata is not public content.
4. Persist a versioned classification and validation verdict. Unknown, incomplete, encrypted-uninspectable, failed or timed-out processing cannot result in release. Reprocessing or human review must still satisfy the required inspection and scanning policy for the exact version; there is no operational skip-scan override.
5. On each access request, evaluate user, current grant/mandate-dependent rules where applicable, exact document version, classification, release state, and deletion/hold state. Persist the material access authorization event before releasing the capability.
6. Issue time-limited access to the exact version or serve through an authorized gateway, according to the approved revocation/audit decision. Distinguish `AccessAuthorized`, `AccessCapabilityIssued` and observed storage retrieval; none proves the human read the document.
7. Revoke future access, reconcile object versions/derivatives/backups and execute approved retention or deletion workflows. Do not confuse a tombstone with completed physical deletion.

S3 presigned requests can be reused and used by a holder of the URL. Expiration is checked when a request starts; an in-progress download can continue after expiration. Document authorization revocation therefore cannot promise immediate cancellation of already-issued URLs or erase downloaded copies. Select the access mechanism only after agreeing this residual exposure. [AWS presigned URL FAQ](https://docs.aws.amazon.com/prescriptive-guidance/latest/presigned-url-best-practices/faq.html), [AWS URL expiration behavior](https://docs.aws.amazon.com/AmazonS3/latest/userguide/using-presigned-url.html)

## Failure containment

| Failure | Proposed behavior |
| --- | --- |
| Identity provider unavailable | No fallback identity or bypass login; existing session behavior follows approved session/revocation policy |
| Authorization store unavailable | Deny protected operations without claiming the user lacks permission; serve a retryable service error |
| Audit insert fails during material mutation | Roll back mutation and outbox write; no unaudited success response |
| Search projection stale/unavailable | Use approved PostgreSQL fallback or explicit degraded result; never broaden visibility or silently invent results |
| Scanner unavailable | Keep uploads quarantined, surface processing state, alert on backlog |
| SQS unavailable | Retain committed outbox events for retry; alert on age and volume |
| Database unavailable | Reject mutations; never acknowledge unpersisted payment/event acceptance |
| Third-party entitlement check uncertain | Preserve explicitly known state; reconcile safely before granting a new entitlement |
| Credential compromise | Contain the relevant role/session, rotate credentials, preserve bounded evidence, follow incident runbook |
| CDN purge fails after confidentiality change | Stop serving through an authoritative visibility gate; alert; do not assert that already published copies can be retrieved |

## Residual risks and approval prerequisites

Confidentiality cannot be restored for information already copied by a legitimate recipient or a public crawler. Malware scanning cannot guarantee content is safe. Technical seller verification cannot independently settle legal authority. Append-only permissions do not make a database immune to privileged tampering. Multi-AZ availability does not replace restore testing.

Before implementing protected journeys, resolve the security decision register, assign control owners, and attach tests to these threat IDs. Before production, review this model against actual data flows and deployment configuration, perform adversarial testing of tenant isolation and document access, and record residual risk acceptance. New integrations, privileged workflows, disclosure rules or offer-process semantics require a model update.
