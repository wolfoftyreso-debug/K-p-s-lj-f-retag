# ADR 0003 — Explicit workspace ownership and authorization

Date: 2026-09-17. Status: accepted under Package A/D02 approval.

## Decision

Workspace is the protected tenant resource in this package. `owner_kind` is exactly `PERSON` or `ORGANIZATION`. Two real foreign keys and an exclusive-owner PostgreSQL check prohibit missing, duplicate or mismatched ownership. Managing ownership is not a conclusion about business title, seller authority or GDPR controller status.

Authorization requires an active internal User, an active WorkspaceMembership and the exact workspace-scoped permission. `workspace.read` and `workspace.update` are the only implemented permissions; update also requires read because it returns protected state. Organization membership and workspace ownership confer no implicit access. The broad product role catalogue, invitations and ownership-transfer workflow are not implemented.

Transaction-local actor/workspace context is set by trusted application code, not copied from an HTTP selector. A narrow `SECURITY DEFINER` function reads membership/permission tables with fixed qualified relation names and a fixed search path. It locks authorization rows so revocation and an in-flight operation serialize. RLS is enabled and forced on protected workspace/audit/outbox relations. A missing context denies access. Composite keys bind permission assignments and audit/event/projection references to the correct workspace.

Runtime roles are separate from the schema owner/migration identity. API and worker identities have distinct capabilities. Runtime startup rejects superuser/bypass/administrative or incompatible role configurations. RLS does not protect against a privileged database operator or a completely compromised trusted application principal boundary; there is no contrary claim.

Startup also verifies that the requested login, `session_user` and `current_user` agree. Role or session switching cannot disguise a privileged login during that check. Valid URI-encoded masking attempts have real PostgreSQL regression coverage; the guard is not an exhaustive detector of every possible later administrative grant or schema drift.

## Error and mutation semantics

Inaccessible and nonexistent workspace IDs return the same `ErrNotFound`, including guessed identifiers. Malformed commands are rejected without data-dependent error details. Update takes an expected version; concurrent stale writers receive `ErrConflict`. A successful update advances the version once and writes audit and outbox in the same transaction.

Personal resources and future buyer-specific sharing grants are not implicitly workspace membership. Those capabilities require later policy; no grant feature is implemented by this ADR.

## Reversibility

Changes to ownership or client isolation need a reviewed migration and policy decision. Changing database adapters does not change the application-owned authorization rule. Synthetic fixtures seed test identities directly using the migration/admin capability; there is no production bootstrap endpoint or test-mode login switch.
