# ADR 0007: transactional security state and typed audit attribution

Date: 2026-10-03. Status: **implemented within P0.4; whole-repository local checks and committed clean-checkout CI passed; provider acceptance open**. This ADR records the persistence/authority boundary, not a provider purchase, administrative workflow or production acceptance. [ADR 0006](0006-identity-provider-and-session-policy.md) owns provider/admission/session policy; the [P0.4 threat model](../security/P0.4-THREAT-MODEL.md) owns negative evidence requirements.

## Context

Package A's audit.events table describes one real material command: workspace name update. Its non-null workspace, previous/current version, actor User and exact metadata constraints also anchor the outbox's composite foreign key. Authentication and revocation have no legitimate workspace/resource version. Making those values optional merely to fit security events into the old table would weaken established evidence constraints and invent meaningless workspace attribution.

Authentication must also remain current when a business transaction authorizes access. Resolving a principal in HTTP middleware cannot by itself prevent a session, account or permission from being revoked between that check and the protected write. A worker is a service actor rather than a human session.

## Decision

Keep historical workspace audit rows and their outbox relationship intact. Mark their existing actor as HUMAN through an additive migration. Introduce audit.security_events with its own constrained action/result/metadata schema and explicit HUMAN or SERVICE attribution. Human attribution refers to a User; service attribution refers to a service principal. Exactly one typed actor reference is present. Security lifecycle events name their target User and, when applicable, internal non-secret session ID. Do not store credentials, hashes or raw claims in evidence.

A security operation constructs its actor from authenticated application state or a separately trusted operator/process capability, never from request-supplied actor/role fields. A service label alone is not authorization. Current-session/all-session revocation remains a human self-service capability; worker process identity does not acquire account administration rights. Any future privileged interface requires its own reviewed capability and attribution. Migration privileges are not a public administrative API.

The current owner-only disable primitive attributes its action to the fixed identity-operator service principal; the API and worker receive no execute grant and cannot choose its actor. This records the controlled operator capability, not the individual human using migration credentials. Named operator identity, credential governance and privileged intervention review remain deployment requirements. A distinct outbox-worker principal must never be substituted for this actor.

Worker composition establishes the canonical outbox-worker SERVICE principal only after its PostgreSQL process capability is validated. It has no human User/session and no human authentication evidence. Consumer receipts receive fixed service attribution constrained in PostgreSQL; this identifies the processor capability, not a particular process instance. Existing Package A receipts are assigned to that same known worker capability by the additive migration. Security ledger session/target references are constrained together and current human actions are limited to self-service, preventing independent valid IDs from producing inconsistent evidence.

Critical security state and its audit record share the same PostgreSQL transaction. Audit failure fails the operation and rolls back state. Session creation/revocation has no fabricated asynchronous consumer: an outbox record is added only when a real external/deferred effect requires it. Existing workspace mutation, audit and outbox still commit atomically; receipt-based worker idempotency remains unchanged.

Protect each workspace command by resolving the session/account and current scoped grants in the same transaction that reads or changes the resource. Use a consistent account-before-session-before-membership/resource lock order. Account-wide revocation and account disable conflict with the account lock, while specific session revocation conflicts with the relevant session lock. Re-evaluate expiry using database time after lock acquisition. Only an accepted committed operation advances idle activity. A command ordered before revocation may complete; no retroactive cancellation guarantee is introduced.

Session introspection and middleware resolution do not advance idle activity. Successful workspace read/update transactions do; failed or rolled-back commands do not. Login rotates only within the same internal User. A previous session for a different User causes rejection and transaction rollback, requiring explicit logout before switching accounts. Session identity is constrained by a composite issuer/subject/User foreign key, rather than independent identifiers that could disagree.

Use narrow PostgreSQL runtime capabilities and fixed-search-path security functions for identity state; do not grant broad direct identity-table mutation to the API. Keep authorization application-owned and workspace-scoped. The API process is trusted to invoke the identity boundary correctly, while the browser cannot supply a trusted internal User ID. RLS and SQL constraints add defense inside that application trust boundary; they do not authenticate the browser or defeat a fully compromised API runtime.

## Alternatives and consequences

One nullable universal audit table would require weakening Package A's current checks and changing the outbox relationship. It would also allow structurally meaningless combinations unless equivalent event-specific constraints were rebuilt. The separate relation keeps each schema reviewable; evidence consumers must deliberately combine ledger types when a cross-cutting investigation needs both.

Application logs are unsuitable as durable evidence because they are a separate write path with different retention and access behavior. Publishing security events only after committing state would introduce an unaudited crash window. Embedding permissions in a session or trusting provider groups would introduce stale authority and organization/workspace confusion.

Row locks add contention for concurrent requests on one account/session. This is an explicit correctness trade-off for the small initial capability set. Lock timeout returns a safe service failure; no bypass or silent fallback is permitted. Throughput changes must preserve tested revocation ordering. Privileged database actors can still alter evidence; append-only runtime permissions are not a tamper-proof ledger or a legal retention policy.

Reversal requires a reviewed migration preserving both ledgers' records and typed attribution, changes to evidence readers, and equivalent integrity/revocation tests. Do not alter the applied Package A migration or discard historical evidence. Privacy retention, legal hold, operator identity lifecycle and vendor event propagation remain separately owned decisions.

## Acceptance evidence

Focused local normal and race runs for internal/store and db/migrations passed, including the seeded Package A upgrade, identity/admission/revocation, runtime privilege and target constraints, audit rollback, idle behavior, waiting-state ordering and existing outbox/idempotency cases. The [threat model evidence table](../security/P0.4-THREAT-MODEL.md) records their exact scope and limits. Whole-repository gates, HTTP integration, scans and exact-source clean-checkout Linux CI subsequently passed; the [P0.4 report](../operations/P0.4-REPORT.md) records the commit, run and artifact proof. No production or provider acceptance follows from this ADR.
