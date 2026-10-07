# Private business and listing-draft slice — proposal

Date: 2026-10-07. Original status: proposed. Current status: **accepted for implementation by the subsequent explicit owner reply; verification pending**. The initial “ser bra ut. vi fortsätter bygget” prompted this concrete proposal rather than silently resolving material policies. The owner then answered “Godkänn det föreslagna paketet” to the question naming this document and its six recommended decisions. That reply approves the private synthetic-tested slice below. It does not approve real-customer use, publication, uploads, provider procurement or production infrastructure. Current approvals and outstanding material policies remain in [DECISIONS.md](DECISIONS.md).

## Outcome and boundary

Deliver the smallest real persisted path from Business to Listing Draft to a protected Preview. Go owns rules and contracts; PostgreSQL owns state, concurrency and durable evidence. The result must survive a process restart and be proven with real PostgreSQL and the existing identity boundary. It is production-grade source work, not permission to launch or process real customer data.

The slice includes:

- Explicit workspace-owned Business and Listing Draft records with separate identities and structured fields.
- Minimal personal-workspace bootstrap, subject to the policy approval below, so the first admitted human is not left with no usable context.
- Create, read and expected-version update commands; a workspace-authorized private preview.
- Atomic material-change audit, additive aggregate-capable event contracts, and real idempotency wherever retries can create duplicate state/effects.
- Negative, concurrency, rollback, migration and committed clean-checkout evidence.

It excludes Publish or any other listing lifecycle transition, uploads/photos/report parsing/storage, buyer grants, search, payments, seller-mandate verification, invitations/bulk delegation, organization creation, provider account procurement/configuration, AWS provisioning and production deployment. The local seller study remains a separate artifact. This package must not connect it through a fixture login or advertise server saving until a real authorized client path exists.

## Reuse already accepted and implemented

| Existing evidence | Reuse in this slice | Limit |
| --- | --- | --- |
| [Workspace ownership](../../db/migrations/000001_package_a.sql) | PERSON/ORGANIZATION managing ownership, explicit active membership, composite tenant-owned references and RLS defense. | Ownership and organization membership do not grant resource access automatically. |
| [Session transaction boundary](../../internal/store/identity.go) and [migration 000002](../../db/migrations/000002_identity_sessions.sql) | Resolve current session/account and scoped grants inside the resource transaction, preserve account/session/grant lock order and post-lock expiry checks, accept activity only after success. | New resource permissions and personal bootstrap are not already approved merely because the mechanism exists. |
| [Workspace command](../../internal/store/workspaces.go) | Expected-version conflict, bounded SQL/error handling, atomic state/evidence and explicit COMMIT uncertainty. | This command changes a Workspace name only; it is not a generic listing command. |
| [Identity HTTP boundary](../../internal/httpapi/identity.go) | Secure cookie, CSRF/Origin, strict body parsing, server request IDs, credential ambiguity rejection and deterministic errors. | No client actor/owner/role claim becomes authority. No development login exists. |
| [Outbox processing](../../internal/store/outbox.go) | Database-clock leases, repeated delivery, receipt/effect atomicity, lost-ACK recovery, monotonic projections and poison-event isolation. | The current handler accepts only WorkspaceNameChanged. New events need their own approved schema and actual effect. |
| [Reviewed migration runner](../../db/migrations/embed.go) | Additive ordered/checksummed migrations and separate migration/runtime roles. | Never edit applied 000001/000002 or use runtime credentials to migrate. |
| [Package A release gate](../operations/PACKAGE-A-RELEASE-GATE.md) and [P0.4 report](../operations/P0.4-REPORT.md) | Reproducibility and source-evidence standard. | Historical source CI does not verify this proposed slice, real-provider conformance or deployment. |

## Material decisions to approve together

The six recommendations in this table were proposals when delivered and are now accepted for this implementation slice by the explicit reply recorded above. A subsequent change to a material policy must be surfaced; the narrow real-data/rollout boundary remains in force.

| Decision | Recommended answer | Why approval is needed |
| --- | --- | --- |
| Draft completeness and first field set | Permit incomplete private drafts. Require valid supplied values and bounded text, not publication completeness. Store the minimal typed fields below; retain the four sale axes independently, including unset/undecided draft states. | Establishes the first business contract. It must not accidentally accept sale combinations, report-recency or publication rules from the design study. |
| Draft authorization | Add only `listing.read_private`, `listing.create` and `listing.update`. Read uses read-private; create/update also require read-private because they return protected state. Existing workspace grants do not silently become listing grants. | Security/authorization policy changes; current Go and PostgreSQL catalogues contain only workspace.read/update. |
| Personal-workspace bootstrap | First approved personal drafting operation may atomically create/reuse one explicitly identified personal drafting workspace, owned by the authenticated User, with active membership and the exact five grants: workspace.read, workspace.update, listing.read_private, listing.create, listing.update. No organization/mandate/verification is created. | Ownership, account admission and initial privilege provisioning are material policies. P0.4 login currently grants none of them. |
| Resource creation retries | Use a durable command-idempotency contract scoped to principal, requested workspace/bootstrap context and operation. Same key/same normalized request returns the existing outcome after current authorization; changed request conflicts. | A lost COMMIT response can otherwise create a second Business/draft/workspace. Receipt scope, replay semantics and retention affect security and transactions. |
| Aggregate audit/event evolution | Preserve the existing Workspace ledger/outbox byte-for-byte as historical schema behavior; add finite, typed aggregate-capable audit/outbox contracts through migration 000003. Choose a real asynchronous responsibility before emitting new draft events. | Current constraints and consumer are intentionally Workspace-only. Versions and targets must become resource-specific without weakening old invariants. |
| Real-data/rollout boundary | Approve implementation and synthetic integration evidence first. Real-user use stays behind provider conformance, privacy/purpose/retention, secure runtime ingress and operating ownership. | An implemented adapter or database is not supplier acceptance or permission to store real personal/commercial information. |

Do not grant every future permission to the personal owner. Future capabilities need their own reviewed grants. Do not automatically expand existing Reader/Editor templates or grants across existing workspaces; migration of any current principals needs explicit policy.

## Minimal domain and persistence contract

Proposed ownership: the new `listings` responsibility owns this small business-sale drafting model; identity/organizations retain account/workspace/membership authority. It does not write those modules' tables directly. Bootstrap is an organizations-owned capability called by an explicitly coordinated transaction.

| Record | Minimum proposed fields | Invariants |
| --- | --- | --- |
| Business | ID, Workspace ID, private label/activity description, optional country/region, version, UTC creation/update timestamps. | Real business concept, distinct from User/Organization/Listing. No asserted registration, ownership title, seller authority or verified claims. Structured optional facts remain optional in a draft. |
| Listing Draft | ID, Workspace ID, Business ID, constant DRAFT state, optional title/description, optional SaleSubject/TransferStructure/SaleContext/SaleMethod, version, UTC timestamps. | Composite `(workspace_id,business_id)` FK; cannot reference another workspace's Business. State cannot be set by the client. No future transition graph is implied. |
| Command receipt | Scoped command identity, normalized-request fingerprint, resource/version outcome and bounded timestamps. | Uniqueness prevents duplicate creation; no credentials, arbitrary bodies or report content in receipt/audit metadata. Current authorization is rechecked on replay. |
| Private preview representation | Source Business/draft versions; explicit display-field allowlist derived from the protected draft. | Workspace-authorized, no-store, no index/SSR/search/public route. Excludes private business label, owners, identity subjects, grants, internal notes and any future source-file/evidence reference. |

The first create command can create Business and Listing Draft together while retaining distinct record IDs. Editing business facts and listing content must check both current resource versions atomically when both change; a listing-only edit does not manufacture a Business revision. Do not assume one Business can have only one Listing forever. The first slice need not expose an existing-business linking/reuse workflow.

Publication requirements, financial data, asking-price semantics, media, translated content and country extensions are omitted rather than silently modelled through an unvalidated JSON blob. Add them only when their real responsibility and relevant policy exist. Bounded draft text/canonical enum validation is technical integrity; a lawful sale-combination conclusion is not.

## Proposed personal-workspace bootstrap

The recommended workflow makes initialization an explicit application command, not an OIDC callback side effect:

1. Validate a currently active authenticated human session; derive User/actor from the trusted boundary.
2. Resolve a requested existing workspace only through its actual membership/grants. A client-supplied workspace ID remains requested context.
3. If the approved personal-bootstrap path is requested, atomically resolve/create the user's personal drafting assignment, Workspace, membership and exact scoped grants. Then create Business/draft if that is the command's requested operation.
4. Write bounded workspace/grant/resource evidence and any approved asynchronous intent in the same transaction; commit the durable command receipt with the created resources.
5. Return the authoritative IDs/versions. On uncertain COMMIT, do not assume rollback; a retry uses the same command identity and checks the durable outcome.

A personal drafting assignment identifies a default context; it must not impose a blanket rule that a User can own only one PERSON workspace or cannot participate in professional client engagements. Its ownership reference must be constrained to that same User. Concurrent first-use requests must produce one assignment/bootstrap, without orphan Workspaces, duplicate grants or session/account lock-order violations. The implementation algorithm needs source review and PostgreSQL concurrency proof before acceptance.

Organization-owned workspace behavior retains explicit membership/grants. This package adds no organization creation, invitation or membership-management bypass. Existing organization scopes can be tested through synthetic setup and legitimately granted application access; affiliation alone remains insufficient. An account, Workspace or draft creates no SellerMandate.

## Proposed private API surface

The contract must be reviewed before implementation. Exact endpoint layout remains a reversible engineering detail once the policy above is approved.

| Command/query | Proposed contract behavior |
| --- | --- |
| Create a private draft, optionally requesting approved personal bootstrap | Authenticated POST; bounded typed Business/listing input and durable command identity. Existing workspace context is authorized; bootstrap derives personal owner/grants server-side. Returns distinct Business/draft IDs and versions. |
| Read a draft | Authenticated workspace/resource-scoped GET; current read-private permission; unknown and inaccessible IDs share the existing 404 boundary. |
| Update a draft | Authenticated, CSRF-protected PATCH with expected resource versions; atomic validation/change/evidence; 409 for a stale version. |
| Preview a draft | Authenticated workspace/resource-scoped GET; read-private permission; authoritative, allowlisted presentation with source versions and incomplete placeholders. This is not a public listing endpoint. |

A concrete candidate path is `/api/v1/workspaces/{workspace_id}/listing-drafts/{draft_id}`, with collection create and `/preview`. If personal bootstrap is combined with first create, use a separately explicit authenticated command path/body rather than pretending an absent client workspace ID is already authorized. Bodies must reject actor/owner/organization/role/permission/state fields, duplicates, case aliases, unknown keys, oversize input and trailing JSON. Keep 401 unauthenticated, 403 CSRF, non-enumerating 404, 409 conflict and 503 dependency-uncertainty semantics.

No new visible authentication UI is part of this backend package. A future actual client needs researched sign-in, approved provider acceptance and an honest persistent-save/recovery UX; it cannot use synthetic test identity as a development fallback.

## Aggregate-capable audit and outbox evolution

The current schema cannot be reused by inserting a different event name:

- `audit.events.action` is fixed to workspace.name_updated; target kind is Workspace; target ID equals workspace ID; version uniqueness is per workspace.
- `eventing.outbox` references that audit/workspace version, and its uniqueness does not distinguish multiple independent aggregates in the same workspace.
- `validDelivery` accepts only WorkspaceNameChanged; consumer receipts only permit workspace_revision_v1; its effect stores a workspace revision.

Do not increment Workspace versions for unrelated Business/draft changes or emit fake WorkspaceNameChanged events. Do not relax checks into arbitrary action/target/body strings.

Recommended additive design, subject to approval:

1. A new intentionally bounded aggregate audit relation identifies workspace, typed target kind/ID, resource previous/new version, derived HUMAN/SERVICE actor, action, UTC time, request/correlation and fixed result/metadata schema. Creation uses version 0→1; updates increment the correct resource version. Typed/composite foreign keys or an equivalently proven target registry must bind the target to its authoritative resource and workspace; an unchecked polymorphic ID is insufficient.
2. A new aggregate outbox relation, when asynchronous intent is required, references the exact audit event, resource kind/ID and version. Uniqueness is per resource/version/event, so two drafts at version 2 in one workspace do not collide. Events carry minimal identifiers/schema/version/context, not description, identity or financial content.
3. New receipts identify the exact event/consumer/workspace and real SERVICE processor. Receipt plus actual local effect commit atomically; ACK stays separate. Existing Workspace handler, receipts and history remain compatible.
4. RLS and runtime privileges remain action-specific and workspace-bound. The API cannot mutate evidence after append; the worker cannot impersonate a human or use a blanket identity/grant-management capability.

The smallest preview is a synchronous authoritative read, with material writes and audit atomicity. It needs no invented asynchronous task. **Before choosing outbox events for this slice, approve the actual effect.** A possible real alternative is an asynchronously maintained private preview/read projection: it requires a real versioned handler, current authorization on reads, authoritative version/staleness semantics, minimal worker access, replay/rebuild behavior and tests. It adds complexity and should be chosen only for an identified responsibility. Otherwise preserve the existing outbox and use the reviewed aggregate audit seam without claiming draft events were processed. Schema generalization must not become unused scaffolding.

## Implementation map after approval

| File/responsibility | Proposed change |
| --- | --- |
| `internal/listings` | Real typed draft domain/service and PostgreSQL-owned persistence, not an empty future module. |
| `internal/httpapi/listings.go` and API composition | Strict protected transport using the existing authenticated/CSRF boundary; no actor-ID shortcut. |
| `internal/store/identity.go` / organizations boundary | Narrow explicit shared transaction coordination and approved personal bootstrap; preserve tested lock order. Do not copy session validation into a second implementation. |
| `internal/identity/identity.go` | Only the approved current draft permissions; role/grant propagation policy explicit. |
| `db/migrations/000003_listing_drafts.sql` | Structured records, composite ownership/version constraints, exact permissions and aggregate evidence; no edit to applied migrations. |
| `api/openapi/listing-drafts.json` | Versioned typed draft/preview/command/error contracts, reviewed before handlers. |
| Worker/event adapter, only if a real effect is selected | Actual aggregate handler/receipts and tested repeated processing; leave existing handler compatible. |
| Relevant unit/HTTP/PostgreSQL/migration tests and evidence docs | The acceptance matrix below, full applicable gates and commit-bound proof. |

A new ADR should record accepted bootstrap/permission/evidence/creation-retry decisions and alternatives/reversal cost. This proposal does not create those accepted ADRs, schemas or endpoints.

## Acceptance matrix

Every PostgreSQL invariant must run against real PostgreSQL using actual restricted API/worker credentials. External identity fixtures remain appropriate for protocol fault tests, not proof of chosen-provider behavior.

| Case | Required proof |
| --- | --- |
| Missing/invalid/expired/revoked session or disabled account | Protected commands fail safely; no state/evidence/permission is created and no fixture fallback occurs. |
| Read/update/preview foreign workspace or guessed resource | Denied without existence leakage; changing workspace/organization/owner/role inputs cannot elevate access. |
| Organization affiliation and managing ownership | Neither bypasses explicit draft permissions. Test PERSON and ORGANIZATION scopes. |
| Cross-workspace Business reference | Rejected both by application boundary and composite PostgreSQL FK/RLS. |
| Incomplete private draft | Validly absent fields persist/reload; supplied malformed values reject; no completed publication/verification claim appears. |
| Personal bootstrap | Actor-derived PERSON owner, exact active membership/grants, no organization/mandate/verification. All state, grants, audit and receipt commit or roll back together. |
| Concurrent first bootstrap/create | One personal assignment/bootstrap; no orphan or duplicate grants. Same idempotency key creates one Business/draft and one set of material evidence. |
| Create replay/body mismatch/uncertain COMMIT | Stable resource outcome for same key/request after current authorization; mismatch conflicts; no duplicate after lost response; revoked principal cannot recover protected state by replay. |
| Two writers / Business-plus-draft update | One expected-version winner; conflict loses no acknowledged update. Failed second-resource/evidence write rolls back the entire coordinated change. |
| Permission revocation, expiry and account disable during lock waits | Existing ordering and post-lock freshness guarantees remain; no stale authorization success. |
| Audit/outbox failure | Fault injection proves all-or-nothing material state/evidence/approved intent; no success followed by a separate critical write. |
| Preview allowlist | Canary private labels/owners/identity/grants/internal references absent. Preview remains authenticated/no-store; arbitrary free text is never certified anonymous. |
| Real asynchronous effect, if selected | Duplicate/concurrent delivery has one durable effect; crash after commit/before ACK safely repeats; older versions do not regress; malformed/poison event does not block healthy work; uncertain commit/lease exhaustion recover safely. |
| Migration/historical compatibility | Empty database and seeded 000002→000003 upgrades; applied checksums/history and old audit/outbox behavior preserved; RLS/runtime privileges and pool context reuse proven. |
| Failure/cancellation/restart | Dependency uncertainty is observable and fail-closed; cancellation releases/rolls back; restart reads durable drafts/receipts correctly. |

Run every applicable repository gate: format, vet, independent builds, normal/integration/race tests, migration/contract validation, dependency/security analysis and secret scans. Review intended files, commit exact source, run clean-checkout CI and inspect its results/artifacts. Record exact counts and not-run checks; earlier Package A/P0.4 evidence cannot stand in for this slice's result. Any actual frontend adds its own strict build/catalog/browser/accessibility gates; the present local design-study tests remain separate.

## Release gates and recommendation

1. Approve/amend the finite policy packet above and record exact scope/evidence in the decision register.
2. Implement and prove the private slice with synthetic data and existing protocol fixtures; no provider procurement or production environment is required for those source tests.
3. Preserve the live-provider acceptance gap: selected methods, recovery/revocation/security-event propagation, processing terms and actual conformance are still unverified.
4. Before admitting real data, resolve D08 purposes/retention/erasure/holds, authorized operators, secrets/TLS runtime and recovery/evidence responsibilities. A prototype filename or a green unit run does not meet this gate.
5. Only then approve a real authenticated client/rollout. Publish, uploads, seller authority and other product capabilities require separate packages and their unresolved D05/D07/D10/D15 decisions.

Recommendation: approve the minimal incomplete-private-draft, explicit-permission and atomic personal-bootstrap policies first. Prefer synchronous protected Preview and atomic aggregate audit until a real asynchronous responsibility is selected. Retain the tested outbox foundation and its at-least-once semantics; do not manufacture work to make the next slice look larger. Stop after the agreed private slice and its committed evidence.
