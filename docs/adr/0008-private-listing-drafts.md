# ADR 0008: private Business and Listing Draft transactions

Date: 2026-10-07. Status: **owner-approved scope and policies; implementation and acceptance evidence remain distinct**. The owner approved the six recommendations in the [private draft slice proposal](../architecture/DRAFT-SLICE-PROPOSAL.md). Approval evidence is recorded in the [decision register](../architecture/DECISIONS.md). This ADR does not authorize real customer data, provider procurement, publication or production deployment.

## Context

The approved seller design is an isolated browser study. Its local examples, file selectors, financial graphics and public-looking preview are not persisted application capabilities. The next real capability is a small private path: Business → Listing Draft → protected Preview. A Business is the economic activity; a Listing Draft is a particular proposed presentation. Neither is an Organization, proof of ownership, SellerMandate or verified claim.

P0.4 authentication admits an internal User and establishes a session. It deliberately creates no workspace or commercial authority. Existing Workspace grants allow only their documented operations. Package A's durable audit/outbox pair represents Workspace name changes; inserting a draft event there or advancing Workspace versions for unrelated content would corrupt its meaning.

Creation also needs a durable retry boundary. A lost response during COMMIT must not create a second Business, draft or personal workspace when the caller retries. Preview can be read synchronously from authoritative records; it has no identified asynchronous responsibility.

## Decision

### Typed private records and incomplete drafts

The [`listings` domain](../../internal/listings/drafts.go), [transactional store](../../internal/store/listingdrafts.go) and [additive migration](../../db/migrations/000003_listing_drafts.sql) own structured Business and Listing Draft records. Both have a workspace ownership path, distinct identifiers, independent positive resource versions and UTC timestamps. A composite workspace/Business relationship prevents a draft from referencing a foreign workspace's Business. The only listing state in this slice is `DRAFT`; client input cannot change it.

The first contract contains private label, activity description, country and region for Business, and title, description and the four separate sale dimensions for Listing Draft. Empty strings mean unset draft values. Supplied values must satisfy bounded text, canonical enum and country-code shape validation, but missing publication information is permitted. Country normalization does not establish that a code exists in an ISO catalogue or that its jurisdiction is a launch market. Technical validity does not establish a legally valid sale combination. Price, financial facts, media, documents, translations and publication requirements are absent from this contract rather than represented as unchecked JSON.

Creation atomically creates both resources while retaining distinct IDs. Updates use a full replacement `PUT` and always require both expected Business and Listing versions. Both locked current versions are compared before mutation. Full replacement makes clearing an optional field unambiguous and avoids an invented patch dialect. It is a reversible transport choice; it does not expand the approved business policy. Only a changed resource advances its version. An exact no-op returns current state and accepts the session without a material revision or audit event.

### Current scoped authorization and personal bootstrap

Add exactly three current permissions: `listing.read_private`, `listing.create` and `listing.update`. Read and protected Preview require read-private. Create/update additionally require their command permission because their outcome exposes protected resource identity. Existing Reader/Editor templates and existing persisted grants are not expanded.

Every operation resolves the active authenticated HUMAN session, account, current workspace membership and required grants inside its PostgreSQL transaction. Caller-supplied workspace IDs request a context; they are not authority. Managing ownership and organization affiliation grant no implicit access. The existing non-enumerating inaccessible/missing-resource boundary is preserved, including for organization-owned workspaces. Revocation and expiry are evaluated through the existing transaction/session boundary; failed or rolled-back operations do not extend idle activity.

An explicit personal-create command may create or reuse one personal drafting assignment for the derived User. It is not a login-callback side effect and does not mean a User can own only one PERSON Workspace. First creation atomically provisions the PERSON-owned Workspace, active membership and exactly `workspace.read`, `workspace.update`, `listing.read_private`, `listing.create`, `listing.update`. Reuse does not reactivate a revoked membership or refill removed grants. It creates no Organization, SellerMandate, verified identity claim or future permission.

Bootstrap coordinates the account lock before resolving the session, avoiding an account shared-to-exclusive lock upgrade between concurrent first-use commands. This intentionally serializes concurrent bootstrap work for that account. Database uniqueness identifies one assignment; rollback cannot leave an orphan workspace, partial grants or a partially created draft. The assignment is an initialization context, not a tenant-access bypass.

### Durable creation outcome and optimistic updates

Creation uses a durable command identity scoped to authenticated User, requested workspace or explicit personal context, and operation. A fingerprint of the normalized typed request distinguishes a repeat from reuse of a key for different content. The receipt commits with business state and audit evidence. Same identity/content returns the original immutable creation outcome: IDs and initial resource versions, plus an explicit replay indicator. It does not pretend that a later resource reread is the original response. A separate authorized GET retrieves current state.

Every replay checks the current session and current required grants. A receipt is evidence of a previous command, not an access token. Changed content under the same scoped identity conflicts. Concurrent identical creations must converge on one durable outcome. Receipts contain a fingerprint and bounded outcome fields, not arbitrary request bodies, credentials or report contents. No automatic receipt expiry or deletion is introduced; a reviewed retention policy must preserve the retry guarantee before cleanup is enabled.

Updates use expected resource versions to reject lost updates. They do not acquire a creation receipt or claim that an uncertain update COMMIT rolled back. Retrying the old versions after a committed change returns a conflict; reconcile through an authorized current GET before a further change. Repeating a stale update cannot silently apply the change twice.

### Typed aggregate audit and synchronous Preview

Keep the historical Workspace name-change audit ledger, outbox, worker consumer and receipt semantics intact. Add `audit.aggregate_events` with the finite actions `workspace.personal_drafting_initialized`, `business.created`, `business.updated`, `listing_draft.created` and `listing_draft.updated`. Targets are constrained Workspace, Business or ListingDraft references. Evidence identifies the workspace, previous/current resource versions, derived HUMAN actor and session, action, SUCCESS result, database timestamp and server request/correlation context. Metadata is exactly `previous_version` and `version`. Creation is version 0→1. The initialization event records the exact approved first bootstrap; reuse changes no grants and emits no initialization event. Resource changes and their evidence share one transaction; an audit failure rolls back the command.

Human attribution comes from the resolved session, never from actor fields in a request. Machine identities remain separate; the existing outbox worker acquires no draft or bootstrap authority. Composite resource constraints bind evidence to its workspace and actual target. A reverse target-version check rejects evidence for a fabricated future revision, and deferred constraints reject a committed Business/draft revision without its corresponding audit. Explicit migration-owner RLS policies permit the narrow SECURITY DEFINER integrity/bootstrap helpers under FORCE RLS without depending on a superuser or BYPASSRLS owner. That owner and its memberships remain forbidden runtime identities; changing function ownership requires reviewing its policies together.

Append-only runtime permissions are not protection against a privileged database operator and do not establish legal retention or a tamper-proof ledger. Audit metadata does not contain business text, provider claims, passwords, file names or unbounded request bodies.

Protected Preview is a synchronous, authorized read with an explicit presentation allowlist and source resource versions. It excludes the private Business label, identity/ownership/grant information and evidence references. Free text can itself identify a business, so exclusion of private fields is not an anonymity certification. Its HTTP response is private, no-store and non-indexable. It is not a public listing, publication approval, buyer access grant, search projection or permission to disclose data anonymously.

No new outbox relation, domain event or consumer is added merely to demonstrate activity. Draft commands have no deferred effect in this package. When a later approved capability genuinely needs asynchronous work, its minimized versioned event must be committed with the corresponding state/audit and consumed idempotently. Existing Workspace events remain real and continue to use Package A's outbox.

## Alternatives and reversal cost

A single Business/Listing JSON document would blur identities and ownership, hide independent version conflicts and weaken database constraints. A typed split costs a coordinated transaction and explicit transport mapping; it preserves the distinction required by the domain.

Giving every authenticated User listing authority, treating Organization membership as workspace access, or extending existing role templates would simplify bootstrap by weakening isolation. The narrow explicit assignment and grants require current permission checks and make later delegated professional UX a separate reviewed responsibility. Invitations, bulk delegation and organization creation remain deferred.

Session middleware alone would allow stale authorization between validation and COMMIT. Transaction locks add contention but preserve revocation ordering. Any optimization must retain the same expiry, current-grant, concurrency and rollback evidence.

Returning current state on an idempotent create replay would be convenient but would conceal the difference between recorded outcome and subsequent edits. Returning immutable IDs/initial versions requires a separate GET and avoids that ambiguity. In-memory receipt caches or short arbitrary retention would reintroduce duplicate creation after restarts or retries.

Relaxing the historical audit/outbox into unchecked polymorphic strings would weaken proven Workspace invariants. An additive typed ledger preserves them at the cost of deliberate cross-ledger evidence queries. An asynchronous preview projection would require staleness, rebuild, worker privileges and real replay semantics without improving the present small synchronous read.

Reversal requires reviewed forward migrations preserving resource IDs, ownership paths, receipt outcomes and audit evidence, plus contract and data-conversion tests. Do not edit applied migrations, silently discard receipts or reuse their keys, flatten distinct resource versions, or delete evidence to return to the earlier schema. Returning to a compatible older binary does not remove the new data.

## Verification and operating boundary

The [private draft runbook](../operations/PRIVATE-DRAFTS.md) records actual source behavior and recovery instructions; the [scoped threat review](../security/PRIVATE-DRAFTS-THREATS.md) maps abuse cases, current boundaries and residual risks. Approval and this ADR are not passing test results. Acceptance requires real PostgreSQL migration/upgrade, restricted-role authorization, concurrency, rollback, current-session revocation and receipt replay evidence, followed by applicable quality/security gates and committed clean-checkout CI. The root delivery report supplies exact executed results and commit/run identifiers; none are inferred here.

The provider-conformance, real-user privacy/purpose/retention, secure ingress and operating-owner gates remain open. The browser study remains disconnected; its local financial/report/photo behavior is not this API contract. Publish, money, uploads, buyer access, search and production infrastructure are excluded. A future client must use the actual approved authentication boundary rather than a test fixture or development-identity fallback.
